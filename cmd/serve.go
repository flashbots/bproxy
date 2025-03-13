package main

import (
	"slices"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/flashbots/bproxy/config"
	"github.com/flashbots/bproxy/server"
)

const (
	categoryAuthRPC = "authrpc"
	categoryRPC     = "rpc"
	categoryMetrics = "metrics"
)

func CommandServe(cfg *config.Config) *cli.Command {
	peersAuthRPC := &cli.StringSlice{}
	peersRPC := &cli.StringSlice{}

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
			Destination: &cfg.AuthRpcProxy.LogResponses,
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
