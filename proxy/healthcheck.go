package proxy

import (
	"context"
	"sync"
	"time"

	"github.com/flashbots/bproxy/data"
	"github.com/flashbots/bproxy/logutils"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type healthcheck struct {
	interval           time.Duration
	name               string
	thresholdHealthy   int
	thresholdUnhealthy int

	handleUnhealthy func(ctx context.Context)

	depth    int
	statuses *data.RingBuffer[bool]
	target   *fasthttp.Client
	ticker   *time.Ticker
	uri      *fasthttp.URI

	isHealthy bool
	mx        sync.Mutex
}

func newHealthcheck(
	name, url string,
	interval time.Duration,
	thresholdHealthy, thresholdUnhealthy int,
	handleUnhealthy func(context.Context),
) (*healthcheck, error) {
	uri := fasthttp.AcquireURI()
	if err := uri.Parse(nil, []byte(url)); err != nil {
		fasthttp.ReleaseURI(uri)
		return nil, err
	}

	return &healthcheck{
		depth:              max(thresholdHealthy, thresholdUnhealthy),
		handleUnhealthy:    handleUnhealthy,
		interval:           interval,
		isHealthy:          true,
		name:               name,
		statuses:           data.NewRingBuffer[bool](max(thresholdHealthy, thresholdUnhealthy)),
		thresholdHealthy:   thresholdHealthy,
		thresholdUnhealthy: thresholdUnhealthy,
		ticker:             time.NewTicker(interval),
		uri:                uri,

		target: &fasthttp.Client{
			MaxConnsPerHost:     1,
			MaxConnWaitTimeout:  interval / 2,
			MaxIdleConnDuration: 2 * interval,
			MaxResponseBodySize: 4096,
			Name:                name + "-healthcheck",
			ReadTimeout:         interval / 2,
			WriteTimeout:        interval / 2,
		},
	}, nil
}

func (h *healthcheck) IsHealthy() bool {
	h.mx.Lock()
	defer h.mx.Unlock()
	return h.isHealthy
}

func (h *healthcheck) run(ctx context.Context) {
	go func() {
		for {
			h.check(ctx, <-h.ticker.C)
		}
	}()
}

func (h *healthcheck) stop() {
	h.ticker.Stop()
	fasthttp.ReleaseURI(h.uri)
}

func (h *healthcheck) check(ctx context.Context, _ time.Time) {
	h.mx.Lock()
	defer h.mx.Unlock()

	l := logutils.LoggerFromContext(ctx).With(
		zap.String("healthcheck", h.name),
	)

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	req.SetURI(h.uri)
	req.Header.SetMethod("GET")
	req.SetTimeout(h.interval / 2)

	if err := h.target.Do(req, res); err == nil {
		switch res.StatusCode() {
		case fasthttp.StatusOK, fasthttp.StatusAccepted:
			h.statuses.Push(true)
		default:
			h.statuses.Push(false)
		}
	} else {
		l.Warn("Failed to query the healthcheck endpoint",
			zap.Error(err),
		)
		h.statuses.Push(false)
	}

	if h.statuses.Length() > h.depth {
		_, _ = h.statuses.Pop()
	}

	isHealthy := true
	for idx := h.depth - 1; idx >= h.depth-h.thresholdHealthy; idx-- {
		if s, ok := h.statuses.At(idx); ok {
			isHealthy = isHealthy && s
		}
	}

	isUnhealthy := true
	for idx := h.depth - 1; idx >= h.depth-h.thresholdUnhealthy; idx-- {
		if s, ok := h.statuses.At(idx); ok {
			isUnhealthy = isUnhealthy && !s
		}
	}

	if h.isHealthy && isUnhealthy {
		h.isHealthy = false
		l.Info("Backend became unhealthy")
	} else if !h.isHealthy && isHealthy {
		h.isHealthy = true
		l.Info("Backend is healthy again")
	}

	if !h.isHealthy {
		h.handleUnhealthy(ctx)
	}
}
