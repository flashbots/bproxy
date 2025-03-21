package proxy

import "github.com/valyala/fasthttp"

type triagedRequest struct {
	proxy  bool
	mirror bool

	jrpcMethod string
	jrpcID     uint64
	txHash     string

	response *fasthttp.Response
}
