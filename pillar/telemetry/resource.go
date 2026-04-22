package telemetry

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"

	"github.com/shiliu-ai/go-atlas/artifact/id"
)

// buildResource assembles the OTel Resource shared across tracing and metrics.
// service.name comes from atlas.Core, never from config. Extra attributes
// supplied via WithResourceAttributes layer on top and take precedence over
// both the service-name defaults and Config.Resource values on key conflict.
func buildResource(serviceName string, cfg ResourceConfig, extra []attribute.KeyValue) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
		semconv.ServiceInstanceID(id.UUID()),
	}
	if cfg.Version != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.Version))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentName(cfg.Environment))
	}
	// Caller-supplied attrs appended last so resource.Merge applies them as
	// the latest-writer on key conflict.
	attrs = append(attrs, extra...)

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}
	return res, nil
}
