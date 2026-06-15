package omni

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newServer returns a test server that responds to each path with the canned
// body from routes (an exact path match), defaulting to 404 otherwise.
func newServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range routes {
		b := body
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(b))
		})
	}
	return httptest.NewServer(mux)
}

func TestNewValidation(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Error("empty baseURL should error")
	}
	if _, err := New("notaurl"); err == nil {
		t.Error("schemeless baseURL should error")
	}
	c, err := New("http://localhost:9090/")
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "http://localhost:9090" {
		t.Errorf("trailing slash not trimmed: %q", c.baseURL)
	}
}

func TestGetReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"parse error: unexpected token"}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL)
	err := c.get(context.Background(), "/api/v1/query", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Type != "bad_data" || apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("APIError = %+v", apiErr)
	}
	if apiErr.Error() == "" {
		t.Error("APIError.Error() should be non-empty")
	}
}

func TestContextCancellation(t *testing.T) {
	srv := newServer(t, map[string]string{"/api/v1/labels": `{"status":"success","data":[]}`})
	defer srv.Close()
	c, _ := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.LabelNames(ctx); err == nil {
		t.Error("cancelled context should produce an error")
	}
}
