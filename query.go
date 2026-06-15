package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Query evaluates expr at a single instant. A zero ts lets the server default to
// "now".
func (c *Client) Query(ctx context.Context, expr string, ts time.Time) (*QueryResult, error) {
	params := url.Values{"query": {expr}}
	if !ts.IsZero() {
		params.Set("time", formatTime(ts))
	}
	var qd queryData
	if err := c.get(ctx, "/api/v1/query", params, &qd); err != nil {
		return nil, err
	}
	return parseQueryData(&qd)
}

// QueryRange evaluates expr at each step over [start, end]. start and end are
// required (a range query needs a real window) and step must be positive.
func (c *Client) QueryRange(ctx context.Context, expr string, start, end time.Time, step time.Duration) (*QueryResult, error) {
	if step <= 0 {
		return nil, fmt.Errorf("omni: step must be positive")
	}
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("omni: start and end are required")
	}
	if end.Before(start) {
		return nil, fmt.Errorf("omni: end must not be before start")
	}
	params := url.Values{
		"query": {expr},
		"start": {formatTime(start)},
		"end":   {formatTime(end)},
		"step":  {strconv.FormatFloat(step.Seconds(), 'f', -1, 64)},
	}
	var qd queryData
	if err := c.get(ctx, "/api/v1/query_range", params, &qd); err != nil {
		return nil, err
	}
	return parseQueryData(&qd)
}

type queryData struct {
	ResultType ResultType      `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

type jsonSample struct {
	Metric Labels      `json:"metric"`
	Value  sampleValue `json:"value"`
}

type jsonSeries struct {
	Metric Labels        `json:"metric"`
	Values []sampleValue `json:"values"`
}

func parseQueryData(qd *queryData) (*QueryResult, error) {
	res := &QueryResult{Type: qd.ResultType}
	switch qd.ResultType {
	case ResultVector:
		var raw []jsonSample
		if err := json.Unmarshal(qd.Result, &raw); err != nil {
			return nil, fmt.Errorf("omni: decoding vector: %w", err)
		}
		res.Vector = make([]Sample, 0, len(raw))
		for _, s := range raw {
			res.Vector = append(res.Vector, Sample{Metric: s.Metric, Timestamp: s.Value.T, Value: s.Value.V})
		}
	case ResultMatrix:
		var raw []jsonSeries
		if err := json.Unmarshal(qd.Result, &raw); err != nil {
			return nil, fmt.Errorf("omni: decoding matrix: %w", err)
		}
		res.Matrix = make([]Series, 0, len(raw))
		for _, s := range raw {
			ser := Series{Metric: s.Metric, Points: make([]Point, 0, len(s.Values))}
			for _, v := range s.Values {
				ser.Points = append(ser.Points, Point{Timestamp: v.T, Value: v.V})
			}
			res.Matrix = append(res.Matrix, ser)
		}
	case ResultScalar:
		var v sampleValue
		if err := json.Unmarshal(qd.Result, &v); err != nil {
			return nil, fmt.Errorf("omni: decoding scalar: %w", err)
		}
		res.Scalar = &Scalar{Timestamp: v.T, Value: v.V}
	default:
		return nil, fmt.Errorf("omni: unknown result type %q", qd.ResultType)
	}
	return res, nil
}
