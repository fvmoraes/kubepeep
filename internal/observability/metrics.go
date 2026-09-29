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
	"time"
)

// Registry stores monotonically increasing counters and last-value gauges.
// It is safe for concurrent use.
type Registry struct {
	mu        sync.Mutex
	counters  map[string]map[string]uint64
	gauges    map[string]map[string]int64
	durations map[string]map[string]*durationWindow
	startedAt time.Time
}

const maximumDurationSamples = 512

type durationWindow struct {
	values []int64
	next   int
}

// MetricSeries is a cardinality-bounded, safe snapshot of one metric series.
// Labels are restricted by the same allowlist used by Render.
type MetricSeries[T int64 | uint64] struct {
	Labels map[string]string
	Value  T
}

type DurationSeries struct {
	Labels map[string]string
	Count  int
	P50    time.Duration
	P95    time.Duration
	P99    time.Duration
	Max    time.Duration
	// Samples is a copied, bounded window used to merge percentiles across
	// safe metric series in the local diagnostics endpoint.
	Samples []time.Duration
}

type Snapshot struct {
	StartedAt time.Time
	Counters  map[string][]MetricSeries[uint64]
	Gauges    map[string][]MetricSeries[int64]
	Durations map[string][]DurationSeries
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:  make(map[string]map[string]uint64),
		gauges:    make(map[string]map[string]int64),
		durations: make(map[string]map[string]*durationWindow),
		startedAt: time.Now().UTC(),
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
	ResourceCacheHitsTotalName:                    {},
	ResourceCacheMissesTotalName:                  {},
	ResourceCacheEvictionsTotalName:               {},
	ResourceCacheInvalidationsTotalName:           {},
	CollectionCacheHitsTotalName:                  {},
	CollectionCacheMissesTotalName:                {},
	CollectionCacheEvictionsTotalName:             {},
	CollectionCacheInvalidationsTotalName:         {},
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
	CursorEntriesName: {}, CursorBytesName: {}, ResourceCacheEntriesName: {}, ResourceCacheBytesName: {}, CollectionCacheEntriesName: {}, CollectionCacheBytesName: {}, WatchActiveName: {}, WatchLagMillisecondsName: {},
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

// ObserveDuration retains a small rolling window for local diagnostics. Only
// metrics and labels already admitted by the bounded registry are accepted;
// resource names, namespaces, selectors and user identities cannot enter it.
func (registry *Registry) ObserveDuration(name string, labels map[string]string, value time.Duration) {
	if registry == nil || value <= 0 {
		return
	}
	if name != ResourceListDurationNanosecondsTotalName && name != KubernetesRequestDurationNanosecondsTotalName {
		return
	}
	key := labelKey(labels)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	bucket := registry.durations[name]
	if bucket == nil {
		bucket = make(map[string]*durationWindow)
		registry.durations[name] = bucket
	}
	window := bucket[key]
	if window == nil {
		if len(bucket) >= 1024 {
			return
		}
		window = &durationWindow{values: make([]int64, 0, maximumDurationSamples)}
		bucket[key] = window
	}
	nanoseconds := value.Nanoseconds()
	if len(window.values) < maximumDurationSamples {
		window.values = append(window.values, nanoseconds)
		return
	}
	window.values[window.next] = nanoseconds
	window.next = (window.next + 1) % maximumDurationSamples
}

// Snapshot returns copied aggregate data for the local diagnostics API.
func (registry *Registry) Snapshot() Snapshot {
	result := Snapshot{
		Counters:  make(map[string][]MetricSeries[uint64]),
		Gauges:    make(map[string][]MetricSeries[int64]),
		Durations: make(map[string][]DurationSeries),
	}
	if registry == nil {
		return result
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	result.StartedAt = registry.startedAt
	for name, bucket := range registry.counters {
		for key, value := range bucket {
			result.Counters[name] = append(result.Counters[name], MetricSeries[uint64]{Labels: decodeLabelKey(key), Value: value})
		}
	}
	for name, bucket := range registry.gauges {
		for key, value := range bucket {
			result.Gauges[name] = append(result.Gauges[name], MetricSeries[int64]{Labels: decodeLabelKey(key), Value: value})
		}
	}
	for name, bucket := range registry.durations {
		for key, window := range bucket {
			values := append([]int64(nil), window.values...)
			if len(values) == 0 {
				continue
			}
			sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
			samples := make([]time.Duration, len(values))
			for index, value := range values {
				samples[index] = time.Duration(value)
			}
			result.Durations[name] = append(result.Durations[name], DurationSeries{
				Labels: decodeLabelKey(key), Count: len(values),
				P50: percentileDuration(values, 0.50), P95: percentileDuration(values, 0.95),
				P99: percentileDuration(values, 0.99), Max: time.Duration(values[len(values)-1]), Samples: samples,
			})
		}
	}
	return result
}

func percentileDuration(sorted []int64, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1)*percentile + 0.5)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return time.Duration(sorted[index])
}

