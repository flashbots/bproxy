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
		cfg *config.Proxy, category string, backendURL, listenAddress string,
	) (flags []cli.Flag, extraMirroredJrpcMethods, peerURLsFlag *cli.StringSlice) {
		extraMirroredJrpcMethods = &cli.StringSlice{}
		peerURLsFlag = &cli.StringSlice{}

		flags = []cli.Flag{
			&cli.StringFlag{ // --xxx-backend
				Category:    strings.ToUpper(category),
				Destination: &cfg.BackendURL,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_BACKEND"},
				Name:        category + "-backend",
				Usage:       "`url` of " + category + " backend",
				Value:       backendURL,
			},

			&cli.DurationFlag{ // --xxx-backend-timeout
				Category:    strings.ToUpper(category),
				Destination: &cfg.BackendTimeout,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_BACKEND_TIMEOUT"},
				Name:        category + "-backend-timeout",
				Usage:       "max `duration` for " + category + " backend requests",
				Value:       time.Second,
			},

			&cli.DurationFlag{ // --xxx-client-idle-connection-timeout
				Category:    strings.ToUpper(category),
				Destination: &cfg.ClientIdleConnectionTimeout,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_CLIENT_IDLE_CONNECTION_TIMEOUT"},
				Name:        category + "-client-idle-connection-timeout",
				Usage:       "`duration` to keep idle " + category + " connections open",
				Value:       30 * time.Second,
			},

			&cli.BoolFlag{ // --xxx-enabled
				Category:    strings.ToUpper(category),
				Destination: &cfg.Enabled,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_ENABLED"},
				Name:        category + "-enabled",
				Usage:       "enable " + category + " proxy",
				Value:       false,
			},

			&cli.StringSliceFlag{ // --xxx-extra-mirrored-jrpc-methods
				Category:    strings.ToUpper(category),
				Destination: extraMirroredJrpcMethods,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_EXTRA_MIRRORED_JRPC_METHODS"},
				Name:        category + "-extra-mirrored-jrpc-methods",
				Usage:       "list of " + category + " jrpc `methods` that will be mirrored in addition to the default",
			},

			&cli.StringFlag{ // --xxx-healthcheck
				Category:    strings.ToUpper(category),
				DefaultText: "disabled",
				Destination: &cfg.HealthcheckURL,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_HEALTHCHECK"},
				Name:        category + "-healthcheck",
				Usage:       "`url` of " + category + " backend healthcheck endpoint",
				Value:       "",
			},

			&cli.DurationFlag{ // --xxx-healthcheck-interval
				Category:    strings.ToUpper(category),
				Destination: &cfg.HealthcheckInterval,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_HEALTHCHECK_INTERVAL"},
				Name:        category + "-healthcheck-interval",
				Usage:       "`interval` between consecutive " + category + " backend healthchecks",
				Value:       time.Second,
			},

			&cli.IntFlag{ // --xxx-healthcheck-threshold-healthy
				Category:    strings.ToUpper(category),
				Destination: &cfg.HealthcheckThresholdHealthy,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_HEALTHCHECK_THRESHOLD_HEALTHY"},
				Name:        category + "-healthcheck-threshold-healthy",
				Usage:       "`count` of consecutive successful healthchecks to consider " + category + " backend to be healthy",
				Value:       2,
			},

			&cli.IntFlag{ // --xxx-healthcheck-threshold-unhealthy
				Category:    strings.ToUpper(category),
				Destination: &cfg.HealthcheckThresholdUnhealthy,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_HEALTHCHECK_THRESHOLD_UNHEALTHY"},
				Name:        category + "-healthcheck-threshold-unhealthy",
				Usage:       "`count` of consecutive failed healthchecks to consider " + category + " backend to be unhealthy",
				Value:       2,
			},

			&cli.StringFlag{ // --xxx-listen-address
				Category:    strings.ToUpper(category),
				Destination: &cfg.ListenAddress,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LISTEN_ADDRESS"},
				Name:        category + "-listen-address",
				Usage:       "`host:port` for " + category + " proxy",
				Value:       listenAddress,
			},

			&cli.BoolFlag{ // --xxx-log-requests
				Category:    strings.ToUpper(category),
				Destination: &cfg.LogRequests,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LOG_REQUESTS"},
				Name:        category + "-log-requests",
				Usage:       "whether to log " + category + " requests",
				Value:       false,
			},

			&cli.BoolFlag{ // --xxx-log-responses
				Category:    strings.ToUpper(category),
				Destination: &cfg.LogResponses,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LOG_RESPONSES"},
				Name:        category + "-log-responses",
				Usage:       "whether to log responses to proxied/mirrored " + category + " requests",
				Value:       false,
			},

			&cli.DurationFlag{ // --xxx-max-backend-connection-wait-timeout
				Category:    strings.ToUpper(category),
				Destination: &cfg.MaxBackendConnectionWaitTimeout,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_MAX_BACKEND_CONNECTION_WAIT_TIMEOUT"},
				Name:        category + "-max-backend-connection-wait-timeout",
				Usage:       "maximum `duration` to wait for a free " + category + " backend connection (0s means don't wait)",
				Value:       0,
			},

			&cli.IntFlag{ // --xxx-max-backend-connections-per-host
				Category:    strings.ToUpper(category),
				Destination: &cfg.MaxBackendConnectionsPerHost,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_MAX_BACKEND_CONNECTIONS_PER_HOST"},
				Name:        category + "-max-backend-connections-per-host",
				Usage:       "maximum connections `count` per " + category + " backend host",
				Value:       1,
			},

			&cli.IntFlag{ // --xxx-max-client-connections-per-ip
				Category:    strings.ToUpper(category),
				Destination: &cfg.MaxClientConnectionsPerIP,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_MAX_CLIENT_CONNECTIONS_PER_IP"},
				Name:        category + "-max-client-connections-per-ip",
				Usage:       "maximum " + category + " client tcp connections `count` per ip (0 means unlimited)",
				Value:       0,
			},

			&cli.IntFlag{ // --xxx-max-request-size
				Category:    strings.ToUpper(category),
				Destination: &cfg.MaxRequestSizeMb,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_MAX_REQUEST_SIZE"},
				Name:        category + "-max-request-size",
				Usage:       "maximum " + category + " request payload size in `megabytes`",
				Value:       15,
			},

			&cli.IntFlag{ // --xxx-max-response-size
				Category:    strings.ToUpper(category),
				Destination: &cfg.MaxResponseSizeMb,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_MAX_RESPONSE_SIZE"},
				Name:        category + "-max-response-size",
				Usage:       "maximum " + category + " response payload size in `megabytes`",
				Value:       160,
			},

			&cli.BoolFlag{ // --peer-tls-insecure-skip-verify
				Category:    strings.ToUpper(category),
				Destination: &cfg.PeerTLSInsecureSkipVerify,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_PEER_TLS_INSECURE_SKIP_VERIFY"},
				Name:        category + "-peer-tls-insecure-skip-verify",
				Usage:       "do not verify " + category + " peers' tls certificates",
				Value:       false,
			},

			&cli.StringSliceFlag{ // --xxx-peers
				Category:    strings.ToUpper(category),
				Destination: peerURLsFlag,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_PEERS"},
				Name:        category + "-peers",
				Usage:       "list of " + category + " peers `urls` to mirror the requests to",
			},

			&cli.BoolFlag{ // --xxx-remove-backend-from-peers
				Category:    strings.ToUpper(category),
				Destination: &cfg.RemoveBackendFromPeers,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_REMOVE_BACKEND_FROM_PEERS"},
				Name:        category + "-remove-backend-from-peers",
				Usage:       "remove " + category + " backend from peers",
				Value:       false,
			},

			&cli.StringFlag{ // --xxx-tls-crt
				Category:    strings.ToUpper(category),
				Destination: &cfg.TLSCertificate,
				DefaultText: "uses plain-text http",
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_TLS_CRT"},
				Name:        category + "-tls-crt",
				Usage:       "`path` to tls certificate",
			},

			&cli.StringFlag{ // --xxx-tls-key
				Category:    strings.ToUpper(category),
				DefaultText: "uses plain-text http",
				Destination: &cfg.TLSKey,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_TLS_KEY"},
				Name:        category + "-tls-key",
				Usage:       "`path` to tls key",
			},
		}

		return
	}

	authrpcFlags, extraMirroredJrpcMethodsAuthRPC, peerURLsAuthRPC := proxyFlags(
		cfg.AuthrpcProxy.Proxy, categoryAuthRPC, "http://127.0.0.1:18551", "0.0.0.0:8551",
	)

	authrpcFlags = append(authrpcFlags,
		&cli.BoolFlag{
			Category:    strings.ToUpper(categoryAuthRPC),
			Destination: &cfg.AuthrpcProxy.DeduplicateFCUs,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthRPC) + "_DEDUPLICATE_FCUS"},
			Name:        categoryAuthRPC + "-deduplicate-fcus",
			Usage:       "deduplicate repetitive fcu messages",
		},
	)

	chaosFlags := []cli.Flag{ // chaos
		&cli.BoolFlag{ // --chaos-enabled
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.Enabled,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_ENABLED"},
			Name:        categoryChaos + "-enabled",
			Usage:       "whether bproxy should be injecting artificial error conditions",
			Value:       false,
		},

		&cli.Float64Flag{ // --chaos-injected-http-error-probability
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.InjectedHttpErrorProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_INJECTED_HTTP_ERROR_PROBABILITY"},
			Name:        categoryChaos + "-injected-http-error-probability",
			Usage:       "probability in `percent` at which to randomly inject http errors into proxied responses",
			Value:       20,
		},

		&cli.Float64Flag{ // --chaos-injected-jrpc-error-probability
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.InjectedJrpcErrorProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_INJECTED_JRPC_ERROR_PROBABILITY"},
			Name:        categoryChaos + "-injected-jrpc-error-probability",
			Usage:       "probability in `percent` at which to randomly inject jrpc errors into proxied responses",
			Value:       20,
		},

		&cli.Float64Flag{ // --chaos-injected-invalid-jrpc-response-probability
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.InjectedInvalidJrpcResponseProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_INJECTED_INVALID_JRPC_RESPONSE_PROBABILITY"},
			Name:        categoryChaos + "-injected-invalid-jrpc-response-probability",
			Usage:       "probability in `percent` at which to randomly inject invalid jrpc into proxied responses",
			Value:       20,
		},

		&cli.DurationFlag{ // --chaos-min-injected-latency
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.MinInjectedLatency,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_MIN_INJECTED_LATENCY"},
			Name:        categoryChaos + "-min-injected-latency",
			Usage:       "min `latency` to enforce on every proxied response",
			Value:       50 * time.Millisecond,
		},

		&cli.DurationFlag{ // --chaos-max-injected-latency
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Chaos.MaxInjectedLatency,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_MAX_INJECTED_LATENCY"},
			Name:        categoryChaos + "-max-injected-latency",
			Usage:       "max `latency` to randomly enforce on every proxied response",
		},
	}

	rpcFlags, extraMirroredJrpcMethodsRPC, peerURLsRPC := proxyFlags(
		cfg.RpcProxy.Proxy, categoryRPC, "http://127.0.0.1:18545", "0.0.0.0:8545",
	)

	metricsFlags := []cli.Flag{ // metrics
		&cli.StringFlag{ // --metrics-listen-address
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
			cfg.AuthrpcProxy.PeerURLs = peerURLsAuthRPC.Value()
			cfg.AuthrpcProxy.ExtraMirroredJrpcMethods = extraMirroredJrpcMethodsAuthRPC.Value()

			cfg.RpcProxy.PeerURLs = peerURLsRPC.Value()
			cfg.RpcProxy.ExtraMirroredJrpcMethods = extraMirroredJrpcMethodsRPC.Value()

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
