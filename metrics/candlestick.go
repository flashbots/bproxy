package metrics

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otelapi "go.opentelemetry.io/otel/metric"
)

type Int64Candlestick struct {
	metric otelapi.Int64ObservableGauge

	attrOpen, attrClose []attribute.KeyValue
	attrHigh, attrLow   []attribute.KeyValue

	open, close, count int64
	high, low          int64

	mx sync.Mutex
}

func NewInt64Candlestick(
	name, description, uom string,
	attributes ...attribute.KeyValue,
) (*Int64Candlestick, error) {
	var err error

	c := &Int64Candlestick{
		attrOpen:  append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("open")}),
		attrClose: append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("close")}),
		attrHigh:  append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("high")}),
		attrLow:   append(attributes, attribute.KeyValue{Key: "type", Value: attribute.StringValue("low")}),
	}

	options := []otelapi.Int64ObservableGaugeOption{
		otelapi.WithDescription(description),
	}

	if uom != "" {
		options = append(options, otelapi.WithUnit(uom))
	}

	if c.metric, err = meter.Int64ObservableGauge(name, options...); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Int64Candlestick) registerCallback(m otelapi.Meter) (otelapi.Registration, error) {
	return m.RegisterCallback(c.observe, c.metric)
}

func (c *Int64Candlestick) observe(ctx context.Context, o otelapi.Observer) error {
	c.mx.Lock()
	defer c.mx.Unlock()

	if c.count == 0 {
		return nil
	}

	c.close = c.close / c.count

	o.ObserveInt64(c.metric, c.open, otelapi.WithAttributes(c.attrOpen...))
	o.ObserveInt64(c.metric, c.close, otelapi.WithAttributes(c.attrClose...))
	o.ObserveInt64(c.metric, c.high, otelapi.WithAttributes(c.attrHigh...))
	o.ObserveInt64(c.metric, c.low, otelapi.WithAttributes(c.attrLow...))

	c.open = c.close
	c.close = 0
	c.count = 0
	c.high = 0
	c.low = 0

	return nil
}

func (c *Int64Candlestick) Record(ctx context.Context, value int64) {
	c.mx.Lock()
	defer c.mx.Unlock()

	if c.count == 0 {
		c.high = value
		c.low = value
	}

	if value > c.high {
		c.high = value
	}

	if value < c.low {
		c.low = value
	}

	c.close += value
	c.count++
}
