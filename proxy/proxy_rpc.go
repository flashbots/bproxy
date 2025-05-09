package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/bproxy/config"
	"github.com/flashbots/bproxy/jrpc"
	"github.com/flashbots/bproxy/triaged"
	"github.com/valyala/fasthttp"

	tdxabi "github.com/google/go-tdx-guest/abi"
	tdx "github.com/google/go-tdx-guest/client"
	tdxpb "github.com/google/go-tdx-guest/proto/tdx"

	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type RpcProxy struct {
	proxy *Proxy
}

func NewRpcProxy(
	cfg *config.RpcProxy,
	chaos *config.Chaos,
) (*RpcProxy, error) {
	p, err := newProxy(&proxyConfig{
		name:  "bproxy-rpc",
		proxy: cfg.Proxy,
		chaos: chaos,
	})
	if err != nil {
		return nil, err
	}

	rpcProxy := &RpcProxy{
		proxy: p,
	}

	rpcProxy.proxy.triage = rpcProxy.triage

	return rpcProxy, nil
}

func (p *RpcProxy) Run(ctx context.Context, failure chan<- error) {
	if p == nil {
		return
	}
	p.proxy.Run(ctx, failure)
}

func (p *RpcProxy) ResetConnections() {
	if p == nil {
		return
	}
	p.proxy.ResetConnections()
}

func (p *RpcProxy) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.proxy.Stop(ctx)
}

func (p *RpcProxy) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}
	return p.proxy.Observe(ctx, o)
}

func (p *RpcProxy) triage(ctx *fasthttp.RequestCtx) (*triaged.Request, *fasthttp.Response) {
	l := p.proxy.logger.With(
		zap.Uint64("connection_id", ctx.ConnID()),
		zap.Uint64("request_id", ctx.ConnRequestNum()),
	)

	errs := make([]error, 0)

	{ // jrpc id as `uint64`
		singleShot := &jrpc.CallWithIdAsUint64{}
		err := json.Unmarshal(ctx.Request.Body(), singleShot)
		if err == nil {
			return p.triageSingle(singleShot, l)
		}
		errs = append(errs, err)

		batch := []jrpc.CallWithIdAsUint64{}
		err = json.Unmarshal(ctx.Request.Body(), &batch)
		if err == nil {
			_batch := make([]jrpc.Call, 0, len(batch))
			for _, call := range batch {
				_batch = append(_batch, call)
			}
			return p.triageBatch(_batch, l)
		}
		errs = append(errs, err)
	}

	{ // jrpc id as `string`
		singleShot := &jrpc.CallWithIdAsString{}
		err := json.Unmarshal(ctx.Request.Body(), singleShot)
		if err == nil {
			return p.triageSingle(singleShot, l)
		}
		errs = append(errs, err)

		batch := []jrpc.CallWithIdAsString{}
		err = json.Unmarshal(ctx.Request.Body(), &batch)
		if err == nil {
			_batch := make([]jrpc.Call, 0, len(batch))
			for _, call := range batch {
				_batch = append(_batch, call)
			}
			return p.triageBatch(_batch, l)
		}
		errs = append(errs, err)
	}

	l.Warn("Failed to parse rpc call body",
		zap.Errors("errors", errs),
	)

	// proxy un-parse-able requests as-is, but don't mirror them
	return &triaged.Request{
		Proxy: true,
	}, fasthttp.AcquireResponse()
}

func (p *RpcProxy) triageSingle(call jrpc.Call, l *zap.Logger) (*triaged.Request, *fasthttp.Response) {
	if call.GetMethod() == "tee_getDcapQuote" {
		return &triaged.Request{
			JrpcMethod: call.GetMethod(),
			JrpcID:     call.GetID(),
		}, p.interceptTeeGetDcapQuote(call)
	}

	// proxy all non sendRawTX calls, but don't mirror them
	if call.GetMethod() != "eth_sendRawTransaction" {
		return &triaged.Request{
			Proxy:      true,
			JrpcMethod: call.GetMethod(),
			JrpcID:     call.GetID(),
		}, fasthttp.AcquireResponse()
	}

	res := &triaged.Request{
		Proxy:      true,
		Prioritise: true,
		Mirror:     true,
		JrpcMethod: call.GetMethod(),
		JrpcID:     call.GetID(),
	}

	if from, tx, err := call.DecodeEthSendRawTransaction(); err == nil {
		res.Transactions = triaged.RequestTransactions{{
			From:  &from,
			To:    tx.To(),
			Hash:  tx.Hash(),
			Nonce: tx.Nonce(),
		}}
	} else {
		l.Warn("Failed to decode eth_sendRawTransaction",
			zap.Error(err),
		)
	}

	return res, fasthttp.AcquireResponse()
}

func (p *RpcProxy) triageBatch(batch []jrpc.Call, l *zap.Logger) (*triaged.Request, *fasthttp.Response) {
	if len(batch) == 0 {
		// no need to proxy empty batches
		return &triaged.Request{}, fasthttp.AcquireResponse()
	}

	methodsSet := make(map[string]struct{}, 0)
	for _, call := range batch {
		if _, known := methodsSet[call.GetMethod()]; !known {
			methodsSet[call.GetMethod()] = struct{}{}
		}
	}

	// proxy all non sendRawTX calls, but don't mirror them
	if _, hasEthSendRawTx := methodsSet["eth_sendRawTransaction"]; !hasEthSendRawTx {
		return &triaged.Request{
			Proxy:      true,
			JrpcMethod: "batch(" + strconv.Itoa(len(batch)) + ")",
			JrpcID:     batch[0].GetID(),
		}, fasthttp.AcquireResponse()
	}

	res := &triaged.Request{
		Proxy:        true,
		Prioritise:   true,
		Mirror:       true,
		JrpcMethod:   "batch(" + strconv.Itoa(len(batch)) + ")",
		JrpcID:       batch[0].GetID(),
		Transactions: make(triaged.RequestTransactions, 0),
	}

	for _, call := range batch {
		if call.GetMethod() != "eth_sendRawTransaction" {
			continue
		}
		if from, tx, err := call.DecodeEthSendRawTransaction(); err == nil {
			res.Transactions = append(res.Transactions, triaged.RequestTransaction{
				From:  &from,
				To:    tx.To(),
				Hash:  tx.Hash(),
				Nonce: tx.Nonce(),
			})
		} else {
			l.Warn("Failed to decode eth_sendRawTransaction",
				zap.Error(err),
			)
		}
	}

	return res, fasthttp.AcquireResponse()
}

func (p *RpcProxy) interceptTeeGetDcapQuote(call jrpc.Call) *fasthttp.Response {
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
