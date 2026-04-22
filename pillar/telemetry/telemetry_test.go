package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	promclient "github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// testTelemetry builds a Telemetry with NoOp tracing and a MeterProvider
// backed by the supplied ManualReader — so tests can assert on collected
// metrics without starting an OTLP collector.
func testTelemetry(t *testing.T, reader sdkmetric.Reader) *Telemetry {
	t.Helper()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(httpServerDurationView()),
		sdkmetric.WithCardinalityLimit(10),
	)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return &Telemetry{
		opts:        defaultOptions(),
		serviceName: "test-service",
		cfg:         Config{Enabled: true, Metrics: MetricsConfig{Enabled: true, HTTP: true}},
		tp:          tracenoop.NewTracerProvider(),
		mp:          mp,
	}
}

// testTelemetryWithTracer is like testTelemetry but wires in a real
// TracerProvider so exemplars can be asserted end-to-end.
func testTelemetryWithTracer(t *testing.T, reader sdkmetric.Reader, tp trace.TracerProvider) *Telemetry {
	t.Helper()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(httpServerDurationView()),
		sdkmetric.WithCardinalityLimit(100),
		sdkmetric.WithExemplarFilter(exemplar.AlwaysOnFilter),
	)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return &Telemetry{
		opts:        defaultOptions(),
		serviceName: "test-service",
		cfg:         Config{Enabled: true, Metrics: MetricsConfig{Enabled: true, HTTP: true}},
		tp:          tp,
		mp:          mp,
	}
}

// collectHTTPDuration returns the histogram data points for
// http.server.request.duration, or fails the test if missing.
func collectHTTPDuration(t *testing.T, reader *sdkmetric.ManualReader) []metricdata.HistogramDataPoint[float64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("expected Histogram[float64], got %T", m.Data)
			}
			return hist.DataPoints
		}
	}
	t.Fatalf("metric http.server.request.duration not recorded; got %d scope(s)", len(rm.ScopeMetrics))
	return nil
}

// mustAttr returns the value of the named attribute, failing the test when
// the attribute is absent.
func mustAttr(t *testing.T, set attribute.Set, key attribute.Key) attribute.Value {
	t.Helper()
	v, ok := set.Value(key)
	if !ok {
		t.Fatalf("attribute %s missing from %v", key, set.Encoded(attribute.DefaultEncoder()))
	}
	return v
}

func newTestEngine(tel *Telemetry) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(tel.httpMiddleware())
	e.GET("/v1/users/:id", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return e
}

func TestHTTPMiddleware_RecordsSemconvHistogram(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	tel := testTelemetry(t, reader)
	engine := newTestEngine(tel)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v1/users/42", nil)
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	points := collectHTTPDuration(t, reader)
	if len(points) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(points))
	}
	route := mustAttr(t, points[0].Attributes, "http.route").AsString()
	if route != "/v1/users/:id" {
		t.Fatalf("http.route = %q, want %q (must be gin FullPath template, not raw URL)", route, "/v1/users/:id")
	}
}

func TestHTTPMiddleware_UnknownRouteFallback(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	tel := testTelemetry(t, reader)
	engine := newTestEngine(tel)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/no/such/path", nil)
	engine.ServeHTTP(rec, req)

	points := collectHTTPDuration(t, reader)
	if len(points) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(points))
	}
	route := mustAttr(t, points[0].Attributes, "http.route").AsString()
	if route != unknownRoute {
		t.Fatalf("http.route = %q, want %q (unmatched routes must collapse)", route, unknownRoute)
	}
}

// TestHTTPMiddleware_AllowListRetainsSemconvAttrs guards against silent
// semconv drift between our View allow-list and the attribute keys otelgin
// actually emits. If otelgin ever upgrades to a semconv version that renames
// one of these keys, the AttributeFilter would drop it and this assertion
// would fail loudly — rather than metrics silently losing their method /
// status_code labels.
func TestHTTPMiddleware_AllowListRetainsSemconvAttrs(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	tel := testTelemetry(t, reader)
	engine := newTestEngine(tel)

	req, _ := http.NewRequest(http.MethodGet, "/v1/users/7", nil)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	points := collectHTTPDuration(t, reader)
	if len(points) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(points))
	}

	// All retained keys must be in the allow-list (no drift admits unknowns).
	for _, kv := range points[0].Attributes.ToSlice() {
		if _, ok := allowedHTTPAttrs[kv.Key]; !ok {
			t.Fatalf("attribute %s leaked past the View allow-list", kv.Key)
		}
	}
	// And the three keys we positively depend on must be present. Drops here
	// would silently strip labels on production histograms.
	for _, key := range []attribute.Key{
		semconv.HTTPRequestMethodKey,
		semconv.HTTPResponseStatusCodeKey,
		semconv.HTTPRouteKey,
	} {
		if _, ok := points[0].Attributes.Value(key); !ok {
			t.Fatalf("required attribute %s missing — check otelgin semconv compatibility", key)
		}
	}
}

