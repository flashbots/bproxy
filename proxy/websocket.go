package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/flashbots/bproxy/config"
	"github.com/flashbots/bproxy/logutils"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/utils"

	"github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type Websocket struct {
	cfg *websocketConfig

	backend  *websocket.Dialer
	frontend *fasthttp.Server
	upgrader *websocket.FastHTTPUpgrader

	healthcheck *healthcheck
	logger      *zap.Logger

	run  func()
	stop func()

	connections   map[string]net.Conn
	pumps         map[string]*websocketPump
	mxConnections sync.Mutex
}

type websocketConfig struct {
	name string

	proxy *config.WebsocketProxy
}

func newWebsocket(cfg *websocketConfig) (*Websocket, error) {
	l := zap.L().With(zap.String("proxy_name", cfg.name))

	p := &Websocket{
		cfg:         cfg,
		logger:      l,
		connections: make(map[string]net.Conn),
		pumps:       make(map[string]*websocketPump),

		upgrader: &websocket.FastHTTPUpgrader{
			ReadBufferSize:  cfg.proxy.ReadBufferSize * 1024 * 1024,
			WriteBufferSize: cfg.proxy.WriteBufferSize * 1024 * 1024,
		},
	}

	p.frontend = &fasthttp.Server{
		ConnState:    p.upstreamConnectionChanged,
		Handler:      p.receive,
		Logger:       logutils.FasthttpLogger(l),
		Name:         cfg.name,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
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

	p.backend = &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
		ReadBufferSize:   cfg.proxy.WriteBufferSize * 1024 * 1024,
		WriteBufferSize:  cfg.proxy.ReadBufferSize * 1024 * 1024,
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
			return nil, err
		}
		p.healthcheck = h
	}

	p.upgrader.CheckOrigin = p.checkOrigin

	return p, nil
}

func (p *Websocket) Run(ctx context.Context, failure chan<- error) {
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

	if p.cfg.proxy.Healthcheck.URL != "" {
		p.healthcheck.run(ctx)
	}
}

func (p *Websocket) ResetConnections() {
	if p == nil {
		return
	}

	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	for addr, pump := range p.pumps {
		err := pump.stop()
		p.logger.Info("Closed ws connection on request",
			zap.Error(err),
			zap.String("remote_addr", addr),
		)
		delete(p.pumps, addr)
	}

	for addr, conn := range p.connections {
		err := conn.Close()
		p.logger.Info("Closed the connection on request",
			zap.Error(err),
			zap.String("remote_addr", addr),
		)
		delete(p.connections, addr)
	}
}

