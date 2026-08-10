package selection

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func testState(t *testing.T) (*State, *Coordinator) {
	t.Helper()
	generation, err := api.NewGenerationStore()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := api.NewSessionStoreWithOptions(api.SessionStoreOptions{TTL: time.Hour, Random: bytes.NewReader(bytes.Repeat([]byte{0x51}, 256))})
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := NewCoordinator(generation, sessions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(coordinator.Close)
	state, err := NewState(coordinator, namespaces.SelectionBinding{
		ClusterProfileID: 1, Context: "dev", ActiveScopeID: 7, Generation: generation.Current(),
	}, namespaces.ScopeResolution{ScopeID: 7, Namespaces: []string{"payments"}})
	if err != nil {
		t.Fatal(err)
	}
	return state, coordinator
}

func TestInactiveMutationPreservesGenerationAndBinding(t *testing.T) {
	state, coordinator := testState(t)
	before, beforeResolution := state.Snapshot()
	result, err := state.Mutate(context.Background(), before.Generation, func(_ context.Context, binding namespaces.SelectionBinding) (namespaces.SelectionCommit, error) {
		if binding != before {
			t.Fatalf("binding=%#v want=%#v", binding, before)
		}
		return func(_ context.Context, current namespaces.SelectionBinding) (namespaces.SelectionMutation, error) {
			if current != before {
				t.Fatalf("commit binding=%#v want=%#v", current, before)
			}
			return namespaces.SelectionMutation{}, nil
		}, nil
	})
	if err != nil || result.Generation != before.Generation || result.Binding != before || result.Changed || result.Resolution.ScopeID != beforeResolution.ScopeID || coordinator.CurrentGeneration() != before.Generation {
		t.Fatalf("result=%#v current=%q err=%v", result, coordinator.CurrentGeneration(), err)
	}
}

func TestScopeActivationPublishesGenerationAndResolution(t *testing.T) {
	state, _ := testState(t)
	before, _ := state.Snapshot()
	result, err := state.Mutate(context.Background(), before.Generation, func(context.Context, namespaces.SelectionBinding) (namespaces.SelectionCommit, error) {
		resolution := namespaces.ScopeResolution{ScopeID: 9, Namespaces: []string{"billing", "payments"}}
		return func(context.Context, namespaces.SelectionBinding) (namespaces.SelectionMutation, error) {
			return namespaces.SelectionMutation{PublishGeneration: true, Activation: &resolution}, nil
		}, nil
	})
	if err != nil || result.Generation == before.Generation || !result.Changed || result.Binding.ActiveScopeID != 9 || result.Resolution.ScopeID != 9 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	binding, resolution := state.Snapshot()
	if binding.Generation != result.Generation || binding.ActiveScopeID != 9 || resolution.ScopeID != 9 || len(resolution.Namespaces) != 2 {
		t.Fatalf("binding=%#v resolution=%#v", binding, resolution)
	}
}

func TestNewIntentCancelsSlowPreparationAndLatestWins(t *testing.T) {
	state, _ := testState(t)
	before, _ := state.Snapshot()
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := state.Mutate(context.Background(), before.Generation, func(ctx context.Context, _ namespaces.SelectionBinding) (namespaces.SelectionCommit, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		finished <- err
	}()
	<-started

	result, err := state.Mutate(context.Background(), before.Generation, activationPreparation(11, []string{"latest"}))
	if err != nil {
		t.Fatal(err)
	}
	if firstErr := <-finished; !errors.Is(firstErr, namespaces.ErrGenerationChanged) {
		t.Fatalf("slow preparation error=%v", firstErr)
	}
	binding, resolution := state.Snapshot()
	if !result.Changed || binding.ActiveScopeID != 11 || resolution.ScopeID != 11 || resolution.Namespaces[0] != "latest" {
		t.Fatalf("result=%#v binding=%#v resolution=%#v", result, binding, resolution)
	}
}

func TestSupersededPreparationThatIgnoresCancellationCannotCommit(t *testing.T) {
	state, _ := testState(t)
	before, _ := state.Snapshot()
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := state.Mutate(context.Background(), before.Generation, func(context.Context, namespaces.SelectionBinding) (namespaces.SelectionCommit, error) {
			close(started)
			<-release
			return activationCommit(10, []string{"stale"}), nil
		})
		finished <- err
	}()
	<-started
	if _, err := state.Mutate(context.Background(), before.Generation, activationPreparation(12, []string{"winner"})); err != nil {
		t.Fatal(err)
	}
	close(release)
	if firstErr := <-finished; !errors.Is(firstErr, namespaces.ErrGenerationChanged) {
		t.Fatalf("superseded error=%v", firstErr)
	}
	_, resolution := state.Snapshot()
	if resolution.ScopeID != 12 || resolution.Namespaces[0] != "winner" {
		t.Fatalf("resolution=%#v", resolution)
	}
}

func TestCommitReReadsBindingWhenInactiveScopeBecomesActive(t *testing.T) {
	state, _ := testState(t)
	state.mu.Lock()
	state.binding.ActiveScopeID = 3
	state.mu.Unlock()
	before, _ := state.Snapshot()
	result, err := state.Mutate(context.Background(), before.Generation, func(_ context.Context, prepared namespaces.SelectionBinding) (namespaces.SelectionCommit, error) {
		if prepared.ActiveScopeID == 9 {
			t.Fatal("scope was already active during preparation")
		}
		state.mu.Lock()
		state.binding.ActiveScopeID = 9
		state.mu.Unlock()
		return func(_ context.Context, current namespaces.SelectionBinding) (namespaces.SelectionMutation, error) {
			if current.ActiveScopeID != 9 {
				t.Fatalf("commit binding=%#v", current)
			}
			resolution := namespaces.ScopeResolution{ScopeID: 9, Namespaces: []string{"now-active"}}
			return namespaces.SelectionMutation{PublishGeneration: true, Activation: &resolution}, nil
		}, nil
	})
	if err != nil || !result.Changed || result.Resolution.ScopeID != 9 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestResultAndIfCurrentUseDefensiveCurrentSnapshots(t *testing.T) {
	state, _ := testState(t)
	if state.ActiveProfileID() != 1 {
		t.Fatalf("active profile=%d", state.ActiveProfileID())
	}
	before, _ := state.Snapshot()
	items := []string{"billing", "payments"}
	result, err := state.Mutate(context.Background(), before.Generation, activationPreparation(13, items))
	if err != nil {
		t.Fatal(err)
	}
	items[0] = "mutated-source"
	result.Resolution.Namespaces[0] = "mutated-result"
	called := false
	if !state.IfCurrent(result.Binding, func() { called = true }) || !called {
		t.Fatal("current binding was not accepted")
	}
	stale := result.Binding
	stale.Generation = before.Generation
	if state.IfCurrent(stale, func() { t.Fatal("stale callback executed") }) {
		t.Fatal("stale binding was accepted")
	}
	_, snapshot := state.Snapshot()
	if snapshot.Namespaces[0] != "billing" {
		t.Fatalf("snapshot aliases result: %#v", snapshot)
	}
}

func activationPreparation(scopeID int64, items []string) namespaces.SelectionPreparation {
	return func(context.Context, namespaces.SelectionBinding) (namespaces.SelectionCommit, error) {
		return activationCommit(scopeID, items), nil
	}
}

func activationCommit(scopeID int64, items []string) namespaces.SelectionCommit {
	return func(context.Context, namespaces.SelectionBinding) (namespaces.SelectionMutation, error) {
		resolution := namespaces.ScopeResolution{ScopeID: scopeID, Namespaces: items}
		return namespaces.SelectionMutation{PublishGeneration: true, Activation: &resolution}, nil
	}
}

func TestFailedContextTransactionPreservesState(t *testing.T) {
	state, _ := testState(t)
	before, beforeResolution := state.Snapshot()
	want := errors.New("rollback")
	_, err := state.ReplaceContext(context.Background(), before.Generation, namespaces.SelectionBinding{ClusterProfileID: 2, Context: "prod"}, namespaces.ScopeResolution{}, func(context.Context) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
	after, afterResolution := state.Snapshot()
	if after != before || afterResolution.ScopeID != beforeResolution.ScopeID {
		t.Fatalf("state changed after rollback: %#v %#v", after, afterResolution)
	}
}

func TestReplaceContextPreparedReturnsImmutableCommittedSnapshot(t *testing.T) {
	state, _ := testState(t)
	before, _ := state.Snapshot()
	preparedItems := []string{"production"}
	transactionCalls := 0
	result, err := state.ReplaceContextPrepared(context.Background(), before.Generation, func(ctx context.Context) (namespaces.SelectionBinding, namespaces.ScopeResolution, func(context.Context) error, error) {
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
		binding := namespaces.SelectionBinding{ClusterProfileID: 2, Context: "prod", Cluster: "cluster-prod"}
		resolution := namespaces.ScopeResolution{ScopeID: 21, ScopeName: "Production", Namespaces: preparedItems}
		return binding, resolution, func(commitContext context.Context) error {
			if err := commitContext.Err(); err != nil {
				return err
			}
			transactionCalls++
			return nil
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if transactionCalls != 1 || !result.Changed || result.Binding.ClusterProfileID != 2 || result.Binding.ActiveScopeID != 21 || result.Resolution.ScopeID != 21 {
		t.Fatalf("calls=%d result=%#v", transactionCalls, result)
	}
	preparedItems[0] = "mutated-source"
	result.Resolution.Namespaces[0] = "mutated-result"
	binding, resolution := state.Snapshot()
	if binding != result.Binding || resolution.Namespaces[0] != "production" {
		t.Fatalf("binding=%#v resolution=%#v result=%#v", binding, resolution, result)
	}
}

func TestReplaceContextPreparedRegistersIntentBeforeRemoteResolution(t *testing.T) {
	state, _ := testState(t)
	before, _ := state.Snapshot()
	started := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := state.ReplaceContextPrepared(context.Background(), before.Generation, func(ctx context.Context) (namespaces.SelectionBinding, namespaces.ScopeResolution, func(context.Context) error, error) {
			close(started)
			<-ctx.Done()
			return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, nil, ctx.Err()
		})
		firstDone <- err
	}()
	<-started

	result, err := state.ReplaceContextPrepared(context.Background(), before.Generation, func(context.Context) (namespaces.SelectionBinding, namespaces.ScopeResolution, func(context.Context) error, error) {
		return namespaces.SelectionBinding{ClusterProfileID: 3, Context: "latest", Cluster: "cluster-latest"}, namespaces.ScopeResolution{}, func(context.Context) error { return nil }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstErr := <-firstDone; !errors.Is(firstErr, namespaces.ErrGenerationChanged) {
		t.Fatalf("superseded resolution error=%v", firstErr)
	}
	if result.Binding.ClusterProfileID != 3 || result.Binding.Context != "latest" || !result.Changed {
		t.Fatalf("result=%#v", result)
	}
}
