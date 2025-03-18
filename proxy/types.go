package proxy

import "github.com/valyala/fasthttp"

type Config struct {
	Name          string
	ListenAddress string

	BackendURI string
	PeerURIs   []string

	LogRequests  bool
	LogResponses bool
}

type triagedRequest struct {
	proxy  bool
	mirror bool

	jrpcMethod string
	jrpcID     uint64
	txHash     string

	response *fasthttp.Response
}
