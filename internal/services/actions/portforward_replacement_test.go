package actions

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

func TestPortForwardReplacementReusesTheOccupiedLocalPort(t *testing.T) {
	t.Parallel()
	manager, adapter, _, _ := replacementManager(t)
	first := createReplacementPod(t, manager, "api", "initial-forward-key", 8080, nil)
	second := createReplacementPod(t, manager, "api", "replacement-forward", 9090, &first.LocalPort)
	if second.ID == first.ID || second.LocalPort != first.LocalPort || second.RemotePort != 9090 || second.SourceKind != "Pod" || second.SourceName != "api" || second.SourcePort != 9090 {
		t.Fatalf("incorrect replacement: first=%+v second=%+v", first, second)
	}
	rows, err := manager.List(testBinding("gen_1"))
	if err != nil || len(rows) != 2 {
		t.Fatalf("list after replacement: %+v %v", rows, err)
	}
	for _, row := range rows {
		if row.ID == first.ID && (row.Status != PortForwardClosed || row.EndedAt == nil) {
			t.Fatalf("previous session not closed: %+v", row)
		}
	}
	if active, peak := adapter.counts(); active != 1 || peak != 1 {
		t.Fatalf("replacement overlapped live upstreams: active=%d peak=%d", active, peak)
	}
}

func TestServicePortForwardReplacementIsIndependentOfPodsAndBackingPod(t *testing.T) {
	t.Parallel()
	manager, _, _, _ := replacementManager(t)
	// A Service's identity must differ from both a same-name Pod and its backing Pod.
	pod := createReplacementPod(t, manager, "api", "same-name-pod-key", 8080, nil)
	backing := createReplacementPod(t, manager, "api-pod", "backing-pod-forward-key", 8080, nil)
	first, _, err := manager.CreateService(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "services", Namespace: "payments", Name: "api"}, "initial-service-key", testServicePortForward("gen_1", "payments", "api", 80))
	if err != nil {
		t.Fatal(err)
	}
	request := testServicePortForward("gen_1", "payments", "api", 81)
	request.LocalPort = &first.LocalPort
	replacement, _, err := manager.CreateService(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "services", Namespace: "payments", Name: "api"}, "replace-service-key", request)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.SourceKind != "Service" || replacement.SourceName != "api" || replacement.SourcePort != 81 || replacement.Pod != "api-pod" || replacement.RemotePort != 8081 {
		t.Fatalf("source and resolved target lost: %+v", replacement)
	}
	rows, err := manager.List(testBinding("gen_1"))
	if err != nil {
		t.Fatal(err)
	}
	active := map[string]bool{}
	for _, row := range rows {
		if row.Status == PortForwardActive {
			active[row.ID] = true
		}
	}
	if len(active) != 3 || !active[pod.ID] || !active[backing.ID] || !active[replacement.ID] || active[first.ID] {
		t.Fatalf("Service replacement changed another origin: %+v", rows)
	}
}

func TestPortForwardConcurrentReplacementNeverOverlapsOneOrigin(t *testing.T) {
	t.Parallel()
	manager, adapter, _, _ := replacementManager(t)
	const callers = 12
	start := make(chan struct{})
	results := make(chan error, callers)
	for index := range callers {
		go func() {
			<-start
			_, _, err := manager.Create(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, fmt.Sprintf("concurrent-replace-%04d", index), testPortForward("gen_1", "payments", "api", 8000+index))
			results <- err
		}()
	}
	close(start)
	for range callers {
		if err := receiveReplacement(t, results); err != nil {
			t.Fatal(err)
		}
	}
	if active, peak := adapter.counts(); active != 1 || peak != 1 {
		t.Fatalf("same origin created concurrent upstreams: active=%d peak=%d", active, peak)
	}
	rows, err := manager.List(testBinding("gen_1"))
	if err != nil || len(rows) != callers {
		t.Fatalf("lost replacement outcomes: rows=%d err=%v", len(rows), err)
	}
}

func TestPortForwardDeniedReplacementPreservesTheWorkingSession(t *testing.T) {
	t.Parallel()
	manager, adapter, authorizer, _ := replacementManager(t)
	first := createReplacementPod(t, manager, "api", "authorized-port-forward", 8080, nil)
	authorizer.mu.Lock()
	authorizer.err = &authorization.PublicError{Code: authorization.CodeForbidden, HTTPStatus: http.StatusForbidden, Message: "Denied"}
	authorizer.mu.Unlock()
	request := testPortForward("gen_1", "payments", "api", 9090)
	request.LocalPort = &first.LocalPort
	_, _, err := manager.Create(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, "denied-replacement", request)
	requireCode(t, err, CodeForbidden)
	rows, err := manager.List(testBinding("gen_1"))
	if err != nil || len(rows) != 1 || rows[0].ID != first.ID || rows[0].Status != PortForwardActive {
		t.Fatalf("denial closed the authorized session: %+v %v", rows, err)
	}
	if active, peak := adapter.counts(); active != 1 || peak != 1 {
		t.Fatalf("denial changed upstreams: active=%d peak=%d", active, peak)
	}
}

