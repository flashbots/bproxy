package main

import (
	"slices"
	"strings"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/flashbots/bproxy/config"
	"github.com/flashbots/bproxy/server"
)

const (
	categoryChaos   = "chaos"
	categoryAuthRPC = "authrpc"
	categoryRPC     = "rpc"
	categoryMetrics = "metrics"
)

func CommandServe(cfg *config.Config) *cli.Command {
	proxyFlags := func(
		cfg *config.Proxy, category string, backend, listenAddress string,
	) (flags []cli.Flag, peersFlag *cli.StringSlice) {
		peersFlag = &cli.StringSlice{}

		flags = []cli.Flag{
			&cli.StringFlag{ // backend
				Category:    strings.ToUpper(category),
				Destination: &cfg.Backend,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_BACKEND"},
				Name:        category + "-backend",
				Usage:       "`url` of backend " + category,
				Value:       backend,
			},

			&cli.BoolFlag{ // enabled
				Category:    strings.ToUpper(category),
				Destination: &cfg.Enabled,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_ENABLED"},
				Name:        category + "-enabled",
				Usage:       "enable " + category + " proxy",
				Value:       false,
			},

			&cli.StringFlag{ // listen-address
				Category:    strings.ToUpper(category),
				Destination: &cfg.ListenAddress,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LISTEN_ADDRESS"},
				Name:        category + "-listen-address",
				Usage:       "`host:port` for " + category + " proxy",
				Value:       listenAddress,
			},

			&cli.BoolFlag{ // log-requests
				Category:    strings.ToUpper(category),
				Destination: &cfg.LogRequests,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LOG_REQUESTS"},
				Name:        category + "-log-requests",
				Usage:       "whether to log " + category + " requests",
				Value:       false,
			},

			&cli.BoolFlag{ // log-responses
				Category:    strings.ToUpper(category),
				Destination: &cfg.LogResponses,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LOG_RESPONSES"},
				Name:        category + "-log-responses",
				Usage:       "whether to log responses to proxied/mirrored " + category + " requests",
				Value:       false,
			},

			&cli.IntFlag{ // max-request-size
				Category:    strings.ToUpper(category),
				Destination: &cfg.MaxRequestSize,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_MAX_REQUEST_SIZE"},
				Name:        category + "-max-request-size",
				Usage:       "maximum " + category + " request payload size in `megabytes`",
				Value:       15,
			},

			&cli.IntFlag{ // max-response-size
				Category:    strings.ToUpper(category),
				Destination: &cfg.MaxResponseSize,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_MAX_RESPONSE_SIZE"},
				Name:        category + "-max-response-size",
				Usage:       "maximum " + category + " response payload size in `megabytes`",
				Value:       160,
			},

			&cli.StringSliceFlag{ // peers
				Category:    strings.ToUpper(category),
				Destination: peersFlag,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_PEERS"},
				Name:        category + "-peers",
				Usage:       "list of `urls` with " + category + " peers to mirror the requests to",
			},

			&cli.BoolFlag{ // remove-backend-from-peers
				Category:    strings.ToUpper(category),
				Destination: &cfg.RemoveBackendFromPeers,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_REMOVE_BACKEND_FROM_PEERS"},
				Name:        category + "-remove-backend-from-peers",
				Usage:       "remove " + category + " backend from peers",
				Value:       false,
			},
		}

		return
	}

	authrpcFlags, peersAuthRPC := proxyFlags(
		cfg.AuthRpcProxy, categoryAuthRPC, "http://127.0.0.1:18551", "0.0.0.0:8551",
	)

	chaosFlags := []cli.Flag{
		&cli.BoolFlag{
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.Enabled,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_ENABLED"},
			Name:        categoryChaos + "-enabled",
			Usage:       "whether bproxy should be injecting artificial error conditions",
			Value:       false,
		},

		&cli.Float64Flag{
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.InjectedHttpErrorProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_INJECTED_HTTP_ERROR_PROBABILITY"},
			Name:        categoryChaos + "-injected-http-error-probability",
			Usage:       "probability in `percent` at which to randomly inject http errors into proxied responses",
			Value:       20,
		},

		&cli.Float64Flag{
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.InjectedJrpcErrorProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_INJECTED_JRPC_ERROR_PROBABILITY"},
			Name:        categoryChaos + "-injected-jrpc-error-probability",
			Usage:       "probability in `percent` at which to randomly inject jrpc errors into proxied responses",
			Value:       20,
		},

		&cli.Float64Flag{
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.InjectedInvalidJrpcResponseProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_INJECTED_INVALID_JRPC_RESPONSE_PROBABILITY"},
			Name:        categoryChaos + "-injected-invalid-jrpc-response-probability",
			Usage:       "probability in `percent` at which to randomly inject invalid jrpc into proxied responses",
			Value:       20,
		},

		&cli.DurationFlag{
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.MinInjectedLatency,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_MIN_INJECTED_LATENCY"},
			Name:        categoryChaos + "-min-injected-latency",
			Usage:       "min `latency` to enforce on every proxied response",
			Value:       50 * time.Millisecond,
		},

		&cli.DurationFlag{
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.MaxInjectedLatency,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_MAX_INJECTED_LATENCY"},
			Name:        categoryChaos + "-max-injected-latency",
			Usage:       "max `latency` to randomly enforce on every proxied response",
		},
	}

	rpcFlags, peersRPC := proxyFlags(
		cfg.RpcProxy, categoryRPC, "http://127.0.0.1:18545", "0.0.0.0:8545",
	)

	metricsFlags := []cli.Flag{
		&cli.StringFlag{
			Category:    strings.ToUpper(categoryMetrics),
			Destination: &cfg.Metrics.ListenAddress,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryMetrics) + "_LISTEN_ADDRESS"},
			Name:        categoryMetrics + "-listen-address",
			Usage:       "`host:port` for metrics server",
			Value:       "0.0.0.0:6785",
		},
	}

	flags := slices.Concat(
		chaosFlags,
		authrpcFlags,
		rpcFlags,
		metricsFlags,
	)

	return &cli.Command{
		Name:  "serve",
		Usage: "run bproxy server",
		Flags: flags,

		Before: func(_ *cli.Context) error {
			cfg.AuthRpcProxy.Peers = peersAuthRPC.Value()
			cfg.RpcProxy.Peers = peersRPC.Value()

			return cfg.Validate()
		},

		Action: func(_ *cli.Context) error {
			s, err := server.New(cfg)
			if err != nil {
				return err
			}
			return s.Run()
		},
	}
}
