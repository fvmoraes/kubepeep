package kubernetesruntime

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	fakediscovery "k8s.io/client-go/discovery/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	metricstypes "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	kubeadapter "github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type fixedDashboardClients struct {
	set       dashboardClientSet
	err       error
	mu        sync.Mutex
	bindings  []namespaces.SelectionBinding
	cancelled int
}

func (provider *fixedDashboardClients) Unary(ctx context.Context, binding namespaces.SelectionBinding) (context.Context, context.CancelFunc, dashboardClientSet, error) {
	provider.mu.Lock()
	provider.bindings = append(provider.bindings, binding)
	provider.mu.Unlock()
	if provider.err != nil {
		return nil, nil, dashboardClientSet{}, provider.err
	}
	requestContext, rawCancel := context.WithCancel(ctx)
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			rawCancel()
			provider.mu.Lock()
			provider.cancelled++
			provider.mu.Unlock()
		})
	}
	return requestContext, cancel, provider.set, nil
}

type dashboardAuthorizationStub struct {
	mu           sync.Mutex
	denyResource string
	keys         []authorization.Key
}

func (stub *dashboardAuthorizationStub) capability(key authorization.Key) authorization.Capability {
	decision := authorization.DecisionAllowed
	reason := authorization.ReasonSARAllowed
	if key.Resource == stub.denyResource {
		decision = authorization.DecisionDenied
		reason = authorization.ReasonSARDenied
	}
	return authorization.Capability{
		Namespace: key.Namespace, APIGroup: key.APIGroup,
		Resource: key.Resource, Subresource: key.Subresource, Verb: key.Verb, ResourceName: key.ResourceName,
		Decision: decision, ReasonCode: reason, ExpiresAt: time.Now().Add(time.Minute),
	}
}

func (stub *dashboardAuthorizationStub) Check(_ context.Context, key authorization.Key) authorization.Capability {
	stub.mu.Lock()
	stub.keys = append(stub.keys, key)
	stub.mu.Unlock()
	return stub.capability(key)
}

func (stub *dashboardAuthorizationStub) Refresh(ctx context.Context, key authorization.Key) authorization.Capability {
	return stub.Check(ctx, key)
}

func (stub *dashboardAuthorizationStub) Revalidate(ctx context.Context, key authorization.Key, _ authorization.OperationKind) (authorization.Capability, error) {
	capability := stub.Check(ctx, key)
	if capability.Decision == authorization.DecisionDenied {
		return capability, &authorization.PublicError{Code: authorization.CodeForbidden, Message: "Kubernetes denied this operation.", HTTPStatus: http.StatusForbidden}
	}
	return capability, nil
}

func (stub *dashboardAuthorizationStub) Guard(ctx context.Context, key authorization.Key, kind authorization.OperationKind, operation authorization.Operation) (authorization.GuardResult, error) {
	capability, err := stub.Revalidate(ctx, key, kind)
	result := authorization.GuardResult{Capability: capability}
	if err != nil {
		return result, err
	}
	result.Executed = true
	return result, operation(ctx)
}

func (*dashboardAuthorizationStub) InvalidateGeneration(string) {}
func (*dashboardAuthorizationStub) InvalidateAll()              {}

func dashboardTestBinding() namespaces.SelectionBinding {
	return namespaces.SelectionBinding{ClusterProfileID: 11, Context: "dev", Cluster: "cluster-a", Generation: "gen-11"}
}

func TestDashboardAdapterPreservesAuthenticationClassification(t *testing.T) {
	t.Parallel()
	converted := toDashboardError(&kubeadapter.SafeError{
		Code: kubeadapter.CodeAuthenticationUnavailable, Message: "safe", Retryable: true,
	})
	type publicError interface {
		Code() string
		PublicMessage() string
	}
	var safe publicError
	if !errors.As(converted, &safe) || safe.Code() != dashboard.CodeAuthenticationUnavailable || strings.Contains(safe.PublicMessage(), "credential") {
		t.Fatalf("authentication classification was lost or unsafe: %#v", converted)
	}

	converted = toDashboardError(&authorization.PublicError{
		Code: authorization.CodeAuthenticationUnavailable, Message: "safe", HTTPStatus: http.StatusServiceUnavailable,
	})
	if !errors.As(converted, &safe) || safe.Code() != dashboard.CodeAuthenticationUnavailable {
		t.Fatalf("authorization authentication classification was lost: %#v", converted)
	}
}

