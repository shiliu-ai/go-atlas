# `pillar/telemetry` — unified OpenTelemetry for Atlas

One Pillar, one OTLP connection, one Resource, three signals.
`telemetry.Pillar()` wires OpenTelemetry traces and metrics into the Atlas
lifecycle with semconv-compliant HTTP RED, exemplars, Prometheus pull
support, Go runtime instrumentation, and a cardinality cap — all off by
default and configured from YAML.

## Quick start

```go
import (
    "github.com/shiliu-ai/go-atlas/atlas"
    "github.com/shiliu-ai/go-atlas/pillar/telemetry"
)

func main() {
    a := atlas.New("my-service", telemetry.Pillar())

    t := telemetry.Of(a)
    tracer := t.Tracer("billing")
    meter  := t.Meter("billing")
    _ = tracer
    _ = meter

    a.MustRun()
}
```

With `telemetry.enabled: false` (or no `telemetry` section at all), every
call returns a no-op — downstream code does not need nil-checks.

## Configuration

```yaml
telemetry:
  enabled: true                 # master switch; false = noop everything
  resource:
    environment: "production"   # deployment.environment.name
    version: "1.2.3"            # service.version (usually injected via ldflags)
  otlp:
    enabled: true
    endpoint: "localhost:4318"
    protocol: "http"            # http | grpc
    insecure: true
    headers: {}                 # optional auth headers
  prometheus:
    enabled: false
    path: "/metrics"            # scrape endpoint path on the main gin engine
  traces:
    enabled: true
    sample_rate: 1.0            # 0 = never sample; 1 = always sample
  metrics:
    enabled: true
    runtime: true               # Go runtime metrics (GC, goroutines, heap)
    http: true                  # auto HTTP RED middleware
    cardinality_limit: 2000     # per-instrument attribute-set cap
    exemplars: true             # exemplar reservoir (no-op when traces off)
```

Omitted sections take sensible defaults. Values the YAML cannot cover
(Views, custom SpanProcessors, shared Prometheus registries, global
registration, runtime strictness) are exposed as `Option`s — see below.

`service.name` is **not** in this section: it is taken from
`atlas.New(name, ...)` as the single source of truth.

### Defaults summary

| Field | Default |
|---|---|
| `otlp.protocol` | `http` |
| `otlp.endpoint` | `localhost:4318` |
| `prometheus.path` | `/metrics` |
| `traces.sample_rate` | `1.0` |
| `metrics.cardinality_limit` | `2000` |

`sample_rate: 0.0` and `cardinality_limit: 0` are explicit values, **not**
treated as "unset" — the config types use pointer fields so the "never
sample" and "unlimited" cases can be configured.

### Validation

`Config.Validate` runs during `Init` after defaults. It rejects:

- unsupported `otlp.protocol` values
- `sample_rate` outside `[0, 1]`
- negative `cardinality_limit`
- relative `prometheus.path`
- `metrics.enabled: true` with no OTLP or Prometheus exporter enabled
  (metrics would be collected but never exported)

## Options

Use functional options for things YAML shouldn't own:

```go
telemetry.Pillar(
    telemetry.WithResourceAttributes(
        attribute.String("deployment.zone", os.Getenv("AZ")),
    ),
    telemetry.WithView(customAggregation()),
    telemetry.WithSpanProcessor(auditProcessor),
    telemetry.WithPrometheusRegistry(prometheus.DefaultRegisterer.(*prom.Registry)),
    telemetry.WithSetGlobals(true),   // default true
    telemetry.WithRuntimeStrict(false), // default false
)
```

| Option | Purpose |
|---|---|
| `WithResourceAttributes(...)` | Append extra attributes to the Resource. |
| `WithView(...)` | Add custom metric Views alongside the built-in HTTP view. |
| `WithSpanProcessor(...)` | Add extra SpanProcessors (e.g. audit, custom export). |
| `WithPrometheusRegistry(reg)` | Share a `*prometheus.Registry` so application `prom.Counter`s appear on `/metrics` together with OTel metrics. |
| `WithSetGlobals(bool)` | Opt out of registering OTel globals (`otel.SetTracerProvider` etc.) when Atlas shares a process with another OTel-aware framework. |
| `WithRuntimeStrict(bool)` | Treat runtime.Start failures as fatal. Default logs and continues — observability failures must not crash the service. |

## What you get out of the box

**HTTP RED metrics.** `telemetry.Pillar` registers an `otelgin` middleware
that emits spans and histogram points for every request. The histogram
uses OTel's semconv-recommended buckets and a `http.route` attribute
derived from `gin.Context.FullPath()`, with an `UNKNOWN` fallback so
unmatched paths don't explode cardinality.

**Attribute allow-list.** A View on `http.server.request.duration` keeps
only the five semconv-approved attributes (`http.request.method`,
`http.response.status_code`, `http.route`, `url.scheme`,
`network.protocol.version`). Handler-level label-leakage is stopped at
the SDK layer, not hoped away.

**Cardinality cap.** OTel's `WithCardinalityLimit` is applied globally;
overflow collapses into the synthetic `otel_metric_overflow=true` bucket
instead of blowing out your metric backend.

**Exemplars.** Histogram buckets carry `trace_id` references so Grafana
/ Tempo can jump from a slow p99 bucket straight to the offending span —
this is the single biggest reason traces and metrics live in one Pillar.

**Go runtime metrics.** `go.opentelemetry.io/contrib/instrumentation/runtime`
is started on the Pillar's MeterProvider, so `process.runtime.go.*`
(GC, heap, goroutines) ship with the rest of your metrics.

