# bproxy

L2 builder proxy that proxies RPC and Authenticated RPC calls to the builder,
and mirrors `eth_sendRawTransaction` (on rpc) and `engine_forkchoiceUpdatedV3`
with extra attributes (on authrpc) to its peers.

## Usage

```text
bproxy serve [command options]

OPTIONS:
   AUTHRPC

   --authrpc-backend url                          url of backend authrpc (default: "http://127.0.0.1:18545") [$BPROXY_AUTHRPC_BACKEND]
   --authrpc-listen-address host:port             host:port for authrpc proxy (default: "0.0.0.0:8545") [$BPROXY_AUTHRPC_LISTEN_ADDRESS]
   --authrpc-peers urls [ --authrpc-peers urls ]  list of urls with authrpc peers to mirror the requests to [$BPROXY_AUTHRPC_PEERS]
   --authrpc-remove-backend-from-peers            remove backend from peers (default: false) [$BPROXY_AUTHRPC_REMOVE_BACKEND_FROM_PEERS]

   RPC

   --rpc-backend url                      url of backend rpc (default: "http://127.0.0.1:18551") [$BPROXY_RPC_BACKEND]
   --rpc-listen-address host:port         host:port for rpc proxy (default: "0.0.0.0:8551") [$BPROXY_RPC_LISTEN_ADDRESS]
   --rpc-peers urls [ --rpc-peers urls ]  list of urls with rpc peers to mirror the requests to [$BPROXY_RPC_PEERS]
   --rpc-remove-backend-from-peers        remove backend from peers (default: false) [$BPROXY_RPC_REMOVE_BACKEND_FROM_PEERS]
```
