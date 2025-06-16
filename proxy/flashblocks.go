package proxy

import (
	"context"

	"github.com/flashbots/bproxy/config"
	otelapi "go.opentelemetry.io/otel/metric"
)

type Flashblocks struct {
	proxy *Websocket
}

func NewFlashblocks(
	cfg *config.Flashblocks,
) (*Flashblocks, error) {
	p, err := newWebsocket(&websocketConfig{
		name:  "bproxy-flashblocks",
		proxy: cfg.WebsocketProxy,
	})
	if err != nil {
		return nil, err
	}

	flashblocks := &Flashblocks{
		proxy: p,
	}

	return flashblocks, nil
}

func (p *Flashblocks) Run(ctx context.Context, failure chan<- error) {
	if p == nil {
		return
	}
	p.proxy.Run(ctx, failure)
}

func (p *Flashblocks) ResetConnections() {
	if p == nil {
		return
	}
	p.proxy.ResetConnections()
}

func (p *Flashblocks) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}
	return p.proxy.Observe(ctx, o)
}

func (p *Flashblocks) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.proxy.Stop(ctx)
}
