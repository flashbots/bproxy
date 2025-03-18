package types

type JrpcCall struct {
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      uint64 `json:"id"`
}
