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

	"github.com/flashbots/bproxy/config"
	"github.com/flashbots/bproxy/logutils"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/triaged"
	"github.com/flashbots/bproxy/utils"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type httpConfig struct {
	name string

	chaos *config.Chaos
	proxy *config.HttpProxy
}

type HTTP struct {
	cfg *httpConfig

	backend  *fasthttp.Client
	frontend *fasthttp.Server
	peer     *fasthttp.Client

	backendURI               *fasthttp.URI
	extraMirroredJrpcMethods map[string]struct{}
	peerURIs                 map[string]*fasthttp.URI

	healthcheck *healthcheck
	logger      *zap.Logger

	triage func(ctx *fasthttp.RequestCtx) (*triaged.Request, *fasthttp.Response)
	run    func()
	stop   func()

	connections         map[string]net.Conn
	drainingConnections map[string]net.Conn
	mxConnections       sync.Mutex

	queueProxyHi chan *proxyJob
	queueProxyLo chan *proxyJob

	queueMirrorHi map[string]chan *mirrorJob
	queueMirrorLo map[string]chan *mirrorJob
}

func newHTTP(cfg *httpConfig) (*HTTP, error) {
	l := zap.L().With(zap.String("proxy_name", cfg.name))

	p := &HTTP{
		cfg:                 cfg,
		logger:              l,
		connections:         make(map[string]net.Conn),
		drainingConnections: make(map[string]net.Conn),
		queueProxyHi:        make(chan *proxyJob, 512),
		queueProxyLo:        make(chan *proxyJob, 512),
	}

	p.triage = func(*fasthttp.RequestCtx) (*triaged.Request, *fasthttp.Response) {
		return &triaged.Request{}, fasthttp.AcquireResponse()
	}

	p.frontend = &fasthttp.Server{
		ConnState:          p.upstreamConnectionChanged,
		Handler:            p.receive,
		IdleTimeout:        cfg.proxy.ClientIdleConnectionTimeout,
		Logger:             logutils.FasthttpLogger(l),
		MaxConnsPerIP:      cfg.proxy.MaxClientConnectionsPerIP,
		MaxRequestBodySize: cfg.proxy.MaxRequestSizeMb * 1024 * 1024,
		Name:               cfg.name,
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       5 * time.Second,
	}

	if cfg.proxy.TLSCertificate != "" && cfg.proxy.TLSKey != "" {
		cert, err := cfg.proxy.LoadTLSCertificate()
		if err != nil {
			return nil, err
		}

		p.frontend.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	p.backend = &fasthttp.Client{
		MaxConnsPerHost:     cfg.proxy.MaxBackendConnectionsPerHost,
		MaxConnWaitTimeout:  cfg.proxy.MaxBackendConnectionWaitTimeout,
		MaxIdleConnDuration: 30 * time.Second,
		MaxResponseBodySize: cfg.proxy.MaxResponseSizeMb * 1024 * 1024,
		Name:                cfg.name,
		ReadTimeout:         cfg.proxy.BackendTimeout,
		WriteTimeout:        5 * time.Second,
	}

	p.backendURI = fasthttp.AcquireURI()
	if err := p.backendURI.Parse(nil, []byte(cfg.proxy.BackendURL)); err != nil {
		fasthttp.ReleaseURI(p.backendURI)
		return nil, err
	}

	if len(cfg.proxy.PeerURLs) > 0 {
		p.peer = &fasthttp.Client{
			MaxConnsPerHost:     cfg.proxy.MaxBackendConnectionsPerHost,
			MaxConnWaitTimeout:  cfg.proxy.MaxBackendConnectionWaitTimeout,
			MaxIdleConnDuration: 30 * time.Second,
			MaxResponseBodySize: cfg.proxy.MaxResponseSizeMb * 1024 * 1024,
			Name:                cfg.name,
			ReadTimeout:         5 * time.Second,
			WriteTimeout:        5 * time.Second,
		}

		if cfg.proxy.PeerTLSInsecureSkipVerify {
			p.peer.TLSConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}

		p.peerURIs = make(map[string]*fasthttp.URI, len(cfg.proxy.PeerURLs))
		p.queueMirrorHi = make(map[string]chan *mirrorJob, len(cfg.proxy.PeerURLs))
		p.queueMirrorLo = make(map[string]chan *mirrorJob, len(cfg.proxy.PeerURLs))

		for _, peerURL := range cfg.proxy.PeerURLs {
			peerURI := fasthttp.AcquireURI()
			if err := peerURI.Parse(nil, []byte(peerURL)); err != nil {
				fasthttp.ReleaseURI(p.backendURI)
				for _, uri := range p.peerURIs {
					fasthttp.ReleaseURI(uri)
				}
				return nil, err
			}
			host := utils.Str(peerURI.Host())

			p.peerURIs[host] = peerURI
			p.queueMirrorHi[host] = make(chan *mirrorJob, 512)
			p.queueMirrorLo[host] = make(chan *mirrorJob, 512)
		}
	}

	if cfg.proxy.Healthcheck.URL != "" {
		h, err := newHealthcheck(
			cfg.name,
			cfg.proxy.Healthcheck.URL,
			cfg.proxy.Healthcheck.Interval,
			p.cfg.proxy.Healthcheck.ThresholdHealthy,
			p.cfg.proxy.Healthcheck.ThresholdUnhealthy,
			p.backendUnhealthy,
		)
		if err != nil {
			fasthttp.ReleaseURI(p.backendURI)
			for _, uri := range p.peerURIs {
				fasthttp.ReleaseURI(uri)
			}
			return nil, err
		}
		p.healthcheck = h
	}

	if len(cfg.proxy.ExtraMirroredJrpcMethods) > 0 {
		p.extraMirroredJrpcMethods = make(map[string]struct{}, len(cfg.proxy.ExtraMirroredJrpcMethods))
		for _, method := range cfg.proxy.ExtraMirroredJrpcMethods {
			method = strings.TrimSpace(method)
			if _, known := p.extraMirroredJrpcMethods[method]; !known {
				p.extraMirroredJrpcMethods[method] = struct{}{}
			}
		}
	}

	return p, nil
}

func (p *HTTP) Run(ctx context.Context, failure chan<- error) {
	if p == nil {
		return
	}

	l := p.logger

	if p.run != nil {
		p.run()
	}

	go func() { // run the proxy
		l.Info("Proxy is going up...",
			zap.String("listen_address", p.cfg.proxy.ListenAddress),
			zap.String("backend", p.cfg.proxy.BackendURL),
			zap.Strings("peers", p.cfg.proxy.PeerURLs),
		)
		if p.cfg.proxy.TLSCertificate != "" && p.cfg.proxy.TLSKey != "" {
			if err := p.frontend.ListenAndServeTLS(p.cfg.proxy.ListenAddress, "", ""); err != nil {
				failure <- err
			}
		} else {
			if err := p.frontend.ListenAndServe(p.cfg.proxy.ListenAddress); err != nil {
				failure <- err
			}
		}
		l.Info("Proxy is down")
	}()

	if p.cfg.proxy.UsePriorityQueue {
		go func() { // run the proxy job loop with priority queue
			for {
				var job *proxyJob
				select {
				case job = <-p.queueProxyHi:
				default:
					select {
					case job = <-p.queueProxyHi:
					case job = <-p.queueProxyLo:
					}
				}
				p.execProxyJob(job)
			}
		}()
	} else {
		go func() { // run the proxy job loop without priority queue
			for {
				var job *proxyJob
				select {
				case job = <-p.queueProxyHi:
				case job = <-p.queueProxyLo:
				}
				p.execProxyJob(job)
			}
		}()

	}

	for host := range p.peerURIs { // run the mirror job loops
		queueMirrorHi := p.queueMirrorHi[host]
		queueMirrorLo := p.queueMirrorLo[host]

		if p.cfg.proxy.UsePriorityQueue { // with priority queue
			go func() {
				var job *mirrorJob
				for {
					select {
					case job = <-queueMirrorHi:
					default:
						select {
						case job = <-queueMirrorHi:
						case job = <-queueMirrorLo:
						}
					}
					p.execMirrorJob(job)
				}
			}()
		} else { // without priority queue
			go func() {
				var job *mirrorJob
				for {
					select {
					case job = <-queueMirrorHi:
					case job = <-queueMirrorLo:
					}
					p.execMirrorJob(job)
				}
			}()
		}
	}

	if p.cfg.proxy.Healthcheck.URL != "" {
		p.healthcheck.run(ctx)
	}
}

func (p *HTTP) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}

	if p.stop != nil {
		p.stop()
	}

	if p.cfg.proxy.Healthcheck.URL != "" {
		p.healthcheck.stop()
	}

	err := p.frontend.ShutdownWithContext(ctx)

	p.ResetConnections()

	fasthttp.ReleaseURI(p.backendURI)
	for _, uri := range p.peerURIs {
		fasthttp.ReleaseURI(uri)
	}

	return err
}

