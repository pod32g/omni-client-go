package omni

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Finding #1: option order must not matter, and WithTimeout must never mutate a
// caller-supplied http.Client.
func TestOptionsOrderIndependentAndNoSharedMutation(t *testing.T) {
	shared := &http.Client{}

	c1, _ := New("http://h:9090", WithTimeout(7*time.Second), WithHTTPClient(shared))
	if c1.httpClient.Timeout != 7*time.Second {
		t.Errorf("timeout dropped when WithTimeout precedes WithHTTPClient: %v", c1.httpClient.Timeout)
	}

	c2, _ := New("http://h:9090", WithHTTPClient(shared), WithTimeout(5*time.Second))
	if c2.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout not applied: %v", c2.httpClient.Timeout)
	}

	if shared.Timeout != 0 {
		t.Errorf("caller's shared client was mutated: Timeout=%v", shared.Timeout)
	}
	if c1.httpClient == shared || c2.httpClient == shared {
		t.Error("client stored caller's pointer instead of a copy")
	}
}

// Finding #2: any non-2xx response must surface as a typed *APIError carrying the
// status code, even when the body is not the JSON envelope.
func TestNon2xxNonJSONIsAPIError(t *testing.T) {
	cases := []struct {
		code int
		body string
	}{
		{http.StatusBadGateway, "<html>502 Bad Gateway</html>"},
		{http.StatusUnauthorized, "Unauthorized"},
		{http.StatusServiceUnavailable, ""},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.code)
			_, _ = w.Write([]byte(tc.body))
		}))
		c, _ := New(srv.URL)
		err := c.get(context.Background(), "/x", nil, nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Errorf("code %d: want *APIError, got %T: %v", tc.code, err, err)
		} else if apiErr.StatusCode != tc.code {
			t.Errorf("code %d: APIError.StatusCode = %d", tc.code, apiErr.StatusCode)
		}
		srv.Close()
	}
}

// Finding #6: an over-large body is an explicit error, not a confusing decode failure.
func TestOversizedBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), 1000))
	}))
	defer srv.Close()
	c, _ := New(srv.URL)
	c.maxBody = 100 // far below the response
	err := c.get(context.Background(), "/x", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Message, "exceeds") {
		t.Errorf("oversized body should be an explicit *APIError, got %T: %v", err, err)
	}
}

// Finding #5: a baseURL with a query/fragment must not corrupt request URLs.
func TestNewNormalizesBaseURL(t *testing.T) {
	c, err := New("http://h:9090/prefix?x=1#frag")
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "http://h:9090/prefix" {
		t.Errorf("baseURL = %q, want query/fragment stripped and path kept", c.baseURL)
	}
	if _, err := New("ftp://h:9090"); err == nil {
		t.Error("non-http(s) scheme should be rejected")
	}
}

// Finding #3: QueryRange must reject zero or inverted time bounds client-side.
func TestQueryRangeValidation(t *testing.T) {
	c, _ := New("http://h:9090")
	ctx := context.Background()
	if _, err := c.QueryRange(ctx, "up", time.Time{}, time.Unix(1, 0), time.Second); err == nil {
		t.Error("zero start should error")
	}
	if _, err := c.QueryRange(ctx, "up", time.Unix(1, 0), time.Time{}, time.Second); err == nil {
		t.Error("zero end should error")
	}
	if _, err := c.QueryRange(ctx, "up", time.Unix(2, 0), time.Unix(1, 0), time.Second); err == nil {
		t.Error("end before start should error")
	}
}

// Finding #4: formatTime must not cap precision at milliseconds.
func TestFormatTimeNotMillisecondCapped(t *testing.T) {
	got := formatTime(time.Unix(100, 123456000))
	if got == "100.123" {
		t.Errorf("formatTime still capped at ms: %q", got)
	}
	f, err := strconv.ParseFloat(got, 64)
	if err != nil || math.Abs(f-100.123456) > 1e-6 {
		t.Errorf("formatTime = %q (parsed %v), want ~100.123456", got, f)
	}
}
