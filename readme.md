# bproxy

L2 builder proxy that proxies RPC and Authenticated RPC calls to the builder,
and mirrors `eth_sendRawTransaction` (on rpc) and `engine_forkchoiceUpdatedV3`
with extra attributes (on authrpc) to its peers.

## Usage

```text
bproxy serve [command options]

OPTIONS:
   AUTHRPC

   --authrpc-backend url                                                                            url of authrpc backend (default: "http://127.0.0.1:18551") [$BPROXY_AUTHRPC_BACKEND]
   --authrpc-backend-timeout duration                                                               max duration for authrpc backend requests (default: 1s) [$BPROXY_AUTHRPC_BACKEND_TIMEOUT]
   --authrpc-client-idle-connection-timeout duration                                                duration to keep idle authrpc connections open (default: 30s) [$BPROXY_AUTHRPC_CLIENT_IDLE_CONNECTION_TIMEOUT]
   --authrpc-deduplicate-fcus                                                                       deduplicate repetitive fcu messages (default: false) [$BPROXY_AUTHRPC_DEDUPLICATE_FCUS]
   --authrpc-enabled                                                                                enable authrpc proxy (default: false) [$BPROXY_AUTHRPC_ENABLED]
   --authrpc-extra-mirrored-jrpc-methods methods [ --authrpc-extra-mirrored-jrpc-methods methods ]  list of authrpc jrpc methods that will be mirrored in addition to the default [$BPROXY_AUTHRPC_EXTRA_MIRRORED_JRPC_METHODS]
   --authrpc-healthcheck url                                                                        url of authrpc backend healthcheck endpoint (default: disabled) [$BPROXY_AUTHRPC_HEALTHCHECK]
   --authrpc-healthcheck-interval interval                                                          interval between consecutive authrpc backend healthchecks (default: 1s) [$BPROXY_AUTHRPC_HEALTHCHECK_INTERVAL]
   --authrpc-healthcheck-threshold-healthy count                                                    count of consecutive successful healthchecks to consider authrpc backend to be healthy (default: 2) [$BPROXY_AUTHRPC_HEALTHCHECK_THRESHOLD_HEALTHY]
   --authrpc-healthcheck-threshold-unhealthy count                                                  count of consecutive failed healthchecks to consider authrpc backend to be unhealthy (default: 2) [$BPROXY_AUTHRPC_HEALTHCHECK_THRESHOLD_UNHEALTHY]
   --authrpc-listen-address host:port                                                               host:port for authrpc proxy (default: "0.0.0.0:8551") [$BPROXY_AUTHRPC_LISTEN_ADDRESS]
   --authrpc-log-requests                                                                           whether to log authrpc requests (default: false) [$BPROXY_AUTHRPC_LOG_REQUESTS]
   --authrpc-log-requests-max-size size                                                             do not log authrpc requests larger than size (default: 4096) [$BPROXY_AUTHRPC_LOG_REQUESTS_MAX_SIZE]
   --authrpc-log-responses                                                                          whether to log responses to proxied/mirrored authrpc requests (default: false) [$BPROXY_AUTHRPC_LOG_RESPONSES]
   --authrpc-log-responses-max-size size                                                            do not log authrpc responses larger than size (default: 4096) [$BPROXY_AUTHRPC_LOG_RESPONSES_MAX_SIZE]
   --authrpc-max-backend-connection-wait-timeout duration                                           maximum duration to wait for a free authrpc backend connection (0s means don't wait) (default: 0s) [$BPROXY_AUTHRPC_MAX_BACKEND_CONNECTION_WAIT_TIMEOUT]
   --authrpc-max-backend-connections-per-host count                                                 maximum connections count per authrpc backend host (default: 1) [$BPROXY_AUTHRPC_MAX_BACKEND_CONNECTIONS_PER_HOST]
   --authrpc-max-client-connections-per-ip count                                                    maximum authrpc client tcp connections count per ip (0 means unlimited) (default: 0) [$BPROXY_AUTHRPC_MAX_CLIENT_CONNECTIONS_PER_IP]
   --authrpc-max-request-size megabytes                                                             maximum authrpc request payload size in megabytes (default: 15) [$BPROXY_AUTHRPC_MAX_REQUEST_SIZE]
   --authrpc-max-response-size megabytes                                                            maximum authrpc response payload size in megabytes (default: 160) [$BPROXY_AUTHRPC_MAX_RESPONSE_SIZE]
   --authrpc-peer-tls-insecure-skip-verify                                                          do not verify authrpc peers' tls certificates (default: false) [$BPROXY_AUTHRPC_PEER_TLS_INSECURE_SKIP_VERIFY]
   --authrpc-peers urls [ --authrpc-peers urls ]                                                    list of authrpc peers urls to mirror the requests to [$BPROXY_AUTHRPC_PEERS]
   --authrpc-remove-backend-from-peers                                                              remove authrpc backend from peers (default: false) [$BPROXY_AUTHRPC_REMOVE_BACKEND_FROM_PEERS]
   --authrpc-tls-crt path                                                                           path to tls certificate (default: uses plain-text http) [$BPROXY_AUTHRPC_TLS_CRT]
   --authrpc-tls-key path                                                                           path to tls key (default: uses plain-text http) [$BPROXY_AUTHRPC_TLS_KEY]
   --authrpc-use-priority-queue                                                                     whether to prioritise "important" calls over the rest (default: false) [$BPROXY_AUTHRPC_USE_PRIORITY_QUEUE]

   CHAOS

   --chaos-enabled                                             whether bproxy should be injecting artificial error conditions (default: false) [$BPROXY_CHAOS_ENABLED]
   --chaos-injected-http-error-probability percent             probability in percent at which to randomly inject http errors into proxied responses (default: 20) [$BPROXY_CHAOS_INJECTED_HTTP_ERROR_PROBABILITY]
   --chaos-injected-invalid-jrpc-response-probability percent  probability in percent at which to randomly inject invalid jrpc into proxied responses (default: 20) [$BPROXY_CHAOS_INJECTED_INVALID_JRPC_RESPONSE_PROBABILITY]
   --chaos-injected-jrpc-error-probability percent             probability in percent at which to randomly inject jrpc errors into proxied responses (default: 20) [$BPROXY_CHAOS_INJECTED_JRPC_ERROR_PROBABILITY]
   --chaos-max-injected-latency latency                        max latency to randomly enforce on every proxied response (default: 0s) [$BPROXY_CHAOS_MAX_INJECTED_LATENCY]
   --chaos-min-injected-latency latency                        min latency to enforce on every proxied response (default: 50ms) [$BPROXY_CHAOS_MIN_INJECTED_LATENCY]

   METRICS

   --metrics-listen-address host:port  host:port for metrics server (default: "0.0.0.0:6785") [$BPROXY_METRICS_LISTEN_ADDRESS]

   RPC

   --rpc-backend url                                                                        url of rpc backend (default: "http://127.0.0.1:18545") [$BPROXY_RPC_BACKEND]
   --rpc-backend-timeout duration                                                           max duration for rpc backend requests (default: 1s) [$BPROXY_RPC_BACKEND_TIMEOUT]
   --rpc-client-idle-connection-timeout duration                                            duration to keep idle rpc connections open (default: 30s) [$BPROXY_RPC_CLIENT_IDLE_CONNECTION_TIMEOUT]
   --rpc-enabled                                                                            enable rpc proxy (default: false) [$BPROXY_RPC_ENABLED]
   --rpc-extra-mirrored-jrpc-methods methods [ --rpc-extra-mirrored-jrpc-methods methods ]  list of rpc jrpc methods that will be mirrored in addition to the default [$BPROXY_RPC_EXTRA_MIRRORED_JRPC_METHODS]
   --rpc-healthcheck url                                                                    url of rpc backend healthcheck endpoint (default: disabled) [$BPROXY_RPC_HEALTHCHECK]
   --rpc-healthcheck-interval interval                                                      interval between consecutive rpc backend healthchecks (default: 1s) [$BPROXY_RPC_HEALTHCHECK_INTERVAL]
   --rpc-healthcheck-threshold-healthy count                                                count of consecutive successful healthchecks to consider rpc backend to be healthy (default: 2) [$BPROXY_RPC_HEALTHCHECK_THRESHOLD_HEALTHY]
   --rpc-healthcheck-threshold-unhealthy count                                              count of consecutive failed healthchecks to consider rpc backend to be unhealthy (default: 2) [$BPROXY_RPC_HEALTHCHECK_THRESHOLD_UNHEALTHY]
   --rpc-listen-address host:port                                                           host:port for rpc proxy (default: "0.0.0.0:8545") [$BPROXY_RPC_LISTEN_ADDRESS]
   --rpc-log-requests                                                                       whether to log rpc requests (default: false) [$BPROXY_RPC_LOG_REQUESTS]
   --rpc-log-requests-max-size size                                                         do not log rpc requests larger than size (default: 4096) [$BPROXY_RPC_LOG_REQUESTS_MAX_SIZE]
   --rpc-log-responses                                                                      whether to log responses to proxied/mirrored rpc requests (default: false) [$BPROXY_RPC_LOG_RESPONSES]
   --rpc-log-responses-max-size size                                                        do not log rpc responses larger than size (default: 4096) [$BPROXY_RPC_LOG_RESPONSES_MAX_SIZE]
   --rpc-max-backend-connection-wait-timeout duration                                       maximum duration to wait for a free rpc backend connection (0s means don't wait) (default: 0s) [$BPROXY_RPC_MAX_BACKEND_CONNECTION_WAIT_TIMEOUT]
   --rpc-max-backend-connections-per-host count                                             maximum connections count per rpc backend host (default: 1) [$BPROXY_RPC_MAX_BACKEND_CONNECTIONS_PER_HOST]
   --rpc-max-client-connections-per-ip count                                                maximum rpc client tcp connections count per ip (0 means unlimited) (default: 0) [$BPROXY_RPC_MAX_CLIENT_CONNECTIONS_PER_IP]
   --rpc-max-request-size megabytes                                                         maximum rpc request payload size in megabytes (default: 15) [$BPROXY_RPC_MAX_REQUEST_SIZE]
   --rpc-max-response-size megabytes                                                        maximum rpc response payload size in megabytes (default: 160) [$BPROXY_RPC_MAX_RESPONSE_SIZE]
   --rpc-peer-tls-insecure-skip-verify                                                      do not verify rpc peers' tls certificates (default: false) [$BPROXY_RPC_PEER_TLS_INSECURE_SKIP_VERIFY]
   --rpc-peers urls [ --rpc-peers urls ]                                                    list of rpc peers urls to mirror the requests to [$BPROXY_RPC_PEERS]
   --rpc-remove-backend-from-peers                                                          remove rpc backend from peers (default: false) [$BPROXY_RPC_REMOVE_BACKEND_FROM_PEERS]
   --rpc-tls-crt path                                                                       path to tls certificate (default: uses plain-text http) [$BPROXY_RPC_TLS_CRT]
   --rpc-tls-key path                                                                       path to tls key (default: uses plain-text http) [$BPROXY_RPC_TLS_KEY]
   --rpc-use-priority-queue                                                                 whether to prioritise "important" calls over the rest (default: false) [$BPROXY_RPC_USE_PRIORITY_QUEUE]
```
