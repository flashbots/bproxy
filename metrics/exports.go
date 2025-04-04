package metrics

import (
	otelapi "go.opentelemetry.io/otel/metric"
)

var (
	RequestSize  otelapi.Int64Histogram
	ResponseSize otelapi.Int64Histogram

	ProxySuccessCount otelapi.Int64Counter
	ProxyFailureCount otelapi.Int64Counter
	ProxyFakeCount    otelapi.Int64Counter

	MirrorSuccessCount otelapi.Int64Counter
	MirrorFailureCount otelapi.Int64Counter

	FrontendConnectionsCount otelapi.Int64ObservableGauge

	TLSValidNotAfter  otelapi.Int64ObservableGauge
	TLSValidNotBefore otelapi.Int64ObservableGauge
)
