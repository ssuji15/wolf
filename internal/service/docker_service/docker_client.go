package dockerservice

import (
	"github.com/moby/moby/client"
	trace "go.opentelemetry.io/otel/trace"
)

func NewDockerClient() (*client.Client, error) {
	return client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
		client.WithTraceProvider(trace.NewNoopTracerProvider()), // Uses a no-op provider, skipping instrumentation
	)
}