func TestDisabled_UsesNoOpProviders(t *testing.T) {
	d := &Telemetry{tp: noopTracerProvider, mp: noopMeterProvider}
	if _, ok := d.tp.(tracenoop.TracerProvider); !ok {
		t.Fatalf("tp = %T, want tracenoop.TracerProvider", d.tp)
	}
	if _, ok := d.mp.(metricnoop.MeterProvider); !ok {
		t.Fatalf("mp = %T, want metricnoop.MeterProvider", d.mp)
	}
	// Exercise the no-op paths end-to-end to verify nothing panics.
	_, span := d.Tracer("any").Start(context.Background(), "op")
	span.End()
	counter, err := d.Meter("any").Int64Counter("no.op")
	if err != nil {
		t.Fatalf("noop meter returned error: %v", err)
	}
	counter.Add(context.Background(), 1)
}

func TestDefaults_AppliedWhenEnabled(t *testing.T) {
	c := Config{Enabled: true}
	c.applyDefaults()
	if c.OTLP.Protocol != "http" {
		t.Fatalf("OTLP.Protocol default = %q, want %q", c.OTLP.Protocol, "http")
	}
	if c.OTLP.Endpoint != DefaultOTLPEndpoint {
		t.Fatalf("OTLP.Endpoint default = %q, want %s", c.OTLP.Endpoint, DefaultOTLPEndpoint)
	}
	if got := c.Traces.sampleRate(); got != DefaultSampleRate {
		t.Fatalf("Traces.SampleRate default = %v, want %v", got, DefaultSampleRate)
	}
	if got := c.Metrics.cardinalityLimit(); got != DefaultCardinalityLimit {
		t.Fatalf("Metrics.CardinalityLimit default = %d, want %d", got, DefaultCardinalityLimit)
	}
	if c.Prometheus.Path != DefaultPrometheusPath {
		t.Fatalf("Prometheus.Path default = %q, want %s", c.Prometheus.Path, DefaultPrometheusPath)
	}
}

func TestDefaults_LeftAloneWhenDisabled(t *testing.T) {
	c := Config{Enabled: false}
	c.applyDefaults()
	if c.OTLP.Protocol != "" || c.Traces.SampleRate != nil || c.Metrics.CardinalityLimit != nil {
		t.Fatalf("defaults applied to disabled config: %+v", c)
	}
}

// TestDefaults_ZeroSampleRateRespected confirms that an explicit 0 sample
// rate survives applyDefaults — the pointer type is the mechanism that lets
// users disable trace sampling without also tripping the master switch.
func TestDefaults_ZeroSampleRateRespected(t *testing.T) {
	zero := 0.0
	c := Config{Enabled: true, Traces: TracesConfig{SampleRate: &zero}}
	c.applyDefaults()
	if c.Traces.sampleRate() != 0 {
		t.Fatalf("sample_rate: 0.0 was overwritten by default, got %v", c.Traces.sampleRate())
	}
	zeroLimit := 0
	c2 := Config{Enabled: true, Metrics: MetricsConfig{CardinalityLimit: &zeroLimit}}
	c2.applyDefaults()
	if c2.Metrics.cardinalityLimit() != 0 {
		t.Fatalf("cardinality_limit: 0 was overwritten by default, got %v", c2.Metrics.cardinalityLimit())
	}
}

func TestBuildResource_IncludesServiceName(t *testing.T) {
	res, err := buildResource("example-service", ResourceConfig{Environment: "prod", Version: "1.2.3"}, nil)
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}
	encoded := res.Encoded(attribute.DefaultEncoder())
	for _, want := range []string{"service.name=example-service", "deployment.environment.name=prod", "service.version=1.2.3"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("resource missing %q, got: %s", want, encoded)
		}
	}
}

// TestBuildResource_ExtraAttributesLayered confirms WithResourceAttributes
// values reach the Resource alongside service.name.
func TestBuildResource_ExtraAttributesLayered(t *testing.T) {
	extra := []attribute.KeyValue{attribute.String("deployment.zone", "us-east-1a")}
	res, err := buildResource("svc", ResourceConfig{}, extra)
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}
	encoded := res.Encoded(attribute.DefaultEncoder())
	if !strings.Contains(encoded, "deployment.zone=us-east-1a") {
		t.Fatalf("extra attr missing, got: %s", encoded)
	}
}