func dashboardTestClients(t *testing.T, objects ...runtime.Object) (*fixedDashboardClients, *kubefake.Clientset, *metricsfake.Clientset) {
	t.Helper()
	kubernetesClient := kubefake.NewSimpleClientset(objects...)
	discovery := kubernetesClient.Discovery().(*fakediscovery.FakeDiscovery)
	discovery.Resources = []*metav1.APIResourceList{{
		GroupVersion: "metrics.k8s.io/v1beta1",
		APIResources: []metav1.APIResource{{Name: "pods", Namespaced: true, Kind: "PodMetrics"}},
	}}
	metric := metricstypes.PodMetrics{
		TypeMeta:   metav1.TypeMeta{APIVersion: "metrics.k8s.io/v1beta1", Kind: "PodMetrics"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api"},
		Window:     metav1.Duration{Duration: time.Minute},
	}
	metricsClient := metricsfake.NewSimpleClientset()
	metricsClient.PrependReactor("list", "pods", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, &metricstypes.PodMetricsList{Items: []metricstypes.PodMetrics{metric}}, nil
	})
	provider := &fixedDashboardClients{set: dashboardClientSet{
		kubernetes: kubernetesClient, streaming: kubernetesClient, metrics: metricsClient,
	}}
	return provider, kubernetesClient, metricsClient
}

func TestDashboardAdapterListsTypedResourcesAndPreservesBinding(t *testing.T) {
	controller := true
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api"}}
	statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "db"}}
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "agent"}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "migration"}}
	cronJob := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "backup"}}
	replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Namespace: "payments", Name: "api-rs",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: types.UID("deployment-uid"), Controller: &controller}},
	}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "payments", Name: "api-1", UID: types.UID("pod-uid"),
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-rs", UID: types.UID("rs-uid"), Controller: &controller}},
	}}
	event := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "warning"}, Type: "Warning", Reason: "Unhealthy"}
	provider, _, _ := dashboardTestClients(t, deployment, statefulSet, daemonSet, job, cronJob, replicaSet, pod, event)
	authorizer := &dashboardAuthorizationStub{}
	adapter := &dashboardAdapter{clients: provider, authorization: authorizer, binding: dashboardTestBinding()}

	pods, err := adapter.ListPods(context.Background(), "payments", dashboard.PageRequest{Limit: 100})
	if err != nil || len(pods.Items) != 1 || pods.Items[0].Name != "api-1" {
		t.Fatalf("pods = %#v, err = %v", pods, err)
	}
	events, err := adapter.ListEvents(context.Background(), "payments", dashboard.PageRequest{Limit: 100})
	if err != nil || len(events.Items) != 1 || events.Items[0].Reason != "Unhealthy" {
		t.Fatalf("events = %#v, err = %v", events, err)
	}
	workloads, err := adapter.ListWorkloads(context.Background(), "payments", dashboard.PageRequest{Limit: 100})
	if err != nil || workloadPageLength(workloads) != 5 || workloads.Continue != "" {
		t.Fatalf("workloads = %#v, err = %v", workloads, err)
	}
	owner, err := adapter.ResolvePodOwner(context.Background(), pod)
	if err != nil || owner == nil || owner.Kind != "Deployment" || owner.Name != "api" {
		t.Fatalf("owner = %#v, err = %v", owner, err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	for _, binding := range provider.bindings {
		if binding != dashboardTestBinding() {
			t.Fatalf("binding = %+v", binding)
		}
	}
	if provider.cancelled != len(provider.bindings) {
		t.Fatalf("cancellations = %d, calls = %d", provider.cancelled, len(provider.bindings))
	}
}

func TestDashboardAdapterKeepsMetricsOptionalAndRBACBound(t *testing.T) {
	provider, _, _ := dashboardTestClients(t)
	authorizer := &dashboardAuthorizationStub{}
	adapter := &dashboardAdapter{clients: provider, authorization: authorizer, binding: dashboardTestBinding()}
	available, err := adapter.Available(context.Background())
	if err != nil || !available {
		t.Fatalf("available = %v, err = %v", available, err)
	}
	decision, err := adapter.CanListPodMetrics(context.Background(), "payments")
	if err != nil || decision != dashboard.PermissionAllowed {
		t.Fatalf("decision = %q, err = %v", decision, err)
	}
	metrics, err := adapter.ListPodMetrics(context.Background(), "payments", dashboard.PageRequest{Limit: 100})
	if err != nil || len(metrics.Items) != 1 || metrics.Window != time.Minute {
		t.Fatalf("metrics = %#v, err = %v", metrics, err)
	}

	authorizer.denyResource = "pods"
	_, err = adapter.ListPods(context.Background(), "payments", dashboard.PageRequest{Limit: 100})
	var safe interface {
		Code() string
		Denied() bool
	}
	if !errors.As(err, &safe) || safe.Code() != dashboard.CodeForbidden || !safe.Denied() {
		t.Fatalf("denied error = %T %v", err, err)
	}
}

func TestDashboardAdapterKeepsAllowedWorkloadKindsAfterDeniedKind(t *testing.T) {
	statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "db"}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "migration"}}
	provider, _, _ := dashboardTestClients(t, statefulSet, job)
	authorizer := &dashboardAuthorizationStub{denyResource: "deployments"}
	adapter := &dashboardAdapter{clients: provider, authorization: authorizer, binding: dashboardTestBinding()}

	page, err := adapter.ListWorkloads(context.Background(), "payments", dashboard.PageRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Deployments) != 0 || len(page.StatefulSets) != 1 || len(page.Jobs) != 1 || len(page.Issues) != 1 {
		t.Fatalf("allowed kinds were discarded after denial: %+v", page)
	}
	if page.Issues[0].Kind != "Deployment" {
		t.Fatalf("issue kind = %q, want Deployment", page.Issues[0].Kind)
	}
	var safe interface {
		Code() string
		Denied() bool
	}
	if !errors.As(page.Issues[0].Err, &safe) || safe.Code() != dashboard.CodeForbidden || !safe.Denied() {
		t.Fatalf("kind issue = %T %v", page.Issues[0].Err, page.Issues[0].Err)
	}

	block := dashboard.NewWorkloadService(adapter, nil, dashboard.QueryBudget{}).List(
		context.Background(), dashboard.Selection{Namespaces: []string{"payments"}},
	)
	if block.Complete || len(block.Value) != 2 || len(block.Errors) != 1 || block.Errors[0].Code != dashboard.CodeForbidden {
		t.Fatalf("partial workload service result = %+v", block)
	}
}

