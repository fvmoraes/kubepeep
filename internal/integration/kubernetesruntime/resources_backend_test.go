package kubernetesruntime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

type fixedResourceClientProvider struct{ set resourceClientSet }

func (provider fixedResourceClientProvider) Unary(ctx context.Context, _ namespaces.SelectionBinding) (context.Context, context.CancelFunc, resourceClientSet, error) {
	requestContext, cancel := context.WithCancel(ctx)
	return requestContext, cancel, provider.set, nil
}

type allowResourceAuthorization struct {
	mu   sync.Mutex
	keys []authorization.Key
}

type selectiveResourceAuthorization struct {
	denied map[string]authorization.Decision
}

func (stub *selectiveResourceAuthorization) Check(_ context.Context, key authorization.Key) authorization.Capability {
	decision := stub.denied[key.APIGroup+"/"+key.Resource+"/"+key.Verb]
	if decision == "" {
		decision = authorization.DecisionAllowed
	}
	return authorization.Capability{Decision: decision, ReasonCode: authorization.ReasonSARAllowed, ExpiresAt: time.Now().Add(time.Minute)}
}

type refreshingResourceAuthorization struct {
	refreshes atomic.Int32
}

type namespaceAwareResourceAuthorization struct {
	mu             sync.Mutex
	globalDecision authorization.Decision
	keys           []authorization.Key
}

func (stub *namespaceAwareResourceAuthorization) Check(_ context.Context, key authorization.Key) authorization.Capability {
	stub.mu.Lock()
	stub.keys = append(stub.keys, key)
	stub.mu.Unlock()
	decision := authorization.DecisionAllowed
	if key.Namespace == "" {
		decision = stub.globalDecision
	}
	return authorization.Capability{Decision: decision}
}

func (*refreshingResourceAuthorization) Check(context.Context, authorization.Key) authorization.Capability {
	return authorization.Capability{Decision: authorization.DecisionAllowed}
}
func (stub *refreshingResourceAuthorization) Refresh(context.Context, authorization.Key) authorization.Capability {
	stub.refreshes.Add(1)
	return authorization.Capability{Decision: authorization.DecisionDenied}
}

func (stub *allowResourceAuthorization) Check(_ context.Context, key authorization.Key) authorization.Capability {
	stub.mu.Lock()
	stub.keys = append(stub.keys, key)
	stub.mu.Unlock()
	return authorization.Capability{Decision: authorization.DecisionAllowed, ReasonCode: authorization.ReasonSARAllowed, ExpiresAt: time.Now().Add(time.Minute)}
}

func TestPageLocalWorkloadAndPodFiltersAndSorts(t *testing.T) {
	workloads := []resources.WorkloadDTO{
		{Namespace: "b", Kind: "Deployment", Name: "api", Status: resources.WorkloadHealthy, AgeSeconds: 20},
		{Namespace: "a", Kind: "StatefulSet", Name: "ledger", Status: resources.WorkloadDegraded, AgeSeconds: 30},
		{Namespace: "a", Kind: "Deployment", Name: "worker", Status: resources.WorkloadDegraded, AgeSeconds: 10},
	}
	filtered := filterSortWorkloads(workloads, resources.ListOptions{Search: "LEDGER", Statuses: []string{"Degraded"}, Sort: "age", Order: resources.OrderDescending})
	if len(filtered) != 1 || filtered[0].Name != "ledger" {
		t.Fatalf("unexpected workload filter result: %#v", filtered)
	}

	problematic := true
	pods := []resources.PodDTO{
		{Namespace: "a", Name: "api-1", Status: "Running", Restarts: 4, Node: stringPointer("node-a"), Owner: &resources.OwnerDTO{Name: "api"}, Problematic: true},
		{Namespace: "a", Name: "api-2", Status: "Running", Restarts: 1, Node: stringPointer("node-a"), Owner: &resources.OwnerDTO{Name: "api"}, Problematic: false},
		{Namespace: "b", Name: "worker", Status: "Failed", Restarts: 12, Node: stringPointer("node-b"), Problematic: true},
	}
	filteredPods := filterSortPods(pods, resources.ListOptions{Search: "api", Workload: "api", Node: "node-a", Restarts: resources.RestartGTE3, Problematic: &problematic, Sort: "restarts", Order: resources.OrderDescending})
	if len(filteredPods) != 1 || filteredPods[0].Name != "api-1" {
		t.Fatalf("unexpected pod filter result: %#v", filteredPods)
	}
}

