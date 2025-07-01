package metrics

import (
	otelapi "go.opentelemetry.io/otel/metric"
)

var (
	Info otelapi.Int64Counter

	RequestSize  otelapi.Int64Histogram
	ResponseSize otelapi.Int64Histogram

	LatencyAuthrpcJwt *Int64Candlestick
	LatencyBackend    otelapi.Int64Histogram
	LatencyTotal      otelapi.Int64Histogram

	ProxySuccessCount otelapi.Int64Counter
	ProxyFailureCount otelapi.Int64Counter
	ProxyFakeCount    otelapi.Int64Counter

	MirrorSuccessCount otelapi.Int64Counter
	MirrorFailureCount otelapi.Int64Counter
	MirrorDropCount    otelapi.Int64Counter

	FrontendConnectionsCount         otelapi.Int64ObservableGauge
	FrontendDrainingConnectionsCount otelapi.Int64ObservableGauge

	TLSValidNotAfter  otelapi.Int64ObservableGauge
	TLSValidNotBefore otelapi.Int64ObservableGauge

	LateFCUCount otelapi.Int64Counter
)