func (p *HTTP) ResetConnections() {
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

func (p *HTTP) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}

	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	o.ObserveInt64(metrics.FrontendConnectionsCount, int64(p.frontend.GetOpenConnectionsCount()), otelapi.WithAttributes(
		attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
	))

	o.ObserveInt64(metrics.FrontendDrainingConnectionsCount, int64(len(p.drainingConnections)), otelapi.WithAttributes(
		attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
	))

	if p.frontend.TLSConfig != nil {
		for _, cert := range p.frontend.TLSConfig.Certificates {
			if cert.Leaf != nil {
				o.ObserveInt64(metrics.TLSValidNotAfter, cert.Leaf.NotAfter.Unix(), otelapi.WithAttributes(
					attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
				))

				o.ObserveInt64(metrics.TLSValidNotBefore, cert.Leaf.NotBefore.Unix(), otelapi.WithAttributes(
					attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
				))
			}
		}
	}

	return nil
}

func (p *HTTP) newProxyJob(ctx *fasthttp.RequestCtx) *proxyJob {
	job := &proxyJob{
		tsReqReceived: ctx.Time(),
		wg:            &sync.WaitGroup{},
	}

	job.triage, job.res = p.triage(ctx)

	{ // prepare the request
		job.req = fasthttp.AcquireRequest()

		ctx.Request.CopyTo(job.req)
		job.req.SetTimeout(p.cfg.proxy.BackendTimeout)
		job.req.SetURI(p.backendURI)
		job.req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
		job.req.Header.Add("x-forwarded-host", utils.Str(ctx.Host()))
		job.req.Header.Add("x-forwarded-proto", utils.Str(ctx.Request.URI().Scheme()))
	}

	loggedFields := make([]zap.Field, 0, 12)

	{ // configure processing mode (proxy, fake, chaos)
		if p.extraMirroredJrpcMethods != nil {
			_, mirror := p.extraMirroredJrpcMethods[job.triage.JrpcMethod]
			job.triage.Mirror = job.triage.Mirror || mirror
		}

		if p.cfg.chaos.Enabled {
			injectHttpError := rand.Float64() < p.cfg.chaos.InjectedHttpErrorProbability/100
			injectJrpcError := rand.Float64() < p.cfg.chaos.InjectedJrpcErrorProbability/100
			injectBadJrpcResponse := rand.Float64() < p.cfg.chaos.InjectedInvalidJrpcResponseProbability/100

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
			zap.Uint64("request_id", ctx.ConnRequestNum()),
			zap.String("remote_addr", ctx.RemoteAddr().String()),
			zap.String("user_agent", string(ctx.UserAgent())),
			zap.String("downstream_host", utils.Str(p.backendURI.Host())),
			zap.String("jrpc_method", job.triage.JrpcMethod),
		)

		if len(job.triage.Transactions) > 0 {
			loggedFields = append(loggedFields,
				zap.Array("txs", job.triage.Transactions),
			)
		}

		if !job.triage.Deadline.IsZero() {
			headsup := time.Until(job.triage.Deadline)
			loggedFields = append(loggedFields,
				zap.Time("deadline", job.triage.Deadline),
				zap.Duration("headsup", headsup),
			)
		}

		job.log = p.logger.With(loggedFields...)
	}

	return job
}