func TestPageLocalEventDefaultDescendingKeepsCanonicalTiesAscending(t *testing.T) {
	one, two := "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"
	items := []resources.EventDTO{{Timestamp: &one, Namespace: "b", ObjectKind: "Pod", ObjectName: "z"}, {Timestamp: &two, Namespace: "a", ObjectKind: "Pod", ObjectName: "a"}, {Timestamp: &one, Namespace: "a", ObjectKind: "Pod", ObjectName: "b"}}
	result := filterSortEvents(items, resources.ListOptions{Sort: "timestamp", Order: resources.OrderDescending})
	if len(result) != 3 || result[0].Timestamp == nil || *result[0].Timestamp != two {
		t.Fatalf("timestamp sort mismatch: %#v", result)
	}
	if result[1].Namespace != "a" || result[2].Namespace != "b" {
		t.Fatalf("descending tie-break mismatch: %#v", result)
	}
}

func TestResourceErrorMappingKeepsPublicClassification(t *testing.T) {
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "missing")
	var domain *resources.DomainError
	if err := mapResourceError(notFound); !errors.As(err, &domain) || domain.Code != resources.CodeNotFound {
		t.Fatalf("not found mapping: %v", err)
	}
	expired := apierrors.NewResourceExpired("expired")
	if err := mapResourceError(expired); !errors.Is(err, resources.ErrResourceExpired) {
		t.Fatalf("resource expiry mapping: %v", err)
	}
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "p", errors.New("sensitive upstream detail"))
	if err := mapResourceError(forbidden); !errors.As(err, &domain) || domain.Code != resources.CodeForbidden || domain.Message != "Access to this resource was denied." {
		t.Fatalf("forbidden mapping: %#v", err)
	}
}

func TestWatchFanoutFallsBackToHTTPBeforeAuthorization(t *testing.T) {
	names := make([]string, 51)
	for index := range names {
		names[index] = "namespace-" + string(rune('a'+index%26)) + string(rune('a'+index/26))
	}
	backend := &ResourceBackend{}
	_, err := backend.AuthorizeTopics(context.Background(), namespaces.SelectionBinding{Generation: "gen"}, namespaces.ScopeResolution{Namespaces: names}, []resources.Topic{resources.TopicWorkloads})
	var domain *resources.DomainError
	if !errors.As(err, &domain) || domain.Code != resources.CodeLimitExceeded {
		t.Fatalf("fanout error=%v", err)
	}
}

