package actions

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestActionsUseExactAuthorizationAndNarrowCommands(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 8, 17, 12, 0, 0, 0, time.FixedZone("local", -3*60*60))}
	generations := &generationStub{generation: "gen_1"}
	authorizer := &authorizerStub{}
	adapter := &actionAdapterStub{}
	audit := &auditStub{}
	service, err := newActionService(authorizer, generations, adapter, audit, clock, time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBinding("gen_1")

	restart, replayed, err := service.Restart(context.Background(), binding, RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, "restart-key-1234", testRestart("gen_1", "payments", "api"))
	if err != nil || replayed || !restart.Accepted || restart.ResourceVersion == nil || *restart.ResourceVersion != "200" {
		t.Fatalf("unexpected restart result: %#v replayed=%v err=%v", restart, replayed, err)
	}
	scaleRequest := testScale("gen_1", "statefulsets", "payments", "ledger", 0)
	scale, err := service.Scale(context.Background(), binding, RouteTarget{Kind: "statefulsets", Namespace: "payments", Name: "ledger"}, scaleRequest)
	if err != nil || scale.Replicas != 0 || scale.ResourceVersion == nil || *scale.ResourceVersion != "201" {
		t.Fatalf("unexpected scale result: %#v err=%v", scale, err)
	}
	deleted, err := service.DeletePod(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}, testDelete("gen_1", "payments", "api-abc"))
	if err != nil || !deleted.Accepted || deleted.ResourceVersion != nil {
		t.Fatalf("unexpected delete result: %#v err=%v", deleted, err)
	}

	calls := authorizer.snapshot()
	if len(calls) != 3 {
		t.Fatalf("expected three fresh reviews, got %#v", calls)
	}
	want := []struct {
		group, resource, subresource, verb, name string
	}{
		{"apps", "deployments", "", "patch", "api"},
		{"apps", "statefulsets", "scale", "update", "ledger"},
		{"", "pods", "", "delete", "api-abc"},
	}
	for index, expected := range want {
		key := calls[index].key
		if key.Generation != "gen_1" || key.Namespace != "payments" || key.APIGroup != expected.group || key.Resource != expected.resource || key.Subresource != expected.subresource || key.Verb != expected.verb || key.ResourceName != expected.name {
			t.Fatalf("review %d was not exact: %#v", index, key)
		}
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.restarts) != 1 || adapter.restarts[0].ExpectedResourceVersion != "123" || adapter.restarts[0].RestartedAt.Location() != time.UTC {
		t.Fatalf("restart command was not minimal: %#v", adapter.restarts)
	}
	if len(adapter.scales) != 1 || adapter.scales[0].Replicas != 0 || adapter.scales[0].ExpectedResourceVersion != "124" {
		t.Fatalf("scale command did not use the scale-only contract: %#v", adapter.scales)
	}
	if len(adapter.deletes) != 1 || adapter.deletes[0].ExpectedUID != "uid-1" || adapter.deletes[0].ExpectedResourceVersion != "125" {
		t.Fatalf("delete preconditions missing: %#v", adapter.deletes)
	}
}

func TestActionValidationPrecedesAuthorizationAndOperation(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	authorizer := &authorizerStub{}
	adapter := &actionAdapterStub{}
	service, err := NewActionService(authorizer, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	binding := testBinding("gen_1")
	restart := testRestart("gen_1", "payments", "api")
	restart.Target.Name = "other"
	_, _, err = service.Restart(context.Background(), binding, RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, "restart-key-1234", restart)
	requireCode(t, err, CodeValidationFailed)

	scale := testScale("gen_1", "deployments", "payments", "api", -1)
	_, err = service.Scale(context.Background(), binding, RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, scale)
	requireCode(t, err, CodeValidationFailed)
	scale.Replicas = math.MaxInt32 + 1
	_, err = service.Scale(context.Background(), binding, RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, scale)
	requireCode(t, err, CodeValidationFailed)

	deleteRequest := testDelete("gen_1", "payments", "api-abc")
	deleteRequest.ExpectedUID = ""
	_, err = service.DeletePod(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}, deleteRequest)
	requireCode(t, err, CodeValidationFailed)
	if len(authorizer.snapshot()) != 0 {
		t.Fatalf("invalid requests reached authorization: %#v", authorizer.snapshot())
	}
}

func TestRestartIdempotencySharesConcurrentExecutionAndReplaysResult(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	authorizer := &authorizerStub{}
	adapter := &actionAdapterStub{started: make(chan struct{}, 1), release: make(chan struct{})}
	service, err := NewActionService(authorizer, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}
	request := testRestart("gen_1", "payments", "api")
	type response struct {
		value    ActionAcceptedDTO
		replayed bool
		err      error
	}
	responses := make(chan response, 2)
	for range 2 {
		go func() {
			value, replayed, callErr := service.Restart(context.Background(), binding, route, "same-restart-key", request)
			responses <- response{value: value, replayed: replayed, err: callErr}
		}()
	}
	<-adapter.started
	close(adapter.release)
	first, second := <-responses, <-responses
	if first.err != nil || second.err != nil || first.value.ResourceVersion == nil || second.value.ResourceVersion == nil || *first.value.ResourceVersion != *second.value.ResourceVersion {
		t.Fatalf("concurrent results differ: %#v %#v", first, second)
	}
	if first.replayed == second.replayed {
		t.Fatalf("exactly one caller must be replayed: %#v %#v", first, second)
	}
	adapter.mu.Lock()
	restarts := len(adapter.restarts)
	adapter.mu.Unlock()
	if restarts != 1 || len(authorizer.snapshot()) != 1 {
		t.Fatalf("duplicate execution escaped registry: restarts=%d reviews=%d", restarts, len(authorizer.snapshot()))
	}
}

