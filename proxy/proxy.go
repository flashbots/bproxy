package proxy

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
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

	triage func(body []byte) triagedRequest
	run    func()
	stop   func()

	connections         map[string]net.Conn
	drainingConnections map[string]net.Conn
	mxConnections       sync.Mutex
}

func newProxy(cfg *Config) (*Proxy, error) {
	l := zap.L().With(zap.String("proxy_name", cfg.Name))

	p := &Proxy{
		cfg:                 cfg,
		logger:              l,
		connections:         make(map[string]net.Conn),
		drainingConnections: make(map[string]net.Conn),
	}

	p.triage = p.defaultTriage

	p.frontend = &fasthttp.Server{
		ConnState:          p.upstreamConnectionChanged,
		Handler:            p.handle,
		IdleTimeout:        5 * time.Minute,
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

	if p.run != nil {
		p.run()
	}

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

	if p.stop != nil {
		p.stop()
	}

	return res
}

func (p *Proxy) ResetConnections() {
	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	for addr, conn := range p.connections {
		if _, alreadyDraining := p.drainingConnections[addr]; alreadyDraining {
			p.logger.Error("Draining connection address collision",
				zap.String("remote_addr", addr),
			)
		}
		p.drainingConnections[addr] = conn
		delete(p.connections, addr)
	}
}

func (p *Proxy) defaultTriage(body []byte) triagedRequest {
	return triagedRequest{}
}

func (p *Proxy) handle(ctx *fasthttp.RequestCtx) {
	var (
		start = time.Now()

		l  *zap.Logger
		wg sync.WaitGroup

		req *fasthttp.Request
		res *fasthttp.Response

		proxy func(req *fasthttp.Request, res *fasthttp.Response) error
	)

	call := p.triage(ctx.Request.Body())

	{ // setup
		req = fasthttp.AcquireRequest()
		defer fasthttp.ReleaseRequest(req)

		ctx.Request.CopyTo(req)
		req.SetURI(p.backendURI)
		req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
		req.Header.Add("x-forwarded-host", str(ctx.Host()))
		req.Header.Add("x-forwarded-proto", str(ctx.Request.URI().Scheme()))

		loggedFields := make([]zap.Field, 0, 12)

		switch {
		case p.cfg.Chaos.Enabled && rand.Float64() < p.cfg.Chaos.InjectedHttpErrorProbability/100:
			proxy = p.injectHttpError
			res = fasthttp.AcquireResponse()
			call.proxy = false
			call.mirror = false
			loggedFields = append(loggedFields,
				zap.Bool("chaos_http_error", true),
			)

		case p.cfg.Chaos.Enabled && rand.Float64() < p.cfg.Chaos.InjectedJrpcErrorProbability/100:
			proxy = p.injectJrpcError(call.jrpcID, req, res)
			res = fasthttp.AcquireResponse()
			call.proxy = false
			call.mirror = false
			loggedFields = append(loggedFields,
				zap.Bool("chaos_jrpc_error", true),
			)

		case call.proxy:
			proxy = p.backend.Do
			res = fasthttp.AcquireResponse()

		default:
			proxy = func(_ *fasthttp.Request, _ *fasthttp.Response) error { return nil }
			res = call.response
			call.mirror = false // request won't be proxied, shouldn't mirror either
		}
		defer fasthttp.ReleaseResponse(res)

		loggedFields = append(loggedFields,
			zap.Bool("proxy", call.proxy),
			zap.Bool("mirror", call.mirror),
			zap.Uint64("connection_id", ctx.ConnID()),
			zap.String("remote_addr", ctx.RemoteAddr().String()),
			zap.String("jrpc_method", call.jrpcMethod),
			zap.Uint64("jrpc_id", call.jrpcID),
		)

		if call.txHash != "" {
			loggedFields = append(loggedFields,
				zap.String("tx_hash", call.txHash),
			)
		}

		l = p.logger.With(loggedFields...)
	}

	wg.Add(1)

	go func() {
		defer wg.Done()

		var (
			loggedFields = make([]zap.Field, 0, 12)
		)

		err := proxy(req, res)

		{ // add log fields
			loggedFields = append(loggedFields,
				zap.String("downstream_host", str(p.backendURI.Host())),
			)

			if p.cfg.LogRequests {
				var jsonRequest interface{}
				if err := json.Unmarshal(req.Body(), &jsonRequest); err == nil {
					loggedFields = append(loggedFields,
						zap.Any("json_request", jsonRequest),
					)
				} else {
					loggedFields = append(loggedFields,
						zap.String("http_request", str(req.Body())),
					)
				}
			}
		}

		metricAttributes := otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
			attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(call.jrpcMethod)},
		)

		if err != nil {
			ctx.SetStatusCode(fasthttp.StatusBadGateway)
			fmt.Fprint(ctx, err.Error())

			loggedFields = append(loggedFields,
				zap.Error(err),
			)

			l.Error("Failed to proxy the request", loggedFields...)
			metrics.ProxyFailureCount.Add(context.Background(), 1, metricAttributes)

			return
		}

		if p.cfg.Chaos.Enabled { // chaos-inject latency
			latency := time.Duration(rand.Int64N(int64(p.cfg.Chaos.MaxInjectedLatency) + 1))
			latency = max(latency, p.cfg.Chaos.MinInjectedLatency)
			time.Sleep(latency - time.Since(start))
			loggedFields = append(loggedFields,
				zap.Bool("chaos_latency", true),
			)
		}
		res.CopyTo(&ctx.Response)

		{ // add log fields
			loggedFields = append(loggedFields,
				zap.Duration("http_latency", time.Since(start)),
				zap.Int("http_status", res.StatusCode()),
			)

			if p.cfg.LogResponses {
				switch str(res.Header.ContentEncoding()) {
				default:
					var jsonResponse interface{}
					if err := json.Unmarshal(res.Body(), &jsonResponse); err == nil {
						loggedFields = append(loggedFields,
							zap.Any("json_response", jsonResponse),
						)
					} else {
						loggedFields = append(loggedFields,
							zap.String("http_response", str(res.Body())),
						)
					}

				case "gzip":
					if body, err := unzip(bytes.NewBuffer(res.Body())); err == nil {
						var jsonResponse interface{}
						if err := json.Unmarshal(body, &jsonResponse); err == nil {
							loggedFields = append(loggedFields,
								zap.Any("json_response", jsonResponse),
							)
						} else {
							loggedFields = append(loggedFields,
								zap.String("http_response", str(body)),
							)
						}
					} else {
						loggedFields = append(loggedFields,
							zap.NamedError("error_gzip", err),
							zap.String("hex_response", hex.EncodeToString(res.Body())),
						)
					}
				}
			}
		}

		if call.proxy {
			l.Info("Proxied the request", loggedFields...)
			metrics.ProxySuccessCount.Add(context.Background(), 1, metricAttributes)
		} else {
			l.Info("Faked the request", loggedFields...)
			metrics.ProxyFakeCount.Add(context.Background(), 1, metricAttributes)
		}
	}()

	if call.mirror {
		for _, uri := range p.peerURIs {
			req := fasthttp.AcquireRequest()
			res := fasthttp.AcquireResponse()

			ctx.Request.CopyTo(req)
			req.SetURI(uri)
			req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
			req.Header.Add("x-forwarded-host", str(ctx.Host()))
			req.Header.Add("x-forwarded-proto", str(ctx.Request.URI().Scheme()))

			go func() {
				var loggedFields = make([]zap.Field, 0, 12)

				//
				// NOTE: must _not_ use (or hold references to) `ctx` down here
				//

				err := p.backend.Do(req, res)

				{ // add log fields
					loggedFields = append(loggedFields,
						zap.String("downstream_host", str(uri.Host())),
					)

					if p.cfg.LogRequests {
						var jsonRequest interface{}
						if err := json.Unmarshal(req.Body(), &jsonRequest); err == nil {
							loggedFields = append(loggedFields,
								zap.Any("json_request", jsonRequest),
							)
						} else {
							loggedFields = append(loggedFields,
								zap.String("http_request", str(req.Body())),
							)
						}
					}
				}

				metricAttributes := otelapi.WithAttributes(
					attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
					attribute.KeyValue{Key: "downstream_host", Value: attribute.StringValue(str(uri.Host()))},
					attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(call.jrpcMethod)},
				)

				if err == nil {
					{ // add log fields
						loggedFields = append(loggedFields,
							zap.Int("http_status", res.StatusCode()),
						)

						if p.cfg.LogResponses {
							switch str(res.Header.ContentEncoding()) {
							default:
								var jsonResponse interface{}
								if err := json.Unmarshal(res.Body(), &jsonResponse); err == nil {
									loggedFields = append(loggedFields,
										zap.Any("json_response", jsonResponse),
									)
								} else {
									loggedFields = append(loggedFields,
										zap.String("http_response", str(res.Body())),
									)
								}

							case "gzip":
								if body, err := unzip(bytes.NewBuffer(res.Body())); err == nil {
									var jsonResponse interface{}
									if err := json.Unmarshal(body, &jsonResponse); err == nil {
										loggedFields = append(loggedFields,
											zap.Any("json_response", jsonResponse),
										)
									} else {
										loggedFields = append(loggedFields,
											zap.String("http_response", str(body)),
										)
									}
								} else {
									loggedFields = append(loggedFields,
										zap.NamedError("error_gzip", err),
										zap.String("hex_response", hex.EncodeToString(res.Body())),
									)
								}
							}
						}
					}

					l.Info("Mirrored the request", loggedFields...)
					metrics.MirrorSuccessCount.Add(context.Background(), 1, metricAttributes)
				} else {
					loggedFields = append(loggedFields,
						zap.Error(err),
					)

					l.Error("Failed to mirror the request", loggedFields...)
					metrics.MirrorFailureCount.Add(context.Background(), 1, metricAttributes)
				}

				_ = l.Sync()

				fasthttp.ReleaseRequest(req)
				fasthttp.ReleaseResponse(res)
			}()
		}
	}

	wg.Wait()

	{ // check if this is a draining connection
		addr := ctx.RemoteIP().String()

		p.mxConnections.Lock()
		defer p.mxConnections.Unlock()

		if conn, isDraining := p.drainingConnections[addr]; isDraining {
			delete(p.drainingConnections, addr)
			err := conn.Close()
			l.Info("Drained the upstream connection as finished handling a request",
				zap.Error(err),
				zap.Int("remaining", len(p.drainingConnections)),
			)
		}
	}

	_ = l.Sync()
}

