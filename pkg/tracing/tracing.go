// Package tracing provides shared OpenTelemetry tracing functionality for Go microservices.
// It offers tracer initialization with OTLP and stdout exporters, along with middleware
// for HTTP and gRPC services, and helper functions for common tracing operations.
package tracing

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// ExporterType defines the type of trace exporter to use.
type ExporterType string

const (
	// ExporterOTLP uses the OTLP gRPC exporter for production use.
	ExporterOTLP ExporterType = "otlp"

	// ExporterStdout uses the stdout exporter for development/debugging.
	ExporterStdout ExporterType = "stdout"

	// ExporterNone disables tracing (useful for testing).
	ExporterNone ExporterType = "none"
)

// Config holds configuration for initializing the tracer.
type Config struct {
	// ServiceName identifies the service in traces.
	ServiceName string

	// Environment is the deployment environment (e.g., "development", "production").
	Environment string

	// Endpoint is the OTLP collector endpoint (e.g., "localhost:4317").
	// Only used when Exporter is ExporterOTLP.
	Endpoint string

	// SampleRate is the fraction of traces to sample (0.0 to 1.0).
	// 1.0 means all traces are sampled.
	SampleRate float64

	// Exporter specifies which exporter to use.
	Exporter ExporterType

	// StdoutWriter is the writer for stdout exporter output.
	// Defaults to os.Stdout if nil.
	StdoutWriter io.Writer

	// Insecure disables TLS for the OTLP connection.
	// Should only be true for development environments.
	Insecure bool
}

// DefaultConfig returns a Config with sensible defaults for development.
func DefaultConfig() *Config {
	return &Config{
		ServiceName:  "unknown",
		Environment:  "development",
		Endpoint:     "localhost:4317",
		SampleRate:   1.0,
		Exporter:     ExporterStdout,
		StdoutWriter: os.Stdout,
		Insecure:     true,
	}
}

// ShutdownFunc is a function that shuts down the tracer provider.
type ShutdownFunc func(context.Context) error

// InitTracer initializes the OpenTelemetry tracer with the given configuration.
// It returns a shutdown function that should be called when the application exits.
// The shutdown function flushes any remaining spans and releases resources.
func InitTracer(cfg *Config) (ShutdownFunc, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Create resource with service information
	res, err := createResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on configuration
	exporter, err := createExporter(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create sampler
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create tracer provider
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}

	if exporter != nil {
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	tp := sdktrace.NewTracerProvider(opts...)

	// Set as global tracer provider
	otel.SetTracerProvider(tp)

	// Set up propagation (W3C Trace Context)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return shutdown function
	return func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}, nil
}

// createResource creates an OpenTelemetry resource with service information.
func createResource(cfg *Config) (*resource.Resource, error) {
	return resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
			attribute.String("service.version", "1.0.0"),
		),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
	)
}

// createExporter creates a trace exporter based on configuration.
func createExporter(cfg *Config) (sdktrace.SpanExporter, error) {
	switch cfg.Exporter {
	case ExporterOTLP:
		return createOTLPExporter(cfg)
	case ExporterStdout:
		return createStdoutExporter(cfg)
	case ExporterNone:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown exporter type: %s", cfg.Exporter)
	}
}

// createOTLPExporter creates an OTLP gRPC exporter.
func createOTLPExporter(cfg *Config) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	return exporter, nil
}

// createStdoutExporter creates a stdout exporter for development.
func createStdoutExporter(cfg *Config) (sdktrace.SpanExporter, error) {
	writer := cfg.StdoutWriter
	if writer == nil {
		writer = os.Stdout
	}

	exporter, err := stdouttrace.New(
		stdouttrace.WithWriter(writer),
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
	}

	return exporter, nil
}

// Tracer returns a named tracer from the global tracer provider.
// Use this to get a tracer for your package or component.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// TracerWithVersion returns a named tracer with a version from the global provider.
func TracerWithVersion(name, version string) trace.Tracer {
	return otel.Tracer(name, trace.WithInstrumentationVersion(version))
}
