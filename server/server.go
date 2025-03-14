package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashbots/bproxy/config"
	"github.com/flashbots/bproxy/logutils"
	"github.com/flashbots/bproxy/metrics"
	"github.com/flashbots/bproxy/proxy"
	"github.com/flashbots/bproxy/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/goccy/go-json"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type Server struct {
	cfg     *config.Config
	failure chan error
	logger  *zap.Logger

	authrpc *proxy.Proxy
	rpc     *proxy.Proxy

	metrics *http.Server
}

func New(cfg *config.Config) (*Server, error) {
	s := &Server{
		cfg:     cfg,
		logger:  zap.L(),
		failure: make(chan error, 16),
	}

	authrpc, err := proxy.New(&proxy.Config{
		BackendURI:    cfg.AuthRpcProxy.Backend,
		ListenAddress: cfg.AuthRpcProxy.ListenAddress,
		LogRequests:   cfg.AuthRpcProxy.LogRequests,
		LogResponses:  cfg.AuthRpcProxy.LogResponses,
		Name:          "bproxy-authrpc",
		Parse:         s.parseAuthRpcCall,
		PeerURIs:      cfg.AuthRpcProxy.Peers,
	})
	if err != nil {
		return nil, err
	}

	rpc, err := proxy.New(&proxy.Config{
		BackendURI:    cfg.RpcProxy.Backend,
		ListenAddress: cfg.RpcProxy.ListenAddress,
		LogRequests:   cfg.RpcProxy.LogRequests,
		LogResponses:  cfg.RpcProxy.LogResponses,
		Name:          "bproxy-rpc",
		Parse:         s.parseRpcCall,
		PeerURIs:      cfg.RpcProxy.Peers,
	})
	if err != nil {
		return nil, err
	}

	s.authrpc = authrpc
	s.rpc = rpc

	mux := http.NewServeMux()
	mux.Handle("/", promhttp.Handler())
	mux.Handle("/metrics", promhttp.Handler())

	s.metrics = &http.Server{
		Addr:              cfg.Metrics.ListenAddress,
		Handler:           mux,
		MaxHeaderBytes:    1024,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	return s, nil
}

func (s *Server) Run() error {
	l := s.logger
	ctx := logutils.ContextWithLogger(context.Background(), l)

	if err := metrics.Setup(ctx, s.observe); err != nil {
		return err
	}

	go func() { // run the metrics server
		l.Info("Metrics server is going up...",
			zap.String("server_listen_address", s.cfg.Metrics.ListenAddress),
		)
		if err := s.metrics.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.failure <- err
		}
		l.Info("Metrics server is down")
	}()

	s.authrpc.Run(ctx, s.failure)
	s.rpc.Run(ctx, s.failure)

	errs := []error{}
	{ // wait until termination or internal failure
		terminator := make(chan os.Signal, 1)
		signal.Notify(terminator, os.Interrupt, syscall.SIGTERM)

		select {
		case stop := <-terminator:
			l.Info("Stop signal received; shutting down...",
				zap.String("signal", stop.String()),
			)
		case err := <-s.failure:
			l.Error("Internal failure; shutting down...",
				zap.Error(err),
			)
			errs = append(errs, err)
		exhaustErrors:
			for { // exhaust the errors
				select {
				case err := <-s.failure:
					l.Error("Extra internal failure",
						zap.Error(err),
					)
					errs = append(errs, err)
				default:
					break exhaustErrors
				}
			}
		}
	}

	{ // stop the rpc proxy
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := s.rpc.Stop(ctx); err != nil {
			l.Error("Failed to shutdown proxy for rpc",
				zap.Error(err),
			)
		}
	}

	{ // stop the authrpc proxy
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := s.authrpc.Stop(ctx); err != nil {
			l.Error("Failed to shutdown proxy for authrpc",
				zap.Error(err),
			)
		}
	}

	{ // stop metrics server
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := s.metrics.Shutdown(ctx); err != nil {
			l.Error("Metrics server shutdown failed",
				zap.Error(err),
			)
		}
	}

	switch len(errs) {
	default:
		return errors.Join(errs...)
	case 1:
		return errs[0]
	case 0:
		return nil
	}
}

func (s *Server) parseAuthRpcCall(body []byte) (bool, string, uint64) {
	{
		call := types.JrpcCall{}
		if err := json.Unmarshal(body, &call); err != nil {
			s.logger.Warn("Failed to parse authrpc call body",
				zap.Error(err),
			)
			return false, "", 0
		}

		if call.Method != "engine_forkchoiceUpdatedV3" {
			return false, call.Method, call.ID
		}
	}

	call := types.EngineForkchoiceUpdatedV3{}
	if err := json.Unmarshal(body, &call); err != nil {
		s.logger.Warn("Failed to parse authrpc call body",
			zap.Error(err),
		)
		return false, "", 0
	}

	if len(call.Params) < 2 {
		return false, call.Method, call.ID
	}

	if call.Params[0] == nil || call.Params[1] == nil {
		return false, call.Method, call.ID
	}

	return true, call.Method, call.ID
}

func (s *Server) parseRpcCall(body []byte) (bool, string, uint64) {
	call := types.JrpcCall{}
	if err := json.Unmarshal(body, &call); err != nil {
		s.logger.Warn("Failed to parse authrpc call body",
			zap.Error(err),
		)
		return false, "", 0
	}

	if call.Method != "eth_sendRawTransaction" {
		return false, call.Method, call.ID
	}

	return true, call.Method, call.ID
}

func (s *Server) observe(ctx context.Context, o otelapi.Observer) error {
	errs := make([]error, 0, 2)

	if err := s.authrpc.Observe(ctx, o); err != nil {
		errs = append(errs, err)
	}

	if err := s.rpc.Observe(ctx, o); err != nil {
		errs = append(errs, err)
	}

	switch len(errs) {
	default:
		return errors.Join(errs...)
	case 1:
		return errs[0]
	case 0:
		return nil
	}
}
