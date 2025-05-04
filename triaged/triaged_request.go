package triaged

import (
	"github.com/valyala/fasthttp"
)

type Request struct {
	Proxy  bool
	Mirror bool

	JrpcMethod string
	JrpcID     string

	Response *fasthttp.Response

	Transactions RequestTransactions
}
