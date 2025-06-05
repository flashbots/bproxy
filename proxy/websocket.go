package proxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
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

type websocketConfig struct {
	name string

	proxy *config.WebsocketProxy
}

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
	websockets    map[string]*websocket.Conn
	mxConnections sync.Mutex
}

func newWebsocket(cfg *websocketConfig) (*Websocket, error) {
	l := zap.L().With(zap.String("proxy_name", cfg.name))

	p := &Websocket{
		cfg:         cfg,
		logger:      l,
		connections: make(map[string]net.Conn),
		websockets:  map[string]*websocket.Conn{},

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

	for addr, conn := range p.connections {
		if ws, exists := p.websockets[addr]; exists {
			err := ws.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "unhealthy"),
			)
			p.logger.Error("Closed ws connection on request",
				zap.Error(err),
			)
			delete(p.websockets, addr)
		}

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

	o.ObserveInt64(metrics.FrontendConnectionsCount, int64(p.frontend.GetOpenConnectionsCount()), otelapi.WithAttributes(
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

	res := p.frontend.ShutdownWithContext(ctx)

	return res
}

func (p *Websocket) checkOrigin(ctx *fasthttp.RequestCtx) bool {
	return true
}

func (p *Websocket) receive(ctx *fasthttp.RequestCtx) {
	l := p.logger.With(
		zap.Uint64("connection_id", ctx.ConnID()),
		zap.Uint64("request_id", ctx.ConnRequestNum()),
		zap.String("remote_addr", ctx.RemoteAddr().String()),
	)
	defer l.Sync() //nolint:errcheck

	if !p.healthcheck.IsHealthy() {
		ctx.Error("unhealthy", fasthttp.StatusServiceUnavailable)
		l.Warn("Refusing the connection b/c backend is (still) unhealthy")
		return
	}

	if strings.ToLower(string(ctx.Request.Header.Peek("connection"))) != "upgrade" {
		ctx.Response.SetStatusCode(fasthttp.StatusUpgradeRequired)
		l.Warn("Non-websocket connection received",
			zap.String("connection_header", string(ctx.Request.Header.Peek("connection"))),
			zap.String("upgrade_header", string(ctx.Request.Header.Peek("Upgrade"))),
		)
		return
	}
	if strings.ToLower(string(ctx.Request.Header.Peek("upgrade"))) != "websocket" {
		ctx.Response.SetStatusCode(fasthttp.StatusNotImplemented)
		l.Warn("Non-websocket connection received",
			zap.String("connection_header", string(ctx.Request.Header.Peek("connection"))),
			zap.String("upgrade_header", string(ctx.Request.Header.Peek("Upgrade"))),
		)
		return
	}

	if err := p.upgrader.Upgrade(ctx, p.websocket); err != nil {
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		l.Error("Failed to upgrade to websocket",
			zap.Error(err),
		)
		return
	}
}

func (p *Websocket) websocket(frontend *websocket.Conn) {
	defer frontend.Close()

	p.mxConnections.Lock()
	p.websockets[frontend.NetConn().RemoteAddr().String()] = frontend
	p.mxConnections.Unlock()

	l := p.logger.With(
		zap.String("remote_addr", frontend.RemoteAddr().String()),
	)

	backend, _, err := p.backend.Dial(p.cfg.proxy.BackendURL, nil)
	if err != nil {
		l.Error("Failed to proxy the websocket stream",
			zap.Error(err),
		)
	}
	defer backend.Close()

	failure := make(chan error, 2)
	go p.websocketPump(backend, frontend, "bld->rb", failure)
	go p.websocketPump(frontend, backend, "rb->bld", failure)

	err = <-failure
	for err != nil { // exhaust errors
		metrics.ProxyFailureCount.Add(context.TODO(), 1, otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
		))
		l.Error("Websocket connection failed",
			zap.Error(err),
		)
		select {
		case err = <-failure:
			// noop
		default:
			err = nil
		}
	}
}

func (p *Websocket) websocketPump(
	from, to *websocket.Conn,
	direction string,
	failure chan<- error,
) {
	l := p.logger.With(
		zap.String("from_addr", from.RemoteAddr().String()),
		zap.String("to_addr", to.RemoteAddr().String()),
		zap.String("direction", direction),
	)

	for {
		msgType, bytes, err := from.ReadMessage()
		if err != nil {
			failure <- fmt.Errorf("read: %w", err)
			return
		}

		ts := time.Now()

		err = to.WriteMessage(msgType, bytes)
		if err != nil {
			failure <- fmt.Errorf("write: %w", err)
			return
		}

		loggedFields := make([]zap.Field, 0, 6)
		loggedFields = append(loggedFields,
			zap.Time("ts_message_received", ts),
			zap.Int("message_size", len(bytes)),
		)

		if p.cfg.proxy.LogMessages && len(bytes) <= p.cfg.proxy.LogMessagesMaxSize {
			var jsonMessage interface{}
			if err := json.Unmarshal(bytes, &jsonMessage); err == nil {
				loggedFields = append(loggedFields,
					zap.Any("json_message", jsonMessage),
				)
			} else {
				loggedFields = append(loggedFields,
					zap.NamedError("error_unmarshal", err),
					zap.String("websocket_message", utils.Str(bytes)),
				)
			}
		}

		metrics.ProxySuccessCount.Add(context.TODO(), 1, otelapi.WithAttributes(
			attribute.KeyValue{Key: "proxy", Value: attribute.StringValue(p.cfg.name)},
			attribute.KeyValue{Key: "direction", Value: attribute.StringValue(direction)},
		))
		l.Info("Proxied message", loggedFields...)
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
		p.connections[addr] = conn

	case fasthttp.StateActive:
		l.Debug("Upstream connection became active")

	case fasthttp.StateIdle:
		l.Debug("Upstream connection became idle")

	case fasthttp.StateHijacked:
		l.Info("Upstream connection was hijacked")

	case fasthttp.StateClosed:
		l.Info("Upstream connection was closed")
		delete(p.connections, addr)
	}
}

func (p *Websocket) connectionsCount() int {
	p.mxConnections.Lock()
	defer p.mxConnections.Unlock()

	return len(p.connections)
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