func TestHTTPMiddleware_CardinalityOverflowCollapses(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	tel := testTelemetry(t, reader) // CardinalityLimit = 10 in testTelemetry
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(tel.httpMiddleware())
	// Register many routes to push distinct http.route label values past
	// the cardinality limit. Overflow lands in a synthetic overflow bucket.
	for i := range 30 {
		route := fmt.Sprintf("/v1/route_%d", i)
		engine.GET(route, func(c *gin.Context) { c.Status(http.StatusOK) })
		req, _ := http.NewRequest(http.MethodGet, route, nil)
		engine.ServeHTTP(httptest.NewRecorder(), req)
	}

	points := collectHTTPDuration(t, reader)
	if len(points) == 0 || len(points) > 11 { // 10 + 1 overflow bucket
		t.Fatalf("cardinality cap not enforced: got %d distinct attribute sets, want <= 11", len(points))
	}
	// At least one point must be the synthetic overflow, which OTel emits
	// with attribute otel.metric.overflow=true.
	sawOverflow := false
	for _, p := range points {
		if v, ok := p.Attributes.Value("otel.metric.overflow"); ok && v.AsBool() {
			sawOverflow = true
			break
		}
	}
	if !sawOverflow {
		t.Fatalf("expected an overflow data point among %d", len(points))
	}
}

// --- Config.Validate tests ---

func TestValidate_RejectsBadProtocol(t *testing.T) {
	c := Config{Enabled: true, OTLP: OTLPConfig{Protocol: "ftp", Endpoint: "localhost:4318"}}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "Protocol") {
		t.Fatalf("want Protocol error, got %v", err)
	}
}

func TestValidate_RejectsSampleRateOutOfRange(t *testing.T) {
	over := 1.5
	c := Config{Enabled: true, OTLP: OTLPConfig{Protocol: "http"}, Traces: TracesConfig{SampleRate: &over}}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "SampleRate") {
		t.Fatalf("want SampleRate error, got %v", err)
	}
}

func TestValidate_RejectsNegativeCardinalityLimit(t *testing.T) {
	neg := -1
	c := Config{Enabled: true, OTLP: OTLPConfig{Protocol: "http"}, Metrics: MetricsConfig{CardinalityLimit: &neg}}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "CardinalityLimit") {
		t.Fatalf("want CardinalityLimit error, got %v", err)
	}
}

func TestValidate_RequiresExporterWhenMetricsEnabled(t *testing.T) {
	c := Config{
		Enabled: true,
		OTLP:    OTLPConfig{Protocol: "http", Enabled: false},
		Metrics: MetricsConfig{Enabled: true},
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "neither OTLP nor Prometheus") {
		t.Fatalf("want missing-exporter error, got %v", err)
	}
}

func TestValidate_RejectsRelativePrometheusPath(t *testing.T) {
	c := Config{
		Enabled:    true,
		OTLP:       OTLPConfig{Protocol: "http"},
		Prometheus: PrometheusConfig{Enabled: true, Path: "metrics"},
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "must start with /") {
		t.Fatalf("want prometheus path error, got %v", err)
	}
}

func TestValidate_DisabledConfigTriviallyValid(t *testing.T) {
	c := Config{Enabled: false}
	if err := c.Validate(); err != nil {
		t.Fatalf("disabled config must validate: %v", err)
	}
}

// --- Exporter construction error paths ---

func TestNewTraceExporter_UnsupportedProtocol(t *testing.T) {
	_, err := newTraceExporter(context.Background(), OTLPConfig{Protocol: "tcp", Endpoint: "x"})
	if err == nil || !strings.Contains(err.Error(), "unsupported OTLP protocol") {
		t.Fatalf("want unsupported-protocol error, got %v", err)
	}
}

func TestNewMetricExporter_UnsupportedProtocol(t *testing.T) {
	_, err := newMetricExporter(context.Background(), OTLPConfig{Protocol: "tcp", Endpoint: "x"})
	if err == nil || !strings.Contains(err.Error(), "unsupported OTLP protocol") {
		t.Fatalf("want unsupported-protocol error, got %v", err)
	}
}

// --- Stop() behaviour ---

