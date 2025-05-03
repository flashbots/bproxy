package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/bproxy/types"
	"github.com/valyala/fasthttp"

	tdxabi "github.com/google/go-tdx-guest/abi"
	tdx "github.com/google/go-tdx-guest/client"
	tdxpb "github.com/google/go-tdx-guest/proto/tdx"

	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type RpcProxy struct {
	Proxy *Proxy
}

func NewRpcProxy(cfg *Config) (*RpcProxy, error) {
	p, err := newProxy(cfg)
	if err != nil {
		return nil, err
	}

	rpcProxy := &RpcProxy{
		Proxy: p,
	}

	rpcProxy.Proxy.triage = rpcProxy.triage

	return rpcProxy, nil
}

func (p *RpcProxy) Run(ctx context.Context, failure chan<- error) {
	if p == nil {
		return
	}
	p.Proxy.Run(ctx, failure)
}

func (p *RpcProxy) ResetConnections() {
	if p == nil {
		return
	}
	p.Proxy.ResetConnections()
}

func (p *RpcProxy) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.Proxy.Stop(ctx)
}

func (p *RpcProxy) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}
	return p.Proxy.Observe(ctx, o)
}

func (p *RpcProxy) triage(body []byte) *triagedRequest {
	errs := make([]error, 0)

	{ // jrpc id as `uint64`
		singleShot := &types.JrpcCall_Uint64{}
		err := json.Unmarshal(body, singleShot)
		if err == nil {
			return p.triageSingle(singleShot)
		}
		errs = append(errs, err)

		batch := []types.JrpcCall_Uint64{}
		err = json.Unmarshal(body, &batch)
		if err == nil {
			_batch := make([]types.JrpcCall, 0, len(batch))
			for _, call := range batch {
				_batch = append(_batch, call)
			}
			return p.triageBatch(_batch)
		}
		errs = append(errs, err)
	}

	{ // jrpc id as `string`
		singleShot := &types.JrpcCall_String{}
		err := json.Unmarshal(body, singleShot)
		if err == nil {
			return p.triageSingle(singleShot)
		}
		errs = append(errs, err)

		batch := []types.JrpcCall_String{}
		err = json.Unmarshal(body, &batch)
		if err == nil {
			_batch := make([]types.JrpcCall, 0, len(batch))
			for _, call := range batch {
				_batch = append(_batch, call)
			}
			return p.triageBatch(_batch)
		}
		errs = append(errs, err)
	}

	p.Proxy.logger.Warn("Failed to parse rpc call body",
		zap.Errors("errors", errs),
	)

	// proxy un-parse-able requests as-is, but don't mirror them
	return &triagedRequest{
		proxy: true,
	}
}

func (p *RpcProxy) triageSingle(call types.JrpcCall) *triagedRequest {
	if call.GetMethod() == "tee_getDcapQuote" {
		return &triagedRequest{
			jrpcMethod: call.GetMethod(),
			jrpcID:     call.GetID(),
			response:   p.interceptTeeGetDcapQuote(call),
		}
	}

	// proxy all non sendRawTX calls, but don't mirror them
	if call.GetMethod() != "eth_sendRawTransaction" {
		return &triagedRequest{
			proxy:      true,
			jrpcMethod: call.GetMethod(),
			jrpcID:     call.GetID(),
		}
	}

	res := &triagedRequest{
		proxy:      true,
		mirror:     true,
		jrpcMethod: call.GetMethod(),
		jrpcID:     call.GetID(),
	}

	if from, tx, err := call.DecodeEthSendRawTransaction(); err == nil {
		res.transactions = []triagedRequestTx{{
			From:  &from,
			To:    tx.To(),
			Hash:  tx.Hash(),
			Nonce: tx.Nonce(),
		}}
	} else {
		p.Proxy.logger.Warn("Failed to decode eth_sendRawTransaction",
			zap.Error(err),
		)
	}

	return res
}

func (p *RpcProxy) triageBatch(batch []types.JrpcCall) *triagedRequest {
	if len(batch) == 0 {
		return &triagedRequest{} // no need to proxy empty batches
	}

	methodsSet := make(map[string]struct{}, 0)
	for _, call := range batch {
		if _, known := methodsSet[call.GetMethod()]; !known {
			methodsSet[call.GetMethod()] = struct{}{}
		}
	}

	// proxy all non sendRawTX calls, but don't mirror them
	if _, hasEthSendRawTx := methodsSet["eth_sendRawTransaction"]; !hasEthSendRawTx {
		return &triagedRequest{
			proxy:      true,
			jrpcMethod: "batch(" + strconv.Itoa(len(batch)) + ")",
			jrpcID:     batch[0].GetID(),
		}
	}

	res := &triagedRequest{
		proxy:        true,
		mirror:       true,
		jrpcMethod:   "batch(" + strconv.Itoa(len(batch)) + ")",
		jrpcID:       batch[0].GetID(),
		transactions: make([]triagedRequestTx, 0),
	}

	for _, call := range batch {
		if call.GetMethod() != "eth_sendRawTransaction" {
			continue
		}
		if from, tx, err := call.DecodeEthSendRawTransaction(); err == nil {
			res.transactions = append(res.transactions, triagedRequestTx{
				From:  &from,
				To:    tx.To(),
				Hash:  tx.Hash(),
				Nonce: tx.Nonce(),
			})
		} else {
			p.Proxy.logger.Warn("Failed to decode eth_sendRawTransaction",
				zap.Error(err),
			)
		}
	}

	return res
}

func (p *RpcProxy) interceptTeeGetDcapQuote(call types.JrpcCall) *fasthttp.Response {
	res := fasthttp.AcquireResponse()

	res.SetStatusCode(fasthttp.StatusOK)
	res.Header.Add("content-type", "application/json; charset=utf-8")

	provider, err := tdx.GetQuoteProvider()
	if err != nil {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32043,"message:"not in tdx"}}`,
			call.GetID(),
		)))
		return res
	}

	var params []string
	if err := json.Unmarshal(call.GetParams(), &params); err != nil {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message:"invalid report data"}}`,
			call.GetID(),
		)))
		return res
	}

	if len(params) != 1 {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message:"only 1 parameter expected"}}`,
			call.GetID(),
		)))
		return res
	}

	reportData, err := hexutil.Decode(params[0])
	if err != nil {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message:"parameter is not a hex string"}}`,
			call.GetID(),
		)))
		return res
	}

	if len(reportData) != 64 {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message:"parameter is not 64 bytes hex"}}`,
			call.GetID(),
		)))
		return res
	}

	rawQuote, err := tdx.GetRawQuote(provider, [64]byte(reportData))
	if err != nil {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32043,"message:"failed to get tdx quote: %s"}}`,
			call.GetID(), err,
		)))
		return res
	}

	_quote, err := tdxabi.QuoteToProto(rawQuote)
	if err != nil {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message:"failed to decode tdx quote: %s"}}`,
			call.GetID(), err,
		)))
		return res
	}

	quote := _quote.(*tdxpb.QuoteV4)
	if quote == nil {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message:"unknown tdx quote format"}}`,
			call.GetID(),
		)))
		return res
	}

	jsonQuote, err := json.Marshal(quote)
	if err != nil {
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message:"failed to marshal tdx quote: %s"}}`,
			call.GetID(), err,
		)))
		return res
	}

	res.SetBody([]byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%s,"result":%s}`,
		call.GetID(), string(jsonQuote),
	)))

	return res
}
