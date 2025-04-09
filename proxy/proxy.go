package proxy

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net"
	"strconv"

	"sync"
	"time"

	"github.com/flashbots/bproxy/logutils"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/types"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type Proxy struct {
	cfg *Config

	backend  *fasthttp.Client
	frontend *fasthttp.Server
	peer     *fasthttp.Client

	backendURI *fasthttp.URI
	peerURIs   []*fasthttp.URI

	healthcheck         *fasthttp.Client
	healthcheckURI      *fasthttp.URI
	healthcheckTicker   *time.Ticker
	healthcheckStatuses *types.RingBuffer[bool]
	healthcheckDepth    int
	isHealthy           bool

	logger *zap.Logger

	triage func(body []byte) *triagedRequest
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
		IdleTimeout:        30 * time.Second,
		Logger:             logutils.FasthttpLogger(l),
		MaxConnsPerIP:      cfg.Proxy.MaxClientConnectionsPerIP,
		MaxRequestBodySize: cfg.Proxy.MaxRequestSizeMb * 1024 * 1024,
		Name:               cfg.Name,
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       5 * time.Second,
	}

	if cfg.Proxy.TLSCertificate != "" && cfg.Proxy.TLSKey != "" {
		cert, err := cfg.Proxy.LoadTLSCertificate()
		if err != nil {
			return nil, err
		}

		p.frontend.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	p.backend = &fasthttp.Client{
		MaxConnsPerHost:     cfg.Proxy.MaxBackendConnectionsPerHost,
		MaxConnWaitTimeout:  cfg.Proxy.MaxBackendConnectionWaitTimeout,
		MaxIdleConnDuration: 30 * time.Second,
		MaxResponseBodySize: cfg.Proxy.MaxResponseSizeMb * 1024 * 1024,
		Name:                cfg.Name,
		ReadTimeout:         5 * time.Second,
		WriteTimeout:        5 * time.Second,
	}

	p.backendURI = fasthttp.AcquireURI()
	if err := p.backendURI.Parse(nil, []byte(cfg.Proxy.BackendURL)); err != nil {
		fasthttp.ReleaseURI(p.backendURI)
		return nil, err
	}

	if len(cfg.Proxy.PeerURLs) > 0 {
		p.peer = &fasthttp.Client{
			MaxConnsPerHost:     cfg.Proxy.MaxBackendConnectionsPerHost,
			MaxConnWaitTimeout:  cfg.Proxy.MaxBackendConnectionWaitTimeout,
			MaxIdleConnDuration: 30 * time.Second,
			MaxResponseBodySize: cfg.Proxy.MaxResponseSizeMb * 1024 * 1024,
			Name:                cfg.Name,
			ReadTimeout:         5 * time.Second,
			WriteTimeout:        5 * time.Second,
		}

		if cfg.Proxy.PeerTLSInsecureSkipVerify {
			p.peer.TLSConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}
	}

	p.peerURIs = make([]*fasthttp.URI, 0, len(cfg.Proxy.PeerURLs))
	for _, peerURL := range cfg.Proxy.PeerURLs {
		peerURI := fasthttp.AcquireURI()
		if err := peerURI.Parse(nil, []byte(peerURL)); err != nil {
			fasthttp.ReleaseURI(p.backendURI)
			for _, uri := range p.peerURIs {
				fasthttp.ReleaseURI(uri)
			}
			return nil, err
		}
		p.peerURIs = append(p.peerURIs, peerURI)
	}

	if cfg.Proxy.HealthcheckURL != "" {
		p.healthcheckURI = fasthttp.AcquireURI()
		if err := p.healthcheckURI.Parse(nil, []byte(cfg.Proxy.HealthcheckURL)); err != nil {
			fasthttp.ReleaseURI(p.backendURI)
			for _, uri := range p.peerURIs {
				fasthttp.ReleaseURI(uri)
			}
			fasthttp.ReleaseURI(p.healthcheckURI)
			return nil, err
		}

		p.healthcheck = &fasthttp.Client{
			MaxConnsPerHost:     1,
			MaxConnWaitTimeout:  cfg.Proxy.HealthcheckInterval / 2,
			MaxIdleConnDuration: 2 * cfg.Proxy.HealthcheckInterval,
			MaxResponseBodySize: 4096,
			Name:                cfg.Name + "-healthcheck",
			ReadTimeout:         cfg.Proxy.HealthcheckInterval / 2,
			WriteTimeout:        cfg.Proxy.HealthcheckInterval / 2,
		}

		p.healthcheckTicker = time.NewTicker(cfg.Proxy.HealthcheckInterval)

		p.healthcheckDepth = max(
			p.cfg.Proxy.HealthcheckThresholdHealthy,
			p.cfg.Proxy.HealthcheckThresholdUnhealthy,
		)

		p.healthcheckStatuses = types.NewRingBuffer[bool](p.healthcheckDepth)

		p.isHealthy = true
	}

	return p, nil
}