func TestDashboardAdapterEmptyMetricsAreAuthoritativeWithoutInventedUsage(t *testing.T) {
	provider, _, metricsClient := dashboardTestClients(t)
	metricsClient.PrependReactor("list", "pods", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, &metricstypes.PodMetricsList{}, nil
	})
	adapter := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{}, binding: dashboardTestBinding()}

	page, err := adapter.ListPodMetrics(context.Background(), "payments", dashboard.PageRequest{Limit: 100})
	if err != nil || len(page.Items) != 0 || page.Window < time.Second {
		t.Fatalf("empty metrics page = %#v, err = %v", page, err)
	}
	block := dashboard.NewMetricsService(adapter, adapter, nil, dashboard.QueryBudget{}).Collect(
		context.Background(), dashboard.Selection{Namespaces: []string{"payments"}},
	)
	if !block.Complete || block.Value.WindowSeconds <= 0 || len(block.Value.Pods) != 0 || len(block.Value.TopCPU) != 0 || len(block.Value.TopMemory) != 0 {
		t.Fatalf("authoritative empty metrics response = %+v", block)
	}
}

func TestEmptyMetricsObservationWindowRoundsUpAndStaysPositive(t *testing.T) {
	for _, test := range []struct {
		elapsed time.Duration
		want    time.Duration
	}{
		{elapsed: 0, want: time.Second},
		{elapsed: time.Nanosecond, want: time.Second},
		{elapsed: time.Second, want: time.Second},
		{elapsed: time.Second + time.Nanosecond, want: 2 * time.Second},
	} {
		if got := emptyMetricsObservationWindow(test.elapsed); got != test.want {
			t.Fatalf("emptyMetricsObservationWindow(%s) = %s, want %s", test.elapsed, got, test.want)
		}
	}
}

