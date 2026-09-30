package kubernetesruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func TestMetricsDiscoveryCacheUsesFullSelectionIdentityAndInvalidates(t *testing.T) {
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
	cache := newMetricsDiscoveryCache(10*time.Minute, func() time.Time { return now })
	binding := namespaces.SelectionBinding{ClusterProfileID: 11, Context: "dev", Cluster: "cluster-a", ActiveScopeID: 7, Generation: "gen-1"}
	calls := 0
	load := func(context.Context) (bool, error) {
		calls++
		return true, nil
	}

	for range 2 {
		available, err := cache.Available(context.Background(), binding, load)
		if err != nil || !available {
			t.Fatalf("available = %v, err = %v", available, err)
		}
	}
	if calls != 1 {
		t.Fatalf("cached discovery calls = %d, want 1", calls)
	}

	otherContext := binding
	otherContext.Context = "prod"
	if _, err := cache.Available(context.Background(), otherContext, load); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("full-identity discovery calls = %d, want 2", calls)
	}

	cache.InvalidateAll()
	if _, err := cache.Available(context.Background(), binding, load); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("post-invalidation discovery calls = %d, want 3", calls)
	}

	now = now.Add(11 * time.Minute)
	if _, err := cache.Available(context.Background(), binding, load); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatalf("post-expiry discovery calls = %d, want 4", calls)
	}
}

func TestMetricsDiscoveryCacheDoesNotCacheErrors(t *testing.T) {
	cache := newMetricsDiscoveryCache(time.Minute, time.Now)
	binding := namespaces.SelectionBinding{ClusterProfileID: 11, Context: "dev", Generation: "gen-1"}
	calls := 0
	load := func(context.Context) (bool, error) {
		calls++
		if calls == 1 {
			return false, errors.New("temporary discovery failure")
		}
		return true, nil
	}

	if _, err := cache.Available(context.Background(), binding, load); err == nil {
		t.Fatal("expected the first discovery error")
	}
	available, err := cache.Available(context.Background(), binding, load)
	if err != nil || !available || calls != 2 {
		t.Fatalf("available = %v, calls = %d, err = %v", available, calls, err)
	}
}
