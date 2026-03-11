// Package serviceclient provides typed HTTP clients for inter-service communication.
//
// It builds on top of httpclient to provide:
//   - Service registry via configuration (map service names to base URLs)
//   - Automatic response unwrapping (parses the standard R{code, message, data} format)
//   - Header forwarding (propagates Authorization, X-Request-ID, etc.)
//   - Per-service timeout/retry configuration
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
//	userSvc := a.Service("user-service")
//
//	// Raw call
//	resp, err := userSvc.Get(ctx, "/v1/users/123")
//
//	// Typed call with automatic response unwrapping
//	var user User
//	err := serviceclient.Do[User](ctx, userSvc, "GET", "/v1/users/123", nil, &user)
package serviceclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shiliu-ai/go-atlas/errors"
	"github.com/shiliu-ai/go-atlas/httpclient"
	"github.com/shiliu-ai/go-atlas/log"
)

// ServiceConfig holds configuration for a single upstream service.
type ServiceConfig struct {
	BaseURL    string        `mapstructure:"base_url"`
	Timeout    time.Duration `mapstructure:"timeout"`
	MaxRetries int           `mapstructure:"max_retries"`
	RetryWait  time.Duration `mapstructure:"retry_wait"`
}

// Client is an HTTP client bound to a specific upstream service.
type Client struct {
	name    string
	baseURL string
	http    *httpclient.Client
	logger  log.Logger
}

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
		http:    httpclient.New(hcfg, logger),
		logger:  logger,
	}
}

// Name returns the service name.
func (c *Client) Name() string { return c.name }

// URL constructs the full URL by joining the service base URL with the given path.
func (c *Client) URL(path string) string {
	if path == "" {
		return c.baseURL
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + path
}

// Get sends a GET request to the service.
func (c *Client) Get(ctx context.Context, path string) (*httpclient.Response, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return c.http.Do(ctx, req)
}

// PostJSON sends a POST request with JSON body to the service.
func (c *Client) PostJSON(ctx context.Context, path string, body any) (*httpclient.Response, error) {
	req, err := c.newJSONRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	return c.http.Do(ctx, req)
}

// PutJSON sends a PUT request with JSON body to the service.
func (c *Client) PutJSON(ctx context.Context, path string, body any) (*httpclient.Response, error) {
	req, err := c.newJSONRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return nil, err
	}
	return c.http.Do(ctx, req)
}

// PatchJSON sends a PATCH request with JSON body to the service.
func (c *Client) PatchJSON(ctx context.Context, path string, body any) (*httpclient.Response, error) {
	req, err := c.newJSONRequest(ctx, http.MethodPatch, path, body)
	if err != nil {
		return nil, err
	}
	return c.http.Do(ctx, req)
}

// Delete sends a DELETE request to the service.
func (c *Client) Delete(ctx context.Context, path string) (*httpclient.Response, error) {
	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	return c.http.Do(ctx, req)
}

// newRequest creates an http.Request with forwarded headers from context.
func (c *Client) newRequest(ctx context.Context, method, path string, body *bytes.Reader) (*http.Request, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, c.URL(path), body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, c.URL(path), nil)
	}
	if err != nil {
		return nil, fmt.Errorf("serviceclient[%s]: new request: %w", c.name, err)
	}
	forwardHeaders(ctx, req)
	return req, nil
}

// newJSONRequest creates a JSON request with forwarded headers.
func (c *Client) newJSONRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("serviceclient[%s]: marshal body: %w", c.name, err)
	}
	req, err := c.newRequest(ctx, method, path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
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
// If the upstream service returns a non-zero business code, it returns
// an *errors.Error with the upstream code and message.
//
// body is optional (pass nil for GET/DELETE).
// result receives the unwrapped "data" field (pass nil to ignore).
func Do[T any](ctx context.Context, c *Client, method, path string, body any, result *T) error {
	var resp *httpclient.Response
	var err error

	switch method {
	case http.MethodGet:
		resp, err = c.Get(ctx, path)
	case http.MethodPost:
		resp, err = c.PostJSON(ctx, path, body)
	case http.MethodPut:
		resp, err = c.PutJSON(ctx, path, body)
	case http.MethodPatch:
		resp, err = c.PatchJSON(ctx, path, body)
	case http.MethodDelete:
		resp, err = c.Delete(ctx, path)
	default:
		return fmt.Errorf("serviceclient[%s]: unsupported method: %s", c.name, method)
	}
	if err != nil {
		return err
	}

	// Non-2xx HTTP status (that wasn't retried away by httpclient).
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New(errors.Code(resp.StatusCode),
			fmt.Sprintf("serviceclient[%s]: HTTP %d", c.name, resp.StatusCode))
	}

	var envelope serviceResponse
	if err := json.Unmarshal(resp.Bytes(), &envelope); err != nil {
		return fmt.Errorf("serviceclient[%s]: unmarshal response: %w", c.name, err)
	}

	// Non-zero business code → upstream business error.
	if envelope.Code != 0 {
		return errors.New(errors.Code(envelope.Code), envelope.Message)
	}

	if result != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return fmt.Errorf("serviceclient[%s]: unmarshal data: %w", c.name, err)
		}
	}
	return nil
}

// GetJSON is a convenience for Do with GET method.
func GetJSON[T any](ctx context.Context, c *Client, path string, result *T) error {
	return Do[T](ctx, c, http.MethodGet, path, nil, result)
}

// PostJSON2 is a convenience for Do with POST method.
// Named PostJSON2 to avoid conflict with Client.PostJSON.
func PostJSON2[T any](ctx context.Context, c *Client, path string, body any, result *T) error {
	return Do[T](ctx, c, http.MethodPost, path, body, result)
}

// PutJSON2 is a convenience for Do with PUT method.
func PutJSON2[T any](ctx context.Context, c *Client, path string, body any, result *T) error {
	return Do[T](ctx, c, http.MethodPut, path, body, result)
}

// DeleteJSON is a convenience for Do with DELETE method.
func DeleteJSON[T any](ctx context.Context, c *Client, path string, result *T) error {
	return Do[T](ctx, c, http.MethodDelete, path, nil, result)
}
