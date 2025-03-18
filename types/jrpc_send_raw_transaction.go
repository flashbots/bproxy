package types

import "encoding/json"

type EthSendRawTransaction struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	ID      uint64          `json:"id"`
	Params  json.RawMessage `json:"params,omitempty"`
}
