package telemetry

import (
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/metric"
)

// startRuntimeMetrics registers Go runtime instrumentation (GC, heap,
// goroutines) against the provided MeterProvider.
func startRuntimeMetrics(mp metric.MeterProvider) error {
	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		return fmt.Errorf("telemetry: runtime metrics: %w", err)
	}
	return nil
}