func TestAllScopeUsesAuthorizedClusterWideListBeyondFanoutLimit(t *testing.T) {
	client := kubefake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "alpha", Name: "api"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "omega", Name: "worker"}},
	)
	authorizer := &allowResourceAuthorization{}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer, now: time.Now}
	names := make([]string, resources.MaximumNamespaces+25)
	for index := range names {
		names[index] = "namespace-" + string(rune('a'+index%26)) + string(rune('a'+index/26))
	}
	result, err := backend.ListPods(context.Background(), namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, namespaces.ScopeResolution{ScopeName: "all", Namespaces: names, PreferGlobal: true}, resources.ListOptions{Limit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Coverage.RequestedNamespaces != len(names) || result.Coverage.CompletedNamespaces != len(names) {
		t.Fatalf("items=%d coverage=%#v", len(result.Items), result.Coverage)
	}
	for _, key := range authorizer.keys {
		if key.Namespace != "" || key.Verb != "list" || key.Resource != "pods" {
			t.Fatalf("non-global authorization key=%#v", key)
		}
	}
}

func TestAllScopeFallsBackToAuthorizedNamespaceListsWithinLimit(t *testing.T) {
	client := kubefake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "alpha", Name: "api"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "beta", Name: "worker"}},
	)
	authorizer := &namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionDenied}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer, now: time.Now}
	result, err := backend.ListPods(context.Background(), namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, namespaces.ScopeResolution{ScopeName: "all", Namespaces: []string{"alpha", "beta"}, PreferGlobal: true}, resources.ListOptions{Limit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Coverage.RequestedNamespaces != 2 || result.Coverage.CompletedNamespaces != 2 || !result.Page.Complete {
		t.Fatalf("items=%d page=%#v coverage=%#v", len(result.Items), result.Page, result.Coverage)
	}
	seenGlobal, seenAlpha, seenBeta := false, false, false
	for _, key := range authorizer.keys {
		seenGlobal = seenGlobal || key.Namespace == ""
		seenAlpha = seenAlpha || key.Namespace == "alpha"
		seenBeta = seenBeta || key.Namespace == "beta"
	}
	if !seenGlobal || !seenAlpha || !seenBeta {
		t.Fatalf("authorization keys=%#v", authorizer.keys)
	}
}

func TestAllScopeWatchUsesGlobalSARAndFallsBackOnlyWithinFanoutLimit(t *testing.T) {
	names := make([]string, resources.MaximumNamespaces+25)
	for index := range names {
		names[index] = "namespace-" + string(rune('a'+index%26)) + string(rune('a'+index/26))
	}
	binding := namespaces.SelectionBinding{Generation: "gen", Context: "ctx"}
	allowed := &allowResourceAuthorization{}
	backend := &ResourceBackend{authorizer: allowed}
	effective, err := backend.AuthorizeTopics(context.Background(), binding, namespaces.ScopeResolution{ScopeName: "all", Namespaces: names, PreferGlobal: true}, []resources.Topic{resources.TopicPods})
	if err != nil || !effective.PreferGlobal || len(effective.Namespaces) != 1 || effective.Namespaces[0] != "" {
		t.Fatalf("effective=%#v err=%v", effective, err)
	}
	for _, key := range allowed.keys {
		if key.Namespace != "" {
			t.Fatalf("non-global watch SAR=%#v", key)
		}
	}

	fallbackAuthorizer := &namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionDenied}
	backend.authorizer = fallbackAuthorizer
	effective, err = backend.AuthorizeTopics(context.Background(), binding, namespaces.ScopeResolution{ScopeName: "all", Namespaces: []string{"alpha", "beta"}, PreferGlobal: true}, []resources.Topic{resources.TopicPods})
	if err != nil || effective.PreferGlobal || len(effective.Namespaces) != 2 {
		t.Fatalf("fallback=%#v err=%v", effective, err)
	}
	_, err = backend.AuthorizeTopics(context.Background(), binding, namespaces.ScopeResolution{ScopeName: "all", Namespaces: names, PreferGlobal: true}, []resources.Topic{resources.TopicPods})
	if resources.ErrorCodeOf(err) != resources.CodeLimitExceeded {
		t.Fatalf("oversized fallback err=%v", err)
	}
}

func TestContainerTerminationProbeRequiresExactGetAndCurrentTerminatedState(t *testing.T) {
	client := kubefake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api"},
		Status:     corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "api", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}}}},
	})
	authorizer := &allowResourceAuthorization{}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer}
	port := resourceLogPort{backend: backend, binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}}
	if !port.ContainerTerminated(context.Background(), resources.LogTarget{Namespace: "payments", Pod: "api", Container: "api"}) {
		t.Fatal("terminated container was not detected")
	}
	if len(authorizer.keys) != 1 || authorizer.keys[0].Verb != "get" || authorizer.keys[0].Resource != "pods" || authorizer.keys[0].ResourceName != "api" {
		t.Fatalf("probe SAR=%#v", authorizer.keys)
	}
	backend.authorizer = &namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionDenied}
	// A non-global authorizer still allows this exact namespace; make the
	// container running to prove historical termination is not inferred.
	pod, _ := client.CoreV1().Pods("payments").Get(context.Background(), "api", metav1.GetOptions{})
	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}
	_, _ = client.CoreV1().Pods("payments").UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
	if port.ContainerTerminated(context.Background(), resources.LogTarget{Namespace: "payments", Pod: "api", Container: "api"}) {
		t.Fatal("running container was reported as terminated")
	}
}

