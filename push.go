package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"
)

// PushRequest is one push of samples to POST /api/v1/push. Each push appends new
// samples (building a time series), so rate() works on pushed counters.
type PushRequest struct {
	Job      string // required
	Instance string // optional; the server defaults it to the caller's remote host
	Series   []PushSeries
}

// PushSeries is one metric in a push: a name, optional extra labels, and either a
// single Value (a sample stamped at receive time) or explicit Samples — set
// exactly one. The server injects job/instance and ignores any reserved label
// (__name__, job, instance) supplied here.
type PushSeries struct {
	Name    string
	Labels  map[string]string
	Value   *float64
	Samples []PushSample
}

// PushSample is one explicit observation. A zero Timestamp is omitted so the
// server stamps it at receive time.
type PushSample struct {
	Timestamp time.Time
	Value     float64
}

// PushResult reports what the server appended.
type PushResult struct {
	SamplesAppended int `json:"samplesAppended"`
	SeriesTouched   int `json:"seriesTouched"`
}

// Push sends req to the server's /api/v1/push endpoint. It validates the request
// locally (job and ≥1 series required; each series needs a name and exactly one
// of Value or Samples) before sending, and returns the server's append counts or
// a typed *APIError.
func (c *Client) Push(ctx context.Context, req *PushRequest) (*PushResult, error) {
	if req == nil {
		return nil, fmt.Errorf("omni: push request is nil")
	}
	if req.Job == "" {
		return nil, fmt.Errorf("omni: push job is required")
	}
	if len(req.Series) == 0 {
		return nil, fmt.Errorf("omni: push requires at least one series")
	}
	wire := wireRequest{Job: req.Job, Instance: req.Instance, Series: make([]wireSeries, 0, len(req.Series))}
	for i, s := range req.Series {
		if s.Name == "" {
			return nil, fmt.Errorf("omni: series[%d]: name is required", i)
		}
		hasValue := s.Value != nil
		hasSamples := len(s.Samples) > 0
		if hasValue == hasSamples {
			return nil, fmt.Errorf("omni: series[%d] %q: set exactly one of Value or Samples", i, s.Name)
		}
		ws := wireSeries{Name: s.Name, Labels: s.Labels}
		if hasValue {
			v := wireValue(*s.Value)
			ws.Value = &v
		} else {
			ws.Samples = make([]wireSample, len(s.Samples))
			for j, sp := range s.Samples {
				var ms int64
				if !sp.Timestamp.IsZero() {
					ms = sp.Timestamp.UnixMilli()
				}
				ws.Samples[j] = wireSample{TimestampMs: ms, Value: wireValue(sp.Value)}
			}
		}
		wire.Series = append(wire.Series, ws)
	}
	var out PushResult
	if err := c.post(ctx, "/api/v1/push", wire, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// post marshals in as JSON, POSTs it (with the push bearer token when set), and
// decodes the {status} envelope's data into out.
func (c *Client) post(ctx context.Context, path string, in, out interface{}) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("omni: encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.pushToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.pushToken)
	}
	return c.do(req, out)
}

// --- wire types ---

type wireRequest struct {
	Job      string       `json:"job"`
	Instance string       `json:"instance,omitempty"`
	Series   []wireSeries `json:"series"`
}

type wireSeries struct {
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels,omitempty"`
	Value   *wireValue        `json:"value,omitempty"`
	Samples []wireSample      `json:"samples,omitempty"`
}

type wireSample struct {
	TimestampMs int64     `json:"timestamp_ms,omitempty"`
	Value       wireValue `json:"value"`
}

// wireValue marshals a float as a JSON number, or as the strings "NaN"/"+Inf"/
// "-Inf" for non-finite values (which JSON numbers cannot represent and which the
// server decodes back to the corresponding float).
type wireValue float64

func (v wireValue) MarshalJSON() ([]byte, error) {
	f := float64(v)
	switch {
	case math.IsNaN(f):
		return []byte(`"NaN"`), nil
	case math.IsInf(f, 1):
		return []byte(`"+Inf"`), nil
	case math.IsInf(f, -1):
		return []byte(`"-Inf"`), nil
	default:
		return json.Marshal(f)
	}
}