func (p *Proxy) Run(ctx context.Context, failure chan<- error) {
	if p == nil {
		return
	}

	l := p.logger

	if p.run != nil {
		p.run()
	}

	go func() { // run the authrpc proxy
		l.Info("Proxy is going up...",
			zap.String("listen_address", p.cfg.Proxy.ListenAddress),
			zap.String("backend", p.cfg.Proxy.BackendURL),
			zap.Strings("peers", p.cfg.Proxy.PeerURLs),
		)
		if p.cfg.Proxy.TLSCertificate != "" && p.cfg.Proxy.TLSKey != "" {
			if err := p.frontend.ListenAndServeTLS(p.cfg.Proxy.ListenAddress, "", ""); err != nil {
				failure <- err
			}
		} else {
			if err := p.frontend.ListenAndServe(p.cfg.Proxy.ListenAddress); err != nil {
				failure <- err
			}
		}
		l.Info("Proxy is down")
	}()

	if p.cfg.Proxy.HealthcheckURL != "" {
		go func() {
			for {
				p.backendHealthcheck(ctx, <-p.healthcheckTicker.C)
			}
		}()
	}
}

func (p *Proxy) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}

	if p.stop != nil {
		p.stop()
	}

	if p.cfg.Proxy.HealthcheckURL != "" {
		p.healthcheckTicker.Stop()
		fasthttp.ReleaseURI(p.healthcheckURI)
	}

	res := p.frontend.ShutdownWithContext(ctx)

	fasthttp.ReleaseURI(p.backendURI)
	for _, uri := range p.peerURIs {
		fasthttp.ReleaseURI(uri)
	}

	return res
}

func (p *Proxy) ResetConnections() {
	if p == nil {
		return
	}

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

func (p *Proxy) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}

	o.ObserveInt64(metrics.FrontendConnectionsCount, int64(p.frontend.GetOpenConnectionsCount()), otelapi.WithAttributes(
		attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
	))

	if p.frontend.TLSConfig != nil {
		for _, cert := range p.frontend.TLSConfig.Certificates {
			if cert.Leaf != nil {
				o.ObserveInt64(metrics.TLSValidNotAfter, cert.Leaf.NotAfter.Unix(), otelapi.WithAttributes(
					attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
				))

				o.ObserveInt64(metrics.TLSValidNotBefore, cert.Leaf.NotBefore.Unix(), otelapi.WithAttributes(
					attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
				))
			}
		}
	}

	return nil
}

func (p *Proxy) defaultTriage(body []byte) *triagedRequest {
	return &triagedRequest{}
}

