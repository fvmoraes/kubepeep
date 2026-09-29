package main

import (
	"bytes"
	"testing"
	"time"
)

func TestPercentileUsesNearestRankIndex(t *testing.T) {
	t.Parallel()
	values := []time.Duration{5 * time.Millisecond, time.Millisecond, 3 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	if got := percentile(values, 0.50); got != 3 {
		t.Fatalf("p50 = %v", got)
	}
	if got := percentile(values, 0.95); got != 5 {
		t.Fatalf("p95 = %v", got)
	}
}

func TestShapedRemoteCompressionAndAIMDComparisons(t *testing.T) {
	payload := bytes.Repeat([]byte(`{"apiVersion":"v1","kind":"Pod"}`), 4_000)
	plain, compressed, err := benchmarkShapedRemote(payload, 3)
	if err != nil {
		t.Fatal(err)
	}
	if compressed.WireBytes >= plain.WireBytes || compressed.P95MS >= plain.P95MS {
		t.Fatalf("compression comparison plain=%#v gzip=%#v", plain, compressed)
	}

	fixed := benchmarkAIMD(t.Context(), false)
	adaptive := benchmarkAIMD(t.Context(), true)
	if fixed.HTTP429 == 0 || adaptive.HTTP429 >= fixed.HTTP429 || adaptive.Reductions == 0 || adaptive.Increases == 0 {
		t.Fatalf("AIMD comparison fixed=%#v adaptive=%#v", fixed, adaptive)
	}
}

func TestPrioritySchedulerProtectsVisibleWork(t *testing.T) {
	t.Parallel()
	result := benchmarkPrefetch(t.Context())
	if result.Deferred != 1 || result.PrioritizedVisibleMS >= result.NaiveVisibleMS {
		t.Fatalf("prefetch comparison = %#v", result)
	}
}