func TestStop_JoinsErrorsInParallel(t *testing.T) {
	boom := errors.New("boom")
	var callCount atomic.Int32
	t1 := &Telemetry{shutdowns: []func(context.Context) error{
		func(ctx context.Context) error { callCount.Add(1); return nil },
		func(ctx context.Context) error { callCount.Add(1); return boom },
		func(ctx context.Context) error { callCount.Add(1); return errors.New("blam") },
	}}
	err := t1.Stop(context.Background())
	if err == nil {
		t.Fatalf("want joined error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("joined error does not wrap boom: %v", err)
	}
	if callCount.Load() != 3 {
		t.Fatalf("want all 3 shutdowns called, got %d", callCount.Load())
	}
}

func TestStop_NoShutdownsIsNoOp(t *testing.T) {
	empty := &Telemetry{}
	if err := empty.Stop(context.Background()); err != nil {
		t.Fatalf("want nil on empty shutdowns, got %v", err)
	}
}

// --- Prometheus route wiring ---

func TestRoutes_MountsPrometheusHandler(t *testing.T) {
	reg := promclient.NewRegistry()
	// Register a single trivial collector so the scrape output is non-empty.
	c := promclient.NewCounter(promclient.CounterOpts{Name: "atlas_test_total", Help: "test"})
	reg.MustRegister(c)
	c.Inc()

	tel := &Telemetry{
		opts:        defaultOptions(),
		cfg:         Config{Enabled: true, Prometheus: PrometheusConfig{Enabled: true, Path: "/metrics"}},
		tp:          tracenoop.NewTracerProvider(),
		mp:          metricnoop.NewMeterProvider(),
		promHandler: promhttpHandler(reg),
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	tel.Routes(&engine.RouterGroup)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "atlas_test_total") {
		t.Fatalf("scrape output missing registered collector: %s", rec.Body.String())
	}
}

func TestRoutes_NoHandlerMountsNothing(t *testing.T) {
	tel := &Telemetry{}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	tel.Routes(&engine.RouterGroup)
	// No panic, no route — verify /metrics returns 404.
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when Prometheus exporter is off", rec.Code)
	}
}

// --- Exemplars: the core win of merging traces + metrics ---

func TestHTTPMiddleware_ExemplarCarriesTraceID(t *testing.T) {
	// Real TracerProvider with an AlwaysOn sampler so exemplars have a
	// trace_id to record.
	spanRec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(spanRec),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	reader := sdkmetric.NewManualReader()
	tel := testTelemetryWithTracer(t, reader, tp)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(tel.httpMiddleware())
	engine.GET("/v1/ping", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req, _ := http.NewRequest(http.MethodGet, "/v1/ping", nil)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	points := collectHTTPDuration(t, reader)
	if len(points) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(points))
	}
	if len(points[0].Exemplars) == 0 {
		t.Fatalf("no exemplars recorded — HTTP middleware did not propagate the active span context")
	}
	traceID := points[0].Exemplars[0].TraceID
	if len(traceID) == 0 {
		t.Fatalf("exemplar missing TraceID — the point of the exemplar")
	}
	// Span must correspond to a trace actually recorded by the tracer.
	spans := spanRec.Ended()
	if len(spans) == 0 {
		t.Fatalf("tracer recorded no spans; middleware may have skipped span creation")
	}
}

// --- OTel internal error routing ---

// capturingLogger records Error() calls for assertion. Only Error is used by
// the handler under test; other levels are satisfied by empty stubs.
type capturingLogger struct {
	errs []string
}

func (l *capturingLogger) Debug(context.Context, string, ...log.Field) {}
func (l *capturingLogger) Info(context.Context, string, ...log.Field)  {}
func (l *capturingLogger) Warn(context.Context, string, ...log.Field)  {}
func (l *capturingLogger) Error(_ context.Context, msg string, fields ...log.Field) {
	for _, f := range fields {
		if f.Key == "error" {
			l.errs = append(l.errs, fmt.Sprintf("%s: %v", msg, f.Value))
			return
		}
	}
	l.errs = append(l.errs, msg)
}
func (l *capturingLogger) WithFields(...log.Field) log.Logger { return l }

func TestOTelErrorHandler_RoutesToLogger(t *testing.T) {
	lg := &capturingLogger{}
	h := newOTelErrorHandler(lg)
	h.Handle(errors.New("collector unreachable"))
	h.Handle(nil) // nil must be a silent drop, not a phantom log line

	if len(lg.errs) != 1 {
		t.Fatalf("want 1 error logged, got %d: %v", len(lg.errs), lg.errs)
	}
	if !strings.Contains(lg.errs[0], "collector unreachable") {
		t.Fatalf("log message missing error cause: %q", lg.errs[0])
	}
}

// --- Zero-value safety: direct &Telemetry{} must not panic ---