func TestStreamReauthorizationUsesFreshSARAndRejectsRevocation(t *testing.T) {
	authorizer := &refreshingResourceAuthorization{}
	backend := &ResourceBackend{authorizer: authorizer}
	binding := namespaces.SelectionBinding{Generation: "gen", Context: "ctx"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
	if err := backend.ReauthorizeLogs(context.Background(), binding, "default", "api"); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("log reauthorization=%v", err)
	}
	if err := backend.ReauthorizeTopics(context.Background(), binding, resolution, []resources.Topic{resources.TopicPods}); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("topic reauthorization=%v", err)
	}
	if authorizer.refreshes.Load() != 2 {
		t.Fatalf("refresh calls=%d", authorizer.refreshes.Load())
	}
}

func TestResourceBackendListsPodsThroughAuthorizedDTOBoundary(t *testing.T) {
	client := kubefake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Minute))}, Status: corev1.PodStatus{Phase: corev1.PodRunning}})
	authorizer := &allowResourceAuthorization{}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer, now: time.Now}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
	result, err := backend.ListPods(context.Background(), binding, resolution, resources.ListOptions{Limit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "api" || result.Items[0].Status != "Running" {
		t.Fatalf("items=%#v", result.Items)
	}
	if len(authorizer.keys) != 1 || authorizer.keys[0].Verb != "list" || authorizer.keys[0].Resource != "pods" {
		t.Fatalf("authorization keys=%#v", authorizer.keys)
	}
}

func TestResourceBackendAppliesNormalizedEventDefaultsToPageSort(t *testing.T) {
	observedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	client := kubefake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "default", Name: "older", CreationTimestamp: metav1.NewTime(observedAt.Add(-time.Minute))},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "older"},
			Reason:         "Older",
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "default", Name: "newer", CreationTimestamp: metav1.NewTime(observedAt)},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "newer"},
			Reason:         "Newer",
		},
	)
	backend := &ResourceBackend{
		clients:    fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}},
		authorizer: &allowResourceAuthorization{},
		now:        func() time.Time { return observedAt },
	}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}

	result, err := backend.ListEvents(context.Background(), binding, resolution, resources.ListOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Items[0].Reason != "Newer" || result.Items[1].Reason != "Older" {
		t.Fatalf("default event order=%#v", result.Items)
	}
}

func TestPodDetailIncludesOnlyUIDMatchedAuthorizedEvents(t *testing.T) {
	podUID := types.UID("pod-uid")
	objects := []runtime.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", UID: podUID}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "core-event", UID: "core-event-uid"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "api", UID: podUID}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "wrong-event", UID: "wrong-event-uid"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "other", UID: "other-uid"}},
		&eventsv1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "duplicate-version", UID: "core-event-uid"}, Regarding: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "api", UID: podUID}},
		&eventsv1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "events-v1", UID: "events-v1-uid"}, Regarding: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "api", UID: podUID}},
	}
	client := kubefake.NewSimpleClientset(objects...)
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}}, now: time.Now}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	detail, err := backend.GetPod(context.Background(), binding, namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}, "default", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.RelatedEvents) != 2 || detail.RelatedEvents[0].Name != "core-event" || detail.RelatedEvents[1].Name != "events-v1" {
		t.Fatalf("related events=%#v", detail.RelatedEvents)
	}
}

func TestPodDetailOmitsEventsWhenListCapabilityIsDenied(t *testing.T) {
	podUID := types.UID("pod-uid")
	client := kubefake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", UID: podUID}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "event", UID: "event-uid"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "api", UID: podUID}},
	)
	authorizer := &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"/events/list": authorization.DecisionDenied, "events.k8s.io/events/list": authorization.DecisionUnknown}}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer, now: time.Now}
	detail, err := backend.GetPod(context.Background(), namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}, "default", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.RelatedEvents) != 0 {
		t.Fatalf("denied events leaked: %#v", detail.RelatedEvents)
	}
}

