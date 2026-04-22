package telemetry

import (
	"errors"
	"fmt"
	"strings"
)

// OTLP protocol values accepted in OTLPConfig.Protocol.
const (
	ProtocolHTTP = "http"
	ProtocolGRPC = "grpc"
)

// DefaultSampleRate is the trace sampling ratio used when the config leaves
// Traces.SampleRate unset.
const DefaultSampleRate = 1.0

// DefaultCardinalityLimit is the maximum number of distinct attribute sets a
// single instrument may track before overflow. Applied when the config leaves
// Metrics.CardinalityLimit unset.
const DefaultCardinalityLimit = 2000

// DefaultOTLPEndpoint is the OTLP/HTTP endpoint assumed when none is
// configured — matches the OpenTelemetry Collector default.
const DefaultOTLPEndpoint = "localhost:4318"

// DefaultPrometheusPath is the path on the main gin engine where the
// Prometheus pull exporter registers its scrape handler.
const DefaultPrometheusPath = "/metrics"

// Config holds telemetry configuration read from the `telemetry` section.
//
// Zero-valued Config runs the pillar in NoOp mode — no exporter connection is
// opened. Set Enabled=true to activate the signals selected via the sub-sections.
type Config struct {
	// Enabled is the master switch. When false the pillar installs NoOp
	// providers and never opens an exporter connection.
	Enabled bool `mapstructure:"enabled"`

	Resource   ResourceConfig   `mapstructure:"resource"`
	OTLP       OTLPConfig       `mapstructure:"otlp"`
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Traces     TracesConfig     `mapstructure:"traces"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
}

// ResourceConfig supplies values for the shared OTel Resource.
// service.name comes from atlas.New(name, ...) — not from here.
type ResourceConfig struct {
	Environment string `mapstructure:"environment"`
	Version     string `mapstructure:"version"`
}

// OTLPConfig configures the OTLP exporter shared by tracing and metrics.
type OTLPConfig struct {
	Enabled  bool              `mapstructure:"enabled"`
	Endpoint string            `mapstructure:"endpoint"`
	Protocol string            `mapstructure:"protocol"` // http | grpc
	Insecure bool              `mapstructure:"insecure"`
	Headers  map[string]string `mapstructure:"headers"`
}

// PrometheusConfig controls the Prometheus pull exporter.
type PrometheusConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// TracesConfig controls tracing-specific behavior.
//
// SampleRate is a pointer so that an explicit sample_rate: 0.0 (never sample)
// can be distinguished from an omitted key (use the default). Values outside
// [0, 1] are rejected by Validate().
type TracesConfig struct {
	Enabled    bool     `mapstructure:"enabled"`
	SampleRate *float64 `mapstructure:"sample_rate"`
}

// MetricsConfig controls metrics-specific behavior.
//
// CardinalityLimit is a pointer so that cardinality_limit: 0 (documented by
// the OTel SDK as "no limit") can be distinguished from an omitted key (use
// the default). Negative values are rejected by Validate().
type MetricsConfig struct {
	Enabled          bool `mapstructure:"enabled"`
	Runtime          bool `mapstructure:"runtime"`
	HTTP             bool `mapstructure:"http"`
	CardinalityLimit *int `mapstructure:"cardinality_limit"`
	Exemplars        bool `mapstructure:"exemplars"`
}

// applyDefaults fills zero-valued fields with Atlas defaults. Only applies
// when the section is active — absent/disabled config runs in NoOp mode and
// the zero values are load-bearing signals to the init path.
func (c *Config) applyDefaults() {
	if !c.Enabled {
		return
	}

	if c.OTLP.Protocol == "" {
		c.OTLP.Protocol = ProtocolHTTP
	}
	if c.OTLP.Endpoint == "" {
		c.OTLP.Endpoint = DefaultOTLPEndpoint
	}

	if c.Prometheus.Path == "" {
		c.Prometheus.Path = DefaultPrometheusPath
	}

	if c.Traces.SampleRate == nil {
		v := DefaultSampleRate
		c.Traces.SampleRate = &v
	}

	if c.Metrics.CardinalityLimit == nil {
		v := DefaultCardinalityLimit
		c.Metrics.CardinalityLimit = &v
	}
}

// sampleRate returns the effective sample rate, preferring the configured
// pointer value over DefaultSampleRate. Safe to call after applyDefaults.
func (c TracesConfig) sampleRate() float64 {
	if c.SampleRate != nil {
		return *c.SampleRate
	}
	return DefaultSampleRate
}

// cardinalityLimit returns the effective cardinality limit. Safe to call
// after applyDefaults.
func (c MetricsConfig) cardinalityLimit() int {
	if c.CardinalityLimit != nil {
		return *c.CardinalityLimit
	}
	return DefaultCardinalityLimit
}

// Validate reports actionable configuration errors. It is called by the
// Pillar's Init after applyDefaults so that both user-supplied and
// defaulted values are checked. A disabled config is trivially valid.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	var errs []error

	switch strings.ToLower(c.OTLP.Protocol) {
	case ProtocolHTTP, ProtocolGRPC:
		// ok
	default:
		errs = append(errs, fmt.Errorf("telemetry: OTLP.Protocol %q: must be %q or %q",
			c.OTLP.Protocol, ProtocolHTTP, ProtocolGRPC))
	}

	if c.OTLP.Enabled && strings.TrimSpace(c.OTLP.Endpoint) == "" {
		errs = append(errs, errors.New("telemetry: OTLP.Endpoint must be set when OTLP is enabled"))
	}

	if sr := c.Traces.sampleRate(); sr < 0 || sr > 1 {
		errs = append(errs, fmt.Errorf("telemetry: Traces.SampleRate %v: must be in [0, 1]", sr))
	}

	if cl := c.Metrics.cardinalityLimit(); cl < 0 {
		errs = append(errs, fmt.Errorf("telemetry: Metrics.CardinalityLimit %d: must be >= 0", cl))
	}

	if c.Prometheus.Enabled {
		p := strings.TrimSpace(c.Prometheus.Path)
		if p == "" {
			errs = append(errs, errors.New("telemetry: Prometheus.Path must be set when Prometheus is enabled"))
		} else if !strings.HasPrefix(p, "/") {
			errs = append(errs, fmt.Errorf("telemetry: Prometheus.Path %q: must start with /", p))
		}
	}

	if c.Enabled && c.Metrics.Enabled && !c.OTLP.Enabled && !c.Prometheus.Enabled {
		errs = append(errs, errors.New(
			"telemetry: Metrics.Enabled but neither OTLP nor Prometheus exporter is enabled — metrics would be collected but never exported"))
	}

	return errors.Join(errs...)
}
