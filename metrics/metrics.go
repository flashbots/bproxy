package metrics

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	otelapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

const (
	metricsNamespace = "bproxy"
)

var (
	meter otelapi.Meter
)

func Setup(
	ctx context.Context,
	observe func(ctx context.Context, o otelapi.Observer) error,
) error {
	for _, setup := range []func(context.Context) error{
		setupMeter, // must come first
		setupInfo,
		setupRequestSize,
		setupResponseSize,
		setupLatencyAuthrpcJwt,
		setupLatencyBackend,
		setupLatencyTotal,
		setupProxySuccessCount,
		setupProxyFailureCount,
		setupProxyFakeCount,
		setupMirrorSuccessCount,
		setupMirrorFailureCount,
		setupMirrorDropCount,
		setupFrontendConnectionsCount,
		setupFrontendDrainingConnectionsCount,
		setupTLSValidNotAfter,
		setupTLSValidNotBefore,
		setupLateFCUCount,
	} {
		if err := setup(ctx); err != nil {
			return err
		}
	}

	if _, err := meter.RegisterCallback(observe,
		FrontendConnectionsCount,
		FrontendDrainingConnectionsCount,
		TLSValidNotAfter,
		TLSValidNotBefore,
	); err != nil {
		return err
	}

	if _, err := LatencyAuthrpcJwt.registerCallback(meter); err != nil {
		return err
	}

	return nil
}

func setupMeter(ctx context.Context) error {
	res, err := resource.New(ctx)
	if err != nil {
		return err
	}

	exporter, err := prometheus.New(
		prometheus.WithNamespace(metricsNamespace),
		prometheus.WithoutScopeInfo(),
	)
	if err != nil {
		return err
	}

	provider := metric.NewMeterProvider(
		metric.WithReader(exporter),
		metric.WithResource(res),
	)

	meter = provider.Meter(metricsNamespace)

	return nil
}

func setupInfo(ctx context.Context) error {
	m, err := meter.Int64Counter("info",
		otelapi.WithDescription("target info metadata (see the labels)"),
	)
	if err != nil {
		return err
	}
	Info = m
	return nil
}

func setupRequestSize(ctx context.Context) error {
	m, err := meter.Int64Histogram("request_size",
		otelapi.WithDescription("sizes of incoming requests"),
		otelapi.WithExplicitBucketBoundaries(0, 1, 16, 256, 4096, 65536, 1048576, 16777216, 268435456, 68719476736),
		otelapi.WithUnit("By"),
	)
	if err != nil {
		return err
	}
	RequestSize = m
	return nil
}

func setupResponseSize(ctx context.Context) error {
	m, err := meter.Int64Histogram("response_size",
		otelapi.WithDescription("sizes of sent responses"),
		otelapi.WithExplicitBucketBoundaries(0, 1, 16, 256, 4096, 65536, 1048576, 16777216, 268435456, 68719476736),
		otelapi.WithUnit("By"),
	)
	if err != nil {
		return err
	}
	ResponseSize = m
	return nil
}

func setupLatencyAuthrpcJwt(ctx context.Context) error {
	m, err := NewInt64Candlestick("latency_authrpc_jwt", "time since authrpc jwt token issued", "ms",
		attribute.KeyValue{Key: "proxy", Value: attribute.StringValue("bproxy-authrpc")},
	)
	if err != nil {
		return err
	}
	LatencyAuthrpcJwt = m
	return nil
}

func setupLatencyBackend(ctx context.Context) error {
	m, err := meter.Int64Histogram("latency_backend",
		otelapi.WithDescription("latency of backend responses"),
		otelapi.WithExplicitBucketBoundaries(0, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024),
		otelapi.WithUnit("ms"),
	)
	if err != nil {
		return err
	}
	LatencyBackend = m
	return nil
}

func setupLatencyTotal(ctx context.Context) error {
	m, err := meter.Int64Histogram("latency_total",
		otelapi.WithDescription("total latency"),
		otelapi.WithUnit("ms"),
		otelapi.WithExplicitBucketBoundaries(0, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024),
	)
	if err != nil {
		return err
	}
	LatencyTotal = m
	return nil
}

func setupProxySuccessCount(ctx context.Context) error {
	m, err := meter.Int64Counter("proxy_success_count",
		otelapi.WithDescription("count of successfully proxied requests"),
	)
	if err != nil {
		return err
	}
	ProxySuccessCount = m
	return nil
}

func setupProxyFailureCount(ctx context.Context) error {
	m, err := meter.Int64Counter("proxy_failure_count",
		otelapi.WithDescription("count of failures to proxy the request"),
	)
	if err != nil {
		return err
	}
	ProxyFailureCount = m
	return nil
}

func setupProxyFakeCount(ctx context.Context) error {
	m, err := meter.Int64Counter("proxy_fake_count",
		otelapi.WithDescription("count of faked responses to the non-proxied requests"),
	)
	if err != nil {
		return err
	}
	ProxyFakeCount = m
	return nil
}

func setupMirrorSuccessCount(ctx context.Context) error {
	m, err := meter.Int64Counter("mirror_success_count",
		otelapi.WithDescription("count of successfully mirrored requests"),
	)
	if err != nil {
		return err
	}
	MirrorSuccessCount = m
	return nil
}

func setupMirrorFailureCount(ctx context.Context) error {
	m, err := meter.Int64Counter("mirror_failure_count",
		otelapi.WithDescription("count of failures to mirror the request"),
	)
	if err != nil {
		return err
	}
	MirrorFailureCount = m
	return nil
}

func setupMirrorDropCount(ctx context.Context) error {
	m, err := meter.Int64Counter("mirror_drop_count",
		otelapi.WithDescription("count of dropped mirror requests"),
	)
	if err != nil {
		return err
	}
	MirrorDropCount = m
	return nil
}

func setupFrontendConnectionsCount(ctx context.Context) error {
	m, err := meter.Int64ObservableGauge("frontend_connections_count",
		otelapi.WithDescription("count of open frontend connections"),
	)
	if err != nil {
		return err
	}
	FrontendConnectionsCount = m
	return nil
}

func setupFrontendDrainingConnectionsCount(ctx context.Context) error {
	m, err := meter.Int64ObservableGauge("frontend_draining_connections_count",
		otelapi.WithDescription("count of draining frontend connections"),
	)
	if err != nil {
		return err
	}
	FrontendDrainingConnectionsCount = m
	return nil
}

func setupTLSValidNotAfter(ctx context.Context) error {
	m, err := meter.Int64ObservableGauge("tls_valid_not_after",
		otelapi.WithDescription("not-after timestamp of tls certificate"),
	)
	if err != nil {
		return err
	}
	TLSValidNotAfter = m
	return nil
}

func setupTLSValidNotBefore(ctx context.Context) error {
	m, err := meter.Int64ObservableGauge("tls_valid_not_before",
		otelapi.WithDescription("not-before timestamp of tls certificate"),
	)
	if err != nil {
		return err
	}
	TLSValidNotBefore = m
	return nil
}

func setupLateFCUCount(ctx context.Context) error {
	m, err := meter.Int64Counter("late_fcu_count",
		otelapi.WithDescription("count of late fcu messages"),
	)
	if err != nil {
		return err
	}
	LateFCUCount = m
	return nil
}