func TestWorkloadDetailIncludesOnlyUIDOwnedAuthorizedRelations(t *testing.T) {
	deploymentUID := types.UID("deployment-uid")
	replicaSetUID := types.UID("replicaset-uid")
	controller := true
	client := kubefake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", UID: deploymentUID}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api-rs", UID: replicaSetUID, Labels: map[string]string{"app": "api"}, OwnerReferences: []metav1.OwnerReference{{UID: deploymentUID, Controller: &controller}}}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foreign-rs", UID: "foreign-rs", Labels: map[string]string{"app": "api"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api-pod", UID: "pod-uid", Labels: map[string]string{"app": "api"}, OwnerReferences: []metav1.OwnerReference{{UID: replicaSetUID, Controller: &controller}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foreign-pod", UID: "foreign-pod", Labels: map[string]string{"app": "api"}, OwnerReferences: []metav1.OwnerReference{{UID: "foreign-rs", Controller: &controller}}}},
	)
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}}, now: time.Now}
	detail, err := backend.GetWorkload(context.Background(), namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}, "deployments", "default", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Related) != 2 || detail.Related[0].Kind != "Pod" || detail.Related[0].Name != "api-pod" || detail.Related[1].Kind != "ReplicaSet" || detail.Related[1].Name != "api-rs" {
		t.Fatalf("related=%#v", detail.Related)
	}
	denied := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"apps/replicasets/list": authorization.DecisionDenied}}, now: time.Now}
	detail, err = denied.GetWorkload(context.Background(), namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}, "deployments", "default", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Related) != 0 {
		t.Fatalf("denied ownership relation leaked: %#v", detail.Related)
	}
}

func TestCronJobDetailUsesCompleteOwnedJobHistoryAndUnknownWhenDenied(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cronUID := types.UID("cron-uid")
	failedAt := metav1.NewTime(now.Add(-time.Hour))
	controller := true
	objects := []runtime.Object{
		&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup", UID: cronUID}, Status: batchv1.CronJobStatus{LastScheduleTime: &failedAt}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup-1", UID: "job-uid", OwnerReferences: []metav1.OwnerReference{{UID: cronUID, Controller: &controller}}}, Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: failedAt}}}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foreign", UID: "foreign-job"}, Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(now)}}}},
	}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
	allowed := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: kubefake.NewSimpleClientset(objects...)}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}}, now: func() time.Time { return now }}
	detail, err := allowed.GetWorkload(context.Background(), binding, resolution, "cronjobs", "default", "backup")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != resources.WorkloadFailed || len(detail.Related) != 1 || detail.Related[0].Name != "backup-1" {
		t.Fatalf("allowed detail=%#v", detail)
	}
	denied := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: kubefake.NewSimpleClientset(objects...)}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"batch/jobs/list": authorization.DecisionDenied}}, now: func() time.Time { return now }}
	detail, err = denied.GetWorkload(context.Background(), binding, resolution, "cronjobs", "default", "backup")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != resources.WorkloadUnknown || len(detail.Related) != 0 {
		t.Fatalf("denied detail invented state or leaked relations: %#v", detail)
	}
}

func TestCronJobListUsesRecentOwnedJobHistoryWithoutHealthyFallback(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cronUID := types.UID("cron-uid")
	failedAt := metav1.NewTime(now.Add(-time.Hour))
	controller := true
	objects := []runtime.Object{
		&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup", UID: cronUID}, Status: batchv1.CronJobStatus{LastScheduleTime: &failedAt}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup-1", UID: "job-uid", OwnerReferences: []metav1.OwnerReference{{UID: cronUID, Controller: &controller}}}, Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: failedAt}}}},
	}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
	options := resources.ListOptions{Limit: 25, Kinds: []resources.WorkloadKind{resources.WorkloadCronJobs}}
	allowed := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: kubefake.NewSimpleClientset(objects...)}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}}, now: func() time.Time { return now }}
	result, err := allowed.ListWorkloads(context.Background(), binding, resolution, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Status != resources.WorkloadFailed {
		t.Fatalf("allowed list=%#v", result.Items)
	}
	denied := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: kubefake.NewSimpleClientset(objects...)}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"batch/jobs/list": authorization.DecisionDenied}}, now: func() time.Time { return now }}
	result, err = denied.ListWorkloads(context.Background(), binding, resolution, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Status != resources.WorkloadUnknown {
		t.Fatalf("denied list invented state: %#v", result.Items)
	}
}