func (p *Proxy) handle(ctx *fasthttp.RequestCtx) {
	var (
		tsReqReceived = time.Now()

		l  *zap.Logger
		wg sync.WaitGroup

		req *fasthttp.Request
		res *fasthttp.Response

		proxy func(req *fasthttp.Request, res *fasthttp.Response) error
	)

	metrics.RequestSize.Record(context.TODO(), int64(ctx.Request.Header.ContentLength()), otelapi.WithAttributes(
		attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
		attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(str(ctx.Request.Header.ContentEncoding()))},
	))

	call := p.triage(ctx.Request.Body())

	{ // setup
		req = fasthttp.AcquireRequest()
		defer fasthttp.ReleaseRequest(req)

		ctx.Request.CopyTo(req)
		req.SetTimeout(p.cfg.Proxy.BackendTimeout)
		req.SetURI(p.backendURI)
		req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
		req.Header.Add("x-forwarded-host", str(ctx.Host()))
		req.Header.Add("x-forwarded-proto", str(ctx.Request.URI().Scheme()))

		loggedFields := make([]zap.Field, 0, 12)

		switch { // configure processing mode (proxy, fake, chaos)
		case p.cfg.Chaos.Enabled:
			if rand.Float64() < p.cfg.Chaos.InjectedHttpErrorProbability/100 { // inject http error
				proxy = p.injectHttpError
				res = fasthttp.AcquireResponse()
				call.proxy = false
				call.mirror = false
				loggedFields = append(loggedFields,
					zap.Bool("chaos_http_error", true),
				)
			}
			if rand.Float64() < p.cfg.Chaos.InjectedJrpcErrorProbability/100 { // inject jrpc error
				proxy = p.injectJrpcError(call, req, res)
				res = fasthttp.AcquireResponse()
				call.proxy = false
				call.mirror = false
				loggedFields = append(loggedFields,
					zap.Bool("chaos_jrpc_error", true),
				)
			}
			if rand.Float64() < p.cfg.Chaos.InjectedInvalidJrpcResponseProbability/100 { // inject bad jrpc response
				proxy = p.injectInvalidJrpcResponse
				res = fasthttp.AcquireResponse()
				call.proxy = false
				call.mirror = false
				loggedFields = append(loggedFields,
					zap.Bool("chaos_invalid_jrpc_response", true),
				)
			}

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
			zap.Time("ts_request_received", tsReqReceived),
			zap.Bool("proxy", call.proxy),
			zap.Bool("mirror", call.mirror),
			zap.Uint64("connection_id", ctx.ConnID()),
			zap.String("remote_addr", ctx.RemoteAddr().String()),
			zap.String("downstream_host", str(p.backendURI.Host())),
			zap.String("jrpc_method", call.jrpcMethod),
		)

		if len(call.transactions) > 0 {
			loggedFields = append(loggedFields,
				zap.Array("txs", call.transactions),
			)
		}

		l = p.logger.With(loggedFields...)
	}

	wg.Add(1)

	go func() {
		defer wg.Done()

		var (
			loggedFields = make([]zap.Field, 0, 12)
			success      = true
		)

		tsReqProxyStart := time.Now()
		err := proxy(req, res)
		tsReqProxyEnd := time.Now()

		metrics.ResponseSize.Record(context.TODO(), int64(res.Header.ContentLength()), otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(str(res.Header.ContentEncoding()))},
		))

		{ // add log fields
			if p.cfg.Proxy.LogRequests {
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
			success = false

			switch str(req.Header.ContentType()) {
			case "application/json":
				res.SetStatusCode(fasthttp.StatusAccepted)
				res.SetBody([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","error":{"code":-32042,"message":%s}}`, strconv.Quote(err.Error()))))
			default:
				res.SetBody([]byte(err.Error()))
				res.SetStatusCode(fasthttp.StatusBadGateway)
			}

			loggedFields = append(loggedFields,
				zap.NamedError("error_backend", err),
			)
		}

		if p.cfg.Chaos.Enabled { // chaos-inject latency
			latency := time.Duration(rand.Int64N(int64(p.cfg.Chaos.MaxInjectedLatency) + 1))
			latency = max(latency, p.cfg.Chaos.MinInjectedLatency)
			time.Sleep(latency - time.Since(tsReqReceived))
			loggedFields = append(loggedFields,
				zap.Bool("chaos_latency", true),
			)
		}
		res.CopyTo(&ctx.Response)
		tsResProxyEnd := time.Now()

		{ // add log fields
			if p.cfg.Proxy.LogResponses {
				var body []byte

				switch str(res.Header.ContentEncoding()) {
				default:
					body = res.Body()
				case "gzip":
					if body, err = res.BodyGunzip(); err != nil {
						loggedFields = append(loggedFields,
							zap.NamedError("error_gunzip", err),
							zap.String("hex_response", hex.EncodeToString(res.Body())),
						)
					}
				}

				if body != nil {
					var jsonResponse interface{}
					if err := json.Unmarshal(body, &jsonResponse); err == nil {
						loggedFields = append(loggedFields,
							zap.Any("json_response", jsonResponse),
						)
					} else {
						loggedFields = append(loggedFields,
							zap.NamedError("error_unmarshal", err),
							zap.String("http_response", str(body)),
						)
					}
				}
			}

			now := time.Now()
			latency_warmup := tsReqProxyStart.Sub(tsReqReceived)
			latency_backend := tsReqProxyEnd.Sub(tsReqProxyStart)
			latency_cooldown := now.Sub(tsResProxyEnd)

			loggedFields = append(loggedFields,
				zap.Int("http_status", res.StatusCode()),
				zap.Duration("latency_warmup", latency_warmup),
				zap.Duration("latency_backend", latency_backend),
				zap.Duration("latency_cooldown", latency_cooldown),
				zap.Duration("latency_total", now.Sub(tsReqReceived)),
			)

			metrics.LatencyBackend.Record(ctx, latency_backend.Milliseconds(), metricAttributes)
			metrics.LatencyProxy.Record(ctx, (latency_warmup + latency_cooldown).Milliseconds(), metricAttributes)
		}

		if !success {
			l.Error("Failed to proxy the request", loggedFields...)
			metrics.ProxyFailureCount.Add(context.TODO(), 1, metricAttributes)
		} else if call.proxy {
			l.Info("Proxied the request", loggedFields...)
			metrics.ProxySuccessCount.Add(context.TODO(), 1, metricAttributes)
		} else {
			l.Info("Faked the request", loggedFields...)
			metrics.ProxyFakeCount.Add(context.TODO(), 1, metricAttributes)
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

				err := p.peer.Do(req, res)

				{ // add log fields
					loggedFields = append(loggedFields,
						zap.String("downstream_host", str(uri.Host())),
					)

					if p.cfg.Proxy.LogRequests {
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

						if p.cfg.Proxy.LogResponses {
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
								if body, err := res.BodyGunzip(); err == nil {
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
					metrics.MirrorSuccessCount.Add(context.TODO(), 1, metricAttributes)
				} else {
					loggedFields = append(loggedFields,
						zap.Error(err),
					)

					l.Error("Failed to mirror the request", loggedFields...)
					metrics.MirrorFailureCount.Add(context.TODO(), 1, metricAttributes)
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
	call *triagedRequest, _ *fasthttp.Request, _ *fasthttp.Response,
) func(_ *fasthttp.Request, _ *fasthttp.Response) error {
	return func(_ *fasthttp.Request, res *fasthttp.Response) error {
		res.SetStatusCode(fasthttp.StatusOK)
		res.Header.Add("content-type", "application/json; charset=utf-8")
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32042,"message:"chaos-injected error"}}`,
			call.jrpcID,
		)))

		return nil
	}
}

func (p *Proxy) injectInvalidJrpcResponse(_ *fasthttp.Request, res *fasthttp.Response) error {
	res.SetStatusCode(fasthttp.StatusOK)
	res.Header.Add("content-type", "application/json; charset=utf-8")
	res.SetBody([]byte("chaos-injected invalid jrpc response"))

	return nil
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

func (p *Proxy) connectionsCount() int {
	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	return len(p.connections)
}

func (p *Proxy) backendHealthcheck(ctx context.Context, _ time.Time) {
	l := logutils.LoggerFromContext(ctx)

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	req.SetURI(p.healthcheckURI)
	req.Header.SetMethod("GET")
	req.SetTimeout(p.cfg.Proxy.HealthcheckInterval / 2)

	if err := p.backend.Do(req, res); err == nil {
		switch res.StatusCode() {
		case fasthttp.StatusOK, fasthttp.StatusAccepted:
			p.healthcheckStatuses.Push(true)
		default:
			p.healthcheckStatuses.Push(false)
		}
	} else {
		l.Warn("Failed to query the healthcheck endpoint",
			zap.Error(err),
			zap.String("proxy", p.cfg.Name),
		)
		p.healthcheckStatuses.Push(false)
	}

	if p.healthcheckStatuses.Length() > p.healthcheckDepth {
		_, _ = p.healthcheckStatuses.Pop()
	}

	isHealthy := true
	for idx := p.healthcheckDepth - 1; idx >= p.healthcheckDepth-p.cfg.Proxy.HealthcheckThresholdHealthy; idx-- {
		if s, ok := p.healthcheckStatuses.At(idx); ok {
			isHealthy = isHealthy && s
		}
	}

	isUnhealthy := true
	for idx := p.healthcheckDepth - 1; idx >= p.healthcheckDepth-p.cfg.Proxy.HealthcheckThresholdUnhealthy; idx-- {
		if s, ok := p.healthcheckStatuses.At(idx); ok {
			isUnhealthy = isUnhealthy && !s
		}
	}

	if p.isHealthy && isUnhealthy {
		p.isHealthy = false
		l.Info("Backend became unhealthy",
			zap.String("proxy", p.cfg.Name),
		)
	} else if !p.isHealthy && isHealthy {
		p.isHealthy = true
		l.Info("Backend is healthy again",
			zap.String("proxy", p.cfg.Name),
		)
	}

	if !p.isHealthy && p.connectionsCount() > 0 {
		l.Warn("Resetting frontend connections b/c backend is (still) unhealthy...",
			zap.String("proxy", p.cfg.Name),
		)
		p.ResetConnections()
	}
}