func TestRestartIdempotencyBindsBodyPathProfileAndGeneration(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	service, err := NewActionService(&authorizerStub{}, generations, &actionAdapterStub{}, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	binding := testBinding("gen_1")
	route := RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}
	request := testRestart("gen_1", "payments", "api")
	if _, _, err := service.Restart(context.Background(), binding, route, "binding-key-1234", request); err != nil {
		t.Fatal(err)
	}

	changedBody := request
	changedBody.ExpectedResourceVersion = "999"
	_, _, err = service.Restart(context.Background(), binding, route, "binding-key-1234", changedBody)
	requireCode(t, err, CodeIdempotencyConflict)

	changedPath := testRestart("gen_1", "payments", "other")
	_, _, err = service.Restart(context.Background(), binding, RouteTarget{Kind: "deployments", Namespace: "payments", Name: "other"}, "binding-key-1234", changedPath)
	requireCode(t, err, CodeIdempotencyConflict)

	changedProfile := binding
	changedProfile.ClusterProfileID = 8
	changedProfileRequest := request
	changedProfileRequest.Target.ClusterProfileID = 8
	_, _, err = service.Restart(context.Background(), changedProfile, route, "binding-key-1234", changedProfileRequest)
	requireCode(t, err, CodeIdempotencyConflict)

	generations.set("gen_2")
	changedGeneration := testBinding("gen_2")
	changedGenerationRequest := testRestart("gen_2", "payments", "api")
	_, _, err = service.Restart(context.Background(), changedGeneration, route, "binding-key-1234", changedGenerationRequest)
	requireCode(t, err, CodeIdempotencyConflict)
}

func TestActionOperationErrorsAreAuthoritativeAndSanitized(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	adapter := &actionAdapterStub{restartErr: apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "api", errors.New("secret upstream detail"))}
	service, err := NewActionService(&authorizerStub{}, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.Restart(context.Background(), testBinding("gen_1"), RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, "conflict-key-123", testRestart("gen_1", "payments", "api"))
	requireCode(t, err, CodeConflict)
	if err.Error() == "secret upstream detail" {
		t.Fatal("upstream error detail escaped the public error")
	}
}

func TestMutationDetachesOnlyAfterDispatchAndGenerationCancelsIt(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	adapter := &actionAdapterStub{started: make(chan struct{}, 1), release: make(chan struct{})}
	service, err := NewActionService(&authorizerStub{}, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	requestContext, cancelRequest := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, callErr := service.Restart(requestContext, testBinding("gen_1"), RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, "detached-key-123", testRestart("gen_1", "payments", "api"))
		result <- callErr
	}()
	<-adapter.started
	cancelRequest()
	close(adapter.release)
	if err := <-result; err != nil {
		t.Fatalf("request cancellation after dispatch canceled mutation: %v", err)
	}

	adapter = &actionAdapterStub{started: make(chan struct{}, 1), release: make(chan struct{})}
	service, err = NewActionService(&authorizerStub{}, generations, adapter, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	result = make(chan error, 1)
	go func() {
		_, _, callErr := service.Restart(context.Background(), testBinding("gen_1"), RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, "generation-key-1", testRestart("gen_1", "payments", "api"))
		result <- callErr
	}()
	<-adapter.started
	generations.set("gen_2")
	service.OnGeneration("gen_2")
	requireCode(t, <-result, CodeGenerationChanged)
}

func TestActionAuditContainsOnlyAllowlistedMetadata(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	audit := &auditStub{}
	service, err := NewActionService(&authorizerStub{}, generations, &actionAdapterStub{}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Scale(context.Background(), testBinding("gen_1"), RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, testScale("gen_1", "deployments", "payments", "api", 3)); err != nil {
		t.Fatal(err)
	}
	events := audit.snapshot()
	if len(events) != 1 || events[0].Operation != "scale" || events[0].Resource != "Deployment/api" || events[0].Namespace != "payments" || events[0].ErrorCode != "" {
		t.Fatalf("unexpected audit metadata: %#v", events)
	}
}

func TestConcurrentActionCallsAreRaceSafe(t *testing.T) {
	generations := &generationStub{generation: "gen_1"}
	service, err := NewActionService(&authorizerStub{}, generations, &actionAdapterStub{}, NoopAuditSink{})
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for index := range 20 {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			name := "api"
			_, _ = service.Scale(context.Background(), testBinding("gen_1"), RouteTarget{Kind: "deployments", Namespace: "payments", Name: name}, testScale("gen_1", "deployments", "payments", name, int64(index)))
		}(index)
	}
	group.Wait()
}
