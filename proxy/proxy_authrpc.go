package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flashbots/bproxy/types"

	"github.com/valyala/fasthttp"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type AuthrpcProxy struct {
	Proxy *Proxy

	seenHeads       map[string]time.Time
	mxSeenHeads     sync.Mutex
	tickerSeenHeads *time.Ticker
}

func NewAuthrpcProxy(cfg *Config) (*AuthrpcProxy, error) {
	p, err := newProxy(cfg)
	if err != nil {
		return nil, err
	}

	ap := &AuthrpcProxy{
		Proxy:           p,
		seenHeads:       make(map[string]time.Time, 60),
		tickerSeenHeads: time.NewTicker(30 * time.Second),
	}
	ap.Proxy.triage = ap.triage
	ap.Proxy.run = ap.run
	ap.Proxy.stop = ap.stop

	return ap, nil
}

func (p *AuthrpcProxy) Run(ctx context.Context, failure chan<- error) {
	if p == nil {
		return
	}
	p.Proxy.Run(ctx, failure)
}

func (p *AuthrpcProxy) ResetConnections() {
	if p == nil {
		return
	}
	p.Proxy.ResetConnections()
}

func (p *AuthrpcProxy) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.Proxy.Stop(ctx)
}

func (p *AuthrpcProxy) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}
	return p.Proxy.Observe(ctx, o)
}

func (p *AuthrpcProxy) run() {
	go func() {
		for {
			<-p.tickerSeenHeads.C
			p.mxSeenHeads.Lock()
			for key, ts := range p.seenHeads {
				if time.Since(ts) > 30*time.Second {
					delete(p.seenHeads, key)
				}
			}
			p.mxSeenHeads.Unlock()
		}
	}()
}

func (p *AuthrpcProxy) stop() {
	p.tickerSeenHeads.Stop()
}

func (p *AuthrpcProxy) triage(body []byte) *triagedRequest {
	var jrpc types.JrpcCall

	{ // proxy & don't mirror un-parse-able requests as-is
		_jrpc := types.JrpcCall_Uint64{}
		if err_Uint64 := json.Unmarshal(body, &_jrpc); err_Uint64 == nil {
			jrpc = _jrpc
		} else {
			_jrpc := types.JrpcCall_String{}
			if err_String := json.Unmarshal(body, &_jrpc); err_String == nil {
				jrpc = _jrpc
			} else {
				p.Proxy.logger.Warn("Failed to parse authrpc call body",
					zap.Error(errors.Join(err_Uint64, err_String)),
				)
				return &triagedRequest{
					proxy: true,
				}
			}
		}
	}

	switch jrpc.GetMethod() {
	default:
		// only proxy
		return &triagedRequest{
			proxy:      true,
			jrpcMethod: jrpc.GetMethod(),
			jrpcID:     jrpc.GetID(),
		}

	case "engine_newPayloadV3":
		// proxy & mirror
		return &triagedRequest{
			proxy:      true,
			mirror:     true,
			jrpcMethod: jrpc.GetMethod(),
			jrpcID:     jrpc.GetID(),
		}

	case "engine_newPayloadV4":
		// proxy & mirror
		return &triagedRequest{
			proxy:      true,
			mirror:     true,
			jrpcMethod: jrpc.GetMethod(),
			jrpcID:     jrpc.GetID(),
		}
	case "miner_setMaxDASize":
		// proxy & mirror
		return &triagedRequest{
			proxy:      true,
			mirror:     true,
			jrpcMethod: jrpc.GetMethod(),
			jrpcID:     jrpc.GetID(),
		}

	case "engine_forkchoiceUpdatedV3":
		{ // proxy & mirror FCUv3 with extra attributes (or FCUv3 we can't parse)
			fcuv3 := types.EngineForkchoiceUpdatedV3{}
			err := json.Unmarshal(body, &fcuv3)
			if err != nil {
				p.Proxy.logger.Warn("Failed to parse FCUv3 call body",
					zap.Error(err),
				)
			}

			if err != nil || fcuv3.HasExtraPayload() {
				return &triagedRequest{
					proxy:      true,
					mirror:     true,
					jrpcMethod: jrpc.GetMethod(),
					jrpcID:     jrpc.GetID(),
				}
			}
		}

		fcuv3 := types.EngineForkchoiceUpdatedV3_WithoutExtraPayload{}
		if err := json.Unmarshal(body, &fcuv3); err != nil {
			p.Proxy.logger.Warn("Failed to parse call body of FCUv3 w/o extra attributes",
				zap.Error(err),
			)
			return &triagedRequest{
				proxy:      true,
				mirror:     true,
				jrpcMethod: jrpc.GetMethod(),
				jrpcID:     jrpc.GetID(),
			}
		}

		//
		// don't mirror or proxy FCUv3 w/o extra attrs which we already seen
		//
		// see also: https://github.com/paradigmxyz/reth/issues/15086
		//
		head, safe, finalised := fcuv3.BlockHashes()
		if p.alreadySeen(head, safe, finalised) {
			p.Proxy.logger.Warn("Ignoring FCUv3 w/o extra attributes b/c we already seen these hashes",
				zap.String("head", head),
				zap.String("safe", safe),
				zap.String("finalised", finalised),
			)

			res := fasthttp.AcquireResponse()
			res.SetStatusCode(fasthttp.StatusOK)
			res.Header.Add("content-type", "application/json; charset=utf-8")
			res.SetBody([]byte(fmt.Sprintf(
				`{"jsonrpc":"2.0","id":%s,"result":{"payloadStatus":{"status":"VALID","latestValidHash":"%s"}}}`,
				jrpc.GetID(), head,
			)))

			return &triagedRequest{
				jrpcMethod: jrpc.GetMethod(),
				jrpcID:     jrpc.GetID(),
				response:   res,
			}
		}

		return &triagedRequest{
			proxy:      true,
			mirror:     true,
			jrpcMethod: jrpc.GetMethod(),
			jrpcID:     jrpc.GetID(),
		}
	}
}

func (p *AuthrpcProxy) alreadySeen(headBlockHash, safeBlockHash, finalisedBlockHash string) bool {
	p.mxSeenHeads.Lock()
	defer p.mxSeenHeads.Unlock()

	key := headBlockHash + "/" + safeBlockHash + "/" + finalisedBlockHash

	if _, seen := p.seenHeads[key]; seen {
		return true
	}

	p.seenHeads[key] = time.Now()

	return false
}
