package kubernetesruntime

import (
	"math"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
)

func TestPerformanceDiagnosticsMergesRawBoundedSamples(t *testing.T) {
	t.Parallel()
	snapshot := observability.Snapshot{
		StartedAt: time.Now().Add(-time.Minute),
		Counters: map[string][]observability.MetricSeries[uint64]{
			observability.ResourceCacheHitsTotalName:   {{Value: 3}},
			observability.ResourceCacheMissesTotalName: {{Value: 1}},
			observability.KubernetesRequestsTotalName:  {{Value: 120}},
			observability.Kubernetes429TotalName:       {{Value: 2}},
		},
		Gauges: map[string][]observability.MetricSeries[int64]{
			observability.WatchActiveName: {{Value: 4}},
		},
		Durations: map[string][]observability.DurationSeries{
			observability.KubernetesRequestDurationNanosecondsTotalName: {
				{Samples: []time.Duration{time.Millisecond, 2 * time.Millisecond}},
				{Samples: []time.Duration{100 * time.Millisecond}},
			},
			observability.ResourceListDurationNanosecondsTotalName: {
				{Labels: map[string]string{"resource": "pods"}, Samples: []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}},
				{Labels: map[string]string{"resource": "pods"}, Samples: []time.Duration{15 * time.Millisecond}},
			},
		},
	}

	got := performanceDiagnostics(snapshot)
	if got.APIServerLatency.Samples != 3 || got.APIServerLatency.P50Milliseconds != 2 || got.APIServerLatency.P99Milliseconds != 100 {
		t.Fatalf("API latency = %#v", got.APIServerLatency)
	}
	if len(got.ResourceSync) != 1 || got.ResourceSync[0].Resource != "pods" || got.ResourceSync[0].Latency.P50Milliseconds != 10 {
		t.Fatalf("resource sync = %#v", got.ResourceSync)
	}
	if got.CacheHitRatio == nil || math.Abs(*got.CacheHitRatio-0.75) > 0.0001 || got.ActiveWatches != 4 || got.Responses429 != 2 || got.RequestsPerMinute < 100 {
		t.Fatalf("performance counters = %#v", got)
	}
}
