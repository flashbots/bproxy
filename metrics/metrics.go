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
		setupProxySuccessCount,
		setupProxyFailureCount,
		setupMirrorSuccessCount,
		setupMirrorFailureCount,
		setupFrontendConnectionsCount,
	} {
		if err := setup(ctx); err != nil {
			return err
		}
	}

	_, err := meter.RegisterCallback(observe,
		FrontendConnectionsCount,
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
