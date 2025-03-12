package types

type EthSendRawTransaction struct {
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      uint64 `json:"id"`
}

type EngineForkchoiceUpdatedV3 struct {
	Version string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	ID      uint64      `json:"id"`
	Params  []*struct{} `json:"params"`
}
