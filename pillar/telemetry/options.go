package telemetry

import (
	promclient "github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Option configures the telemetry Pillar at registration time.
//
// Options give library consumers hooks that the YAML config intentionally
// does not cover: custom Views, extra Resource attributes, additional
// SpanProcessors, a shared Prometheus registry, and behavioural toggles
// (global provider registration, runtime-instrumentation strict mode).
type Option func(*options)

// options collects fields configured via functional Options. Centralising
// the defaults here keeps the Pillar zero value meaningful.
type options struct {
	resourceAttrs  []attribute.KeyValue
	views          []sdkmetric.View
	spanProcessors []sdktrace.SpanProcessor

	promRegistry *promclient.Registry

	// setGlobals controls whether Init calls otel.SetTracerProvider /
	// SetMeterProvider / SetTextMapPropagator. True by default so downstream
	// contrib instrumentations (that reach for the globals) just work; set
	// false when multiple frameworks share the process and Atlas must not
	// clobber globals installed elsewhere.
	setGlobals bool

	// runtimeStrict surfaces errors from runtime.Start instead of logging
	// and continuing. Off by default — runtime metrics are nice-to-have,
	// not load-bearing for an application to boot.
	runtimeStrict bool
}

// defaultOptions returns the options applied when no functional Option is
// passed to Pillar.
func defaultOptions() options {
	return options{setGlobals: true}
}

// WithResourceAttributes appends extra attributes to the OTel Resource.
// Conflicting keys last-writer-win; values supplied here override the ones
// populated from Config.Resource.
func WithResourceAttributes(attrs ...attribute.KeyValue) Option {
	return func(o *options) { o.resourceAttrs = append(o.resourceAttrs, attrs...) }
}

// WithView appends Views to the MeterProvider in addition to the built-in
// HTTP-server view. Useful for customising aggregation on domain metrics.
func WithView(views ...sdkmetric.View) Option {
	return func(o *options) { o.views = append(o.views, views...) }
}

// WithSpanProcessor appends SpanProcessors to the TracerProvider. The default
// batch processor wrapping the OTLP exporter is always installed; processors
// added here run alongside it.
func WithSpanProcessor(sp ...sdktrace.SpanProcessor) Option {
	return func(o *options) { o.spanProcessors = append(o.spanProcessors, sp...) }
}

// WithPrometheusRegistry wires the Prometheus pull exporter into a shared
// *prometheus.Registry, letting application code register its own Collectors
// alongside the OTel-exported histograms and counters.
//
// When omitted, the Pillar creates a private registry and only OTel metrics
// appear on the /metrics endpoint.
func WithPrometheusRegistry(reg *promclient.Registry) Option {
	return func(o *options) { o.promRegistry = reg }
}

// WithSetGlobals controls registration of OTel globals (TracerProvider,
// MeterProvider, TextMapPropagator). Defaults to true. Disable when Atlas
// is embedded in a process that already configures OTel elsewhere.
func WithSetGlobals(set bool) Option {
	return func(o *options) { o.setGlobals = set }
}

// WithRuntimeStrict treats runtime.Start failures as fatal during Init.
// Default behaviour logs a warning and continues, matching the principle
// that observability failures must not take an application offline.
func WithRuntimeStrict(strict bool) Option {
	return func(o *options) { o.runtimeStrict = strict }
}