func (p *Websocket) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}

	o.ObserveInt64(metrics.FrontendConnectionsActiveCount, int64(p.connectionsCount()), otelapi.WithAttributes(
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

func (p *Websocket) Stop(ctx context.Context) error {
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

	return err
}

func (p *Websocket) checkOrigin(ctx *fasthttp.RequestCtx) bool {
	return true
}

func (p *Websocket) receive(ctx *fasthttp.RequestCtx) {
	l := p.logger.With(
		zap.Uint64("connection_id", ctx.ConnID()),
		zap.Uint64("request_id", ctx.ConnRequestNum()),
		zap.String("remote_addr", ctx.RemoteAddr().String()),
		zap.String("user_agent", string(ctx.UserAgent())),
	)
	defer l.Sync() //nolint:errcheck

	if !p.healthcheck.IsHealthy() {
		ctx.Error("unhealthy", fasthttp.StatusServiceUnavailable)
		ctx.SetConnectionClose()
		l.Warn("Refusing the connection b/c backend is (still) unhealthy")
		return
	}

	if strings.ToLower(string(ctx.Request.Header.Peek("connection"))) != "upgrade" {
		ctx.Response.SetStatusCode(fasthttp.StatusUpgradeRequired)
		ctx.SetConnectionClose()
		l.Warn("Non-websocket connection received",
			zap.String("connection_header", string(ctx.Request.Header.Peek("connection"))),
			zap.String("upgrade_header", string(ctx.Request.Header.Peek("Upgrade"))),
		)
		return
	}
	if strings.ToLower(string(ctx.Request.Header.Peek("upgrade"))) != "websocket" {
		ctx.Response.SetStatusCode(fasthttp.StatusNotImplemented)
		ctx.SetConnectionClose()
		l.Warn("Non-websocket connection received",
			zap.String("connection_header", string(ctx.Request.Header.Peek("connection"))),
			zap.String("upgrade_header", string(ctx.Request.Header.Peek("Upgrade"))),
		)
		return
	}

	if err := p.upgrader.Upgrade(ctx, p.websocket(ctx)); err != nil {
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetConnectionClose()
		l.Error("Failed to upgrade to websocket",
			zap.Error(err),
		)
		return
	}

	{ // emit metrics
		metrics.Info.Add(context.TODO(), 1, otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
			attribute.KeyValue{Key: "user_agent", Value: attribute.StringValue(string(ctx.UserAgent()))},
		))
	}
}

func (p *Websocket) websocket(ctx *fasthttp.RequestCtx) func(frontend *websocket.Conn) {
	return func(frontend *websocket.Conn) {
		defer frontend.Close()

		l := p.logger.With(
			zap.String("remote_addr", frontend.RemoteAddr().String()),
			zap.Uint64("connection_id", ctx.ConnID()),
		)

		backend, _, err := p.backend.Dial(p.cfg.proxy.BackendURL, nil)
		if err != nil {
			l.Error("Failed to proxy the websocket stream",
				zap.Error(err),
			)
			return
		}
		defer backend.Close()

		addr := frontend.NetConn().RemoteAddr().String()
		pump := newWebsocketPump(p.cfg, frontend, backend, l)

		p.mxConnections.Lock()
		p.pumps[addr] = pump
		p.mxConnections.Unlock()

		reason := pump.run()
		if reason != nil {
			metrics.ProxyFailureCount.Add(context.TODO(), 1, otelapi.WithAttributes(
				attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
			))
			l.Error("Websocket connection failed",
				zap.Error(reason),
			)
		}

		err = p.closeWebsocket(pump.backend, reason)
		l.Info("Closed backend connection",
			zap.Error(err),
		)

		err = p.closeWebsocket(pump.frontend, reason)
		l.Info("Closed frontend connection",
			zap.Error(err),
		)

		p.mxConnections.Lock()
		delete(p.pumps, addr)
		p.mxConnections.Unlock()
	}
}

func (p *Websocket) upstreamConnectionChanged(conn net.Conn, state fasthttp.ConnState) {
	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	addr := conn.RemoteAddr().String()

	l := p.logger.With(
		zap.String("remote_addr", addr),
	)

	switch state {
	case fasthttp.StateNew:
		l.Info("Upstream connection was established")
		metrics.FrontendConnectionsEstablishedCount.Add(context.TODO(), 1, otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
		))
		p.connections[addr] = conn

	case fasthttp.StateActive:
		l.Debug("Upstream connection became active")

	case fasthttp.StateIdle:
		l.Debug("Upstream connection became idle")

	case fasthttp.StateHijacked:
		l.Info("Upstream connection was hijacked")
		delete(p.connections, addr)

	case fasthttp.StateClosed:
		l.Info("Upstream connection was closed")
		metrics.FrontendConnectionsClosedCount.Add(context.TODO(), 1, otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
		))
		delete(p.connections, addr)
	}
}

func (p *Websocket) connectionsCount() int {
	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	return len(p.connections) + len(p.pumps)
}

func (p *Websocket) backendUnhealthy(ctx context.Context) {
	l := logutils.LoggerFromContext(ctx)

	// only reset the connections at sharp seconds
	time.Sleep(time.Until(
		time.Unix(time.Now().Unix()+1, 0).Add(5 * time.Millisecond),
	))

	if p.connectionsCount() > 0 {
		l.Warn("Resetting connections b/c backend is (still) unhealthy...",
			zap.String("proxy_name", p.cfg.name),
		)
		p.ResetConnections()
	}
}

func (p *Websocket) closeWebsocket(conn *websocket.Conn, reason error) error {
	if reason == nil {
		return errors.Join(
			conn.WriteControl(
				websocket.CloseMessage, nil, utils.Deadline(p.cfg.proxy.ControlTimeout),
			),
			conn.Close(),
		)
	}

	return errors.Join(
		conn.WriteControl(
			// maxControlFramePayloadSize == 125
			websocket.CloseMessage, []byte(reason.Error())[:125], utils.Deadline(p.cfg.proxy.ControlTimeout),
		),
		conn.Close(),
	)
}
