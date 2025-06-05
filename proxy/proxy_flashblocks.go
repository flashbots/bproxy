package proxy

import (
	"context"

	"github.com/flashbots/bproxy/config"
	otelapi "go.opentelemetry.io/otel/metric"
)

type FlashblocksProxy struct {
	proxy *Websocket
}

func NewFlashblocksProxy(
	cfg *config.FlashblocksProxy,
) (*FlashblocksProxy, error) {
	p, err := newWebsocket(&websocketConfig{
		name:  "bproxy-flashblocks",
		proxy: cfg.WebsocketProxy,
	})
	if err != nil {
		return nil, err
	}

	flashblocksProxy := &FlashblocksProxy{
		proxy: p,
	}

	return flashblocksProxy, nil
}

func (p *FlashblocksProxy) Run(ctx context.Context, failure chan<- error) {
	if p == nil {
		return
	}
	p.proxy.Run(ctx, failure)
}

func (p *FlashblocksProxy) ResetConnections() {
	if p == nil {
		return
	}
	p.proxy.ResetConnections()
}

func (p *FlashblocksProxy) Observe(ctx context.Context, o otelapi.Observer) error {
	if p == nil {
		return nil
	}
	return p.proxy.Observe(ctx, o)
}

func (p *FlashblocksProxy) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.proxy.Stop(ctx)
}
