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
	peersAuthRPC := &cli.StringSlice{}
	peersRPC := &cli.StringSlice{}

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

	authrpcFlags := []cli.Flag{
		&cli.StringFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: &cfg.AuthRpcProxy.Backend,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_BACKEND"},
			Name:        categoryAuthRPC + "-backend",
			Usage:       "`url` of backend authrpc",
			Value:       "http://127.0.0.1:18551",
		},

		&cli.StringFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: &cfg.AuthRpcProxy.ListenAddress,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_LISTEN_ADDRESS"},
			Name:        categoryAuthRPC + "-listen-address",
			Usage:       "`host:port` for authrpc proxy",
			Value:       "0.0.0.0:8551",
		},

		&cli.BoolFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: &cfg.AuthRpcProxy.LogRequests,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_LOG_REQUESTS"},
			Name:        categoryAuthRPC + "-log-requests",
			Usage:       "whether to log authrpc requests",
			Value:       false,
		},

		&cli.BoolFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: &cfg.AuthRpcProxy.LogResponses,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_LOG_RESPONSES"},
			Name:        categoryAuthRPC + "-log-responses",
			Usage:       "whether to log responses to proxied/mirrored authrpc requests",
			Value:       false,
		},

		&cli.StringSliceFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: peersAuthRPC,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_PEERS"},
			Name:        categoryAuthRPC + "-peers",
			Usage:       "list of `urls` with authrpc peers to mirror the requests to",
		},

		&cli.BoolFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: &cfg.AuthRpcProxy.RemoveBackendFromPeers,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_REMOVE_BACKEND_FROM_PEERS"},
			Name:        categoryAuthRPC + "-remove-backend-from-peers",
			Usage:       "remove backend from peers",
			Value:       false,
		},
	}

	rpcFlags := []cli.Flag{
		&cli.StringFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: &cfg.RpcProxy.Backend,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_BACKEND"},
			Name:        categoryRPC + "-backend",
			Usage:       "`url` of backend rpc",
			Value:       "http://127.0.0.1:18545",
		},

		&cli.StringFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: &cfg.RpcProxy.ListenAddress,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_LISTEN_ADDRESS"},
			Name:        categoryRPC + "-listen-address",
			Usage:       "`host:port` for rpc proxy",
			Value:       "0.0.0.0:8545",
		},

		&cli.BoolFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: &cfg.RpcProxy.LogRequests,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_LOG_REQUESTS"},
			Name:        categoryRPC + "-log-requests",
			Usage:       "whether to log rpc requests",
			Value:       false,
		},

		&cli.BoolFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: &cfg.RpcProxy.LogResponses,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_LOG_RESPONSES"},
			Name:        categoryRPC + "-log-responses",
			Usage:       "whether to log responses to proxied/mirrored rpc requests",
			Value:       false,
		},

		&cli.StringSliceFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: peersRPC,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_PEERS"},
			Name:        categoryRPC + "-peers",
			Usage:       "list of `urls` with rpc peers to mirror the requests to",
		},

		&cli.BoolFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: &cfg.RpcProxy.RemoveBackendFromPeers,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_REMOVE_BACKEND_FROM_PEERS"},
			Name:        categoryRPC + "-remove-backend-from-peers",
			Usage:       "remove backend from peers",
			Value:       false,
		},
	}

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
