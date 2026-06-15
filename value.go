package omni

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
)

// Labels is a metric's label set, including the reserved __name__ label.
type Labels map[string]string

// ResultType identifies the shape of a query result.
type ResultType string

const (
	ResultVector ResultType = "vector"
	ResultMatrix ResultType = "matrix"
	ResultScalar ResultType = "scalar"
)

// Sample is one element of an instant vector: a label set with a single value at
// an instant.
type Sample struct {
	Metric    Labels
	Timestamp time.Time
	Value     float64
}

// Point is a single (timestamp, value) pair within a range-vector series.
type Point struct {
	Timestamp time.Time
	Value     float64
}

// Series is one series of a range query: a label set with points over time.
type Series struct {
	Metric Labels
	Points []Point
}

// Scalar is a single numeric value at a timestamp.
type Scalar struct {
	Timestamp time.Time
	Value     float64
}

// QueryResult is the typed result of a query. Exactly one of Vector, Matrix, or
// Scalar is populated, per Type.
type QueryResult struct {
	Type   ResultType
	Vector []Sample
	Matrix []Series
	Scalar *Scalar
}

// sampleValue parses Prometheus' [<unix_seconds_float>, "<value_string>"] pair
// into a timestamp and a float (the string value parses "+Inf"/"-Inf"/"NaN").
type sampleValue struct {
	T time.Time
	V float64
}

func (s *sampleValue) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("omni: expected [timestamp, value], got %d elements", len(raw))
	}
	var ts float64
	if err := json.Unmarshal(raw[0], &ts); err != nil {
		return fmt.Errorf("omni: bad sample timestamp: %w", err)
	}
	var vs string
	if err := json.Unmarshal(raw[1], &vs); err != nil {
		return fmt.Errorf("omni: bad sample value: %w", err)
	}
	v, err := strconv.ParseFloat(vs, 64)
	if err != nil {
		return fmt.Errorf("omni: parsing sample value %q: %w", vs, err)
	}
	s.T = floatSecondsToTime(ts)
	s.V = v
	return nil
}

// floatSecondsToTime converts a fractional-seconds Unix timestamp to a UTC time.
func floatSecondsToTime(sec float64) time.Time {
	whole, frac := math.Modf(sec)
	return time.Unix(int64(whole), int64(math.Round(frac*1e9))).UTC()
}
