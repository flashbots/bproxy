package proxy

import (
	"context"
	"encoding/json"

	"github.com/flashbots/bproxy/types"
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

func (p *RpcProxy) triage(body []byte) triagedRequest {
	// proxy un-parse-able requests as-is, but don't mirror them
	jrpc := types.JrpcCall{}
	if err := json.Unmarshal(body, &jrpc); err != nil {
		p.Proxy.logger.Warn("Failed to parse rpc call body",
			zap.Error(err),
		)
		return triagedRequest{
			proxy: true,
		}
	}

	// proxy all non sendRawTX calls, but don't mirror them
	if jrpc.Method != "eth_sendRawTransaction" {
		return triagedRequest{
			proxy:      true,
			jrpcMethod: jrpc.Method,
			jrpcID:     jrpc.ID,
		}
	}

	txHash, err := decodeTxHash(body)
	if err != nil {
		p.Proxy.logger.Warn("Failed to decode eth_sendRawTransaction hash",
			zap.Error(err),
		)
	}

	return triagedRequest{
		proxy:      true,
		mirror:     true,
		jrpcMethod: jrpc.Method,
		jrpcID:     jrpc.ID,
		txHash:     txHash,
	}
}