func (p *Proxy) injectHttpError(_ *fasthttp.Request, res *fasthttp.Response) error {
	res.SetStatusCode(fasthttp.StatusInternalServerError)
	res.SetBody([]byte("chaos-injected error"))

	return nil
}

func (p *Proxy) injectJrpcError(
	jrpcID uint64, _ *fasthttp.Request, _ *fasthttp.Response,
) func(_ *fasthttp.Request, _ *fasthttp.Response) error {
	return func(_ *fasthttp.Request, res *fasthttp.Response) error {
		res.SetStatusCode(fasthttp.StatusOK)
		res.Header.Add("content-type", "application/json; charset=utf-8")
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%d,"error":{"code":-32042,"message:"chaos-injected error"}}`,
			jrpcID,
		)))

		return nil
	}
}

func (p *Proxy) upstreamConnectionChanged(conn net.Conn, state fasthttp.ConnState) {
	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	addr := conn.RemoteAddr().String()

	l := p.logger.With(
		zap.String("remote_addr", addr),
	)

	switch state {
	case fasthttp.StateNew:
		l.Info("Upstream connection was established")
		p.connections[addr] = conn

	case fasthttp.StateActive:
		l.Debug("Upstream connection became active")

	case fasthttp.StateIdle:
		l.Debug("Upstream connection became idle")
		if _, draining := p.drainingConnections[addr]; draining {
			delete(p.drainingConnections, addr)
			err := conn.Close()
			l.Info("Drained the upstream connection as it became idle",
				zap.Error(err),
				zap.Int("remaining", len(p.drainingConnections)),
			)
		}

	case fasthttp.StateHijacked:
		l.Info("Upstream connection was hijacked")
		delete(p.connections, addr)
		delete(p.drainingConnections, addr)

	case fasthttp.StateClosed:
		l.Info("Upstream connection was closed")
		delete(p.connections, addr)
		delete(p.drainingConnections, addr)
	}
}

func (p *Proxy) Observe(ctx context.Context, o otelapi.Observer) error {
	o.ObserveInt64(metrics.FrontendConnectionsCount, int64(p.frontend.GetOpenConnectionsCount()), otelapi.WithAttributes(
		attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
	))
	return nil
}
