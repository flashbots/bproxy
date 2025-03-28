package proxy

import (
	"github.com/valyala/fasthttp"

	ethcommon "github.com/ethereum/go-ethereum/common"
)

type triagedRequest struct {
	proxy  bool
	mirror bool

	jrpcMethod string
	jrpcID     uint64

	response *fasthttp.Response

	tx *triagedRequestTx
}

type triagedRequestTx struct {
	From  *ethcommon.Address
	To    *ethcommon.Address
	Hash  ethcommon.Hash
	Nonce uint64
}
