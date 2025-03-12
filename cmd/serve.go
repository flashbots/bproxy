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
)

func CommandServe(cfg *config.Config) *cli.Command {
	peersAuthRPC := &cli.StringSlice{}
	peersRPC := &cli.StringSlice{}

	authrpcFlags := []cli.Flag{
		&cli.StringFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: &cfg.Proxy.BackendAuthRPC,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_BACKEND"},
			Name:        categoryAuthRPC + "-backend",
			Usage:       "`url` of backend authrpc",
			Value:       "http://127.0.0.1:18545",
		},

		&cli.StringFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: &cfg.Proxy.ListenAddressAuthRPC,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_LISTEN_ADDRESS"},
			Name:        categoryAuthRPC + "-listen-address",
			Usage:       "`host:port` for authrpc proxy",
			Value:       "0.0.0.0:8545",
		},

		&cli.StringSliceFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: peersAuthRPC,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_PEERS"},
			Name:        categoryAuthRPC + "-peers",
			Usage:       "list of `urls` with authrpc peers to mirror the requests to",
		},
	}

	rpcFlags := []cli.Flag{
		&cli.StringFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: &cfg.Proxy.BackendRPC,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_BACKEND"},
			Name:        categoryRPC + "-backend",
			Usage:       "`url` of backend rpc",
			Value:       "http://127.0.0.1:18551",
		},

		&cli.StringFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: &cfg.Proxy.ListenAddressRPC,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_LISTEN_ADDRESS"},
			Name:        categoryRPC + "-listen-address",
			Usage:       "`host:port` for rpc proxy",
			Value:       "0.0.0.0:8551",
		},

		&cli.StringSliceFlag{
			Category:    strings.ToUpper(categoryRPC),
			Destination: peersRPC,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryRPC) + "_PEERS"},
			Name:        categoryRPC + "-peers",
			Usage:       "list of `urls` with rpc peers to mirror the requests to",
		},
	}

	flags := slices.Concat(
		authrpcFlags,
		rpcFlags,
	)

	return &cli.Command{
		Name:  "serve",
		Usage: "run bproxy server",
		Flags: flags,

		Before: func(_ *cli.Context) error {
			cfg.Proxy.PeersAuthRPC = peersAuthRPC.Value()
			cfg.Proxy.PeersRPC = peersRPC.Value()

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
