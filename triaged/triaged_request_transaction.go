package triaged

import (
	"go.uber.org/zap/zapcore"

	ethcommon "github.com/ethereum/go-ethereum/common"
)

type RequestTransactions []RequestTransaction

type RequestTransaction struct {
	From  *ethcommon.Address
	To    *ethcommon.Address
	Hash  ethcommon.Hash
	Nonce uint64
}

func (tx RequestTransaction) MarshalLogObject(e zapcore.ObjectEncoder) error {
	if tx.From != nil {
		e.AddString("from", tx.From.String())
	}
	if tx.To != nil {
		e.AddString("to", tx.To.String())
	}
	e.AddString("hash", tx.Hash.String())
	e.AddUint64("nonce", tx.Nonce)
	return nil
}

func (txs RequestTransactions) MarshalLogArray(e zapcore.ArrayEncoder) error {
	for _, tx := range txs {
		if err := e.AppendObject(tx); err != nil {
			return err
		}
	}
	return nil
}
