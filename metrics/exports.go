package metrics

import (
	otelapi "go.opentelemetry.io/otel/metric"
)

var (
	ProxySuccessCount otelapi.Int64Counter
	ProxyFailureCount otelapi.Int64Counter
	ProxyFakeCount    otelapi.Int64Counter

	MirrorSuccessCount otelapi.Int64Counter
	MirrorFailureCount otelapi.Int64Counter

	FrontendConnectionsCount otelapi.Int64ObservableGauge
)
