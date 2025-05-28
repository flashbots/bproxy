package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/flashbots/bproxy/config"
	"github.com/flashbots/bproxy/jrpc"
	"github.com/flashbots/bproxy/jwt"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/triaged"
	"github.com/flashbots/bproxy/utils"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type authrpcProxyConfig struct {
	deduplicateFCUs bool
}

type AuthrpcProxy struct {
	proxy *HTTP

	cfg *authrpcProxyConfig

	seenHeads       map[string]time.Time
	mxSeenHeads     sync.Mutex
	tickerSeenHeads *time.Ticker
}

func NewAuthrpcProxy(
	cfg *config.AuthrpcProxy,
	chaos *config.Chaos,
) (*AuthrpcProxy, error) {
	p, err := newHTTP(&httpConfig{
		name:  "bproxy-authrpc",
		proxy: cfg.HttpProxy,
		chaos: chaos,
	})
	if err != nil {
		return nil, err
	}

	ap := &AuthrpcProxy{
		proxy: p,

		seenHeads:       make(map[string]time.Time, 60),
		tickerSeenHeads: time.NewTicker(30 * time.Second),

		cfg: &authrpcProxyConfig{
			deduplicateFCUs: cfg.DeduplicateFCUs,
		},
	}
	ap.proxy.triage = ap.triage
	ap.proxy.run = ap.run
	ap.proxy.stop = ap.stop

	return ap, nil
}

func (p *AuthrpcProxy) Run(ctx context.Context, failure chan<- error) {
	if p == nil {
		return
	}
	p.proxy.Run(ctx, failure)
}

func (p *AuthrpcProxy) ResetConnections() {
	if p == nil {
		return
	}
	p.proxy.ResetConnections()
}

func (p *AuthrpcProxy) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.proxy.Stop(ctx)
}

func (p *AuthrpcProxy) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}
	return p.proxy.Observe(ctx, o)
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