func TestCronJobWatchConversionConsultsCompleteRecentJobHistory(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cronUID := types.UID("cron-uid")
	failedAt := metav1.NewTime(now.Add(-time.Hour))
	controller := true
	cron := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup", UID: cronUID}, Status: batchv1.CronJobStatus{LastScheduleTime: &failedAt}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup-1", UID: "job-uid", OwnerReferences: []metav1.OwnerReference{{UID: cronUID, Controller: &controller}}}, Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: failedAt}}}}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: kubefake.NewSimpleClientset(job)}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}}, now: func() time.Time { return now }, watchBindings: map[string]namespaces.SelectionBinding{"gen": binding}}
	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cron)
	if err != nil {
		t.Fatal(err)
	}
	converted, err := (&resourceWatchPort{backend: backend}).convertRuntime(context.Background(), resources.WatchKey{Generation: "gen", Context: "ctx", Scope: "scope", Topic: resources.TopicWorkloads, GVR: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, Namespace: "default"}, &unstructured.Unstructured{Object: object})
	if err != nil {
		t.Fatal(err)
	}
	workload, ok := converted.(resources.WorkloadDTO)
	if !ok || workload.Status != resources.WorkloadFailed {
		t.Fatalf("converted=%#v", converted)
	}
}

func TestSecretListUsesOnlyMetadataClientAndNeverTypedSecret(t *testing.T) {
	scheme := metadatafake.NewTestScheme()
	metav1.AddMetaToScheme(scheme)
	metadataObject := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "credentials", UID: "uid-1"}}
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, metadataObject)
	typedClient := kubefake.NewSimpleClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "credentials"}, Data: map[string][]byte{"token": []byte("must-never-be-read")}})
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: typedClient, metadata: metadataClient}}, authorizer: &allowResourceAuthorization{}, now: time.Now}
	page, err := backend.listSecretPage(context.Background(), binding, resources.PageRequest{Origin: resources.Origin{Namespace: "default", Version: "v1", Resource: "secrets"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Metadata.Name != "credentials" {
		t.Fatalf("metadata items=%#v", page.Items)
	}
	detail, err := backend.GetSecret(context.Background(), binding, namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}, "default", "credentials")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Metadata.Name != "credentials" {
		t.Fatalf("detail=%#v", detail)
	}
	if actions := typedClient.Actions(); len(actions) != 0 {
		t.Fatalf("typed Secret client was accessed: %#v", actions)
	}
	actions := metadataClient.Actions()
	if len(actions) != 2 || actions[0].GetVerb() != "list" || actions[1].GetVerb() != "get" || actions[0].GetResource().Resource != "secrets" || actions[1].GetResource().Resource != "secrets" {
		t.Fatalf("metadata actions=%#v", actions)
	}
}

func stringPointer(value string) *string { return &value }

func TestListNodesIsClusterScopedWithoutNamespaceCounts(t *testing.T) {
	client := kubefake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", CreationTimestamp: metav1.Now()}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "1.32.0"}}},
	)
	authorizer := &allowResourceAuthorization{}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer, now: time.Now}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeSource: "none"}
	result, err := backend.ListNodes(context.Background(), binding, resolution, resources.ListOptions{Limit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "worker-1" || !result.Items[0].Ready {
		t.Fatalf("items=%#v", result.Items)
	}
	if result.Coverage.RequestedNamespaces != 0 || result.Coverage.CompletedNamespaces != 0 || len(result.Coverage.DeniedNamespaces) != 0 {
		t.Fatalf("cluster coverage must not report namespace counts: %#v", result.Coverage)
	}
	for _, key := range authorizer.keys {
		if key.Namespace != "" || key.Resource != "nodes" {
			t.Fatalf("non-cluster authorization key=%#v", key)
		}
	}
}

func TestListNodesDeniedIsAuthoritativeNotEmpty(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	authorizer := &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"/nodes/list": authorization.DecisionDenied}}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer, now: time.Now}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	_, err := backend.ListNodes(context.Background(), binding, namespaces.ScopeResolution{}, resources.ListOptions{Limit: 10}, nil)
	var domain *resources.DomainError
	if !errors.As(err, &domain) || domain.Code != resources.CodeForbidden {
		t.Fatalf("denied list error=%v", err)
	}
	unknown := &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"/nodes/list": authorization.DecisionUnknown}}
	backend = &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: unknown, now: time.Now}
	_, err = backend.ListNodes(context.Background(), binding, namespaces.ScopeResolution{}, resources.ListOptions{Limit: 10}, nil)
	if !errors.As(err, &domain) || domain.Code != resources.CodeAuthorizationUnavailable {
		t.Fatalf("unknown list error=%v", err)
	}
}
