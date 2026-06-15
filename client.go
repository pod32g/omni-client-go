// Package omni is a Go client for the omni-metrics HTTP API — a Prometheus-shaped
// metrics server. It wraps the /api/v1 query and metadata endpoints with typed
// results and errors.
//
// omni-metrics is pull-based: this client reads from a running server; it does
// not push samples. Create a client with New and call Query, QueryRange, Series,
// LabelNames, LabelValues, or Targets.
package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to an omni-metrics server. It is safe for concurrent use.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the underlying http.Client (e.g. for custom transports,
// TLS, or proxies). A nil client is ignored.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithTimeout sets the per-request timeout on the default http.Client.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// New creates a Client for the server at baseURL (e.g. "http://localhost:9090").
func New(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("omni: baseURL is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("omni: invalid baseURL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("omni: baseURL must include a scheme and host, got %q", baseURL)
	}
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// APIError is returned when the server replies with a {status:"error"} envelope
// or a non-2xx status.
type APIError struct {
	StatusCode int
	Type       string // the server's errorType, e.g. "bad_data"
	Message    string
}

func (e *APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("omni: api error (%s): %s", e.Type, e.Message)
	}
	return fmt.Sprintf("omni: api error (HTTP %d): %s", e.StatusCode, e.Message)
}

// apiResponse is the Prometheus-compatible envelope.
type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
}

// get performs a GET, validates the {status} envelope, and unmarshals data into
// out (which may be nil).
func (c *Client) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}

	var env apiResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("omni: decoding response (HTTP %d): %w", resp.StatusCode, err)
	}
	if env.Status != "success" {
		return &APIError{StatusCode: resp.StatusCode, Type: env.ErrorType, Message: env.Error}
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("omni: decoding data: %w", err)
		}
	}
	return nil
}

// formatTime renders a time as fractional Unix seconds, the form the API parses.
func formatTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', 3, 64)
}
