package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// Config holds HTTP client configuration.
type Config struct {
	Timeout    time.Duration `mapstructure:"timeout"`
	MaxRetries int           `mapstructure:"max_retries"`
	RetryWait  time.Duration `mapstructure:"retry_wait"`
}

// Client is an HTTP client with timeout, retry, tracing, and logging.
type Client struct {
	http   *http.Client
	cfg    Config
	logger log.Logger
}

// NewClient creates a new Client (used internally and by other pillars).
func NewClient(cfg Config, logger log.Logger) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.RetryWait == 0 {
		cfg.RetryWait = 500 * time.Millisecond
	}
	if logger == nil {
		logger = log.Global()
	}

	return &Client{
		http:   &http.Client{Timeout: cfg.Timeout},
		cfg:    cfg,
		logger: logger,
	}
}

// Response wraps http.Response with convenient helpers.
type Response struct {
	*http.Response
	body []byte
}

// Bytes returns the response body as bytes.
func (r *Response) Bytes() []byte { return r.body }

// String returns the response body as string.
func (r *Response) String() string { return string(r.body) }

// JSON decodes the response body into dst.
func (r *Response) JSON(dst any) error {
	return json.Unmarshal(r.body, dst)
}

// retryableStatusCodes defines which HTTP status codes should trigger a retry.
var retryableStatusCodes = map[int]bool{
	http.StatusTooManyRequests:     true, // 429
	http.StatusInternalServerError: true, // 500
	http.StatusBadGateway:          true, // 502
	http.StatusServiceUnavailable:  true, // 503
	http.StatusGatewayTimeout:      true, // 504
}

// idempotentMethods are safe to retry without explicit opt-in.
var idempotentMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
	http.MethodOptions: true,
}

// Do executes an HTTP request with tracing, logging, and retry.
// Non-idempotent methods (POST, PATCH) are not retried unless the error
// is a connection-level failure (the request was never sent).
func (c *Client) Do(ctx context.Context, req *http.Request) (*Response, error) {
	tracer := otel.Tracer("httpclient")
	ctx, span := tracer.Start(ctx, fmt.Sprintf("HTTP %s %s", req.Method, req.URL.Path))
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", req.Method),
		attribute.String("http.url", req.URL.String()),
	)

	// Inject trace context into outgoing request headers.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	req = req.WithContext(ctx)

	// Cache request body for retries.
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("httpclient: read request body: %w", err)
		}
		req.Body.Close()
	}

	canRetry := idempotentMethods[req.Method]
	var lastErr error
	attempts := c.cfg.MaxRetries + 1

	for i := range attempts {
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		start := time.Now()
		resp, err := c.http.Do(req)
		latency := time.Since(start)

		if err != nil {
			lastErr = err
			c.logger.Warn(ctx, "httpclient request failed",
				log.F("method", req.Method),
				log.F("url", req.URL.String()),
				log.F("attempt", i+1),
				log.F("error", err),
			)
			// Connection-level errors are safe to retry for any method.
			if i < attempts-1 {
				if waitErr := backoffSleep(ctx, c.cfg.RetryWait, i); waitErr != nil {
					return nil, waitErr
				}
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("httpclient: read response body: %w", err)
		}

		c.logger.Debug(ctx, "httpclient request",
			log.F("method", req.Method),
			log.F("url", req.URL.String()),
			log.F("status", resp.StatusCode),
			log.F("latency", latency.String()),
			log.F("attempt", i+1),
		)

		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

		// Retry on retryable status codes, but only for idempotent methods.
		if retryableStatusCodes[resp.StatusCode] && canRetry && i < attempts-1 {
			lastErr = fmt.Errorf("httpclient: server error %d", resp.StatusCode)
			if waitErr := backoffSleep(ctx, c.cfg.RetryWait, i); waitErr != nil {
				return nil, waitErr
			}
			continue
		}

		return &Response{Response: resp, body: body}, nil
	}

	span.RecordError(lastErr)
	span.SetStatus(codes.Error, lastErr.Error())
	return nil, fmt.Errorf("httpclient: all %d attempts failed: %w", attempts, lastErr)
}

// backoffSleep sleeps with exponential backoff + jitter, respecting context cancellation.
func backoffSleep(ctx context.Context, base time.Duration, attempt int) error {
	backoff := base * (1 << attempt) // exponential: base, 2*base, 4*base, ...
	// Add jitter: +/-50% of backoff.
	jitter := time.Duration(rand.Int64N(int64(backoff))) - backoff/2
	backoff += jitter
	if backoff < 0 {
		backoff = base
	}

	select {
	case <-time.After(backoff):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Get is a convenience method for GET requests.
func (c *Client) Get(ctx context.Context, url string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}

// PostJSON is a convenience method for POST requests with JSON body.
func (c *Client) PostJSON(ctx context.Context, url string, body any) (*Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("httpclient: marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.Do(ctx, req)
}

// PutJSON is a convenience method for PUT requests with JSON body.
func (c *Client) PutJSON(ctx context.Context, url string, body any) (*Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("httpclient: marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.Do(ctx, req)
}

// Delete is a convenience method for DELETE requests.
func (c *Client) Delete(ctx context.Context, url string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, req)
}