**Prometheus pull.** Set `prometheus.enabled: true` and
`/metrics` (configurable) is mounted on the main gin engine. Pass
`WithPrometheusRegistry` to mix in your own collectors.

**Safe partial failure.** If trace-exporter setup succeeds but
metric-exporter setup fails, `Init` unwinds the staged tracer provider
(bounded by a 5-second timeout) instead of leaking goroutines.

**Parallel, bounded shutdown.** `Stop(ctx)` flushes both providers
concurrently; errors are joined via `errors.Join`.

## Recommended stack

- **Collector**: `otel-collector` receiving OTLP/HTTP on :4318 or OTLP/gRPC
  on :4317, fanning traces to Tempo/Jaeger and metrics to Prometheus or
  a remote-write compatible TSDB.
- **Traces UI**: Tempo + Grafana (Tempo Explore panel links to logs by
  `trace_id`).
- **Metrics UI**: Prometheus + Grafana. Exemplars in histograms should be
  enabled in Prometheus (`--enable-feature=exemplar-storage`) so Grafana
  can render the trace links.
- **Logs**: keep your existing stack. `aether/log` already emits
  `trace_id` fields from the active span, so correlation works without
  extra config.

## Business metrics recipe

The Pillar's primary job for downstream services is to give you a reliable
`Meter` for **custom business alerts** — order counts, payment failures,
tenant quota hits, etc. The framework deliberately does **not** ship
typed-label facades or auto-instrument your DB/cache — you keep full control
over your domain signals.

### Template

```go
package order

import (
    "context"

    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/metric"

    "github.com/shiliu-ai/go-atlas/pillar/telemetry"
)

type Metrics struct {
    Created metric.Int64Counter   // total orders created
    Failed  metric.Int64Counter   // total failures, by reason
    Latency metric.Float64Histogram // end-to-end latency, seconds
}

func NewMetrics(t *telemetry.Telemetry) (*Metrics, error) {
    m := t.Meter("order")

    created, err := m.Int64Counter("order.created",
        metric.WithDescription("Orders created successfully"))
    if err != nil {
        return nil, err
    }
    failed, err := m.Int64Counter("order.failed",
        metric.WithDescription("Orders that failed; see reason attribute"))
    if err != nil {
        return nil, err
    }
    latency, err := m.Float64Histogram("order.duration",
        metric.WithUnit("s"),
        metric.WithDescription("End-to-end order creation latency"))
    if err != nil {
        return nil, err
    }
    return &Metrics{Created: created, Failed: failed, Latency: latency}, nil
}

func (m *Metrics) RecordCreated(ctx context.Context, channel string) {
    m.Created.Add(ctx, 1, metric.WithAttributes(
        attribute.String("channel", channel), // low-cardinality: web/app/api
    ))
}

func (m *Metrics) RecordFailure(ctx context.Context, reason string) {
    m.Failed.Add(ctx, 1, metric.WithAttributes(
        attribute.String("reason", reason), // enum-like: payment, inventory, auth
    ))
}
```

### Naming rules

- **Use dots, not underscores.** OTel converts `order.created` to Prometheus'
  `order_created_total` automatically. Writing `order_created_total`
  yourself produces a double-suffixed `order_created_total_total`.
- **Use the `metric.WithUnit` tag**, not a suffix in the name. `WithUnit("s")`
  is authoritative; appending `_seconds` to the name is duplication the
  Collector will strip wrong.
- **Counters describe a total**, so the name should be a noun phrase in the
  past tense: `order.created`, `payment.failed`, not `create_order`.
- **Histograms describe a distribution**, name with the measured dimension:
  `order.duration`, `request.body.size`.

### Attribute rules

Attributes become Prometheus labels, and **every unique combination of
attribute values is a distinct time series**. Keep labels bounded.

| Safe label examples | Unsafe — do NOT use |
|---|---|
| `channel` (web/app/api) | `user_id` |
| `tenant` (if you have < 100 tenants) | `request_id` |
| `region` | `trace_id` |
| `reason` (enum of failure modes) | raw URL, query string |
| `http.method`, `http.status_code` | email address, free-form input |

If in doubt, test with `rate(your_metric_count[5m])` and count distinct
series. A healthy business metric has a few hundred series at most.

Cardinality is globally capped (default 2000 per instrument) — overflow
collapses to `otel_metric_overflow=true` instead of exploding your TSDB,
but once a metric hits overflow its dashboards go fuzzy. Prevention is
cheaper than cleanup.

### Example alert rule

```yaml
# Alertmanager: fire when order failure ratio > 5% for 5 minutes
- alert: OrderFailureRateHigh
  expr: |
    sum(rate(order_failed_total[5m]))
      / sum(rate(order_created_total[5m] offset 0s) + rate(order_failed_total[5m])) > 0.05
  for: 5m
  labels:
    severity: page
  annotations:
    summary: "Order failure rate above 5%"
    runbook: "https://wiki/runbooks/orders#failure-rate"
```

### Self-monitoring

The Pillar routes all OTel SDK internal errors (OTLP export failures, batch
processor drops, reader collection errors) through `aether/log`. If
`telemetry.Of(a)` ever stops delivering metrics, a log line with
`msg="telemetry: otel internal error"` surfaces in the standard log stream —
attach a log-based alert on that as a second line of defence against silent
metric loss.

## Testing

Use `sdkmetric.NewManualReader` for deterministic assertions against
recorded points — no OTLP collector needed. The package's own test file
demonstrates the pattern; `TestHTTPMiddleware_AllowListRetainsSemconvAttrs`
and `TestHTTPMiddleware_ExemplarCarriesTraceID` are particularly useful
to copy for domain tests.
