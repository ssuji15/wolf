package job_tracer

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

func InitTracer(ctx context.Context, serviceName, collector string) func() {

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(collector),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("failed to create OTLP exporter: %v", err)
	}

	mexporter, err := otlpmetrichttp.New(
		ctx,
		otlpmetrichttp.WithInsecure(),
		otlpmetrichttp.WithEndpoint(collector),
	)
	if err != nil {
		log.Fatalf("failed to create metric exporter: %v", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
			semconv.DeploymentEnvironment("dev"),
		),
	)
	if err != nil {
		log.Fatalf("failed to create otel resource: %v", err)
	}

	meterProvider := metric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(mexporter)), metric.WithResource(res))
	otel.SetMeterProvider(meterProvider)

	batchOptions := []sdktrace.BatchSpanProcessorOption{
		sdktrace.WithBatchTimeout(500 * time.Millisecond),
		sdktrace.WithExportTimeout(2 * time.Second),

		sdktrace.WithMaxQueueSize(2048),
		// sdktrace.WithMaxExportBatchSize(50), // Max spans per export call (default 512)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, batchOptions...),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func() {
		_ = tp.Shutdown(ctx)
	}
}

func GetTracer() trace.Tracer {
	return otel.Tracer("jobtracer")
}
