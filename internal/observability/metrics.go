// Package observability provides the process-local metrics registry behind
// the optional /metrics endpoint. It has no external dependencies, never
// records payload or credential data, and renders the Prometheus text
// exposition format. Counters and gauges are bounded by their allowlisted
// metric names and label keys.
package observability

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Registry stores monotonically increasing counters and last-value gauges.
// It is safe for concurrent use.
type Registry struct {
	mu       sync.Mutex
	counters map[string]map[string]uint64
	gauges   map[string]map[string]int64
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]map[string]uint64),
		gauges:   make(map[string]map[string]int64),
	}
}

var allowedMetrics = map[string]struct{}{
	"kubepeep_requests_total": {},
}

var allowedLabels = map[string]struct{}{
	"method": {},
	"route":  {},
	"status": {},
}

// IncCounter increments an allowlisted counter by one for the given labels.
// Unknown metric or label names are ignored so caller mistakes can never
// grow unbounded cardinality.
func (registry *Registry) IncCounter(name string, labels map[string]string) {
	if _, ok := allowedMetrics[name]; !ok {
		return
	}
	key := labelKey(labels)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	bucket, ok := registry.counters[name]
	if !ok {
		bucket = make(map[string]uint64)
		registry.counters[name] = bucket
	}
	bucket[key]++
}

// SetGauge is reserved for allowlisted gauges; currently none are exposed, so
// unknown names are ignored.
func (registry *Registry) SetGauge(name string, labels map[string]string, value int64) {
	_ = name
	_ = labels
	_ = value
}

func labelKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		if _, ok := allowedLabels[key]; !ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, "\x1f")
}

// Render produces the Prometheus text exposition format. Output is empty when
// nothing was recorded.
func (registry *Registry) Render() string {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	var builder strings.Builder
	names := make([]string, 0, len(registry.counters))
	for name := range registry.counters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		bucket := registry.counters[name]
		labelTuples := make([][2]string, 0, len(bucket))
		for key := range bucket {
			labelTuples = append(labelTuples, [2]string{key, strconv.FormatUint(bucket[key], 10)})
		}
		sort.Slice(labelTuples, func(left, right int) bool { return labelTuples[left][0] < labelTuples[right][0] })
		builder.WriteString("# TYPE " + name + " counter\n")
		for _, tuple := range labelTuples {
			builder.WriteString(name)
			builder.WriteString(renderLabels(tuple[0]))
			builder.WriteString(" ")
			builder.WriteString(tuple[1])
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func renderLabels(key string) string {
	if key == "" {
		return ""
	}
	pairs := strings.Split(key, "\x1f")
	escaped := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		name, value, _ := strings.Cut(pair, "=")
		// %q already escapes quotes, backslashes and newlines in the exact
		// form the Prometheus text format expects.
		escaped = append(escaped, fmt.Sprintf("%s=%q", name, value))
	}
	return "{" + strings.Join(escaped, ",") + "}"
}

// RequestsTotalName is the allowlisted counter for local HTTP requests.
const RequestsTotalName = "kubepeep_requests_total"
