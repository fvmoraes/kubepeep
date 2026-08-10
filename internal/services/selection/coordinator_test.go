package selection

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
)

func newTestCoordinator(t *testing.T, random []byte, hooks ...InvalidateHook) (*Coordinator, *api.GenerationStore, *api.SessionStore) {
	t.Helper()
	generation, err := api.NewGenerationStore()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := api.NewSessionStoreWithOptions(api.SessionStoreOptions{
		TTL: time.Hour, Random: bytes.NewReader(random),
	})
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := NewCoordinator(generation, sessions, hooks...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(coordinator.Close)
	return coordinator, generation, sessions
}

func TestNewIntentCancelsPredecessorAndOnlyLatestCanCommit(t *testing.T) {
	coordinator, generation, _ := newTestCoordinator(t, bytes.Repeat([]byte{0x42}, 96))
	first := coordinator.Begin(context.Background())
	second := coordinator.Begin(context.Background())
	select {
	case <-first.Context().Done():
	default:
		t.Fatal("predecessor context was not canceled")
	}
	if _, err := first.Commit(generation.Current(), func(context.Context) error { return nil }); !errors.Is(err, ErrSuperseded) {
		t.Fatalf("superseded commit error = %v", err)
	}
	committed := false
	before := generation.Current()
	next, err := second.Commit(before, func(context.Context) error { committed = true; return nil })
	if err != nil || !committed || next == before {
		t.Fatalf("next=%q committed=%v err=%v", next, committed, err)
	}
}

func TestTransactionFailurePreservesGenerationCSRFAndWork(t *testing.T) {
	coordinator, generation, sessions := newTestCoordinator(t, bytes.Repeat([]byte{0x43}, 96))
	beforeGeneration := generation.Current()
	beforeSession, err := sessions.Current("http://127.0.0.1:2748", beforeGeneration)
	if err != nil {
		t.Fatal(err)
	}
	work, _ := coordinator.WorkContext()
	want := errors.New("synthetic rollback")
	intent := coordinator.Begin(context.Background())
	if _, err := intent.Commit(beforeGeneration, func(context.Context) error { return want }); !errors.Is(err, want) {
		t.Fatalf("commit error = %v", err)
	}
	if generation.Current() != beforeGeneration {
		t.Fatal("generation changed after failed transaction")
	}
	afterSession, err := sessions.Current("http://127.0.0.1:2748", beforeGeneration)
	if err != nil || afterSession.CSRFToken != beforeSession.CSRFToken {
		t.Fatalf("session changed after failed transaction: %#v err=%v", afterSession, err)
	}
	select {
	case <-work.Done():
		t.Fatal("generation work was canceled after failed transaction")
	default:
	}
}

func TestSuccessfulCommitCancelsWorkRotatesCSRFAndCallsHooks(t *testing.T) {
	var hookCalls atomic.Int32
	random := append(bytes.Repeat([]byte{0x44}, 32), bytes.Repeat([]byte{0x45}, 64)...)
	coordinator, generation, sessions := newTestCoordinator(t, random, func(string) { hookCalls.Add(1) })
	before := generation.Current()
	oldSession, err := sessions.Current("http://127.0.0.1:2748", before)
	if err != nil {
		t.Fatal(err)
	}
	oldWork, _ := coordinator.WorkContext()
	intent := coordinator.Begin(context.Background())
	next, err := intent.Commit(before, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-oldWork.Done():
	case <-time.After(time.Second):
		t.Fatal("old generation work was not canceled")
	}
	newWork, observed := coordinator.WorkContext()
	if observed != next || newWork == oldWork || hookCalls.Load() != 1 {
		t.Fatalf("observed=%q next=%q sameWork=%v hooks=%d", observed, next, newWork == oldWork, hookCalls.Load())
	}
	newSession, err := sessions.Current("http://127.0.0.1:2748", next)
	if err != nil || newSession.CSRFToken == oldSession.CSRFToken {
		t.Fatalf("CSRF did not rotate: old=%q new=%q err=%v", oldSession.CSRFToken, newSession.CSRFToken, err)
	}
}

func TestGenerationMismatchDoesNotRunTransaction(t *testing.T) {
	coordinator, _, _ := newTestCoordinator(t, bytes.Repeat([]byte{0x45}, 96))
	called := false
	intent := coordinator.Begin(context.Background())
	if _, err := intent.Commit("gen_stale", func(context.Context) error { called = true; return nil }); !errors.Is(err, ErrGenerationChanged) || called {
		t.Fatalf("error=%v called=%v", err, called)
	}
}

func TestCSRFRotationFailureStillPublishesCommittedSelection(t *testing.T) {
	coordinator, generation, sessions := newTestCoordinator(t, bytes.Repeat([]byte{0x46}, 32))
	before := generation.Current()
	if _, err := sessions.Current("http://127.0.0.1:2748", before); err != nil {
		t.Fatal(err)
	}
	published := ""
	intent := coordinator.Begin(context.Background())
	next, err := intent.CommitConditional(before, func(context.Context) (bool, error) {
		return true, nil
	}, func(value string) {
		published = value
	})
	var stateError *PublishedStateError
	if !errors.As(err, &stateError) || stateError.Stage != "csrf" {
		t.Fatalf("expected csrf publication error, got %v", err)
	}
	if next == before || generation.Current() != next || published != next {
		t.Fatalf("before=%q next=%q current=%q published=%q", before, next, generation.Current(), published)
	}
}