func TestPortForwardReplacementIsAllowedAtTheGlobalLimit(t *testing.T) {
	t.Parallel()
	manager, adapter, _, _ := replacementManager(t)
	var initial PortForwardDTO
	for index := range MaximumPortForwardSessions {
		dto := createReplacementPod(t, manager, fmt.Sprintf("api-%d", index), fmt.Sprintf("limit-initial-%04d", index), 8080, nil)
		if index == 0 {
			initial = dto
		}
	}
	createReplacementPod(t, manager, "api-0", "replace-at-full-limit", 9090, &initial.LocalPort)
	if active, peak := adapter.counts(); active != MaximumPortForwardSessions || peak != MaximumPortForwardSessions {
		t.Fatalf("replacement changed global limit: active=%d peak=%d", active, peak)
	}
	_, _, err := manager.Create(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "extra"}, "extra-at-full-limit", testPortForward("gen_1", "payments", "extra", 8080))
	requireCode(t, err, CodeLimitExceeded)
}

func TestPortForwardReplacementKeepsItsSlotWhileClosingThePreviousUpstream(t *testing.T) {
	t.Parallel()
	manager, adapter, _, _ := replacementManager(t)
	for index := range MaximumPortForwardSessions {
		createReplacementPod(t, manager, fmt.Sprintf("api-%d", index), fmt.Sprintf("slot-initial-%04d", index), 8080, nil)
	}
	adapter.mu.Lock()
	old := adapter.handles[0]
	old.closeStarted, old.closeRelease = make(chan struct{}), make(chan struct{})
	adapter.mu.Unlock()
	releaseOld := sync.OnceFunc(func() { close(old.closeRelease) })
	t.Cleanup(releaseOld)
	replacement := make(chan error, 1)
	go func() {
		_, _, err := manager.Create(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-0"}, "replacement-slot-key", testPortForward("gen_1", "payments", "api-0", 9090))
		replacement <- err
	}()
	receiveReplacement(t, old.closeStarted)
	_, _, extraErr := manager.Create(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "extra"}, "extra-during-replacement", testPortForward("gen_1", "payments", "extra", 8080))
	releaseOld()
	replacementErr := receiveReplacement(t, replacement)
	if replacementErr != nil {
		t.Fatalf("replacement lost its occupied slot during cleanup: %v (extra request: %v)", replacementErr, extraErr)
	}
	requireCode(t, extraErr, CodeLimitExceeded)
}

func TestPortForwardCanceledReplacementDoesNotWaitForTheSameOrigin(t *testing.T) {
	t.Parallel()
	manager, adapter, authorizer, _ := replacementManager(t)
	adapter.started, adapter.release = make(chan struct{}, 1), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		_, _, err := manager.Create(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, "first-blocking-forward", testPortForward("gen_1", "payments", "api", 8080))
		first <- err
	}()
	receiveReplacement(t, adapter.started)
	ctx, cancel := context.WithCancel(t.Context())
	second := make(chan error, 1)
	go func() {
		_, _, err := manager.Create(ctx, testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, "canceled-waiting-forward", testPortForward("gen_1", "payments", "api", 9090))
		second <- err
	}()
	cancel()
	requireCode(t, receiveReplacement(t, second), CodeClientCanceled)
	close(adapter.release)
	if err := receiveReplacement(t, first); err != nil {
		t.Fatal(err)
	}
	if len(authorizer.snapshot()) != 1 {
		t.Fatal("canceled replacement reached authorization or upstream")
	}
}

func TestPortForwardSetupTerminatesOnCancellationGenerationAndShutdown(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"request canceled", "generation changed", "shutdown"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			manager, adapter, _, generations := replacementManager(t)
			adapter.started, adapter.release = make(chan struct{}, 1), make(chan struct{})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, _, err := manager.Create(ctx, testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, "interrupted-setup-key", testPortForward("gen_1", "payments", "api", 8080))
				result <- err
			}()
			receiveReplacement(t, adapter.started)
			code := CodeClientCanceled
			binding := testBinding("gen_1")
			switch name {
			case "request canceled":
				cancel()
			case "generation changed":
				code = CodeGenerationChanged
				generations.set("gen_2")
				manager.OnGeneration("gen_2")
				binding = testBinding("gen_2")
			case "shutdown":
				code = CodeServerShutdown
				manager.Shutdown()
			}
			requireCode(t, receiveReplacement(t, result), code)
			rows, err := manager.List(binding)
			if err != nil || len(rows) != 0 {
				t.Fatalf("interrupted setup published a session: %+v %v", rows, err)
			}
		})
	}
}

