package omni

import (
	"sync"
	"testing"
)

// findSeries returns the Value of the series with the given name and single
// optional label, or nil if absent.
func findSeries(series []PushSeries, name string, labelKey, labelVal string) *float64 {
	for _, s := range series {
		if s.Name != name {
			continue
		}
		if labelKey == "" {
			if len(s.Labels) == 0 {
				return s.Value
			}
			continue
		}
		if s.Labels[labelKey] == labelVal {
			return s.Value
		}
	}
	return nil
}

func TestRegistryAddAndSet(t *testing.T) {
	r := NewRegistry()
	r.Add("hits_total", nil, 1)
	r.Add("hits_total", nil, 2) // accumulates
	r.Set("temp", map[string]string{"room": "a"}, 21.5)
	r.Set("temp", map[string]string{"room": "a"}, 22.0) // overwrites
	r.Add("hits_total", map[string]string{"path": "/x"}, 5)

	series := r.Series()
	if len(series) != 3 {
		t.Fatalf("series = %d, want 3", len(series))
	}
	if v := findSeries(series, "hits_total", "", ""); v == nil || *v != 3 {
		t.Errorf("hits_total{} = %v, want 3", v)
	}
	if v := findSeries(series, "temp", "room", "a"); v == nil || *v != 22.0 {
		t.Errorf("temp{room=a} = %v, want 22", v)
	}
	if v := findSeries(series, "hits_total", "path", "/x"); v == nil || *v != 5 {
		t.Errorf("hits_total{path=/x} = %v, want 5", v)
	}
}

func TestRegistrySeriesIsIndependentSnapshot(t *testing.T) {
	r := NewRegistry()
	r.Set("g", nil, 1)
	series := r.Series()
	r.Set("g", nil, 99) // mutate after snapshot
	if v := findSeries(series, "g", "", ""); v == nil || *v != 1 {
		t.Errorf("snapshot must not change after registry mutation: got %v", v)
	}
}

func TestRegistryLabelCopyIsolation(t *testing.T) {
	r := NewRegistry()
	labels := map[string]string{"k": "v1"}
	r.Set("g", labels, 1)
	labels["k"] = "v2" // caller mutates their map after the call
	series := r.Series()
	if findSeries(series, "g", "k", "v1") == nil {
		t.Errorf("registry must snapshot labels, not alias the caller's map")
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Add("c", nil, 1)
		}()
	}
	wg.Wait()
	if v := findSeries(r.Series(), "c", "", ""); v == nil || *v != 100 {
		t.Errorf("concurrent Add = %v, want 100", v)
	}
}
