package actions

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestPortForwardUsesExactSARAndOwnsLoopbackLifecycle(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	authorizer := &authorizerStub{}
	adapter := &portForwardAdapterStub{}
	audit := &auditStub{}
	manager, err := NewPortForwardService(context.Background(), authorizer, generations, adapter, audit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}
	dto, replayed, err := manager.Create(context.Background(), binding, route, "port-forward-key", testPortForward("gen_1", "payments", "api-abc", 8080))
	if err != nil || replayed {
		t.Fatalf("create failed: %#v replayed=%v err=%v", dto, replayed, err)
	}
	if dto.Status != PortForwardActive || dto.LocalAddress != "127.0.0.1" || dto.LocalPort < 1024 || !dto.ExpiresAt.Equal(dto.CreatedAt.Add(DefaultPortForwardDuration)) {
		t.Fatalf("invalid port-forward DTO: %#v", dto)
	}
	calls := authorizer.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected one SAR, got %#v", calls)
	}
	key := calls[0].key
	if key.APIGroup != "" || key.Resource != "pods" || key.Subresource != "portforward" || key.Verb != "create" || key.ResourceName != "api-abc" || calls[0].kind != "upgrade" {
		t.Fatalf("incorrect port-forward SAR: %#v", calls[0])
	}
	adapter.mu.Lock()
	listener := adapter.listeners[0]
	adapter.mu.Unlock()
	if address := listener.Addr().(*net.TCPAddr); !address.IP.Equal(net.ParseIP("127.0.0.1")) || address.Port != dto.LocalPort {
		t.Fatalf("listener was not retained on exact loopback: %v", address)
	}
	if err := manager.Close(context.Background(), binding, dto.ID, PortForwardDeleteRequest{Confirmed: true, ExpectedGeneration: "gen_1"}); err != nil {
		t.Fatal(err)
	}
	rows, err := manager.List(binding)
	if err != nil || len(rows) != 1 || rows[0].Status != PortForwardClosed || rows[0].EndedAt == nil || rows[0].EndReason == nil || *rows[0].EndReason != "closed" {
		t.Fatalf("closed session was not retained: %#v err=%v", rows, err)
	}
	if err := manager.Close(context.Background(), binding, dto.ID, PortForwardDeleteRequest{Confirmed: true, ExpectedGeneration: "gen_1"}); ErrorCodeOf(err) != CodeSessionGone {
		t.Fatalf("second close must return SESSION_GONE, got %v", err)
	}
}

func TestPortForwardValidationAndOccupiedPortFailSafely(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	authorizer := &authorizerStub{}
	adapter := &portForwardAdapterStub{}
	manager, err := NewPortForwardService(context.Background(), authorizer, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}
	invalid := testPortForward("gen_1", "payments", "api-abc", 0)
	_, _, err = manager.Create(context.Background(), binding, route, "invalid-port-key", invalid)
	requireCode(t, err, CodeValidationFailed)
	if len(authorizer.snapshot()) != 0 {
		t.Fatal("invalid port reached SAR")
	}

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	request := testPortForward("gen_1", "payments", "api-abc", 8080)
	request.LocalPort = &port
	_, _, err = manager.Create(context.Background(), binding, route, "occupied-port-key", request)
	requireCode(t, err, CodeConflict)
	adapter.mu.Lock()
	starts := len(adapter.commands)
	adapter.mu.Unlock()
	if starts != 0 {
		t.Fatalf("occupied port reached upstream adapter %d times", starts)
	}
}

