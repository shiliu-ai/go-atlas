// Package serviceclient provides typed HTTP clients for inter-service communication.
//
// It builds on top of httpclient to provide:
//   - Service registry via configuration (map service names to base URLs)
//   - Automatic response unwrapping (parses the standard R{code, message, data} format)
//   - Header forwarding (propagates Authorization, X-Request-ID, etc.)
//   - Per-service timeout/retry configuration
//   - Request options (query params, extra headers, per-request timeout)
//
// Configuration example:
//
//	services:
//	  user-service:
//	    base_url: "http://user-service:8080/user-service"
//	    timeout: 5s
//	  order-service:
//	    base_url: "http://order-service:8080/order-service"
//	    max_retries: 3
//
// Usage:
//
//	userSvc := serviceclient.Of(a).MustGet("user-service")
//
//	// Typed call with automatic response unwrapping
//	var user User
//	err := serviceclient.Get[User](ctx, userSvc, "/v1/users/123", &user)
//
//	// With query parameters
//	var users []User
//	err := serviceclient.Get[[]User](ctx, userSvc, "/v1/users", &users,
//	    serviceclient.WithQuery(url.Values{"page": {"1"}, "size": {"20"}}),
//	)
package serviceclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/shiliu-ai/go-atlas/aether/errors"
	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/pillar/httpclient"
)

// Service defines the interface for inter-service communication.
// Business code should depend on this interface for testability.
type Service interface {
	// Name returns the service name.
	Name() string
	// DoRaw sends an HTTP request and returns the raw response.
	DoRaw(ctx context.Context, method, path string, body any, opts ...RequestOption) (*httpclient.Response, error)
}

// ServiceError represents an error returned by an upstream service call.
// It wraps the underlying *errors.Error so callers can distinguish upstream
// errors from locally created ones via errors.As.
type ServiceError struct {
	serviceName string
	method      string
	path        string
	err         *errors.Error
}

func (e *ServiceError) ServiceName() string { return e.serviceName }
func (e *ServiceError) Method() string      { return e.method }
func (e *ServiceError) Path() string        { return e.path }
func (e *ServiceError) Code() errors.Code {
	if e.err == nil {
		return 0
	}
	return e.err.Code()
}
func (e *ServiceError) Message() string {
	if e.err == nil {
		return ""
	}
	return e.err.Message()
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("serviceclient[%s] %s %s: %v", e.serviceName, e.method, e.path, e.err)
}

func (e *ServiceError) Unwrap() error {
	if e.err == nil {
		return nil
	}
	return e.err
}

func newServiceError(serviceName, method, path string, code errors.Code, message string) *ServiceError {
	return &ServiceError{
		serviceName: serviceName,
		method:      method,
		path:        path,
		err:         errors.New(code, message),
	}
}

// ServiceConfig holds configuration for a single upstream service.
type ServiceConfig struct {
	BaseURL    string        `mapstructure:"base_url"`
	Timeout    time.Duration `mapstructure:"timeout"`
	MaxRetries int           `mapstructure:"max_retries"`
	RetryWait  time.Duration `mapstructure:"retry_wait"`
}

// RequestOption configures a single outgoing request.
type RequestOption func(*requestConfig)

type requestConfig struct {
	query       url.Values
	headers     http.Header
	timeout     time.Duration
	rawBody     io.Reader
	contentType string
}

// WithQuery adds query parameters to the request URL.
func WithQuery(params url.Values) RequestOption {
	return func(cfg *requestConfig) {
		if cfg.query == nil {
			cfg.query = make(url.Values)
		}
		for k, vs := range params {
			for _, v := range vs {
				cfg.query.Add(k, v)
			}
		}
	}
}

// WithHeader adds an extra header to the request.
func WithHeader(key, value string) RequestOption {
	return func(cfg *requestConfig) {
		if cfg.headers == nil {
			cfg.headers = make(http.Header)
		}
		cfg.headers.Set(key, value)
	}
}

// WithTimeout overrides the per-request timeout.
func WithTimeout(d time.Duration) RequestOption {
	return func(cfg *requestConfig) { cfg.timeout = d }
}

// WithRawBody provides a pre-built request body with an explicit content type,
// bypassing the default JSON serialization. The caller is responsible for
// constructing the body (e.g., multipart form, XML, binary stream).
//
// Panics if reader is nil or contentType is empty — these are programming
// errors that should be caught at construction time, not deferred to request
// execution.
//
// When WithRawBody is set, the body parameter of DoRaw (and typed wrappers
// like Post) MUST be nil — passing both is a usage error and returns an error.
func WithRawBody(reader io.Reader, contentType string) RequestOption {
	if reader == nil {
		panic("serviceclient: WithRawBody reader must not be nil")
	}
	if contentType == "" {
		panic("serviceclient: WithRawBody content type must not be empty")
	}
	return func(cfg *requestConfig) {
		cfg.rawBody = reader
		cfg.contentType = contentType
	}
}

// Client is an HTTP client bound to a specific upstream service.
// It implements the Service interface.
type Client struct {
	name    string
	baseURL string
	http    *httpclient.Client
	logger  log.Logger
}

// compile-time interface check
var _ Service = (*Client)(nil)

// newClient creates a Client for the given service.
func newClient(name string, cfg ServiceConfig, defaults httpclient.Config, logger log.Logger) *Client {
	// Merge per-service overrides with global defaults.
	hcfg := defaults
	if cfg.Timeout > 0 {
		hcfg.Timeout = cfg.Timeout
	}
	if cfg.MaxRetries > 0 {
		hcfg.MaxRetries = cfg.MaxRetries
	}
	if cfg.RetryWait > 0 {
		hcfg.RetryWait = cfg.RetryWait
	}

	return &Client{
		name:    name,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		http:    httpclient.NewClient(hcfg, logger),
		logger:  logger,
	}
}

