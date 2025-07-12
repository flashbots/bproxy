package metrics

import (
	otelapi "go.opentelemetry.io/otel/metric"
)

var (
	FrontendConnectionsActiveCount      otelapi.Int64ObservableGauge
	FrontendConnectionsClosedCount      otelapi.Int64Counter
	FrontendConnectionsDrainingCount    otelapi.Int64ObservableGauge
	FrontendConnectionsEstablishedCount otelapi.Int64Counter

	Info otelapi.Int64Counter

	LateFCUCount otelapi.Int64Counter

	LatencyAuthrpcJwt *Int64Candlestick
	LatencyBackend    otelapi.Int64Histogram
	LatencyTotal      otelapi.Int64Histogram

	MirrorSuccessCount otelapi.Int64Counter
	MirrorFailureCount otelapi.Int64Counter
	MirrorDropCount    otelapi.Int64Counter

	ProxySuccessCount otelapi.Int64Counter
	ProxyFailureCount otelapi.Int64Counter
	ProxyFakeCount    otelapi.Int64Counter

	RequestSize  otelapi.Int64Histogram
	ResponseSize otelapi.Int64Histogram

	TLSValidNotAfter  otelapi.Int64ObservableGauge
	TLSValidNotBefore otelapi.Int64ObservableGauge
)
