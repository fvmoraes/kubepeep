package actions

import (
	"context"
	"strings"
	"testing"
	"time"
)

func newTestWorkloadService(t *testing.T) (*Service, *authorizerStub, *actionAdapterStub) {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)}
	generations := &generationStub{generation: "gen_1"}
	authorizer := &authorizerStub{}
	adapter := &actionAdapterStub{}
	service, err := newActionService(context.Background(), authorizer, generations, adapter, NoopAuditSink{}, clock, time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return service, authorizer, adapter
}

func testWorkloadDelete(generation, targetKind, namespace, name string) WorkloadDeleteRequest {
	return WorkloadDeleteRequest{
		Confirmation: Confirmation{
			Confirmed:          true,
			Action:             ActionDeleteWorkload,
			ConsequenceCode:    ConsequenceDeleteResource,
			Target:             testTarget(targetKind, namespace, name),
			ExpectedGeneration: generation,
		},
		ExpectedUID:             "uid-wl-1",
		ExpectedResourceVersion: "126",
	}
}

func testCronJobSuspend(generation string, suspend bool) CronJobSuspendRequest {
	consequence := ConsequenceSuspendCronJob
	if !suspend {
		consequence = ConsequenceResumeCronJob
	}
	return CronJobSuspendRequest{
		Suspend: suspend,
		Confirmation: Confirmation{
			Confirmed:          true,
			Action:             ActionUpdateCronJobSuspend,
			ConsequenceCode:    consequence,
			Target:             testTarget("CronJob", "payments", "nightly"),
			ExpectedGeneration: generation,
		},
		ExpectedResourceVersion: "127",
	}
}

func testCronJobTrigger(generation string) CronJobTriggerRequest {
	return CronJobTriggerRequest{
		Confirmation: Confirmation{
			Confirmed:          true,
			Action:             ActionTriggerCronJob,
			ConsequenceCode:    ConsequenceCreateJobFromCronJob,
			Target:             testTarget("CronJob", "payments", "nightly"),
			ExpectedGeneration: generation,
		},
	}
}

func TestRestartSupportsStatefulSetsAndDaemonSetsWithExactCapabilities(t *testing.T) {
	service, authorizer, _ := newTestWorkloadService(t)
	binding := testBinding("gen_1")

	statefulRequest := testRestart("gen_1", "payments", "ledger")
	statefulRequest.Target.Kind = "StatefulSet"
	result, _, err := service.Restart(context.Background(), binding, RouteTarget{Kind: "statefulsets", Namespace: "payments", Name: "ledger"}, "restart-key-sts-1", statefulRequest)
	if err != nil || !result.Accepted {
		t.Fatalf("statefulset restart failed: %#v err=%v", result, err)
	}

	daemonRequest := testRestart("gen_1", "payments", "agent")
	daemonRequest.Target.Kind = "DaemonSet"
	result, _, err = service.Restart(context.Background(), binding, RouteTarget{Kind: "daemonsets", Namespace: "payments", Name: "agent"}, "restart-key-ds-01", daemonRequest)
	if err != nil || !result.Accepted {
		t.Fatalf("daemonset restart failed: %#v err=%v", result, err)
	}

	rejected := testRestart("gen_1", "payments", "api")
	_, _, err = service.Restart(context.Background(), binding, RouteTarget{Kind: "cronjobs", Namespace: "payments", Name: "api"}, "restart-key-cj-1", rejected)
	requireCode(t, err, CodeValidationFailed)

	calls := authorizer.snapshot()
	if len(calls) != 2 {
		t.Fatalf("expected two reviews, got %#v", calls)
	}
	if calls[0].key.Resource != "statefulsets" || calls[0].key.Verb != "patch" || calls[0].key.ResourceName != "ledger" {
		t.Fatalf("statefulset review was not exact: %#v", calls[0].key)
	}
	if calls[1].key.Resource != "daemonsets" || calls[1].key.Verb != "patch" || calls[1].key.ResourceName != "agent" {
		t.Fatalf("daemonset review was not exact: %#v", calls[1].key)
	}
}

