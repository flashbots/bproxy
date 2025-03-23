# bproxy

L2 builder proxy that proxies RPC and Authenticated RPC calls to the builder,
and mirrors `eth_sendRawTransaction` (on rpc) and `engine_forkchoiceUpdatedV3`
with extra attributes (on authrpc) to its peers.

## Usage

```text
bproxy serve [command options]

OPTIONS:
   AUTHRPC

   --authrpc-backend url                          url of backend authrpc (default: "http://127.0.0.1:18551") [$BPROXY_AUTHRPC_BACKEND]
   --authrpc-enabled                              enable authrpc proxy (default: false) [$BPROXY_AUTHRPC_ENABLED]
   --authrpc-listen-address host:port             host:port for authrpc proxy (default: "0.0.0.0:8551") [$BPROXY_AUTHRPC_LISTEN_ADDRESS]
   --authrpc-log-requests                         whether to log authrpc requests (default: false) [$BPROXY_AUTHRPC_LOG_REQUESTS]
   --authrpc-log-responses                        whether to log responses to proxied/mirrored authrpc requests (default: false) [$BPROXY_AUTHRPC_LOG_RESPONSES]
   --authrpc-max-request-size megabytes           maximum authrpc request payload size in megabytes (default: 15) [$BPROXY_AUTHRPC_MAX_REQUEST_SIZE]
   --authrpc-max-response-size megabytes          maximum authrpc response payload size in megabytes (default: 160) [$BPROXY_AUTHRPC_MAX_RESPONSE_SIZE]
   --authrpc-peers urls [ --authrpc-peers urls ]  list of urls with authrpc peers to mirror the requests to [$BPROXY_AUTHRPC_PEERS]
   --authrpc-remove-backend-from-peers            remove authrpc backend from peers (default: false) [$BPROXY_AUTHRPC_REMOVE_BACKEND_FROM_PEERS]

   CHAOS

   --chaos-enabled                                  whether bproxy should be injecting artificial error conditions (default: false) [$BPROXY_CHAOS_ENABLED]
   --chaos-injected-http-error-probability percent  probability in percent at which to randomly inject http errors into proxied responses (default: 20) [$BPROXY_CHAOS_INJECTED_HTTP_ERROR_PROBABILITY]
   --chaos-injected-jrpc-error-probability percent  probability in percent at which to randomly inject jrpc errors into proxied responses (default: 20) [$BPROXY_CHAOS_INJECTED_JRPC_ERROR_PROBABILITY]
   --chaos-max-injected-latency latency             max latency to randomly enforce on every proxied response (default: 0s) [$BPROXY_CHAOS_MAX_INJECTED_LATENCY]
   --chaos-min-injected-latency latency             min latency to enforce on every proxied response (default: 50ms) [$BPROXY_CHAOS_MIN_INJECTED_LATENCY]

   METRICS

   --metrics-listen-address host:port  host:port for metrics server (default: "0.0.0.0:6785") [$BPROXY_METRICS_LISTEN_ADDRESS]

   RPC

   --rpc-backend url                      url of backend rpc (default: "http://127.0.0.1:18545") [$BPROXY_RPC_BACKEND]
   --rpc-enabled                          enable rpc proxy (default: false) [$BPROXY_RPC_ENABLED]
   --rpc-listen-address host:port         host:port for rpc proxy (default: "0.0.0.0:8545") [$BPROXY_RPC_LISTEN_ADDRESS]
   --rpc-log-requests                     whether to log rpc requests (default: false) [$BPROXY_RPC_LOG_REQUESTS]
   --rpc-log-responses                    whether to log responses to proxied/mirrored rpc requests (default: false) [$BPROXY_RPC_LOG_RESPONSES]
   --rpc-max-request-size megabytes       maximum rpc request payload size in megabytes (default: 15) [$BPROXY_RPC_MAX_REQUEST_SIZE]
   --rpc-max-response-size megabytes      maximum rpc response payload size in megabytes (default: 160) [$BPROXY_RPC_MAX_RESPONSE_SIZE]
   --rpc-peers urls [ --rpc-peers urls ]  list of urls with rpc peers to mirror the requests to [$BPROXY_RPC_PEERS]
   --rpc-remove-backend-from-peers        remove rpc backend from peers (default: false) [$BPROXY_RPC_REMOVE_BACKEND_FROM_PEERS]
```
