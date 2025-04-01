package proxy

import (
	"github.com/valyala/fasthttp"
	"go.uber.org/zap/zapcore"

	ethcommon "github.com/ethereum/go-ethereum/common"
)

type triagedRequest struct {
	proxy  bool
	mirror bool

	jrpcMethod string
	jrpcID     uint64

	response *fasthttp.Response

	transactions triagedRequestTxs
}

type triagedRequestTx struct {
	From  *ethcommon.Address
	To    *ethcommon.Address
	Hash  ethcommon.Hash
	Nonce uint64
}

func (tx triagedRequestTx) MarshalLogObject(e zapcore.ObjectEncoder) error {
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

type triagedRequestTxs []triagedRequestTx

func (txs triagedRequestTxs) MarshalLogArray(e zapcore.ArrayEncoder) error {
	for _, tx := range txs {
		if err := e.AppendObject(tx); err != nil {
			return err
		}
	}
	return nil
}
