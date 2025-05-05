package proxy

import (
	"sync"
	"time"

	"github.com/flashbots/bproxy/triaged"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type jobProxy struct {
	tsReqReceived time.Time

	log *zap.Logger
	wg  *sync.WaitGroup

	req    *fasthttp.Request
	res    *fasthttp.Response
	triage *triaged.Request

	proxy func(req *fasthttp.Request, res *fasthttp.Response) error

	jrpcMethodForMetrics string
}

type jobMirror struct {
	log *zap.Logger

	req *fasthttp.Request
	res *fasthttp.Response

	jrpcMethodForMetrics string
}
