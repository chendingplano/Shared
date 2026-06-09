package observability

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

type ShutdownFunc func(context.Context) error

func Init(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	if cfg.ServiceName == "" {
		return nil, errors.New("observability service name is required")
	}
	if cfg.OTLPEndpoint == "" {
		return nil, errors.New("observability OTLP endpoint is required")
	}

	res := resource.NewWithAttributes("",
		attribute.String("service.name", cfg.ServiceName),
		attribute.String("deployment.environment.name", cfg.Environment),
	)

	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.OTLPEndpoint),
		otlptracehttp.WithHeaders(cfg.Headers),
		otlptracehttp.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, err
	}

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(cfg.OTLPEndpoint),
		otlpmetrichttp.WithHeaders(cfg.Headers),
		otlpmetrichttp.WithTimeout(5*time.Second),
	)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(10*time.Second))),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(shutdownCtx context.Context) error {
		var shutdownErr error
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
		return shutdownErr
	}, nil
}