// Name returns the service name.
func (c *Client) Name() string { return c.name }

// buildURL constructs the full URL by joining the service base URL with the given path and query params.
func (c *Client) buildURL(path string, query url.Values) string {
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// DoRaw sends an HTTP request to the service and returns the raw response.
// This is the low-level method; prefer the typed generic functions (Get, Post, etc.)
// which automatically unwrap the standard response envelope.
func (c *Client) DoRaw(ctx context.Context, method, path string, body any, opts ...RequestOption) (*httpclient.Response, error) {
	var rcfg requestConfig
	for _, opt := range opts {
		opt(&rcfg)
	}

	// Apply per-request timeout.
	if rcfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rcfg.timeout)
		defer cancel()
	}

	fullURL := c.buildURL(path, rcfg.query)

	var req *http.Request
	var err error

	switch {
	case rcfg.rawBody != nil && body != nil:
		return nil, fmt.Errorf("serviceclient[%s]: WithRawBody and body parameter are mutually exclusive", c.name)
	case rcfg.rawBody != nil:
		req, err = http.NewRequestWithContext(ctx, method, fullURL, rcfg.rawBody)
		if err != nil {
			return nil, fmt.Errorf("serviceclient[%s]: new request: %w", c.name, err)
		}
		req.Header.Set("Content-Type", rcfg.contentType)
	case body != nil:
		data, merr := json.Marshal(body)
		if merr != nil {
			return nil, fmt.Errorf("serviceclient[%s]: marshal body: %w", c.name, merr)
		}
		req, err = http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("serviceclient[%s]: new request: %w", c.name, err)
		}
		req.Header.Set("Content-Type", "application/json")
	default:
		req, err = http.NewRequestWithContext(ctx, method, fullURL, nil)
		if err != nil {
			return nil, fmt.Errorf("serviceclient[%s]: new request: %w", c.name, err)
		}
	}

	// Forward headers from context (Authorization, X-Request-ID, X-Trace-ID, etc.)
	forwardHeaders(ctx, req)

	// Apply per-request extra headers (after forwarding, so they can override).
	for key, vals := range rcfg.headers {
		for _, v := range vals {
			req.Header.Set(key, v)
		}
	}

	return c.http.Do(ctx, req)
}

// serviceResponse is the standard response envelope from atlas-based services.
type serviceResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Do performs a typed request to the service, automatically unwrapping
// the standard R{code, message, data} response envelope.
//
// It adds OpenTelemetry span attributes for service-level observability.
//
// If the upstream service returns a non-zero business code, it returns
// an *errors.Error with the upstream code and message.
//
// body is optional (pass nil for GET/DELETE).
// result receives the unwrapped "data" field (pass nil to ignore).
func Do[T any](ctx context.Context, c Service, method, path string, body any, result *T, opts ...RequestOption) error {
	tracer := otel.Tracer("serviceclient")
	ctx, span := tracer.Start(ctx, fmt.Sprintf("SVC %s %s", c.Name(), path))
	defer span.End()

	span.SetAttributes(
		attribute.String("service.name", c.Name()),
		attribute.String("service.method", method),
		attribute.String("service.path", path),
	)

	resp, err := c.DoRaw(ctx, method, path, body, opts...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	// Non-2xx HTTP status.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncateBytes(resp.Bytes(), 200))
		span.SetStatus(codes.Error, msg)
		return newServiceError(c.Name(), method, path, errors.Code(resp.StatusCode), msg)
	}

	var envelope serviceResponse
	if err := json.Unmarshal(resp.Bytes(), &envelope); err != nil {
		return fmt.Errorf("serviceclient[%s]: unmarshal response: %w", c.Name(), err)
	}

	span.SetAttributes(attribute.Int("service.business_code", envelope.Code))

	// Non-zero business code -> upstream business error.
	if envelope.Code != 0 {
		span.SetStatus(codes.Error, envelope.Message)
		return newServiceError(c.Name(), method, path, errors.Code(envelope.Code), envelope.Message)
	}

	if result != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return fmt.Errorf("serviceclient[%s]: unmarshal data: %w", c.Name(), err)
		}
	}
	return nil
}

// Get performs a typed GET request to the service.
func Get[T any](ctx context.Context, c Service, path string, result *T, opts ...RequestOption) error {
	return Do[T](ctx, c, http.MethodGet, path, nil, result, opts...)
}

// Post performs a typed POST request to the service.
func Post[T any](ctx context.Context, c Service, path string, body any, result *T, opts ...RequestOption) error {
	return Do[T](ctx, c, http.MethodPost, path, body, result, opts...)
}

// Put performs a typed PUT request to the service.
func Put[T any](ctx context.Context, c Service, path string, body any, result *T, opts ...RequestOption) error {
	return Do[T](ctx, c, http.MethodPut, path, body, result, opts...)
}

// Patch performs a typed PATCH request to the service.
func Patch[T any](ctx context.Context, c Service, path string, body any, result *T, opts ...RequestOption) error {
	return Do[T](ctx, c, http.MethodPatch, path, body, result, opts...)
}

// Delete performs a typed DELETE request to the service.
func Delete[T any](ctx context.Context, c Service, path string, result *T, opts ...RequestOption) error {
	return Do[T](ctx, c, http.MethodDelete, path, nil, result, opts...)
}

// truncateBytes returns the first n bytes of b as a string, appending "..." if truncated.
func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
