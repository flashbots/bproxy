package proxy

import (
	"context"
	"fmt"
	"net"

	"sync"
	"time"

	"github.com/flashbots/bproxy/logutils"
	"github.com/flashbots/bproxy/metrics"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type Proxy struct {
	cfg *Config

	backend  *fasthttp.Client
	frontend *fasthttp.Server

	backendURI *fasthttp.URI
	peerURIs   []*fasthttp.URI

	logger *zap.Logger

	parse func(body []byte) (bool, string, uint64)
}

type Config struct {
	Name          string
	ListenAddress string
	LogRequests   bool
	LogResponses  bool

	BackendURI string
	PeerURIs   []string

	Parse func(body []byte) (doMirror bool, jrpcMethod string, jrpcID uint64)
}

func New(cfg *Config) (*Proxy, error) {
	l := zap.L().With(zap.String("proxy_name", cfg.Name))

	p := &Proxy{
		cfg:    cfg,
		logger: l,
		parse:  cfg.Parse,
	}

	p.frontend = &fasthttp.Server{
		ConnState:          p.upstreamConnectionChanged,
		Handler:            p.handle,
		IdleTimeout:        30 * time.Second,
		Logger:             logutils.FasthttpLogger(l),
		MaxRequestBodySize: 64 * 1024,
		Name:               cfg.Name,
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       5 * time.Second,
	}

	p.backend = &fasthttp.Client{
		MaxIdleConnDuration: 30 * time.Second,
		Name:                cfg.Name,
		ReadTimeout:         5 * time.Second,
		WriteTimeout:        5 * time.Second,
	}

	p.backendURI = fasthttp.AcquireURI()
	if err := p.backendURI.Parse(nil, []byte(cfg.BackendURI)); err != nil {
		fasthttp.ReleaseURI(p.backendURI)
		return nil, err
	}

	p.peerURIs = make([]*fasthttp.URI, 0, len(cfg.PeerURIs))
	for _, peerURI := range cfg.PeerURIs {
		uri := fasthttp.AcquireURI()
		if err := uri.Parse(nil, []byte(peerURI)); err != nil {
			fasthttp.ReleaseURI(p.backendURI)
			for _, uri := range p.peerURIs {
				fasthttp.ReleaseURI(uri)
			}
			return nil, err
		}
		p.peerURIs = append(p.peerURIs, uri)
	}

	return p, nil
}

func (p *Proxy) Run(ctx context.Context, failure chan<- error) {
	l := p.logger

	go func() { // run the authrpc proxy
		l.Info("Proxy is going up...",
			zap.String("listen_address", p.cfg.ListenAddress),
			zap.String("backend", p.cfg.BackendURI),
			zap.Strings("peers", p.cfg.PeerURIs),
		)
		if err := p.frontend.ListenAndServe(p.cfg.ListenAddress); err != nil {
			failure <- err
		}
		l.Info("Proxy is down")
	}()
}

func (p *Proxy) Stop(ctx context.Context) error {
	res := p.frontend.ShutdownWithContext(ctx)

	fasthttp.ReleaseURI(p.backendURI)
	for _, uri := range p.peerURIs {
		fasthttp.ReleaseURI(uri)
	}

	return res
}

func (p *Proxy) handle(ctx *fasthttp.RequestCtx) {
	l := p.logger

	var (
		wg sync.WaitGroup

		doMirror   bool
		jrpcMethod string
		jrpcID     uint64
	)

	connectionID := ctx.ConnID()
	remoteIP := ctx.RemoteIP().String()

	if ctx.IsPost() {
		doMirror, jrpcMethod, jrpcID = p.parse(ctx.PostBody())
	}

	{
		wg.Add(1)

		req := fasthttp.AcquireRequest()
		defer fasthttp.ReleaseRequest(req)

		res := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseResponse(res)

		ctx.Request.CopyTo(req)
		req.SetURI(p.backendURI)
		req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
		req.Header.Add("x-forwarded-host", str(ctx.Host()))
		req.Header.Add("x-forwarded-proto", str(ctx.Request.URI().Scheme()))

		go func() {
			err := p.backend.Do(req, res)

			loggedFields := make([]zap.Field, 0, 12)

			loggedFields = append(loggedFields,
				zap.Bool("mirror", doMirror),
				zap.String("jrpc_method", jrpcMethod),
				zap.Uint64("jrpc_id", jrpcID),
				zap.Uint64("http_upstream_connection_id", connectionID),
				zap.String("http_upstream_ip", remoteIP),
				zap.String("http_downstream_host", str(p.backendURI.Host())),
			)

			if p.cfg.LogRequests {
				loggedFields = append(loggedFields,
					zap.String("http_request", str(req.Body())),
				)
			}

			if err == nil {
				res.CopyTo(&ctx.Response)

				loggedFields = append(loggedFields,
					zap.Int("http_status", res.StatusCode()),
				)

				if p.cfg.LogResponses {
					loggedFields = append(loggedFields,
						zap.String("http_response", str(res.Body())),
					)
				}

				l.Info("Proxied the request", loggedFields...)

				metrics.ProxySuccessCount.Add(context.Background(), 1, otelapi.WithAttributes(
					attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
					attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(jrpcMethod)},
					attribute.KeyValue{Key: "http_status", Value: attribute.IntValue(res.StatusCode())},
				))
			} else {
				ctx.SetStatusCode(fasthttp.StatusBadGateway)
				fmt.Fprint(ctx, err.Error())

				loggedFields = append(loggedFields,
					zap.Error(err),
				)

				l.Error("Failed to proxy the request", loggedFields...)

				metrics.ProxyFailureCount.Add(context.Background(), 1, otelapi.WithAttributes(
					attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
					attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(jrpcMethod)},
				))
			}

			wg.Done()
		}()
	}

	if doMirror {
		for _, uri := range p.peerURIs {
			req := fasthttp.AcquireRequest()
			res := fasthttp.AcquireResponse()

			ctx.Request.CopyTo(req)
			req.SetURI(uri)
			req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
			req.Header.Add("x-forwarded-host", str(ctx.Host()))
			req.Header.Add("x-forwarded-proto", str(ctx.Request.URI().Scheme()))

			go func() {
				//
				// NOTE: must _not_ use (or hold references to) `ctx` down here
				//

				err := p.backend.Do(req, res)

				loggedFields := make([]zap.Field, 0, 12)

				loggedFields = append(loggedFields,
					zap.String("jrpc_method", jrpcMethod),
					zap.Uint64("jrpc_id", jrpcID),
					zap.Uint64("http_upstream_connection_id", connectionID),
					zap.String("http_upstream_ip", remoteIP),
					zap.String("http_downstream_host", str(uri.Host())),
				)

				if p.cfg.LogRequests {
					loggedFields = append(loggedFields,
						zap.String("http_request", str(req.Body())),
					)
				}

				if err == nil {
					loggedFields = append(loggedFields,
						zap.Int("http_status", res.StatusCode()),
					)

					if p.cfg.LogResponses {
						loggedFields = append(loggedFields,
							zap.String("http_response", str(res.Body())),
						)
					}

					l.Info("Mirrored the request", loggedFields...)

					metrics.MirrorSuccessCount.Add(context.Background(), 1, otelapi.WithAttributes(
						attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
						attribute.KeyValue{Key: "downstream_host", Value: attribute.StringValue(str(uri.Host()))},
						attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(jrpcMethod)},
						attribute.KeyValue{Key: "http_status", Value: attribute.IntValue(res.StatusCode())},
					))
				} else {
					loggedFields = append(loggedFields,
						zap.Error(err),
					)

					l.Error("Failed to mirror the request", loggedFields...)

					metrics.MirrorFailureCount.Add(context.Background(), 1, otelapi.WithAttributes(
						attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
						attribute.KeyValue{Key: "downstream_host", Value: attribute.StringValue(str(uri.Host()))},
						attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(jrpcMethod)},
					))
				}

				_ = l.Sync()

				fasthttp.ReleaseRequest(req)
				fasthttp.ReleaseResponse(res)
			}()
		}
	}

	wg.Wait()

	_ = l.Sync()
}

func (p *Proxy) upstreamConnectionChanged(conn net.Conn, state fasthttp.ConnState) {
	logFields := []zap.Field{
		zap.String("local_ip", conn.LocalAddr().String()),
		zap.String("remote_ip", conn.RemoteAddr().String()),
	}

	switch state {
	case fasthttp.StateNew:
		p.logger.Info("Upstream connection was established", logFields...)
	case fasthttp.StateActive:
		p.logger.Debug("Upstream connection became active", logFields...)
	case fasthttp.StateIdle:
		p.logger.Debug("Upstream connection became idle", logFields...)
	case fasthttp.StateHijacked:
		p.logger.Info("Upstream connection was hijacked", logFields...)
	case fasthttp.StateClosed:
		p.logger.Info("Upstream connection was closed", logFields...)
	}
}