func (p *HTTP) newMirrorJob(ctx *fasthttp.RequestCtx, pjob *proxyJob, uri *fasthttp.URI) *mirrorJob {
	req := fasthttp.AcquireRequest()
	res := fasthttp.AcquireResponse()

	ctx.Request.CopyTo(req)
	req.SetTimeout(p.cfg.proxy.BackendTimeout)
	req.SetURI(uri)
	req.Header.Add("x-forwarded-for", ctx.RemoteIP().String())
	req.Header.Add("x-forwarded-host", utils.Str(ctx.Host()))
	req.Header.Add("x-forwarded-proto", utils.Str(ctx.Request.URI().Scheme()))

	return &mirrorJob{
		log:                  pjob.log,
		host:                 utils.Str(uri.Host()),
		req:                  req,
		res:                  res,
		jrpcMethodForMetrics: pjob.triage.JrpcMethod,
	}
}

func (p *HTTP) receive(ctx *fasthttp.RequestCtx) {
	pj := p.newProxyJob(ctx)
	defer fasthttp.ReleaseResponse(pj.res)
	defer fasthttp.ReleaseRequest(pj.req)

	pj.wg.Add(1)

	{ // schedule jobs
		if pj.triage.Prioritise {
			p.queueProxyHi <- pj
		} else {
			p.queueProxyLo <- pj
		}

		if pj.triage.Mirror {
			for host, uri := range p.peerURIs {
				mj := p.newMirrorJob(ctx, pj, uri)
				if pj.triage.Prioritise {
					select {
					case p.queueMirrorHi[host] <- mj:
						// no-op
					default:
						// better not mirror than block on hot-path
						metrics.MirrorDropCount.Add(context.TODO(), 1, otelapi.WithAttributes(
							attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
							attribute.KeyValue{Key: "mirror_host", Value: attribute.StringValue(mj.host)},
							attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(mj.jrpcMethodForMetrics)},
						))
						pj.log.Warn("Dropped mirrored high-prio call b/c the queue is full")
					}
				} else {
					select {
					case p.queueMirrorLo[host] <- mj:
						// no-op
					default:
						// better not mirror than block on hot-path
						metrics.MirrorDropCount.Add(context.TODO(), 1, otelapi.WithAttributes(
							attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
							attribute.KeyValue{Key: "mirror_host", Value: attribute.StringValue(mj.host)},
							attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(mj.jrpcMethodForMetrics)},
						))
						pj.log.Warn("Dropped mirrored low-prio call b/c the queue is full")
					}
				}
			}
		}
	}

	pj.wg.Wait()

	pj.res.CopyTo(&ctx.Response)

	{ // check if this is a draining connection
		addr := ctx.RemoteIP().String()

		p.mxConnections.Lock()
		defer p.mxConnections.Unlock()

		if _, isDraining := p.drainingConnections[addr]; isDraining {
			ctx.SetConnectionClose()
			pj.log.Info("Marked draining upstream connection to be closed",
				zap.Int("remaining", len(p.drainingConnections)),
			)
		}
	}

	_ = pj.log.Sync()
}