func (p *AuthrpcProxy) triage(ctx *fasthttp.RequestCtx) (
	triage *triaged.Request, res *fasthttp.Response,
) {
	l := p.proxy.logger.With(
		zap.Uint64("connection_id", ctx.ConnID()),
		zap.Uint64("request_id", ctx.ConnRequestNum()),
	)

	token := strings.TrimPrefix(utils.Str(ctx.Request.Header.Peek("authorization")), "Bearer ")
	if iat, err := jwt.IssuedAt(token); err == nil {
		metrics.LatencyAuthrpcJwt.Record(context.TODO(), int64(time.Since(iat).Milliseconds()))
	} else {
		l.Warn("Failed to parse iat claim from jwt",
			zap.Error(err),
		)
	}

	call, err := jrpc.Unmarshal(ctx.Request.Body())
	if err != nil { // proxy & don't mirror un-parse-able requests as-is
		l.Warn("Failed to parse authrpc call body",
			zap.Error(err),
		)
		return &triaged.Request{
			Proxy:      true,
			JrpcMethod: "unknown",
		}, fasthttp.AcquireResponse()
	}

	switch {
	default: // only proxy by default
		return &triaged.Request{
			Proxy:      true,
			JrpcID:     call.GetID(),
			JrpcMethod: call.GetMethod(),
		}, fasthttp.AcquireResponse()

	case strings.HasPrefix(call.GetMethod(), "engine_getPayload"): // proxy with priority
		return &triaged.Request{
			Proxy:      true,
			Prioritise: true,
			JrpcID:     call.GetID(),
			JrpcMethod: call.GetMethod(),
		}, fasthttp.AcquireResponse()

	case strings.HasPrefix(call.GetMethod(), "engine_newPayload"): // proxy & mirror with priority
		return &triaged.Request{
			Proxy:      true,
			Prioritise: true,
			Mirror:     true,
			JrpcID:     call.GetID(),
			JrpcMethod: call.GetMethod(),
		}, fasthttp.AcquireResponse()

	case strings.HasPrefix(call.GetMethod(), "miner_setMaxDASize"): // proxy & mirror
		return &triaged.Request{
			Proxy:      true,
			Mirror:     true,
			JrpcID:     call.GetID(),
			JrpcMethod: call.GetMethod(),
		}, fasthttp.AcquireResponse()

	case strings.HasPrefix(call.GetMethod(), "engine_forkchoiceUpdated"):
		fcuv3 := jrpc.ForkchoiceUpdatedV3{}
		if err := json.Unmarshal(ctx.Request.Body(), &fcuv3); err != nil { // proxy & mirror FCUv3 we can't parse
			l.Warn("Failed to parse FCU call body",
				zap.Error(err),
			)
			return &triaged.Request{
				Proxy:      true,
				Mirror:     true,
				JrpcID:     call.GetID(),
				JrpcMethod: call.GetMethod(),
			}, fasthttp.AcquireResponse()
		}

		switch fcuv3.ParamsCount() {
		case 2:
			pa := jrpc.ForkchoiceUpdatedV3_PayloadAttributes{}
			if err := json.Unmarshal(fcuv3.Params[1], &pa); err == nil {
				if blockTimestamp, err := pa.GetTimestamp(); err == nil {
					headsup := time.Until(blockTimestamp)
					if headsup < 200*time.Millisecond {
						l.Warn("Received FCUwPayload with less than 200ms left to build the block",
							zap.String("timestamp", pa.Timestamp),
							zap.Duration("headsup", headsup),
						)
						metrics.LateFCUCount.Add(context.TODO(), 1, otelapi.WithAttributes(
							attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.proxy.cfg.name)},
						))
					}
					return &triaged.Request{
						Proxy:      true,
						Prioritise: true,
						Mirror:     true,
						Deadline:   blockTimestamp,
						JrpcID:     call.GetID(),
						JrpcMethod: call.GetMethod() + "_withPayload",
					}, fasthttp.AcquireResponse()
				} else {
					l.Warn("Failed to parse block timestamp from FCUwPayload",
						zap.Error(err),
					)
				}
			} else {
				l.Warn("Failed to parse extra param of FCU",
					zap.Error(err),
				)
			}
			return &triaged.Request{
				Proxy:      true,
				Prioritise: true,
				Mirror:     true,
				JrpcID:     call.GetID(),
				JrpcMethod: call.GetMethod() + "_withPayload",
			}, fasthttp.AcquireResponse()

		case 1:
			if p.cfg.deduplicateFCUs {
				fs := jrpc.ForkchoiceUpdatedV3_ForkchoiceState{}
				if err := json.Unmarshal(fcuv3.Params[0], &fs); err == nil {
					h, s, f := fs.GetHashes()
					if p.alreadySeen(h, s, f) {
						//
						// don't mirror or proxy FCUv3 w/o payload that we already seen
						//
						// see also: https://github.com/paradigmxyz/reth/issues/15086
						//
						l.Warn("Ignoring FCUv3 w/o payload b/c we already seen these hashes",
							zap.String("head", h),
							zap.String("safe", s),
							zap.String("finalised", f),
						)
						return &triaged.Request{
							JrpcID:     call.GetID(),
							JrpcMethod: call.GetMethod(),
						}, p.interceptEngineForkchoiceUpdatedV3(call, h)
					}
				} else {
					l.Warn("Failed to parse forkchoice state from FCUv3",
						zap.Error(err),
					)
				}
			}

			return &triaged.Request{
				Proxy:      true,
				Mirror:     true,
				JrpcID:     call.GetID(),
				JrpcMethod: call.GetMethod(),
			}, fasthttp.AcquireResponse()

		default:
			return &triaged.Request{
				Proxy:      true,
				Mirror:     true,
				JrpcID:     call.GetID(),
				JrpcMethod: call.GetMethod(),
			}, fasthttp.AcquireResponse()
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

func (p *AuthrpcProxy) interceptEngineForkchoiceUpdatedV3(call jrpc.Call, head string) *fasthttp.Response {
	res := fasthttp.AcquireResponse()

	res.SetStatusCode(fasthttp.StatusOK)
	res.Header.Add("content-type", "application/json; charset=utf-8")
	res.SetBody([]byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%s,"result":{"payloadStatus":{"status":"VALID","latestValidHash":"%s"}}}`,
		call.GetID(), head,
	)))

	return res
}
