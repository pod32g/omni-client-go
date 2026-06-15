package omni

import (
	"sort"
	"strings"
	"sync"
)

// Registry is a minimal in-process metric store for a process that pushes instead
// of being scraped: accumulate counters with Add and set gauges with Set, then
// hand Series to a PushRequest. It is safe for concurrent use.
//
// A metric is identified by its name plus its label set, so the same name with
// different labels is tracked as distinct series.
type Registry struct {
	mu      sync.Mutex
	metrics map[string]*regEntry
}

type regEntry struct {
	name   string
	labels map[string]string
	value  float64
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{metrics: map[string]*regEntry{}}
}

// Add increments the counter identified by name and labels by delta (creating it
// at zero first). Use for counters.
func (r *Registry) Add(name string, labels map[string]string, delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entryLocked(name, labels).value += delta
}

// Set assigns the value of the gauge identified by name and labels. Use for
// gauges.
func (r *Registry) Set(name string, labels map[string]string, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entryLocked(name, labels).value = value
}

// Series returns an independent snapshot of every metric as PushSeries, sorted by
// name and labels for deterministic output. Labels and values are copied, so the
// result is safe to hand to Push while the registry continues to be updated.
func (r *Registry) Series() []PushSeries {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.metrics))
	for k := range r.metrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]PushSeries, 0, len(r.metrics))
	for _, k := range keys {
		e := r.metrics[k]
		v := e.value
		out = append(out, PushSeries{Name: e.name, Labels: copyLabels(e.labels), Value: &v})
	}
	return out
}

// entryLocked finds or creates the entry for name+labels. Caller holds mu.
func (r *Registry) entryLocked(name string, labels map[string]string) *regEntry {
	key := metricKey(name, labels)
	e := r.metrics[key]
	if e == nil {
		e = &regEntry{name: name, labels: copyLabels(labels)}
		r.metrics[key] = e
	}
	return e
}

// copyLabels returns a copy of labels, or nil when empty, so neither the caller's
// map nor the registry's internal map can be mutated through the other.
func copyLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	return cp
}

// metricKey builds a stable identity from a name and its (order-independent)
// labels.
func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	for _, k := range keys {
		b.WriteByte(0xff)
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}
