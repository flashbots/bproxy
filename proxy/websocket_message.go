package proxy

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/flashbots/bproxy/flashblock"
)

type websocketMessage struct {
	msgType int
	bytes   []byte
	ts      time.Time
}

func (m *websocketMessage) chaosMangle() {
	flashblock := flashblock.Flashblock{}
	if err := json.Unmarshal(m.bytes, &flashblock); err != nil {
		return
	}

	mangles := 2
	if flashblock.Diff != nil {
		mangles = 10
	}

	switch rand.IntN(mangles) {
	case 0: // payload id
		flashblock.PayloadID = mangleHex(flashblock.PayloadID)
	case 1: // index
		flashblock.Index = (flashblock.Index + 1) % 5
	case 2: // state root
		flashblock.Diff.StateRoot = mangleHex(flashblock.Diff.StateRoot)
	case 3: // receipts root
		flashblock.Diff.ReceiptsRoot = mangleHex(flashblock.Diff.ReceiptsRoot)
	case 4: // logs bloom
		flashblock.Diff.LogsBloom = mangleHex(flashblock.Diff.LogsBloom)
	case 5: // gas used
		flashblock.Diff.GasUsed = mangleHex(flashblock.Diff.GasUsed)
	case 6: // block hash
		flashblock.Diff.BlockHash = mangleHex(flashblock.Diff.BlockHash)
	case 7: // transactions
		if len(flashblock.Diff.Transactions) > 1 {
			tx := flashblock.Diff.Transactions[0]
			flashblock.Diff.Transactions = flashblock.Diff.Transactions[1:]
			flashblock.Diff.Transactions = append(flashblock.Diff.Transactions, tx)
		} else {
			flashblock.Diff.Transactions = []string{}
		}
	case 8: // withdrawals
		if len(flashblock.Diff.Withdrawals) > 1 {
			w := flashblock.Diff.Withdrawals[0]
			flashblock.Diff.Withdrawals = flashblock.Diff.Withdrawals[1:]
			flashblock.Diff.Withdrawals = append(flashblock.Diff.Withdrawals, w)
		} else {
			flashblock.Diff.Withdrawals = []json.RawMessage{}
		}
	case 9: // withdrawals root
		flashblock.Diff.WithdrawalsRoot = mangleHex(flashblock.Diff.WithdrawalsRoot)
	}
}

func mangleHex(hex string) string {
	prefix := ""
	if strings.HasPrefix(hex, "0x") {
		prefix = "0x"
	}
	hex = strings.TrimPrefix(hex, prefix)
	return prefix + hex[2:] + hex[:2]
}
