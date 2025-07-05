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
	categoryChaos       = "chaos"
	categoryAuthrpc     = "authrpc"
	categoryFlashblocks = "flashblocks"
	categoryRPC         = "rpc"
	categoryMetrics     = "metrics"
)

func CommandServe(cfg *config.Config) *cli.Command {
	makeProxyFlags := func(
		cfg *config.HttpProxy, category string, backendURL, listenAddress string,
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

			&cli.BoolFlag{ // --chaos-xxx-enabled
				Category:    strings.ToUpper(categoryChaos),
				Destination: &cfg.Chaos.Enabled,
				EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos+"_"+category) + "_ENABLED"},
				Name:        categoryChaos + "-" + category + "-enabled",
				Usage:       "whether " + category + " proxy should be injecting artificial error conditions",
				Value:       false,
			},

			&cli.Float64Flag{ // --chaos-xxx-injected-http-error-probability
				Category:    strings.ToUpper(categoryChaos),
				Destination: &cfg.Chaos.InjectedHttpErrorProbability,
				EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos+"_"+category) + "_INJECTED_HTTP_ERROR_PROBABILITY"},
				Name:        categoryChaos + "-" + category + "-injected-http-error-probability",
				Usage:       "probability in `percent` at which to randomly inject http errors into responses processed by " + category + " proxy",
				Value:       0,
			},

			&cli.Float64Flag{ // --chaos-xxx-injected-jrpc-error-probability
				Category:    strings.ToUpper(categoryChaos),
				Destination: &cfg.Chaos.InjectedJrpcErrorProbability,
				EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos+"_"+category) + "_INJECTED_JRPC_ERROR_PROBABILITY"},
				Name:        categoryChaos + "-" + category + "-injected-jrpc-error-probability",
				Usage:       "probability in `percent` at which to randomly inject jrpc errors into responses processed by " + category + " proxy",
				Value:       0,
			},

			&cli.Float64Flag{ // --chaos-xxx-injected-invalid-jrpc-response-probability
				Category:    strings.ToUpper(categoryChaos),
				Destination: &cfg.Chaos.InjectedInvalidJrpcResponseProbability,
				EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos+"_"+category) + "_INJECTED_INVALID_JRPC_RESPONSE_PROBABILITY"},
				Name:        categoryChaos + "-" + category + "-injected-invalid-jrpc-response-probability",
				Usage:       "probability in `percent` at which to randomly inject invalid jrpc into responses processed by " + category + " proxy",
				Value:       0,
			},

			&cli.DurationFlag{ // --chaos-xxx-min-injected-latency
				Category:    strings.ToUpper(categoryChaos),
				Destination: &cfg.Chaos.MinInjectedLatency,
				EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos+"_"+category) + "_MIN_INJECTED_LATENCY"},
				Name:        categoryChaos + "-" + category + "-min-injected-latency",
				Usage:       "min `latency` to enforce on every response processed by " + category + " proxy",
			},

			&cli.DurationFlag{ // --chaos-xxx-max-injected-latency
				Category:    strings.ToUpper(categoryChaos),
				Destination: &cfg.Chaos.MaxInjectedLatency,
				EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos+"_"+category) + "_MAX_INJECTED_LATENCY"},
				Name:        categoryChaos + "-" + category + "-max-injected-latency",
				Usage:       "max `latency` to randomly enforce on every response processed by " + category + " proxy",
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
				Destination: &cfg.Healthcheck.URL,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_HEALTHCHECK"},
				Name:        category + "-healthcheck",
				Usage:       "`url` of " + category + " backend healthcheck endpoint",
				Value:       "",
			},

			&cli.DurationFlag{ // --xxx-healthcheck-interval
				Category:    strings.ToUpper(category),
				Destination: &cfg.Healthcheck.Interval,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_HEALTHCHECK_INTERVAL"},
				Name:        category + "-healthcheck-interval",
				Usage:       "`interval` between consecutive " + category + " backend healthchecks",
				Value:       time.Second,
			},

			&cli.IntFlag{ // --xxx-healthcheck-threshold-healthy
				Category:    strings.ToUpper(category),
				Destination: &cfg.Healthcheck.ThresholdHealthy,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_HEALTHCHECK_THRESHOLD_HEALTHY"},
				Name:        category + "-healthcheck-threshold-healthy",
				Usage:       "`count` of consecutive successful healthchecks to consider " + category + " backend to be healthy",
				Value:       2,
			},

			&cli.IntFlag{ // --xxx-healthcheck-threshold-unhealthy
				Category:    strings.ToUpper(category),
				Destination: &cfg.Healthcheck.ThresholdUnhealthy,
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

			&cli.IntFlag{ // --xxx-log-requests-max-size
				Category:    strings.ToUpper(category),
				Destination: &cfg.LogRequestsMaxSize,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LOG_REQUESTS_MAX_SIZE"},
				Name:        category + "-log-requests-max-size",
				Usage:       "do not log " + category + " requests larger than `size`",
				Value:       4096,
			},

			&cli.BoolFlag{ // --xxx-log-responses
				Category:    strings.ToUpper(category),
				Destination: &cfg.LogResponses,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LOG_RESPONSES"},
				Name:        category + "-log-responses",
				Usage:       "whether to log responses to proxied/mirrored " + category + " requests",
				Value:       false,
			},

			&cli.IntFlag{ // --xxx-log-responses-max-size
				Category:    strings.ToUpper(category),
				Destination: &cfg.LogResponsesMaxSize,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_LOG_RESPONSES_MAX_SIZE"},
				Name:        category + "-log-responses-max-size",
				Usage:       "do not log " + category + " responses larger than `size`",
				Value:       4096,
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
				Usage:       "`path` to " + category + " tls certificate",
			},

			&cli.StringFlag{ // --xxx-tls-key
				Category:    strings.ToUpper(category),
				DefaultText: "uses plain-text http",
				Destination: &cfg.TLSKey,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_TLS_KEY"},
				Name:        category + "-tls-key",
				Usage:       "`path` to " + category + " tls key",
			},

			&cli.BoolFlag{ // --xxx-use-priority-queue
				Category:    strings.ToUpper(category),
				Destination: &cfg.UsePriorityQueue,
				EnvVars:     []string{envPrefix + strings.ToUpper(category) + "_USE_PRIORITY_QUEUE"},
				Name:        category + "-use-priority-queue",
				Usage:       `whether to prioritise "important" calls over the rest`,
			},
		}

		return
	}

	authrpcFlags, extraMirroredJrpcMethodsAuthRPC, peerURLsAuthRPC := makeProxyFlags(
		cfg.Authrpc.HttpProxy, categoryAuthrpc, "http://127.0.0.1:18551", "0.0.0.0:8551",
	)

	authrpcFlags = append(authrpcFlags, // --authrpc-xxx
		&cli.BoolFlag{ // --authrpc-deduplicate-fcus
			Category:    strings.ToUpper(categoryAuthrpc),
			Destination: &cfg.Authrpc.DeduplicateFCUs,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryAuthrpc) + "_DEDUPLICATE_FCUS"},
			Name:        categoryAuthrpc + "-deduplicate-fcus",
			Usage:       "deduplicate repetitive fcu messages",
		},
	)

	flashblocksFlags := []cli.Flag{ // --flashblocks-xxx
		&cli.StringFlag{ // --flashblocks-backend
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.BackendURL,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_BACKEND"},
			Name:        categoryFlashblocks + "-backend",
			Usage:       "`url` of flashblocks backend",
			Value:       "ws://127.0.0.1:11111",
		},

		&cli.DurationFlag{ // --flashblocks-backward-timeout
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.BackwardTimeout,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_BACKWARD_TIMEOUT"},
			Name:        categoryFlashblocks + "-backward-timeout",
			Usage:       "max `duration` for flashblocks frontend reads and backend writes (0s means no timeout)",
			Value:       0,
		},

		&cli.Float64Flag{ // --chaos-flashblocks-dropped-message-probability
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Flashblocks.Chaos.DroppedMessageProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_FLASHBLOCKS_DROPPED_MESSAGE_PROBABILITY"},
			Name:        categoryChaos + "-flashblocks-dropped-message-probability",
			Usage:       "probability in `percent` at which to randomly drop messages processed by flashblocks proxy",
			Value:       0,
		},

		&cli.BoolFlag{ // --chaos-flashblocks-enabled
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Flashblocks.Chaos.Enabled,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_FLASHBLOCKS_ENABLED"},
			Name:        categoryChaos + "-flashblocks-enabled",
			Usage:       "whether flashblocks proxy should be injecting artificial error conditions",
			Value:       false,
		},

		&cli.Float64Flag{ // --chaos-flashblocks-injected-invalid-flashblock-payload-probability
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Flashblocks.Chaos.InjectedInvalidFlashblockPayloadProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_FLASHBLOCKS_INJECTED_INVALID_FLASHBLOCK_PAYLOAD_PROBABILITY"},
			Name:        categoryChaos + "-flashblocks-injected-invalid-flashblock-payload-probability",
			Usage:       "probability in `percent` at which to randomly inject an invalid flashblock",
			Value:       0,
		},

		&cli.Float64Flag{ // --chaos-flashblocks-injected-malformed-json-message-probability
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Flashblocks.Chaos.InjectedMalformedJsonMessageProbability,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_FLASHBLOCKS_INJECTED_MALFORMED_JSON_MESSAGE_PROBABILITY"},
			Name:        categoryChaos + "-flashblocks-injected-malformed-json-message-probability",
			Usage:       "probability in `percent` at which to randomly inject a malformed json message",
			Value:       0,
		},

		&cli.DurationFlag{ // --chaos-flashblocks-min-injected-latency
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Flashblocks.Chaos.MinInjectedLatency,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_FLASHBLOCKS_MIN_INJECTED_LATENCY"},
			Name:        categoryChaos + "-flashblocks-min-injected-latency",
			Usage:       "min `latency` to enforce on every response processed by flashblocks proxy",
		},

		&cli.DurationFlag{ // --chaos-flashblocks-max-injected-latency
			Category:    strings.ToUpper(categoryChaos),
			Destination: &cfg.Flashblocks.Chaos.MaxInjectedLatency,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryChaos) + "_FLASHBLOCKS_MAX_INJECTED_LATENCY"},
			Name:        categoryChaos + "-flashblocks-max-injected-latency",
			Usage:       "max `latency` to randomly enforce on every response processed by flashblocks proxy",
		},

		&cli.DurationFlag{ // --flashblocks-control-timeout
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.ControlTimeout,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_CONTROL_TIMEOUT"},
			Name:        categoryFlashblocks + "-control-timeout",
			Usage:       "max `duration` for control websocket messages reads and writes (0s means no timeout)",
			Value:       2 * time.Second,
		},

		&cli.BoolFlag{ // --flashblocks-enabled
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.Enabled,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_ENABLED"},
			Name:        categoryFlashblocks + "-enabled",
			Usage:       "enable flashblocks proxy",
			Value:       false,
		},

		&cli.DurationFlag{ // --flashblocks-forward-timeout
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.ForwardTimeout,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_FORWARD_TIMEOUT"},
			Name:        categoryFlashblocks + "-forward-timeout",
			Usage:       "max `duration` for flashblocks backend reads and frontend writes (0s means no timeout)",
			Value:       5 * time.Second,
		},

		&cli.StringFlag{ // --flashblocks-healthcheck
			Category:    strings.ToUpper(categoryFlashblocks),
			DefaultText: "disabled",
			Destination: &cfg.Flashblocks.Healthcheck.URL,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_HEALTHCHECK"},
			Name:        categoryFlashblocks + "-healthcheck",
			Usage:       "`url` of flashblocks backend healthcheck endpoint",
			Value:       "",
		},

		&cli.DurationFlag{ // --flashblocks-healthcheck-interval
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.Healthcheck.Interval,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_HEALTHCHECK_INTERVAL"},
			Name:        categoryFlashblocks + "-healthcheck-interval",
			Usage:       "`interval` between consecutive flashblocks backend healthchecks",
			Value:       time.Second,
		},

		&cli.IntFlag{ // --flashblocks-healthcheck-threshold-healthy
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.Healthcheck.ThresholdHealthy,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_HEALTHCHECK_THRESHOLD_HEALTHY"},
			Name:        categoryFlashblocks + "-healthcheck-threshold-healthy",
			Usage:       "`count` of consecutive successful healthchecks to consider flashblocks backend to be healthy",
			Value:       2,
		},

		&cli.IntFlag{ // --flashblocks-healthcheck-threshold-unhealthy
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.Healthcheck.ThresholdUnhealthy,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_HEALTHCHECK_THRESHOLD_UNHEALTHY"},
			Name:        categoryFlashblocks + "-healthcheck-threshold-unhealthy",
			Usage:       "`count` of consecutive failed healthchecks to consider flashblocks backend to be unhealthy",
			Value:       2,
		},

		&cli.StringFlag{ // --flashblocks-listen-address
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.ListenAddress,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_LISTEN_ADDRESS"},
			Name:        categoryFlashblocks + "-listen-address",
			Usage:       "`host:port` for flashblocks proxy",
			Value:       "0.0.0.0:1111",
		},

		&cli.BoolFlag{ // --flashblocks-log-messages
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.LogMessages,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_LOG_MESSAGES"},
			Name:        categoryFlashblocks + "-log-messages",
			Usage:       "whether to log flashblocks messages",
			Value:       false,
		},

		&cli.IntFlag{ // --flashblocks-log-messages-max-size
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.LogMessagesMaxSize,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_LOG_MESSAGES_MAX_SIZE"},
			Name:        categoryFlashblocks + "-log-messages-max-size",
			Usage:       "do not log flashblocks messages larger than `size`",
			Value:       4096,
		},

		&cli.IntFlag{ // --flashblocks-read-buffer-size
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.ReadBufferSize,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_READ_BUFFER_SIZE"},
			Name:        categoryFlashblocks + "-read-buffer-size",
			Usage:       "flashblocks read buffer size in `megabytes` (messages from client)",
			Value:       16,
		},

		&cli.StringFlag{ // --flashblocks-tls-crt
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.TLSCertificate,
			DefaultText: "uses plain-text http",
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_TLS_CRT"},
			Name:        categoryFlashblocks + "-tls-crt",
			Usage:       "`path` to flashblocks tls certificate",
		},

		&cli.StringFlag{ // --flashblocks-tls-key
			Category:    strings.ToUpper(categoryFlashblocks),
			DefaultText: "uses plain-text http",
			Destination: &cfg.Flashblocks.TLSKey,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_TLS_KEY"},
			Name:        categoryFlashblocks + "-tls-key",
			Usage:       "`path` to flashblocks tls key",
		},

		&cli.IntFlag{ // --flashblocks-write-buffer-size
			Category:    strings.ToUpper(categoryFlashblocks),
			Destination: &cfg.Flashblocks.WriteBufferSize,
			EnvVars:     []string{envPrefix + strings.ToUpper(categoryFlashblocks) + "_WRITE_BUFFER_SIZE"},
			Name:        categoryFlashblocks + "-write-buffer-size",
			Usage:       "flashblocks write buffer size in `megabytes` (messages from backend)",
			Value:       16,
		},
	}

	rpcFlags, extraMirroredJrpcMethodsRPC, peerURLsRPC := makeProxyFlags(
		cfg.Rpc.HttpProxy, categoryRPC, "http://127.0.0.1:18545", "0.0.0.0:8545",
	)

	metricsFlags := []cli.Flag{ // --metrics-xxx
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
		authrpcFlags,
		flashblocksFlags,
		rpcFlags,
		metricsFlags,
	)

	return &cli.Command{
		Name:  "serve",
		Usage: "run bproxy server",
		Flags: flags,

		Before: func(_ *cli.Context) error {
			cfg.Authrpc.PeerURLs = peerURLsAuthRPC.Value()
			cfg.Authrpc.ExtraMirroredJrpcMethods = extraMirroredJrpcMethodsAuthRPC.Value()

			cfg.Rpc.PeerURLs = peerURLsRPC.Value()
			cfg.Rpc.ExtraMirroredJrpcMethods = extraMirroredJrpcMethodsRPC.Value()

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