func decodeLabelKey(key string) map[string]string {
	labels := make(map[string]string)
	if key == "" {
		return labels
	}
	for _, pair := range strings.Split(key, "\x1f") {
		name, value, ok := strings.Cut(pair, "=")
		if ok {
			labels[name] = value
		}
	}
	return labels
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
	ResourceListItemsReceivedTotalName    = "kubepeep_resource_list_items_received_total"
	ResourceListItemsReturnedTotalName    = "kubepeep_resource_list_items_returned_total"
	ResourceListsTotalName                = "kubepeep_resource_lists_total"
	CursorEntriesName                     = "kubepeep_cursor_entries"
	CursorBytesName                       = "kubepeep_cursor_bytes"
	CursorHitsTotalName                   = "kubepeep_cursor_hits_total"
	CursorMissesTotalName                 = "kubepeep_cursor_misses_total"
	CursorExpiredTotalName                = "kubepeep_cursor_expired_total"
	CursorEvictedTotalName                = "kubepeep_cursor_evicted_total"
	ResourceCacheEntriesName              = "kubepeep_resource_cache_entries"
	ResourceCacheBytesName                = "kubepeep_resource_cache_bytes"
	ResourceCacheHitsTotalName            = "kubepeep_resource_cache_hits_total"
	ResourceCacheMissesTotalName          = "kubepeep_resource_cache_misses_total"
	ResourceCacheEvictionsTotalName       = "kubepeep_resource_cache_evictions_total"
	ResourceCacheInvalidationsTotalName   = "kubepeep_resource_cache_invalidations_total"
	CollectionCacheEntriesName            = "kubepeep_collection_cache_entries"
	CollectionCacheBytesName              = "kubepeep_collection_cache_bytes"
	CollectionCacheHitsTotalName          = "kubepeep_collection_cache_hits_total"
	CollectionCacheMissesTotalName        = "kubepeep_collection_cache_misses_total"
	CollectionCacheEvictionsTotalName     = "kubepeep_collection_cache_evictions_total"
	CollectionCacheInvalidationsTotalName = "kubepeep_collection_cache_invalidations_total"
	WatchActiveName                       = "kubepeep_watch_active"
	WatchReconnectsTotalName              = "kubepeep_watch_reconnects_total"
	WatchExpiredTotalName                 = "kubepeep_watch_expired_total"
	WatchEventsTotalName                  = "kubepeep_watch_events_total"
	WatchLagMillisecondsName              = "kubepeep_watch_lag_milliseconds"
	KubernetesRequestsTotalName           = "kubepeep_kubernetes_requests_total"
	ClientThrottleTotalName               = "kubepeep_client_throttle_total"
	ClientThrottleNanosecondsTotalName    = "kubepeep_client_throttle_nanoseconds_total"
	TraceExportErrorsTotalName            = "kubepeep_trace_export_errors_total"
)
