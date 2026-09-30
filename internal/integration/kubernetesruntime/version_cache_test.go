package kubernetesruntime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func TestVersionCacheCoalescesSuccessfulChecksAndExpiresPerGeneration(t *testing.T) {
	now := time.Unix(100, 0)
	cache := newVersionCache(time.Minute, func() time.Time { return now })
	defer cache.Close()
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Cluster: "cluster", ActiveScopeID: 2, Generation: "gen-1"}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	load := func(context.Context) kubernetes.ConnectivityResult {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return kubernetes.ConnectivityResult{Status: kubernetes.ConnectivityHealthy, Code: "OK", Version: "v1.35.0"}
	}

	results := make(chan kubernetes.ConnectivityResult, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := cache.Check(t.Context(), binding, load)
			if err != nil {
				t.Errorf("check: %v", err)
				return
			}
			results <- result
		}()
	}
	<-started
	close(release)
	group.Wait()
	close(results)
	for result := range results {
		if result.Version != "v1.35.0" {
			t.Fatalf("version = %q", result.Version)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("coalesced calls = %d", calls.Load())
	}

	if _, err := cache.Check(t.Context(), binding, load); err != nil || calls.Load() != 1 {
		t.Fatalf("cache hit calls=%d err=%v", calls.Load(), err)
	}
	now = now.Add(time.Minute)
	if _, err := cache.Check(t.Context(), binding, load); err != nil || calls.Load() != 2 {
		t.Fatalf("expired calls=%d err=%v", calls.Load(), err)
	}

	binding.Generation = "gen-2"
	cache.InvalidateAll()
	if _, err := cache.Check(t.Context(), binding, load); err != nil || calls.Load() != 3 {
		t.Fatalf("generation calls=%d err=%v", calls.Load(), err)
	}
}

func TestVersionCacheDoesNotCacheConnectivityFailures(t *testing.T) {
	cache := newVersionCache(time.Minute, nil)
	defer cache.Close()
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	calls := 0
	load := func(context.Context) kubernetes.ConnectivityResult {
		calls++
		return kubernetes.ConnectivityResult{Status: kubernetes.ConnectivityDegraded, Code: kubernetes.CodeClusterUnavailable}
	}
	for range 2 {
		if _, err := cache.Check(t.Context(), binding, load); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("failed result was cached: calls=%d", calls)
	}
}