func (p *HTTP) execProxyJob(job *proxyJob) {
	defer job.wg.Done()

	tsReqProxyStart := time.Now()
	err := job.proxy(job.req, job.res)
	tsReqProxyEnd := time.Now()
	success := (err == nil)

	loggedFields := make([]zap.Field, 0, 12)

	loggedFields = append(loggedFields,
		zap.Int("request_size", len(job.req.Body())),
		zap.Int("response_size", len(job.res.Body())),
	)

	if err != nil {
		switch utils.Str(job.req.Header.ContentType()) {
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

	if p.cfg.chaos.Enabled { // chaos-inject latency
		latency := time.Duration(rand.Int64N(int64(p.cfg.chaos.MaxInjectedLatency) + 1))
		latency = max(latency, p.cfg.chaos.MinInjectedLatency)
		time.Sleep(latency - time.Since(job.tsReqReceived))
		loggedFields = append(loggedFields,
			zap.Bool("latency_chaos", true),
		)
	}

	{ // add log fields
		if p.cfg.proxy.LogRequests && len(job.req.Body()) <= p.cfg.proxy.LogRequestsMaxSize {
			var jsonRequest interface{}
			if err := json.Unmarshal(job.req.Body(), &jsonRequest); err == nil {
				loggedFields = append(loggedFields,
					zap.Any("json_request", jsonRequest),
				)
			} else {
				loggedFields = append(loggedFields,
					zap.NamedError("error_unmarshal", err),
					zap.String("http_request", utils.Str(job.req.Body())),
				)
			}
		}

		if p.cfg.proxy.LogResponses && len(job.res.Body()) <= p.cfg.proxy.LogResponsesMaxSize {
			var body []byte

			switch utils.Str(job.res.Header.ContentEncoding()) {
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
						zap.String("http_response", utils.Str(body)),
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
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
			attribute.KeyValue{Key: "jrpc_method", Value: attribute.StringValue(job.triage.JrpcMethod)},
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

func (p *HTTP) execMirrorJob(job *mirrorJob) {
	defer fasthttp.ReleaseRequest(job.req)
	defer fasthttp.ReleaseResponse(job.res)

	err := p.peer.Do(job.req, job.res)

	loggedFields := make([]zap.Field, 0, 12)
	{ // add log fields
		loggedFields = append(loggedFields,
			zap.String("mirror_host", job.host),
			zap.Int("request_size", len(job.req.Body())),
			zap.Int("response_size", len(job.res.Body())),
		)

		if p.cfg.proxy.LogRequests && len(job.req.Body()) <= p.cfg.proxy.LogRequestsMaxSize {
			var jsonRequest interface{}
			if err := json.Unmarshal(job.req.Body(), &jsonRequest); err == nil {
				loggedFields = append(loggedFields,
					zap.Any("json_request", jsonRequest),
				)
			} else {
				loggedFields = append(loggedFields,
					zap.String("http_request", utils.Str(job.req.Body())),
				)
			}
		}

		if err == nil {
			loggedFields = append(loggedFields,
				zap.Int("http_status", job.res.StatusCode()),
			)

			if p.cfg.proxy.LogResponses && len(job.res.Body()) <= p.cfg.proxy.LogResponsesMaxSize {
				switch utils.Str(job.res.Header.ContentEncoding()) {
				default:
					var jsonResponse interface{}
					if err := json.Unmarshal(job.res.Body(), &jsonResponse); err == nil {
						loggedFields = append(loggedFields,
							zap.Any("json_response", jsonResponse),
						)
					} else {
						loggedFields = append(loggedFields,
							zap.String("http_response", utils.Str(job.res.Body())),
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
								zap.String("http_response", utils.Str(body)),
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
	}

	{ // emit logs and metrics
		metricAttributes := otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
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

func (p *HTTP) injectHttpError(_ *fasthttp.Request, res *fasthttp.Response) error {
	res.SetStatusCode(fasthttp.StatusInternalServerError)
	res.SetBody([]byte("chaos-injected error"))

	return nil
}

func (p *HTTP) injectJrpcError(
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

func (p *HTTP) injectInvalidJrpcResponse(_ *fasthttp.Request, res *fasthttp.Response) error {
	res.SetStatusCode(fasthttp.StatusOK)
	res.Header.Add("content-type", "application/json; charset=utf-8")
	res.SetBody([]byte("chaos-injected invalid jrpc response"))

	return nil
}

func (p *HTTP) upstreamConnectionChanged(conn net.Conn, state fasthttp.ConnState) {
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

func (p *HTTP) connectionsCount() int {
	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	return len(p.connections)
}

func (p *HTTP) backendUnhealthy(ctx context.Context) {
	l := logutils.LoggerFromContext(ctx)

	if p.connectionsCount() > 0 {
		l.Warn("Resetting frontend connections b/c backend is (still) unhealthy...",
			zap.String("proxy_name", p.cfg.name),
		)
		p.ResetConnections()
	}
}
