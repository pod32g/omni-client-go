package omni

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// pushEcho is a test server that captures the decoded push body and returns a
// success envelope with the given counts.
func pushEcho(t *testing.T, wantAuth string, captured *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/push" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Errorf("Authorization = %q, want %q", got, wantAuth)
		}
		body, _ := io.ReadAll(r.Body)
		if captured != nil {
			_ = json.Unmarshal(body, captured)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"samplesAppended":3,"seriesTouched":2}}`)
	}))
}

func TestPushSendsWireShape(t *testing.T) {
	var got map[string]any
	srv := pushEcho(t, "", &got)
	defer srv.Close()
	c, _ := New(srv.URL)

	v := 1500.0
	res, err := c.Push(context.Background(), &PushRequest{
		Job:      "batch",
		Instance: "worker-7",
		Series: []PushSeries{
			{Name: "records_total", Value: &v},
			{Name: "latency", Samples: []PushSample{{Timestamp: time.UnixMilli(1000), Value: 0.1}, {Value: 0.2}}},
		},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if res.SamplesAppended != 3 || res.SeriesTouched != 2 {
		t.Errorf("result = %+v", res)
	}
	if got["job"] != "batch" || got["instance"] != "worker-7" {
		t.Errorf("identity wrong: %v", got)
	}
	series := got["series"].([]any)
	if len(series) != 2 {
		t.Fatalf("series = %d, want 2", len(series))
	}
	s0 := series[0].(map[string]any)
	if s0["name"] != "records_total" || s0["value"].(float64) != 1500 {
		t.Errorf("series[0] = %v", s0)
	}
	s1 := series[1].(map[string]any)
	samples := s1["samples"].([]any)
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2", len(samples))
	}
	if samples[0].(map[string]any)["timestamp_ms"].(float64) != 1000 {
		t.Errorf("explicit timestamp not sent: %v", samples[0])
	}
	// A zero timestamp must be omitted so the server stamps it at receive time.
	if _, present := samples[1].(map[string]any)["timestamp_ms"]; present {
		t.Errorf("zero timestamp should be omitted, got %v", samples[1])
	}
}

func TestPushEncodesNonFinite(t *testing.T) {
	var got map[string]any
	srv := pushEcho(t, "", &got)
	defer srv.Close()
	c, _ := New(srv.URL)

	inf := math.Inf(1)
	nan := math.NaN()
	if _, err := c.Push(context.Background(), &PushRequest{Job: "j", Series: []PushSeries{
		{Name: "ceiling", Value: &inf},
		{Name: "missing", Value: &nan},
	}}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	series := got["series"].([]any)
	if series[0].(map[string]any)["value"] != "+Inf" {
		t.Errorf("+Inf not encoded as string: %v", series[0])
	}
	if series[1].(map[string]any)["value"] != "NaN" {
		t.Errorf("NaN not encoded as string: %v", series[1])
	}
}

func TestPushAuthHeader(t *testing.T) {
	srv := pushEcho(t, "Bearer s3cr3t", nil)
	defer srv.Close()
	c, _ := New(srv.URL, WithPushAuth("s3cr3t"))
	v := 1.0
	if _, err := c.Push(context.Background(), &PushRequest{Job: "j", Series: []PushSeries{{Name: "a", Value: &v}}}); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestPushClientValidation(t *testing.T) {
	c, _ := New("http://127.0.0.1:0")
	v := 1.0
	cases := []struct {
		name string
		req  *PushRequest
	}{
		{"nil", nil},
		{"empty job", &PushRequest{Series: []PushSeries{{Name: "a", Value: &v}}}},
		{"no series", &PushRequest{Job: "j"}},
		{"empty name", &PushRequest{Job: "j", Series: []PushSeries{{Value: &v}}}},
		{"both value and samples", &PushRequest{Job: "j", Series: []PushSeries{{Name: "a", Value: &v, Samples: []PushSample{{Value: 1}}}}}},
		{"neither", &PushRequest{Job: "j", Series: []PushSeries{{Name: "a"}}}},
	}
	for _, tc := range cases {
		if _, err := c.Push(context.Background(), tc.req); err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}
}

func TestPushSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"status":"error","errorType":"bad_data","error":"job must not be empty"}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL)
	v := 1.0
	_, err := c.Push(context.Background(), &PushRequest{Job: "j", Series: []PushSeries{{Name: "a", Value: &v}}})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Type != "bad_data" {
		t.Errorf("errorType = %q", apiErr.Type)
	}
}
