package api

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type checkerFunc struct {
	name string
	fn   func(context.Context) error
}

func (c checkerFunc) Name() string                    { return c.name }
func (c checkerFunc) Check(ctx context.Context) error { return c.fn(ctx) }

func TestCheckerSnapshotProviderSanitizesFailuresAndAppliesDeadline(t *testing.T) {
	base := InitialSnapshot()
	provider, err := NewCheckerSnapshotProvider(base, 10*time.Millisecond,
		HealthyCheck(ComponentApplication, checkerFunc{name: "application", fn: func(context.Context) error { return nil }}, "Application is ready.", "APP_UNAVAILABLE", "Application is unavailable."),
		HealthyCheck(ComponentSQLite, checkerFunc{name: "sqlite", fn: func(context.Context) error { return errors.New("password=do-not-leak") }}, "SQLite is available.", "SQLITE_UNAVAILABLE", "SQLite is unavailable."),
		HealthyCheck(ComponentCluster, checkerFunc{name: "cluster", fn: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}}, "Cluster is available.", "CLUSTER_UNAVAILABLE", "The cluster is temporarily unavailable."),
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := provider.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Components.Application.Status != StatusHealthy {
		t.Fatalf("application status = %s", snapshot.Components.Application.Status)
	}
	if snapshot.Components.SQLite.Status != StatusUnhealthy {
		t.Fatalf("sqlite status = %s", snapshot.Components.SQLite.Status)
	}
	if snapshot.Components.Cluster.Status != StatusDegraded {
		t.Fatalf("cluster status = %s", snapshot.Components.Cluster.Status)
	}
	if strings.Contains(snapshot.Components.SQLite.Message, "password") {
		t.Fatal("checker error leaked into public snapshot")
	}
	if snapshot.Components.Cluster.CheckedAt == nil {
		t.Fatal("completed timed-out check has no checkedAt")
	}
}

func TestCheckerSnapshotProviderRecoversCheckerPanic(t *testing.T) {
	provider, err := NewCheckerSnapshotProvider(InitialSnapshot(), time.Second,
		HealthyCheck(ComponentApplication, checkerFunc{name: "application", fn: func(context.Context) error {
			panic("sensitive panic")
		}}, "Application is ready.", "APP_UNAVAILABLE", "Application is unavailable."),
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := provider.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Components.Application.Status != StatusUnhealthy || snapshot.Components.Application.Message != "Application is unavailable." {
		t.Fatalf("unexpected panic state: %#v", snapshot.Components.Application)
	}
}

func TestCheckerSnapshotProviderBoundsNonCooperativeChecker(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	firstReturned := make(chan struct{})
	var calls atomic.Int32
	checker := checkerFunc{name: "application", fn: func(context.Context) error {
		call := calls.Add(1)
		if call == 1 {
			close(started)
			<-release
			close(firstReturned)
		}
		return nil
	}}
	provider, err := NewCheckerSnapshotProvider(
		InitialSnapshot(),
		25*time.Millisecond,
		HealthyCheck(ComponentApplication, checker, "Application is ready.", "APP_UNAVAILABLE", "Application is unavailable."),
	)
	if err != nil {
		t.Fatal(err)
	}

	type snapshotResult struct {
		snapshot Snapshot
		err      error
	}
	result := make(chan snapshotResult, 1)
	go func() {
		snapshot, err := provider.Snapshot(context.Background())
		result <- snapshotResult{snapshot: snapshot, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("checker did not start")
	}

	var first snapshotResult
	select {
	case first = <-result:
	case <-time.After(500 * time.Millisecond):
		close(release)
		t.Fatal("snapshot remained blocked past its checker deadline")
	}
	if first.err != nil {
		t.Fatal(first.err)
	}
	if first.snapshot.Components.Application.Status != StatusUnhealthy {
		t.Fatalf("timed-out component status = %s", first.snapshot.Components.Application.Status)
	}

	second, err := provider.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Components.Application.Status != StatusUnhealthy || calls.Load() != 1 {
		t.Fatalf("in-flight checker was duplicated: status=%s calls=%d", second.Components.Application.Status, calls.Load())
	}

	close(release)
	select {
	case <-firstReturned:
	case <-time.After(time.Second):
		t.Fatal("released checker did not return")
	}

	deadline := time.Now().Add(time.Second)
	for {
		next, err := provider.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if next.Components.Application.Status == StatusHealthy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("checker slot was not released after the checker returned")
		}
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatalf("checker calls = %d, want 2", calls.Load())
	}
}