func TestPortForwardLimitIsExactlyEightAndGenerationCleansAll(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	adapter := &portForwardAdapterStub{}
	manager, err := NewPortForwardService(context.Background(), &authorizerStub{}, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	binding := testBinding("gen_1")
	for index := 0; index < MaximumPortForwardSessions; index++ {
		name := fmt.Sprintf("api-%d", index)
		_, _, err := manager.Create(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: name}, fmt.Sprintf("forward-limit-%04d", index), testPortForward("gen_1", "payments", name, 8000+index))
		if err != nil {
			t.Fatalf("session %d failed: %v", index, err)
		}
	}
	_, _, err = manager.Create(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-9"}, "forward-limit-9999", testPortForward("gen_1", "payments", "api-9", 9000))
	requireCode(t, err, CodeLimitExceeded)
	adapter.mu.Lock()
	if len(adapter.commands) != MaximumPortForwardSessions {
		t.Fatalf("limit allowed %d upstream sessions", len(adapter.commands))
	}
	adapter.mu.Unlock()

	generations.set("gen_2")
	manager.OnGeneration("gen_2")
	rows, err := manager.List(testBinding("gen_2"))
	if err != nil || len(rows) != 0 {
		t.Fatalf("old generation survived: %#v err=%v", rows, err)
	}
	waitFor(t, func() bool {
		adapter.mu.Lock()
		defer adapter.mu.Unlock()
		for _, listener := range adapter.listeners {
			probe, err := net.Listen("tcp", listener.Addr().String())
			if err != nil {
				return false
			}
			_ = probe.Close()
		}
		return true
	})
}

func TestPortForwardIdempotencyPreventsSecondListener(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	adapter := &portForwardAdapterStub{started: make(chan struct{}, 1), release: make(chan struct{})}
	manager, err := NewPortForwardService(context.Background(), &authorizerStub{}, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}
	request := testPortForward("gen_1", "payments", "api-abc", 8080)
	type response struct {
		dto      PortForwardDTO
		replayed bool
		err      error
	}
	responses := make(chan response, 2)
	for range 2 {
		go func() {
			dto, replayed, callErr := manager.Create(context.Background(), binding, route, "same-forward-key", request)
			responses <- response{dto: dto, replayed: replayed, err: callErr}
		}()
	}
	<-adapter.started
	close(adapter.release)
	first, second := <-responses, <-responses
	if first.err != nil || second.err != nil || first.dto.ID != second.dto.ID || first.replayed == second.replayed {
		t.Fatalf("idempotent responses differ: %#v %#v", first, second)
	}
	adapter.mu.Lock()
	count := len(adapter.commands)
	adapter.mu.Unlock()
	if count != 1 {
		t.Fatalf("created %d listeners/upstreams", count)
	}

	changed := request
	changed.RemotePort = 9090
	_, _, err = manager.Create(context.Background(), binding, route, "same-forward-key", changed)
	requireCode(t, err, CodeIdempotencyConflict)
}

func TestPortForwardExpiryPodGoneAndTerminalRetention(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	adapter := &portForwardAdapterStub{}
	manager, err := newPortForwardService(context.Background(), &authorizerStub{}, generations, adapter, netLoopbackBinder{}, NoopAuditSink{}, systemClock{}, &identifierStub{}, 20*time.Millisecond, 30*time.Millisecond, time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	binding := testBinding("gen_1")
	dto, _, err := manager.Create(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: "expiring"}, "expiring-key-123", testPortForward("gen_1", "payments", "expiring", 8080))
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		rows, _ := manager.List(binding)
		return len(rows) == 1 && rows[0].ID == dto.ID && rows[0].Status == PortForwardExpired
	})
	time.Sleep(35 * time.Millisecond)
	rows, err := manager.List(binding)
	if err != nil || len(rows) != 0 {
		t.Fatalf("terminal session outlived retention: %#v err=%v", rows, err)
	}

	adapter2 := &portForwardAdapterStub{}
	manager2, err := NewPortForwardService(context.Background(), &authorizerStub{}, generations, adapter2, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager2.Shutdown)
	second, _, err := manager2.Create(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: "gone"}, "pod-gone-key-123", testPortForward("gen_1", "payments", "gone", 8080))
	if err != nil {
		t.Fatal(err)
	}
	adapter2.mu.Lock()
	handle := adapter2.handles[0]
	adapter2.mu.Unlock()
	handle.finish(ErrPortForwardPodGone)
	waitFor(t, func() bool {
		rows, _ := manager2.List(binding)
		return len(rows) == 1 && rows[0].ID == second.ID && rows[0].Status == PortForwardPodGone
	})
}