func TestDeleteWorkloadRequiresPreconditionsAndExactCapability(t *testing.T) {
	service, authorizer, _ := newTestWorkloadService(t)
	binding := testBinding("gen_1")

	request := testWorkloadDelete("gen_1", "Deployment", "payments", "api")
	result, err := service.DeleteWorkload(context.Background(), binding, RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, request)
	if err != nil || !result.Accepted || result.Action != ActionDeleteWorkload {
		t.Fatalf("unexpected delete result: %#v err=%v", result, err)
	}

	cronJobRequest := testWorkloadDelete("gen_1", "CronJob", "payments", "nightly")
	if _, err := service.DeleteWorkload(context.Background(), binding, RouteTarget{Kind: "cronjobs", Namespace: "payments", Name: "nightly"}, cronJobRequest); err != nil {
		t.Fatalf("cronjob delete failed: %v", err)
	}

	missingUID := testWorkloadDelete("gen_1", "Deployment", "payments", "api")
	missingUID.ExpectedUID = ""
	if _, err := service.DeleteWorkload(context.Background(), binding, RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, missingUID); err == nil {
		t.Fatal("expected missing UID to fail validation")
	}

	unsupported := testWorkloadDelete("gen_1", "Pod", "payments", "api-abc")
	if _, err := service.DeleteWorkload(context.Background(), binding, RouteTarget{Kind: "pods", Namespace: "payments", Name: "api-abc"}, unsupported); err == nil {
		t.Fatal("expected pod kind to be rejected by the workload delete action")
	}

	calls := authorizer.snapshot()
	if len(calls) != 2 {
		t.Fatalf("expected two reviews, got %#v", calls)
	}
	if calls[0].key.Resource != "deployments" || calls[0].key.Verb != "delete" {
		t.Fatalf("deployment delete review was not exact: %#v", calls[0].key)
	}
	if calls[1].key.Resource != "cronjobs" || calls[1].key.Verb != "delete" {
		t.Fatalf("cronjob delete review was not exact: %#v", calls[1].key)
	}
}

func TestUpdateCronJobSuspendValidatesConsequencePerDirection(t *testing.T) {
	service, authorizer, _ := newTestWorkloadService(t)
	binding := testBinding("gen_1")

	suspend := testCronJobSuspend("gen_1", true)
	result, err := service.UpdateCronJobSuspend(context.Background(), binding, RouteTarget{Kind: "cronjobs", Namespace: "payments", Name: "nightly"}, suspend)
	if err != nil || !result.Accepted {
		t.Fatalf("suspend failed: %#v err=%v", result, err)
	}

	resume := testCronJobSuspend("gen_1", false)
	if _, err := service.UpdateCronJobSuspend(context.Background(), binding, RouteTarget{Kind: "cronjobs", Namespace: "payments", Name: "nightly"}, resume); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	swapped := testCronJobSuspend("gen_1", true)
	swapped.ConsequenceCode = ConsequenceResumeCronJob
	if _, err := service.UpdateCronJobSuspend(context.Background(), binding, RouteTarget{Kind: "cronjobs", Namespace: "payments", Name: "nightly"}, swapped); err == nil {
		t.Fatal("expected swapped consequence to fail validation")
	}

	wrongKind := testCronJobSuspend("gen_1", true)
	if _, err := service.UpdateCronJobSuspend(context.Background(), binding, RouteTarget{Kind: "deployments", Namespace: "payments", Name: "nightly"}, wrongKind); err == nil {
		t.Fatal("expected non-cronjob route to fail validation")
	}

	calls := authorizer.snapshot()
	if len(calls) != 2 {
		t.Fatalf("expected two reviews, got %#v", calls)
	}
	if calls[0].key.Resource != "cronjobs" || calls[0].key.Verb != "patch" || calls[0].key.ResourceName != "nightly" {
		t.Fatalf("suspend review was not exact: %#v", calls[0].key)
	}
}

func TestTriggerCronJobCreatesGeneratedJobName(t *testing.T) {
	service, authorizer, _ := newTestWorkloadService(t)
	binding := testBinding("gen_1")

	result, err := service.TriggerCronJob(context.Background(), binding, RouteTarget{Kind: "cronjobs", Namespace: "payments", Name: "nightly"}, testCronJobTrigger("gen_1"))
	if err != nil || !result.Accepted {
		t.Fatalf("trigger failed: %#v err=%v", result, err)
	}
	if result.ResourceVersion == nil {
		t.Fatal("expected trigger result to carry the created job resource version")
	}

	unconfirmed := testCronJobTrigger("gen_1")
	unconfirmed.Confirmed = false
	if _, err := service.TriggerCronJob(context.Background(), binding, RouteTarget{Kind: "cronjobs", Namespace: "payments", Name: "nightly"}, unconfirmed); err == nil {
		t.Fatal("expected unconfirmed trigger to fail validation")
	}

	calls := authorizer.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected one review, got %#v", calls)
	}
	key := calls[0].key
	if key.Resource != "jobs" || key.Verb != "create" || key.ResourceName != "" {
		t.Fatalf("trigger review was not namespace create on jobs: %#v", key)
	}
}

func TestRandomSuffixStaysDNSFriendly(t *testing.T) {
	for range 64 {
		value := randomSuffix()
		if len(value) != 5 {
			t.Fatalf("unexpected suffix length: %q", value)
		}
		if strings.Trim(value, "abcdefghijklmnopqrstuvwxyz012345") != "" {
			t.Fatalf("suffix left the alphabet: %q", value)
		}
	}
}
