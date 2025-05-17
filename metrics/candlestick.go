package metrics

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otelapi "go.opentelemetry.io/otel/metric"
)

type Int64Candlestick struct {
	gauge otelapi.Int64ObservableGauge

	attrOpen, attrClose []attribute.KeyValue
	attrHigh, attrLow   []attribute.KeyValue
	attrVolume          []attribute.KeyValue

	open, close int64
	high, low   int64
	volume      int64

	mx sync.Mutex
}

func NewInt64Candlestick(
	name, description, uom string,
	attributes ...attribute.KeyValue,
) (c *Int64Candlestick, err error) {
	c = &Int64Candlestick{
		attrOpen:   append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("open")}),
		attrClose:  append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("close")}),
		attrHigh:   append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("high")}),
		attrLow:    append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("low")}),
		attrVolume: append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("volume")}),
	}

	options := []otelapi.Int64ObservableGaugeOption{
		otelapi.WithDescription(description),
	}
	if uom != "" {
		options = append(options, otelapi.WithUnit(uom))
	}

	if c.gauge, err = meter.Int64ObservableGauge(name, options...); err != nil {
		return nil, err
	}

	return
}

func (c *Int64Candlestick) registerCallback(m otelapi.Meter) (otelapi.Registration, error) {
	return m.RegisterCallback(c.observe, c.gauge)
}

func (c *Int64Candlestick) observe(ctx context.Context, o otelapi.Observer) error {
	c.mx.Lock()
	defer c.mx.Unlock()

	if c.volume == 0 {
		return nil
	}

	c.close = c.close / c.volume

	o.ObserveInt64(c.gauge, c.open, otelapi.WithAttributes(c.attrOpen...))
	o.ObserveInt64(c.gauge, c.close, otelapi.WithAttributes(c.attrClose...))
	o.ObserveInt64(c.gauge, c.high, otelapi.WithAttributes(c.attrHigh...))
	o.ObserveInt64(c.gauge, c.low, otelapi.WithAttributes(c.attrLow...))
	o.ObserveInt64(c.gauge, c.volume, otelapi.WithAttributes(c.attrVolume...))

	c.open = c.close
	c.close = 0
	c.high = 0
	c.low = 0
	c.volume = 0

	return nil
}

func (c *Int64Candlestick) Record(ctx context.Context, value int64) {
	c.mx.Lock()
	defer c.mx.Unlock()

	c.close += value

	if c.volume == 0 || value > c.high {
		c.high = value
	}

	if c.volume == 0 || value < c.low {
		c.low = value
	}

	c.volume++
}
