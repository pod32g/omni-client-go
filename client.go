// Package omni is a Go client for the omni-metrics HTTP API — a Prometheus-shaped
// metrics server. It wraps the /api/v1 query and metadata endpoints with typed
// results and errors.
//
// Create a client with New and call Query, QueryRange, Series, LabelNames,
// LabelValues, or Targets to read. To write, use Push (POST /api/v1/push) — for a
// process that has no HTTP server to be scraped — optionally building the payload
// from a Registry.
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

// defaultMaxBody bounds a response body to guard against an unbounded payload.
const defaultMaxBody = 64 << 20

// Client talks to an omni-metrics server. It is safe for concurrent use.
type Client struct {
	baseURL    string
	httpClient *http.Client
	maxBody    int64
	pushToken  string

	timeout    time.Duration
	hasTimeout bool
}

// Option configures a Client. Options are independent of order.
type Option func(*Client)

// WithHTTPClient sets the underlying http.Client (e.g. for custom transports,
// TLS, or proxies). The supplied client is shallow-copied, so the client library
// never mutates a value the caller may use elsewhere. A nil client is ignored.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			cp := *h
			c.httpClient = &cp
		}
	}
}

// WithTimeout sets the per-request timeout. It is applied to the client the
// library owns after all options run, so it is independent of option order and
// never mutates a client passed via WithHTTPClient.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.timeout = d
		c.hasTimeout = true
	}
}

// WithPushAuth sets a bearer token sent on push writes (POST /api/v1/push) as
// "Authorization: Bearer <token>". Read requests are unaffected. An empty token
// is ignored.
func WithPushAuth(token string) Option {
	return func(c *Client) { c.pushToken = token }
}

// New creates a Client for the server at baseURL (e.g. "http://localhost:9090").
// Any query string or fragment on baseURL is dropped; a path is kept (useful
// behind a reverse proxy).
func New(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("omni: baseURL is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("omni: invalid baseURL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("omni: baseURL scheme must be http or https, got %q", baseURL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("omni: baseURL must include a host, got %q", baseURL)
	}
	// Normalize: drop any query/fragment so request URLs (built by concatenation)
	// cannot be corrupted; keep scheme, userinfo, host, and path.
	u.RawQuery = ""
	u.Fragment = ""

	c := &Client{
		baseURL:    strings.TrimRight(u.String(), "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxBody:    defaultMaxBody,
	}
	for _, o := range opts {
		o(c)
	}
	if c.hasTimeout {
		c.httpClient.Timeout = c.timeout
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
	return c.do(req, out)
}

// do sends req, enforces the body limit, validates the {status} envelope, and
// unmarshals data into out (which may be nil). Shared by get and post.
func (c *Client) do(req *http.Request, out interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read one byte past the limit so an over-large body is detectable rather
	// than silently truncated into a confusing decode error.
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > c.maxBody {
		return &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("response exceeds %d byte limit", c.maxBody)}
	}

	var env apiResponse
	if err := json.Unmarshal(body, &env); err != nil {
		// A non-2xx response whose body is not the envelope (a proxy/gateway HTML
		// 502, a plain-text 401, an empty 503, ...) still surfaces as a typed
		// *APIError carrying the status, per the documented contract.
		if resp.StatusCode/100 != 2 {
			return &APIError{StatusCode: resp.StatusCode, Message: statusMessage(resp.StatusCode, body)}
		}
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
// Full precision (no artificial millisecond cap); float64 still bounds resolution
// to roughly microseconds at current epoch magnitudes.
func formatTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', -1, 64)
}

// statusMessage builds an APIError message for a non-envelope error response,
// preferring a trimmed body snippet and falling back to the HTTP status text.
func statusMessage(status int, body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return http.StatusText(status)
	}
	const max = 200
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
