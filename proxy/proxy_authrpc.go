package proxy

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/flashbots/bproxy/types"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type AuthrpcProxy struct {
	Proxy *Proxy

	seenHeads       map[string]time.Time
	mxSeenHeads     sync.Mutex
	tickerSeenHeads time.Ticker
}

func NewAuthrpcProxy(cfg *Config) (*AuthrpcProxy, error) {
	p, err := newProxy(cfg)
	if err != nil {
		return nil, err
	}

	ap := &AuthrpcProxy{
		Proxy:           p,
		seenHeads:       make(map[string]time.Time, 60),
		tickerSeenHeads: *time.NewTicker(30 * time.Second),
	}
	ap.Proxy.triage = ap.triage
	ap.Proxy.run = ap.run
	ap.Proxy.stop = ap.stop

	return ap, nil
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

func (p *AuthrpcProxy) triage(body []byte) triagedRequest {
	// proxy & don't mirror un-parse-able requests as-is
	jrpc := types.JrpcCall{}
	if err := json.Unmarshal(body, &jrpc); err != nil {
		p.Proxy.logger.Warn("Failed to parse authrpc call body",
			zap.Error(err),
		)
		return triagedRequest{
			proxy: true,
		}
	}

	switch jrpc.Method {
	default:
		// only proxy
		return triagedRequest{
			proxy:      true,
			jrpcMethod: jrpc.Method,
			jrpcID:     jrpc.ID,
		}

	case "engine_newPayloadV3":
		// mirror & proxy all
		return triagedRequest{
			proxy:      true,
			mirror:     true,
			jrpcMethod: jrpc.Method,
			jrpcID:     jrpc.ID,
		}

	case "engine_forkchoiceUpdatedV3":
		{ // mirror and proxy FCUv3 with extra attributes, or FCUv3 we cant parse
			fcuv3 := types.EngineForkchoiceUpdatedV3{}
			err := json.Unmarshal(body, &fcuv3)
			if err != nil {
				p.Proxy.logger.Warn("Failed to parse FCUv3 call body",
					zap.Error(err),
				)
			}

			if err != nil || fcuv3.HasExtraPayload() {
				return triagedRequest{
					proxy:      true,
					mirror:     true,
					jrpcMethod: jrpc.Method,
					jrpcID:     jrpc.ID,
				}
			}
		}

		fcuv3 := types.EngineForkchoiceUpdatedV3_WithoutExtraPayload{}
		if err := json.Unmarshal(body, &fcuv3); err != nil {
			p.Proxy.logger.Warn("Failed to parse call body of FCUv3 w/o extra attributes",
				zap.Error(err),
			)
			return triagedRequest{
				proxy:      true,
				mirror:     true,
				jrpcMethod: jrpc.Method,
				jrpcID:     jrpc.ID,
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
			res.Header.Add("content-type", "application/json; charset=utf-8")
			res.SetBody([]byte(fmt.Sprintf(
				`{"jsonrpc":"2.0","id":%d,"result":{"payloadStatus":{"status":"VALID","latestValidHash":"%s"}}}`,
				jrpc.ID, head,
			)))

			return triagedRequest{
				jrpcMethod: jrpc.Method,
				jrpcID:     jrpc.ID,
				response:   res,
			}
		}

		return triagedRequest{
			proxy:      true,
			mirror:     true,
			jrpcMethod: jrpc.Method,
			jrpcID:     jrpc.ID,
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