// TestZeroTelemetry_FallsBackToNoop guards the footgun of users (or DI
// frameworks) constructing &Telemetry{} outside Pillar(). The type falls
// back to NoOp providers instead of nil-dereferencing, so dependent code
// can write against the interface in tests without worrying about init.
func TestZeroTelemetry_FallsBackToNoop(t *testing.T) {
	z := &Telemetry{}
	// Provider accessors return the NoOp singletons.
	if z.TracerProvider() != noopTracerProvider {
		t.Fatal("TracerProvider on zero Telemetry did not return NoOp")
	}
	if z.MeterProvider() != noopMeterProvider {
		t.Fatal("MeterProvider on zero Telemetry did not return NoOp")
	}
	// Tracer/Meter invocations must be safe.
	_, span := z.Tracer("any").Start(context.Background(), "op")
	span.End()
	ctr, err := z.Meter("any").Int64Counter("no.op")
	if err != nil {
		t.Fatalf("noop meter returned error: %v", err)
	}
	ctr.Add(context.Background(), 1)
}

// --- Middleware gating ---

func TestMiddleware_NotInstalledWhenDisabled(t *testing.T) {
	tel := &Telemetry{cfg: Config{Enabled: false}}
	if mw := tel.Middleware(); mw != nil {
		t.Fatalf("Middleware should be nil when telemetry disabled, got %d", len(mw))
	}
}

func TestMiddleware_NotInstalledWhenAllSignalsOff(t *testing.T) {
	// Enabled at master level but neither metrics.http nor traces on.
	tel := &Telemetry{cfg: Config{
		Enabled: true,
		Metrics: MetricsConfig{Enabled: true, HTTP: false},
		Traces:  TracesConfig{Enabled: false},
	}}
	if mw := tel.Middleware(); mw != nil {
		t.Fatalf("Middleware should be nil when no HTTP signal is active, got %d", len(mw))
	}
}

func TestMiddleware_IgnoresHTTPFlagWhenMetricsDisabled(t *testing.T) {
	// metrics.http=true but metrics.enabled=false: no metric producer, so no
	// point running otelgin. Traces also off — middleware should be skipped.
	tel := &Telemetry{cfg: Config{
		Enabled: true,
		Metrics: MetricsConfig{Enabled: false, HTTP: true},
		Traces:  TracesConfig{Enabled: false},
	}}
	if mw := tel.Middleware(); mw != nil {
		t.Fatalf("Middleware should be nil when metrics.enabled=false, got %d", len(mw))
	}
}

// --- Options: setGlobals opt-out ---

// TestOptions_SetGlobalsDefault and TestOptions_SetGlobalsOptOut pin the
// behavioural contract of WithSetGlobals. This is cheap insurance against
// accidental regressions in Init that would silently clobber (or stop
// clobbering) OTel globals — a change downstream users would feel only
// via broken context propagation, hours away from the code change.
func TestOptions_SetGlobalsDefault(t *testing.T) {
	o := defaultOptions()
	if !o.setGlobals {
		t.Fatal("setGlobals default should be true")
	}
}

func TestOptions_SetGlobalsOptOut(t *testing.T) {
	o := defaultOptions()
	WithSetGlobals(false)(&o)
	if o.setGlobals {
		t.Fatal("WithSetGlobals(false) did not disable global registration")
	}
}

// --- Pillar Init / cleanup ---

func TestInit_PartialFailureCleansUp(t *testing.T) {
	// Simulate the "traces init OK, metrics init fails" case by running
	// cleanupAfterInitFailure directly after manually staging a shutdown.
	called := false
	tel := &Telemetry{
		tp: tracenoop.NewTracerProvider(),
		mp: metricnoop.NewMeterProvider(),
		shutdowns: []func(context.Context) error{
			func(ctx context.Context) error { called = true; return nil },
		},
	}
	tel.cleanupAfterInitFailure()
	if !called {
		t.Fatal("cleanupAfterInitFailure did not run staged shutdown")
	}
	if len(tel.shutdowns) != 0 {
		t.Fatalf("shutdowns not cleared after cleanup: %d remain", len(tel.shutdowns))
	}
	if tel.tp != noopTracerProvider || tel.mp != noopMeterProvider {
		t.Fatal("providers not reset to noop after cleanup")
	}
}

// promhttpHandler is a convenience to avoid re-importing promhttp in every
// test that needs a handler over a raw registry.
func promhttpHandler(reg *promclient.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatherers := promclient.Gatherers{reg}
		mfs, err := gatherers.Gather()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, mf := range mfs {
			for _, m := range mf.Metric {
				fmt.Fprintf(w, "%s %v\n", mf.GetName(), m.Counter.GetValue())
			}
		}
	})
}