func TestWorkloadContinuationIsClosedAndRoundTrips(t *testing.T) {
	encoded, err := encodeWorkloadContinue(workloadContinue{Kind: workloadJobs, Token: "opaque-upstream"})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeWorkloadContinue(encoded)
	if err != nil || decoded.Kind != workloadJobs || decoded.Token != "opaque-upstream" {
		t.Fatalf("decoded = %+v, err = %v", decoded, err)
	}
	for _, invalid := range []string{"not-base64!", "e30", strings.Repeat("x", maximumInternalContinueBytes+1)} {
		if _, err := decodeWorkloadContinue(invalid); err == nil {
			t.Fatalf("accepted invalid token %q", invalid)
		}
	}
}

func TestDashboardBackendStoresOnlyGenerationScopedScanCounter(t *testing.T) {
	provider, _, _ := dashboardTestClients(t)
	backend := newDashboardBackend(provider, &dashboardAuthorizationStub{})
	ctx, cancel, sequence := backend.beginScan(context.Background(), "gen-1")
	result := dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]{
		Value:    []dashboard.LogMatchDTO{{Namespace: "payments", Pod: "api", Container: "api", Excerpt: "redacted"}},
		Complete: true,
	}
	backend.finishScan(sequence, "gen-1", ctx, result)
	cancel()
	counter := backend.currentLogCounter("gen-1")
	if counter.State != dashboard.CounterAvailable || counter.Value == nil || *counter.Value != 1 {
		t.Fatalf("counter = %+v", counter)
	}
	if other := backend.currentLogCounter("gen-2"); other.State != dashboard.CounterNotCollected || other.Value != nil {
		t.Fatalf("other generation counter = %+v", other)
	}

	oldContext, oldCancel, _ := backend.beginScan(context.Background(), "gen-1")
	_, replacementCancel, _ := backend.beginScan(context.Background(), "gen-1")
	defer replacementCancel()
	defer oldCancel()
	select {
	case <-oldContext.Done():
	case <-time.After(time.Second):
		t.Fatal("new scan did not cancel predecessor")
	}

	canceledContext, canceled, canceledSequence := backend.beginScan(context.Background(), "gen-1")
	canceled()
	backend.finishScan(canceledSequence, "gen-1", canceledContext, result)
	if current := backend.currentLogCounter("gen-1"); current.State != dashboard.CounterNotCollected {
		t.Fatalf("canceled counter = %+v", current)
	}
}

func TestDashboardBackendGenerationChangeCancelsScanAndDropsCounter(t *testing.T) {
	backend := newDashboardBackend(&fixedDashboardClients{}, &dashboardAuthorizationStub{})
	ctx, cancel, sequence := backend.beginScan(context.Background(), "gen-old")
	defer cancel()
	backend.OnGeneration("gen-new")
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("generation change did not cancel the active scan")
	}
	backend.finishScan(sequence, "gen-old", context.Background(), dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]{
		Value: []dashboard.LogMatchDTO{{Pod: "stale"}}, Complete: true, Errors: []dashboard.PartialError{},
	})
	if counter := backend.currentLogCounter("gen-new"); counter.State != dashboard.CounterNotCollected || counter.Value != nil {
		t.Fatalf("counter=%#v", counter)
	}
}

func TestDashboardBackendSummaryUsesBoundedServices(t *testing.T) {
	ready := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api", CreationTimestamp: metav1.Now()},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "api", Ready: ready, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}}},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api", Generation: 1, CreationTimestamp: metav1.Now()},
		Spec:       appsv1.DeploymentSpec{Replicas: int32Pointer(1), Strategy: appsv1.DeploymentStrategy{RollingUpdate: &appsv1.RollingUpdateDeployment{MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0}}}},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, ReadyReplicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1},
	}
	provider, _, _ := dashboardTestClients(t, pod, deployment)
	backend := newDashboardBackend(provider, &dashboardAuthorizationStub{})
	result := backend.Summary(context.Background(), dashboardTestBinding(), namespaces.ScopeResolution{ScopeName: "payments", Namespaces: []string{"payments"}})
	if !result.Complete || result.Value.PodsTotal.Value == nil || *result.Value.PodsTotal.Value != 1 || result.Value.Namespaces.Value == nil || *result.Value.Namespaces.Value != 1 {
		t.Fatalf("summary = %+v", result)
	}
}

func int32Pointer(value int32) *int32 { return &value }
