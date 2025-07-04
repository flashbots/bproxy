package flashblock

import "encoding/json"

type Flashblock struct {
	PayloadID string          `json:"payload_id"`
	Index     int             `json:"index"`
	Diff      *Diff           `json:"diff"`
	Metadata  json.RawMessage `json:"metadata"`
}

type Diff struct {
	StateRoot       string            `json:"state_root"`
	ReceiptsRoot    string            `json:"receipts_root"`
	LogsBloom       string            `json:"logs_bloom"`
	GasUsed         string            `json:"gas_used"`
	BlockHash       string            `json:"block_hash"`
	Transactions    []string          `json:"transactions"`
	Withdrawals     []json.RawMessage `json:"withdrawals"`
	WithdrawalsRoot string            `json:"withdrawals_root"`
}
