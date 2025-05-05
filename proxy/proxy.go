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
	"strings"

	"sync"
	"time"

	"github.com/flashbots/bproxy/data"
	"github.com/flashbots/bproxy/logutils"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/triaged"

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

	backendURI               *fasthttp.URI
	extraMirroredJrpcMethods map[string]struct{}
	peerURIs                 []*fasthttp.URI

	healthcheck         *fasthttp.Client
	healthcheckURI      *fasthttp.URI
	healthcheckTicker   *time.Ticker
	healthcheckStatuses *data.RingBuffer[bool]
	healthcheckDepth    int
	isHealthy           bool

	logger *zap.Logger

	triage func(body []byte) (*triaged.Request, *fasthttp.Response)
	run    func()
	stop   func()

	connections         map[string]net.Conn
	drainingConnections map[string]net.Conn
	mxConnections       sync.Mutex

	jobQueueHi chan *jobProxy
	jobQueueLo chan *jobProxy
}

func newProxy(cfg *Config) (*Proxy, error) {
	l := zap.L().With(zap.String("proxy_name", cfg.Name))

	p := &Proxy{
		cfg:                 cfg,
		logger:              l,
		connections:         make(map[string]net.Conn),
		drainingConnections: make(map[string]net.Conn),
		jobQueueHi:          make(chan *jobProxy, 512),
		jobQueueLo:          make(chan *jobProxy, 512),
	}

	p.triage = func(body []byte) (*triaged.Request, *fasthttp.Response) {
		return &triaged.Request{}, fasthttp.AcquireResponse()
	}

	p.frontend = &fasthttp.Server{
		ConnState:          p.upstreamConnectionChanged,
		Handler:            p.receive,
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
		ReadTimeout:         cfg.Proxy.BackendTimeout,
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

		p.healthcheckStatuses = data.NewRingBuffer[bool](p.healthcheckDepth)

		p.isHealthy = true
	}

	if len(cfg.Proxy.ExtraMirroredJrpcMethods) > 0 {
		p.extraMirroredJrpcMethods = make(map[string]struct{}, len(cfg.Proxy.ExtraMirroredJrpcMethods))
		for _, method := range cfg.Proxy.ExtraMirroredJrpcMethods {
			method = strings.TrimSpace(method)
			if _, known := p.extraMirroredJrpcMethods[method]; !known {
				p.extraMirroredJrpcMethods[method] = struct{}{}
			}
		}
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

	go func() { // run the proxy
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

	go func() { // run the job loop
		for {
			select {
			case job := <-p.jobQueueHi:
				p.execJobProxy(job)
			default:
				select {
				case job := <-p.jobQueueHi:
					p.execJobProxy(job)
				case job := <-p.jobQueueLo:
					p.execJobProxy(job)
				}
			}
		}
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

func (p *Proxy) newJobProxy(ctx *fasthttp.RequestCtx) *jobProxy {
	job := &jobProxy{
		tsReqReceived: time.Now(),
		wg:            &sync.WaitGroup{},
	}

	job.triage, job.res = p.triage(ctx.Request.Body())

	{ // prepare the request
		job.req = fasthttp.AcquireRequest()

		ctx.Request.CopyTo(job.req)
		job.req.SetTimeout(p.cfg.Proxy.BackendTimeout)
		job.req.SetURI(p.backendURI)
		job.req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
		job.req.Header.Add("x-forwarded-host", str(ctx.Host()))
		job.req.Header.Add("x-forwarded-proto", str(ctx.Request.URI().Scheme()))
	}

	loggedFields := make([]zap.Field, 0, 12)

	{ // configure processing mode (proxy, fake, chaos)
		if p.extraMirroredJrpcMethods != nil {
			_, mirror := p.extraMirroredJrpcMethods[job.triage.JrpcMethod]
			job.triage.Mirror = job.triage.Mirror || mirror
		}

		if p.cfg.Chaos.Enabled {
			injectHttpError := rand.Float64() < p.cfg.Chaos.InjectedHttpErrorProbability/100
			injectJrpcError := rand.Float64() < p.cfg.Chaos.InjectedJrpcErrorProbability/100
			injectBadJrpcResponse := rand.Float64() < p.cfg.Chaos.InjectedInvalidJrpcResponseProbability/100

			if injectHttpError || injectJrpcError || injectBadJrpcResponse {
				job.triage.Proxy = false
				job.triage.Mirror = false
			}

			switch {
			case injectHttpError:
				job.proxy = p.injectHttpError
				loggedFields = append(loggedFields,
					zap.Bool("chaos_http_error", true),
				)
			case injectJrpcError:
				job.proxy = p.injectJrpcError(job.triage, job.req, job.res)
				loggedFields = append(loggedFields,
					zap.Bool("chaos_jrpc_error", true),
				)
			case injectBadJrpcResponse:
				job.proxy = p.injectInvalidJrpcResponse
				loggedFields = append(loggedFields,
					zap.Bool("chaos_invalid_jrpc_response", true),
				)
			}
		}

		if job.proxy == nil { // are not injecting chaos
			if job.triage.Proxy { // will proxy
				job.proxy = p.backend.Do
			} else { // will fake
				job.proxy = func(_ *fasthttp.Request, _ *fasthttp.Response) error { return nil }
			}
		}
	}

	{ // setup logger
		loggedFields = append(loggedFields,
			zap.Time("ts_request_received", job.tsReqReceived),
			zap.Bool("proxy", job.triage.Proxy),
			zap.Bool("mirror", job.triage.Mirror),
			zap.Uint64("connection_id", ctx.ConnID()),
			zap.String("remote_addr", ctx.RemoteAddr().String()),
			zap.String("downstream_host", str(p.backendURI.Host())),
			zap.String("jrpc_method", job.triage.JrpcMethod),
		)

		if len(job.triage.Transactions) > 0 {
			loggedFields = append(loggedFields,
				zap.Array("txs", job.triage.Transactions),
			)
		}

		job.log = p.logger.With(loggedFields...)
	}

	job.jrpcMethodForMetrics = job.triage.JrpcMethod
	if job.triage.Mirror {
		job.jrpcMethodForMetrics += "+"
	}

	return job
}

func (p *Proxy) newJobMirror(ctx *fasthttp.RequestCtx, pjob *jobProxy, uri *fasthttp.URI) *jobMirror {
	req := fasthttp.AcquireRequest()
	res := fasthttp.AcquireResponse()

	ctx.Request.CopyTo(req)
	req.SetURI(uri)
	req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
	req.Header.Add("x-forwarded-host", str(ctx.Host()))
	req.Header.Add("x-forwarded-proto", str(ctx.Request.URI().Scheme()))

	return &jobMirror{
		log:                  pjob.log,
		host:                 str(uri.Host()),
		req:                  req,
		res:                  res,
		jrpcMethodForMetrics: pjob.jrpcMethodForMetrics,
	}
}

func (p *Proxy) receive(ctx *fasthttp.RequestCtx) {
	job := p.newJobProxy(ctx)
	defer fasthttp.ReleaseResponse(job.res)
	defer fasthttp.ReleaseRequest(job.req)

	job.wg.Add(1)

	if job.triage.Prioritise {
		p.jobQueueHi <- job
	} else {
		p.jobQueueLo <- job
	}

	if job.triage.Mirror {
		for _, uri := range p.peerURIs {
			go p.execJobMirror(p.newJobMirror(ctx, job, uri))
		}
	}

	job.wg.Wait()

	job.res.CopyTo(&ctx.Response)

	{ // check if this is a draining connection
		addr := ctx.RemoteIP().String()

		p.mxConnections.Lock()
		defer p.mxConnections.Unlock()

		if conn, isDraining := p.drainingConnections[addr]; isDraining {
			delete(p.drainingConnections, addr)
			err := conn.Close()
			job.log.Info("Drained the upstream connection as finished handling a request",
				zap.Error(err),
				zap.Int("remaining", len(p.drainingConnections)),
			)
		}
	}

	_ = job.log.Sync()
}

func (p *Proxy) execJobProxy(job *jobProxy) {
	defer job.wg.Done()

	tsReqProxyStart := time.Now()
	err := job.proxy(job.req, job.res)
	tsReqProxyEnd := time.Now()
	success := (err == nil)

	loggedFields := make([]zap.Field, 0, 12)

	if err != nil {
		switch str(job.req.Header.ContentType()) {
		case "application/json":
			job.res.SetStatusCode(fasthttp.StatusAccepted)
			job.res.SetBody([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","error":{"code":-32042,"message":%s}}`, strconv.Quote(err.Error()))))
		default:
			job.res.SetBody([]byte(err.Error()))
			job.res.SetStatusCode(fasthttp.StatusBadGateway)
		}

		loggedFields = append(loggedFields,
			zap.NamedError("error_backend", err),
		)
	}

	if p.cfg.Chaos.Enabled { // chaos-inject latency
		latency := time.Duration(rand.Int64N(int64(p.cfg.Chaos.MaxInjectedLatency) + 1))
		latency = max(latency, p.cfg.Chaos.MinInjectedLatency)
		time.Sleep(latency - time.Since(job.tsReqReceived))
		loggedFields = append(loggedFields,
			zap.Bool("latency_chaos", true),
		)
	}

	{ // add log fields
		if p.cfg.Proxy.LogRequests {
			var jsonRequest interface{}
			if err := json.Unmarshal(job.req.Body(), &jsonRequest); err == nil {
				loggedFields = append(loggedFields,
					zap.Any("json_request", jsonRequest),
				)
			} else {
				loggedFields = append(loggedFields,
					zap.NamedError("error_unmarshal", err),
					zap.String("http_request", str(job.req.Body())),
				)
			}
		}

		if p.cfg.Proxy.LogResponses {
			var body []byte

			switch str(job.res.Header.ContentEncoding()) {
			default:
				body = job.res.Body()
			case "gzip":
				if body, err = job.res.BodyGunzip(); err != nil {
					loggedFields = append(loggedFields,
						zap.NamedError("error_gunzip", err),
						zap.String("hex_response", hex.EncodeToString(job.res.Body())),
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

		loggedFields = append(loggedFields,
			zap.Int("http_status", job.res.StatusCode()),
			zap.Duration("latency_backend", tsReqProxyEnd.Sub(tsReqProxyStart)),
			zap.Duration("latency_total", time.Since(job.tsReqReceived)),
		)
	}

	{ // emit logs and metrics
		metricAttributes := otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
			attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(job.jrpcMethodForMetrics)},
		)

		metrics.RequestSize.Record(context.TODO(), int64(job.req.Header.ContentLength()), metricAttributes)
		metrics.ResponseSize.Record(context.TODO(), int64(job.res.Header.ContentLength()), metricAttributes)
		metrics.LatencyBackend.Record(context.TODO(), tsReqProxyEnd.Sub(tsReqProxyStart).Milliseconds(), metricAttributes)
		metrics.LatencyTotal.Record(context.TODO(), time.Since(job.tsReqReceived).Milliseconds(), metricAttributes)

		if !success {
			metrics.ProxyFailureCount.Add(context.TODO(), 1, metricAttributes)
			job.log.Error("Failed to proxy the request", loggedFields...)
		} else if job.triage.Proxy {
			metrics.ProxySuccessCount.Add(context.TODO(), 1, metricAttributes)
			job.log.Info("Proxied the request", loggedFields...)
		} else {
			metrics.ProxyFakeCount.Add(context.TODO(), 1, metricAttributes)
			job.log.Info("Faked the request", loggedFields...)
		}
	}
}

func (p *Proxy) execJobMirror(job *jobMirror) {
	defer fasthttp.ReleaseRequest(job.req)
	defer fasthttp.ReleaseResponse(job.res)

	err := p.peer.Do(job.req, job.res)

	loggedFields := make([]zap.Field, 0, 12)
	{ // add log fields
		if err == nil {
			loggedFields = append(loggedFields,
				zap.Int("http_status", job.res.StatusCode()),
			)

			if p.cfg.Proxy.LogResponses {
				switch str(job.res.Header.ContentEncoding()) {
				default:
					var jsonResponse interface{}
					if err := json.Unmarshal(job.res.Body(), &jsonResponse); err == nil {
						loggedFields = append(loggedFields,
							zap.Any("json_response", jsonResponse),
						)
					} else {
						loggedFields = append(loggedFields,
							zap.String("http_response", str(job.res.Body())),
						)
					}

				case "gzip":
					if body, err := job.res.BodyGunzip(); err == nil {
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
							zap.String("hex_response", hex.EncodeToString(job.res.Body())),
						)
					}
				}
			}
		} else {
			loggedFields = append(loggedFields,
				zap.NamedError("error_mirror", err),
			)
		}

		loggedFields = append(loggedFields,
			zap.String("mirror_host", job.host),
		)

		if p.cfg.Proxy.LogRequests {
			var jsonRequest interface{}
			if err := json.Unmarshal(job.req.Body(), &jsonRequest); err == nil {
				loggedFields = append(loggedFields,
					zap.Any("json_request", jsonRequest),
				)
			} else {
				loggedFields = append(loggedFields,
					zap.String("http_request", str(job.req.Body())),
				)
			}
		}
	}

	{ // emit logs and metrics
		metricAttributes := otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.Name)},
			attribute.KeyValue{Key: "mirror_host", Value: attribute.StringValue(job.host)},
			attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(job.jrpcMethodForMetrics)},
		)

		if err == nil {
			job.log.Info("Mirrored the request", loggedFields...)
			metrics.MirrorSuccessCount.Add(context.TODO(), 1, metricAttributes)
		} else {
			job.log.Error("Failed to mirror the request", loggedFields...)
			metrics.MirrorFailureCount.Add(context.TODO(), 1, metricAttributes)
		}

		_ = job.log.Sync()
	}
}

func (p *Proxy) injectHttpError(_ *fasthttp.Request, res *fasthttp.Response) error {
	res.SetStatusCode(fasthttp.StatusInternalServerError)
	res.SetBody([]byte("chaos-injected error"))

	return nil
}

func (p *Proxy) injectJrpcError(
	call *triaged.Request, _ *fasthttp.Request, _ *fasthttp.Response,
) func(_ *fasthttp.Request, _ *fasthttp.Response) error {
	return func(_ *fasthttp.Request, res *fasthttp.Response) error {
		res.SetStatusCode(fasthttp.StatusOK)
		res.Header.Add("content-type", "application/json; charset=utf-8")
		res.SetBody([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%s,"error":{"code":-32042,"message:"chaos-injected error"}}`,
			call.JrpcID,
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