func TestPortForwardSuccessfulSessionOutlivesItsRequest(t *testing.T) {
	t.Parallel()
	manager, adapter, _, _ := replacementManager(t)
	ctx, cancel := context.WithCancel(t.Context())
	_, _, err := manager.Create(ctx, testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, "completed-request-key", testPortForward("gen_1", "payments", "api", 8080))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	adapter.mu.Lock()
	lifetime := adapter.lifetimes[0]
	adapter.mu.Unlock()
	if lifetime.Err() != nil {
		t.Fatalf("successful request cancellation closed persistent tunnel: %v", lifetime.Err())
	}
	rows, err := manager.List(testBinding("gen_1"))
	if err != nil || len(rows) != 1 || rows[0].Status != PortForwardActive {
		t.Fatalf("successful request lost its session: %+v %v", rows, err)
	}
}

func TestPortForwardCanceledSetupCannotPublishALateUpstream(t *testing.T) {
	t.Parallel()
	manager, adapter, _, _ := replacementManager(t)
	adapter.started, adapter.release = make(chan struct{}, 1), make(chan struct{})
	adapter.ignoreSetupCancellation = true
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := manager.Create(ctx, testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: "api"}, "late-upstream-forward", testPortForward("gen_1", "payments", "api", 8080))
		result <- err
	}()
	receiveReplacement(t, adapter.started)
	cancel()
	close(adapter.release)
	requireCode(t, receiveReplacement(t, result), CodeClientCanceled)
	if active, _ := adapter.counts(); active != 0 {
		t.Fatal("late upstream remained open after canceled setup")
	}
	rows, err := manager.List(testBinding("gen_1"))
	if err != nil || len(rows) != 0 {
		t.Fatalf("late upstream was published: %+v %v", rows, err)
	}
}

func replacementManager(t *testing.T) (*PortForwardManager, *replacementAdapter, *authorizerStub, *generationStub) {
	t.Helper()
	adapter, authorizer, generations := &replacementAdapter{}, &authorizerStub{}, &generationStub{generation: "gen_1"}
	manager, err := NewPortForwardService(t.Context(), authorizer, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	return manager, adapter, authorizer, generations
}

func createReplacementPod(t *testing.T, manager *PortForwardManager, name, key string, remote int, local *int) PortForwardDTO {
	t.Helper()
	request := testPortForward("gen_1", "payments", name, remote)
	request.LocalPort = local
	dto, replayed, err := manager.Create(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "pods", Namespace: "payments", Name: name}, key, request)
	if err != nil || replayed {
		t.Fatalf("create %s: replayed=%v err=%v", name, replayed, err)
	}
	return dto
}

func receiveReplacement[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	select {
	case result := <-channel:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("port-forward operation failed to settle")
		var zero T
		return zero
	}
}

type replacementAdapter struct {
	mu                      sync.Mutex
	active                  int
	peak                    int
	lifetimes               []context.Context
	handles                 []*replacementHandle
	started                 chan struct{}
	release                 chan struct{}
	ignoreSetupCancellation bool
}

func (a *replacementAdapter) ResolveService(_ context.Context, target MutationTarget, port int) (ResolvedServicePort, error) {
	target.Kind, target.Name = "Pod", target.Name+"-pod"
	return ResolvedServicePort{Target: target, RemotePort: port + 8000}, nil
}

func (a *replacementAdapter) Start(setup, lifetime context.Context, _ PortForwardCommand, _ net.Listener) (PortForwardHandle, error) {
	if a.started != nil {
		a.started <- struct{}{}
		if a.ignoreSetupCancellation {
			<-a.release
		} else {
			select {
			case <-a.release:
			case <-setup.Done():
				return nil, setup.Err()
			}
		}
	}
	a.mu.Lock()
	a.active++
	a.peak = max(a.active, a.peak)
	a.lifetimes = append(a.lifetimes, lifetime)
	handle := &replacementHandle{adapter: a, done: make(chan error, 1)}
	a.handles = append(a.handles, handle)
	a.mu.Unlock()
	return handle, nil
}

func (a *replacementAdapter) counts() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active, a.peak
}

type replacementHandle struct {
	adapter      *replacementAdapter
	done         chan error
	once         sync.Once
	closeStarted chan struct{}
	closeRelease chan struct{}
}

func (h *replacementHandle) Wait() error { return <-h.done }

func (h *replacementHandle) Close() error {
	h.once.Do(func() {
		h.adapter.mu.Lock()
		h.adapter.active--
		h.adapter.mu.Unlock()
		if h.closeStarted != nil {
			close(h.closeStarted)
			<-h.closeRelease
		}
		h.done <- context.Canceled
	})
	return nil
}
