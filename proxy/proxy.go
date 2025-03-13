package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
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

	BackendURI string
	PeerURIs   []string

	Parse func(body []byte) (doMirror bool, jrpcMethod string, jrpcID uint64)
}

func New(cfg *Config) (*Proxy, error) {
	p := &Proxy{
		cfg:    cfg,
		logger: zap.L().With(zap.String("proxy_name", cfg.Name)),
		parse:  cfg.Parse,
	}

	p.frontend = &fasthttp.Server{
		Handler:            p.handle,
		IdleTimeout:        30 * time.Second,
		MaxRequestBodySize: 64 * 1024,
		Name:               cfg.Name,
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       5 * time.Second,
	}

	p.backend = &fasthttp.Client{
		MaxIdleConnDuration: 30 * time.Second,
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

	var wg sync.WaitGroup

	connectionID := ctx.ConnID()

	{
		wg.Add(1)

		l.Info("Proxying request",
			zap.Uint64("http_connection_id", connectionID),
			zap.String("http_remote_ip", ctx.RemoteIP().String()),
			zap.String("http_method", str(ctx.Method())),
			zap.String("http_path", str(ctx.Path())),
			zap.String("backend", p.backendURI.String()),
		)

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
			if err := p.backend.Do(req, res); err == nil {
				res.CopyTo(&ctx.Response)
				l.Debug("Done proxying request",
					zap.Uint64("http_connection_id", connectionID),
					zap.Int("http_status", res.StatusCode()),
				)
			} else {
				ctx.SetStatusCode(fasthttp.StatusBadGateway)
				fmt.Fprint(ctx, err.Error())
				l.Error("Proxy backend failed",
					zap.Uint64("http_connection_id", connectionID),
					zap.Error(err),
				)
			}

			wg.Done()
		}()
	}

	if ctx.IsPost() {
		if doMirror, jrpcMethod, jrpcID := p.parse(ctx.PostBody()); doMirror {
			for _, uri := range p.peerURIs {
				l.Info("Mirroring request",
					zap.Uint64("http_connection_id", connectionID),
					zap.String("http_remote_ip", ctx.RemoteIP().String()),
					zap.String("http_method", str(ctx.Method())),
					zap.String("http_path", str(ctx.Path())),
					zap.String("jrpc_method", jrpcMethod),
					zap.Uint64("jrpc_jrpc_request_idid", jrpcID),
					zap.String("backend", uri.String()),
				)

				req := fasthttp.AcquireRequest()
				res := fasthttp.AcquireResponse()

				ctx.Request.CopyTo(req)
				req.SetURI(uri)
				req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
				req.Header.Add("x-forwarded-host", str(ctx.Host()))
				req.Header.Add("x-forwarded-proto", str(ctx.Request.URI().Scheme()))

				go func() {
					// must not hold references to ctx down here

					if err := p.backend.Do(req, res); err == nil {
						l.Debug("Done mirroring request",
							zap.Uint64("http_connection_id", connectionID),
							zap.Int("http_status", res.StatusCode()),
						)
					} else {
						l.Warn("Mirroring peer failed",
							zap.Uint64("http_connection_id", connectionID),
							zap.Error(err),
						)
					}
					fasthttp.ReleaseRequest(req)
					fasthttp.ReleaseResponse(res)
				}()
			}
		}
	}

	wg.Wait()
}
