package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestWorkloadServiceClassifiesAcrossPagesAndSortsKinds(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	cron := batchv1.CronJob{ObjectMeta: workloadMeta("cron", now)}
	cron.UID = "cron-owner"
	failed := failedOwnedJob("job", cron.UID, now.Add(-time.Hour))
	port := &fakeWorkloadPort{responses: map[string][]WorkloadPage{
		"payments": {
			{CronJobs: []batchv1.CronJob{cron}, Continue: "next"},
			{Jobs: []batchv1.Job{failed}},
		},
	}}
	service := NewWorkloadService(port, fixedClock{now}, QueryBudget{MaxPages: 2})
	block := service.List(context.Background(), Selection{Namespaces: []string{"payments"}})
	if !block.Complete || block.Truncated || block.Coverage.CompletedNamespaces != 1 || len(block.Value) != 2 {
		t.Fatalf("unexpected workload block: %+v", block)
	}
	if block.Value[0].Kind != "Job" || block.Value[0].Status != WorkloadFailed || block.Value[1].Kind != "CronJob" || block.Value[1].Status != WorkloadFailed {
		t.Fatalf("cross-page owner or kind ordering failed: %+v", block.Value)
	}
}

func TestWorkloadServiceUsesExplicitStatefulSetFieldPresence(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	stateful := appsv1.StatefulSet{ObjectMeta: workloadMeta("stateful", now), Status: appsv1.StatefulSetStatus{ObservedGeneration: 2, ReadyReplicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1}}
	port := &fakeWorkloadPort{responses: map[string][]WorkloadPage{
		"payments": {{StatefulSets: []appsv1.StatefulSet{stateful}, StatefulSetAvailable: map[types.UID]bool{}}},
	}}
	block := NewWorkloadService(port, fixedClock{now}, QueryBudget{}).List(context.Background(), Selection{Namespaces: []string{"payments"}})
	if len(block.Value) != 1 || block.Value[0].Status != WorkloadUnknown || block.Value[0].Available != nil {
		t.Fatalf("absent available field became healthy: %+v", block.Value)
	}
}

func TestWorkloadServicePendingCursorAndFailureAreVisible(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	deployment := appsv1.Deployment{ObjectMeta: workloadMeta("deployment", now), Status: appsv1.DeploymentStatus{ObservedGeneration: 2, ReadyReplicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1}}
	port := &fakeWorkloadPort{
		responses: map[string][]WorkloadPage{"a": {{Deployments: []appsv1.Deployment{deployment}, Continue: "more"}}},
		failures:  map[string]error{"b": NewDeniedError()},
	}
	block := NewWorkloadService(port, fixedClock{now}, QueryBudget{MaxPages: 1}).List(context.Background(), Selection{Namespaces: []string{"a", "b"}})
	if block.Complete || !block.Truncated || len(block.Value) != 1 || len(block.Coverage.DeniedNamespaces) != 1 {
		t.Fatalf("incomplete workload result appeared complete: %+v", block)
	}
}

func TestWorkloadServiceRetainsAllowedKindsAndSanitizesKindIssues(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	deployment := appsv1.Deployment{
		ObjectMeta: workloadMeta("allowed", now),
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2, ReadyReplicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1,
		},
	}
	port := &fakeWorkloadPort{responses: map[string][]WorkloadPage{
		"payments": {{
			Deployments: []appsv1.Deployment{deployment},
			Issues: []WorkloadIssue{
				{Kind: "StatefulSet", Err: NewDeniedError()},
				{Kind: "Job", Err: errors.New("sensitive upstream detail")},
			},
		}},
	}}
	block := NewWorkloadService(port, fixedClock{now}, QueryBudget{}).List(
		context.Background(), Selection{Namespaces: []string{"payments"}},
	)
	if block.Complete || len(block.Value) != 1 || block.Value[0].Name != "allowed" || len(block.Errors) != 2 {
		t.Fatalf("allowed workload was discarded with a kind failure: %+v", block)
	}
	if len(block.Coverage.DeniedNamespaces) != 1 || len(block.Coverage.Failed) != 1 {
		t.Fatalf("partial kind coverage was not retained: %+v", block.Coverage)
	}
	for _, item := range block.Errors {
		if item.Message == "sensitive upstream detail" {
			t.Fatal("raw upstream error escaped the dashboard block")
		}
	}
}

func TestUnknownWorkloadMakesDegradedCounterUnavailable(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	stateful := appsv1.StatefulSet{
		ObjectMeta: workloadMeta("unknown", now),
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 2, ReadyReplicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1,
		},
	}
	port := &fakeWorkloadPort{responses: map[string][]WorkloadPage{
		"payments": {{StatefulSets: []appsv1.StatefulSet{stateful}, StatefulSetAvailable: map[types.UID]bool{}}},
	}}
	block := NewWorkloadService(port, fixedClock{now}, QueryBudget{}).Degraded(
		context.Background(), Selection{Namespaces: []string{"payments"}},
	)
	if block.Complete || block.Truncated || len(block.Value) != 0 {
		t.Fatalf("unknown workload fabricated an authoritative degraded zero: %+v", block)
	}
	counter := counterForBlock(int64(len(block.Value)), block.Complete, block.Truncated, block.Errors)
	if counter.State != CounterUnavailable || counter.Value != nil {
		t.Fatalf("unknown degraded counter = %+v, want unavailable without zero", counter)
	}
}

type fakeWorkloadPort struct {
	responses map[string][]WorkloadPage
	failures  map[string]error
	calls     map[string]int
}

func (port *fakeWorkloadPort) ListWorkloads(_ context.Context, namespace string, _ PageRequest) (WorkloadPage, error) {
	if err := port.failures[namespace]; err != nil {
		return WorkloadPage{}, err
	}
	if port.calls == nil {
		port.calls = make(map[string]int)
	}
	index := port.calls[namespace]
	port.calls[namespace]++
	if index >= len(port.responses[namespace]) {
		return WorkloadPage{}, nil
	}
	return port.responses[namespace][index], nil
}
