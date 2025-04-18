package proxy

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/flashbots/bproxy/types"

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

	{ // uint64
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
			for _, jrpc := range batch {
				_batch = append(_batch, jrpc)
			}
			return p.triageBatch(_batch)
		}
		errs = append(errs, err)
	}

	{ // string
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
			for _, jrpc := range batch {
				_batch = append(_batch, jrpc)
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
