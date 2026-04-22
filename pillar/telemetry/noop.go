package telemetry

import (
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Both noop providers are stateless singletons — allocating them once at
// package init lets disabled branches install them with a plain assignment.
var (
	noopTracerProvider trace.TracerProvider = tracenoop.NewTracerProvider()
	noopMeterProvider  metric.MeterProvider = metricnoop.NewMeterProvider()
)
