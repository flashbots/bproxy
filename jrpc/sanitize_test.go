package jrpc

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func createSignedTx(t *testing.T, nonce uint64) (rawTx, txHash string) {
	t.Helper()
	key, _ := crypto.GenerateKey()
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(1),
		Nonce:     nonce,
		GasTipCap: big.NewInt(1e9),
		GasFeeCap: big.NewInt(100e9),
		Gas:       21000,
		To:        &common.Address{},
	})
	signed, _ := types.SignTx(tx, types.NewLondonSigner(big.NewInt(1)), key)
	bytes, _ := signed.MarshalBinary()
	return hexutil.Encode(bytes), signed.Hash().Hex()
}

func TestSanitize_ReplacesWithHash(t *testing.T) {
	rawTx, expectedHash := createSignedTx(t, 0)

	methods := []struct {
		name  string
		input string
		path  func(any) string
	}{
		{
			name:  "eth_sendRawTransaction",
			input: `{"method":"eth_sendRawTransaction","params":["` + rawTx + `"]}`,
			path:  func(m any) string { return m.(map[string]any)["params"].([]any)[0].(string) },
		},
		{
			name:  "engine_newPayloadV4",
			input: `{"method":"engine_newPayloadV4","params":[{"transactions":["` + rawTx + `"]}]}`,
			path: func(m any) string {
				return m.(map[string]any)["params"].([]any)[0].(map[string]any)["transactions"].([]any)[0].(string)
			},
		},
		{
			name:  "engine_forkchoiceUpdatedV3",
			input: `{"method":"engine_forkchoiceUpdatedV3","params":[{},{"transactions":["` + rawTx + `"]}]}`,
			path: func(m any) string {
				return m.(map[string]any)["params"].([]any)[1].(map[string]any)["transactions"].([]any)[0].(string)
			},
		},
		{
			name:  "eth_sendBundle",
			input: `{"method":"eth_sendBundle","params":[{"txs":["` + rawTx + `"]}]}`,
			path: func(m any) string {
				return m.(map[string]any)["params"].([]any)[0].(map[string]any)["txs"].([]any)[0].(string)
			},
		},
		{
			name:  "engine_getPayloadV4 response",
			input: `{"result":{"executionPayload":{"transactions":["` + rawTx + `"]}}}`,
			path: func(m any) string {
				return m.(map[string]any)["result"].(map[string]any)["executionPayload"].(map[string]any)["transactions"].([]any)[0].(string)
			},
		},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			var msg any
			json.Unmarshal([]byte(m.input), &msg)
			Sanitize(msg)
			if got := m.path(msg); got != expectedHash {
				t.Errorf("got %s, want %s", got, expectedHash)
			}
		})
	}
}

func TestSanitize_InvalidTxShowsError(t *testing.T) {
	input := `{"method":"eth_sendRawTransaction","params":["0xdeadbeef"]}`
	var msg any
	json.Unmarshal([]byte(input), &msg)
	Sanitize(msg)

	got := msg.(map[string]any)["params"].([]any)[0].(string)
	if !strings.HasPrefix(got, "[error") {
		t.Errorf("expected error message, got %s", got)
	}
}

func TestSanitize_BatchRequest(t *testing.T) {
	rawTx, expectedHash := createSignedTx(t, 0)
	input := `[{"method":"eth_sendRawTransaction","params":["` + rawTx + `"]},{"method":"eth_blockNumber","params":[]}]`

	var msg any
	json.Unmarshal([]byte(input), &msg)
	Sanitize(msg)

	batch := msg.([]any)
	got := batch[0].(map[string]any)["params"].([]any)[0].(string)
	if got != expectedHash {
		t.Errorf("got %s, want %s", got, expectedHash)
	}
}

func TestSanitize_PreservesOtherFields(t *testing.T) {
	rawTx, expectedHash := createSignedTx(t, 0)
	input := `{"method":"engine_newPayloadV4","params":[{"transactions":["` + rawTx + `"],"blockNumber":"0x1"}]}`

	var msg any
	json.Unmarshal([]byte(input), &msg)
	Sanitize(msg)

	payload := msg.(map[string]any)["params"].([]any)[0].(map[string]any)
	if payload["transactions"].([]any)[0] != expectedHash {
		t.Error("transaction should be replaced with hash")
	}
	if payload["blockNumber"] != "0x1" {
		t.Error("blockNumber should be preserved")
	}
}

func TestSanitize_UnrelatedMethodUnchanged(t *testing.T) {
	input := `{"method":"eth_blockNumber","params":["0x1"]}`
	var msg any
	json.Unmarshal([]byte(input), &msg)
	Sanitize(msg)

	if msg.(map[string]any)["params"].([]any)[0] != "0x1" {
		t.Error("unrelated method params should be unchanged")
	}
}
