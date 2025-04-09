package metrics

import (
	"context"

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
		setupRequestSize,
		setupResponseSize,
		setupLatencyBackend,
		setupLatencyProxy,
		setupProxySuccessCount,
		setupProxyFailureCount,
		setupProxyFakeCount,
		setupMirrorSuccessCount,
		setupMirrorFailureCount,
		setupFrontendConnectionsCount,
		setupTLSValidNotAfter,
		setupTLSValidNotBefore,
	} {
		if err := setup(ctx); err != nil {
			return err
		}
	}

	_, err := meter.RegisterCallback(observe,
		FrontendConnectionsCount,
		TLSValidNotAfter,
		TLSValidNotBefore,
	)
	if err != nil {
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

func setupLatencyBackend(ctx context.Context) error {
	m, err := meter.Int64Histogram("latency_backend",
		otelapi.WithDescription("latency of backend responses"),
		otelapi.WithExplicitBucketBoundaries(0, 1, 2, 4, 8, 16, 32, 64, 128, 256),
		otelapi.WithUnit("ms"),
	)
	if err != nil {
		return err
	}
	LatencyBackend = m
	return nil
}

func setupLatencyProxy(ctx context.Context) error {
	m, err := meter.Int64Histogram("latency_proxy",
		otelapi.WithDescription("latency added by proxy"),
		otelapi.WithUnit("ms"),
		otelapi.WithExplicitBucketBoundaries(0, 1, 2, 3, 4, 6, 8, 12, 16, 24),
	)
	if err != nil {
		return err
	}
	LatencyProxy = m
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
