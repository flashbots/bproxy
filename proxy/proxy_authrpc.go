package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/flashbots/bproxy/jrpc"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/triaged"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
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

func (p *AuthrpcProxy) triage(ctx *fasthttp.RequestCtx) (
	triage *triaged.Request, res *fasthttp.Response,
) {
	l := p.Proxy.logger.With(
		zap.Uint64("connection_id", ctx.ConnID()),
		zap.Uint64("request_id", ctx.ConnRequestNum()),
	)

	call, err := jrpc.Unmarshal(ctx.Request.Body())
	if err != nil { // proxy & don't mirror un-parse-able requests as-is
		l.Warn("Failed to parse authrpc call body",
			zap.Error(err),
		)
		return &triaged.Request{
			Proxy: true,
		}, fasthttp.AcquireResponse()
	}

	defer func() {
		triage.JrpcID = call.GetID()
		triage.JrpcMethod = call.GetMethod()
	}()

	switch {
	default: // only proxy by default
		return &triaged.Request{
			Proxy: true,
		}, fasthttp.AcquireResponse()

	case strings.HasPrefix(call.GetMethod(), "engine_getPayload"): // proxy with priority
		return &triaged.Request{
			Proxy:      true,
			Prioritise: true,
		}, fasthttp.AcquireResponse()

	case strings.HasPrefix(call.GetMethod(), "engine_newPayload"): // proxy & mirror with priority
		return &triaged.Request{
			Proxy:      true,
			Prioritise: true,
			Mirror:     true,
		}, fasthttp.AcquireResponse()

	case strings.HasPrefix(call.GetMethod(), "miner_setMaxDASize"): // proxy & mirror
		return &triaged.Request{
			Proxy:  true,
			Mirror: true,
		}, fasthttp.AcquireResponse()

	case strings.HasPrefix(call.GetMethod(), "engine_forkchoiceUpdated"):
		fcuv3 := jrpc.ForkchoiceUpdatedV3{}
		if err := json.Unmarshal(ctx.Request.Body(), &fcuv3); err != nil { // proxy & mirror FCUv3 we can't parse
			l.Warn("Failed to parse FCU call body",
				zap.Error(err),
			)
			return &triaged.Request{
				Proxy:  true,
				Mirror: true,
			}, fasthttp.AcquireResponse()
		}

		switch fcuv3.ParamsCount() {
		case 2:
			p1 := jrpc.ForkchoiceUpdatedV3Param1{}
			if err := json.Unmarshal(fcuv3.Params[1], &p1); err == nil {
				if blockTimestamp, err := p1.GetTimestamp(); err == nil {
					headsup := time.Until(blockTimestamp)
					if headsup < 200*time.Millisecond {
						l.Warn("Received FCU w/ extra param with less than 200ms to build the block",
							zap.String("timestamp", p1.Timestamp),
							zap.Duration("headsup", headsup),
						)
						metrics.LateFCUCount.Add(context.TODO(), 1, otelapi.WithAttributes(
							attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.Proxy.cfg.Name)},
						))
					}
					return &triaged.Request{
						Proxy:      true,
						Prioritise: true,
						Mirror:     true,
						Deadline:   blockTimestamp,
					}, fasthttp.AcquireResponse()
				} else {
					l.Warn("Failed to parse block timestamp from FCU w/ extra param",
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
			}, fasthttp.AcquireResponse()

		case 1:
			p0 := jrpc.ForkchoiceUpdatedV3Param0{}
			if err := json.Unmarshal(fcuv3.Params[0], &p0); err == nil {
				h, s, f := p0.GetHashes()
				if p.alreadySeen(h, s, f) {
					//
					// don't mirror or proxy FCUv3 w/o extra attrs which we already seen
					//
					// see also: https://github.com/paradigmxyz/reth/issues/15086
					//
					l.Warn("Ignoring FCUv3 w/o extra attributes b/c we already seen these hashes",
						zap.String("head", h),
						zap.String("safe", s),
						zap.String("finalised", f),
					)
					return &triaged.Request{}, p.interceptEngineForkchoiceUpdatedV3(call, h)
				}
			} else {
				l.Warn("Failed to parse parameter 0 of FCUv3",
					zap.Error(err),
				)
			}
			return &triaged.Request{
				Proxy:  true,
				Mirror: true,
			}, fasthttp.AcquireResponse()

		default:
			return &triaged.Request{
				Proxy:  true,
				Mirror: true,
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
