package omni

import (
	"math"
	"testing"
)

func TestSampleValueUnmarshal(t *testing.T) {
	cases := []struct {
		in      string
		wantV   float64
		wantSec int64
	}{
		{`[1781538033.843, "1"]`, 1, 1781538033},
		{`[100, "1.5"]`, 1.5, 100},
		{`[100, "+Inf"]`, math.Inf(1), 100},
		{`[100, "-Inf"]`, math.Inf(-1), 100},
		{`[100, "-0.5"]`, -0.5, 100},
	}
	for _, c := range cases {
		var s sampleValue
		if err := s.UnmarshalJSON([]byte(c.in)); err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if s.V != c.wantV {
			t.Errorf("%s value = %v, want %v", c.in, s.V, c.wantV)
		}
		if s.T.Unix() != c.wantSec {
			t.Errorf("%s ts = %d, want %d", c.in, s.T.Unix(), c.wantSec)
		}
	}

	var nan sampleValue
	if err := nan.UnmarshalJSON([]byte(`[1, "NaN"]`)); err != nil || !math.IsNaN(nan.V) {
		t.Errorf("NaN value not parsed: v=%v err=%v", nan.V, err)
	}

	var bad sampleValue
	if err := bad.UnmarshalJSON([]byte(`[1]`)); err == nil {
		t.Error("a 1-element array should error")
	}
	if err := bad.UnmarshalJSON([]byte(`[1, 2]`)); err == nil {
		t.Error("a numeric (non-string) value should error")
	}
}

func TestSampleValueSubSecond(t *testing.T) {
	var s sampleValue
	if err := s.UnmarshalJSON([]byte(`[1.5, "1"]`)); err != nil {
		t.Fatal(err)
	}
	if s.T.Nanosecond() != 500_000_000 {
		t.Errorf("sub-second timestamp lost: %d ns", s.T.Nanosecond())
	}
}
