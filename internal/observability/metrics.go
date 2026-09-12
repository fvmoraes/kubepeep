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

const (
	ResourceListDurationNanosecondsTotalName      = "kubepeep_resource_list_duration_nanoseconds_total"
	KubernetesRequestDurationNanosecondsTotalName = "kubepeep_kubernetes_request_duration_nanoseconds_total"
	KubernetesResponseBytesTotalName              = "kubepeep_kubernetes_response_bytes_total"
	Kubernetes429TotalName                        = "kubepeep_kubernetes_429_total"
)

var allowedMetrics = map[string]struct{}{
	"kubepeep_requests_total":                     {},
	"kubepeep_resource_list_items_received_total": {},
	"kubepeep_resource_list_items_returned_total": {},
	ResourceListsTotalName:                        {},
	ResourceListDurationNanosecondsTotalName:      {},
	CursorHitsTotalName:                           {},
	CursorMissesTotalName:                         {},
	CursorExpiredTotalName:                        {},
	CursorEvictedTotalName:                        {},
	WatchReconnectsTotalName:                      {},
	WatchExpiredTotalName:                         {},
	WatchEventsTotalName:                          {},
	KubernetesRequestsTotalName:                   {},
	KubernetesRequestDurationNanosecondsTotalName: {},
	KubernetesResponseBytesTotalName:              {},
	Kubernetes429TotalName:                        {},
	ClientThrottleTotalName:                       {},
	ClientThrottleNanosecondsTotalName:            {},
	TraceExportErrorsTotalName:                    {},
}

var allowedGauges = map[string]struct{}{
	CursorEntriesName: {}, CursorBytesName: {}, WatchActiveName: {}, WatchLagMillisecondsName: {},
}

var allowedLabels = map[string]struct{}{
	"method":   {},
	"route":    {},
	"status":   {},
	"resource": {},
	"strategy": {},
	"traffic":  {},
}

// IncCounter increments an allowlisted counter by one for the given labels.
// Unknown metric or label names are ignored so caller mistakes can never
// grow unbounded cardinality.
func (registry *Registry) IncCounter(name string, labels map[string]string) {
	registry.AddCounter(name, labels, 1)
}

// AddCounter adds delta to an allowlisted counter for the given labels.
// Unknown metric or label names are ignored so caller mistakes can never
// grow unbounded cardinality.
func (registry *Registry) AddCounter(name string, labels map[string]string, delta uint64) {
	if registry == nil {
		return
	}
	if _, ok := allowedMetrics[name]; !ok || delta == 0 {
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
	if _, exists := bucket[key]; exists || len(bucket) < 1024 {
		bucket[key] += delta
	}
}

// SetGauge stores the latest observation for an allowlisted gauge.
func (registry *Registry) SetGauge(name string, labels map[string]string, value int64) {
	registry.updateGauge(name, labels, value, false)
}

// AddGauge adjusts a concurrent activity gauge without losing updates.
func (registry *Registry) AddGauge(name string, labels map[string]string, delta int64) {
	registry.updateGauge(name, labels, delta, true)
}

func (registry *Registry) updateGauge(name string, labels map[string]string, value int64, add bool) {
	if registry == nil {
		return
	}
	if _, ok := allowedGauges[name]; !ok {
		return
	}
	key := labelKey(labels)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	bucket := registry.gauges[name]
	if bucket == nil {
		bucket = make(map[string]int64)
		registry.gauges[name] = bucket
	}
	if _, exists := bucket[key]; !exists && len(bucket) >= 1024 {
		return
	}
	if add {
		bucket[key] += value
	} else {
		bucket[key] = value
	}
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
	for name := range registry.gauges {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		bucket := registry.counters[name]
		labelTuples := make([][2]string, 0, len(bucket))
		for key := range bucket {
			labelTuples = append(labelTuples, [2]string{key, strconv.FormatUint(bucket[key], 10)})
		}
		kind := "counter"
		if gauges, ok := registry.gauges[name]; ok {
			kind = "gauge"
			for key, value := range gauges {
				labelTuples = append(labelTuples, [2]string{key, strconv.FormatInt(value, 10)})
			}
		}
		sort.Slice(labelTuples, func(left, right int) bool { return labelTuples[left][0] < labelTuples[right][0] })
		builder.WriteString("# TYPE " + name + " " + kind + "\n")
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

// Resource list over-fetch instrumentation: items received from Kubernetes
// versus items returned to the UI. received/returned is the over-fetch ratio.
const (
	ResourceListItemsReceivedTotalName = "kubepeep_resource_list_items_received_total"
	ResourceListItemsReturnedTotalName = "kubepeep_resource_list_items_returned_total"
	ResourceListsTotalName             = "kubepeep_resource_lists_total"
	CursorEntriesName                  = "kubepeep_cursor_entries"
	CursorBytesName                    = "kubepeep_cursor_bytes"
	CursorHitsTotalName                = "kubepeep_cursor_hits_total"
	CursorMissesTotalName              = "kubepeep_cursor_misses_total"
	CursorExpiredTotalName             = "kubepeep_cursor_expired_total"
	CursorEvictedTotalName             = "kubepeep_cursor_evicted_total"
	WatchActiveName                    = "kubepeep_watch_active"
	WatchReconnectsTotalName           = "kubepeep_watch_reconnects_total"
	WatchExpiredTotalName              = "kubepeep_watch_expired_total"
	WatchEventsTotalName               = "kubepeep_watch_events_total"
	WatchLagMillisecondsName           = "kubepeep_watch_lag_milliseconds"
	KubernetesRequestsTotalName        = "kubepeep_kubernetes_requests_total"
	ClientThrottleTotalName            = "kubepeep_client_throttle_total"
	ClientThrottleNanosecondsTotalName = "kubepeep_client_throttle_nanoseconds_total"
	TraceExportErrorsTotalName         = "kubepeep_trace_export_errors_total"
)
