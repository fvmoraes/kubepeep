package kubernetesruntime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func jobRun(namespace, name, cronUID string, created int64) *batchv1.Job {
	controller := true
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, UID: types.UID(namespace + "/" + name), CreationTimestamp: metav1.NewTime(time.Unix(created, 0)), OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "CronJob", Name: "schedule", UID: types.UID(cronUID), Controller: &controller}}}}
}

type jobRunClientProviderFunc func(context.Context, namespaces.SelectionBinding) (context.Context, context.CancelFunc, resourceClientSet, error)

func (provider jobRunClientProviderFunc) Unary(ctx context.Context, binding namespaces.SelectionBinding) (context.Context, context.CancelFunc, resourceClientSet, error) {
	return provider(ctx, binding)
}

func TestLatestJobRunPodsRetainsRetriesAndIndependentOwners(t *testing.T) {
	t.Parallel()
	old, latest := jobRun("a", "old", "cron", 1), jobRun("a", "latest", "cron", 2)
	old.Status.Succeeded, latest.Status.Failed = 1, 1
	client := kubefake.NewSimpleClientset(old, latest, jobRun("a", "other", "other-cron", 1), jobRun("b", "old", "cron", 1))
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &allowResourceAuthorization{}}
	t.Cleanup(backend.Close)
	pods := []resources.PodDTO{
		{Namespace: "a", Name: "old-pod", Owner: &resources.OwnerDTO{APIVersion: "batch/v1", UID: "a/old", Kind: "Job", Name: "old"}},
		{Namespace: "a", Name: "latest-failed", Owner: &resources.OwnerDTO{APIVersion: "batch/v1", UID: "a/latest", Kind: "Job", Name: "latest"}},
		{Namespace: "a", Name: "latest-retry", Owner: &resources.OwnerDTO{APIVersion: "batch/v1", UID: "a/latest", Kind: "Job", Name: "latest"}},
		{Namespace: "a", Name: "other", Owner: &resources.OwnerDTO{APIVersion: "batch/v1", UID: "a/other", Kind: "Job", Name: "other"}},
		{Namespace: "a", Name: "standalone-job", Owner: &resources.OwnerDTO{APIVersion: "batch/v1", UID: "a/manual", Kind: "Job", Name: "manual"}},
		{Namespace: "a", Name: "application"},
		{Namespace: "b", Name: "old", Owner: &resources.OwnerDTO{APIVersion: "batch/v1", UID: "b/old", Kind: "Job", Name: "old"}},
	}
	got, partial := backend.latestJobRunPods(t.Context(), namespaces.SelectionBinding{Generation: "gen"}, pods)
	if partial || !reflect.DeepEqual(got, pods[1:]) {
		t.Fatalf("pods=%+v partial=%v", got, partial)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "list" {
			t.Fatalf("unexpected mutation: %v", action)
		}
	}
}

func TestLatestJobRunPodsPreservesAmbiguousOwnerIdentity(t *testing.T) {
	t.Parallel()
	client := kubefake.NewSimpleClientset(jobRun("a", "old", "cron", 1), jobRun("a", "latest", "cron", 2))
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &allowResourceAuthorization{}}
	t.Cleanup(backend.Close)
	pods := []resources.PodDTO{
		{Namespace: "a", Name: "historical", Owner: &resources.OwnerDTO{Kind: "Job", Name: "old", APIVersion: "batch/v1", UID: "a/old"}},
		{Namespace: "a", Name: "custom-kind", Owner: &resources.OwnerDTO{Kind: "Job", Name: "old", APIVersion: "example.com/v1", UID: "a/old"}},
		{Namespace: "a", Name: "recreated-job", Owner: &resources.OwnerDTO{Kind: "Job", Name: "old", APIVersion: "batch/v1", UID: "recreated-uid"}},
		{Namespace: "a", Name: "unknown-uid", Owner: &resources.OwnerDTO{Kind: "Job", Name: "old", APIVersion: "batch/v1"}},
		{Namespace: "a", Name: "unknown-group", Owner: &resources.OwnerDTO{Kind: "Job", Name: "old", UID: "a/old"}},
	}
	visible, partial := backend.latestJobRunPods(t.Context(), namespaces.SelectionBinding{Generation: "gen"}, pods)
	if partial || !reflect.DeepEqual(visible, pods[1:]) {
		t.Fatalf("ambiguous owner was hidden: visible=%v partial=%v", visible, partial)
	}
}

func TestLatestJobRunPodsPreservesNativePaginationAndExplicitHistory(t *testing.T) {
	t.Parallel()
	controller := true
	oldPod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "old-pod", OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: "old", UID: "a/old", Controller: &controller}}}}
	newPod := *oldPod.DeepCopy()
	newPod.Name = "latest-pod"
	newPod.OwnerReferences[0].Name = "latest"
	newPod.OwnerReferences[0].UID = "a/latest"
	client := kubefake.NewSimpleClientset(jobRun("a", "old", "cron", 1), jobRun("a", "latest", "cron", 2))
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		options := action.(interface{ GetListOptions() metav1.ListOptions }).GetListOptions()
		if options.Continue == "next" {
			return true, &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "1"}, Items: []corev1.Pod{newPod}}, nil
		}
		return true, &corev1.PodList{ListMeta: metav1.ListMeta{Continue: "next", ResourceVersion: "1"}, Items: []corev1.Pod{oldPod}}, nil
	})
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &allowResourceAuthorization{}, now: time.Now}
	t.Cleanup(backend.Close)
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	scope := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"a"}}
	options := resources.ListOptions{Limit: 1, LatestJobRuns: true}
	first, err := backend.ListPods(t.Context(), binding, scope, options, nil)
	if err != nil || len(first.Items) != 0 || first.Page.Complete || first.Cursor == nil {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := backend.ListPods(t.Context(), binding, scope, options, first.Cursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].Name != "latest-pod" || !second.Page.Complete {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	for _, options := range []resources.ListOptions{{Limit: 1}, {Limit: 1, LatestJobRuns: true, Workload: "old"}, {Limit: 1, LatestJobRuns: true, FieldSelector: "metadata.name=old-pod"}} {
		all, err := backend.ListPods(t.Context(), binding, scope, options, nil)
		if err != nil || len(all.Items) != 1 || all.Items[0].Name != "old-pod" {
			t.Fatalf("history=%+v err=%v", all, err)
		}
	}
}

func TestJobRunCacheExpiresAndRechecksAuthorization(t *testing.T) {
	t.Parallel()
	client := kubefake.NewSimpleClientset(jobRun("a", "old", "cron", 1), jobRun("a", "latest", "cron", 2))
	now := time.Now()
	authorizer := &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer, now: func() time.Time { return now }}
	t.Cleanup(backend.Close)
	binding := namespaces.SelectionBinding{Generation: "gen"}
	for range 2 {
		older, complete := backend.olderJobRuns(t.Context(), binding, "a")
		if !complete || !older["a/old"] {
			t.Fatalf("older=%v complete=%v", older, complete)
		}
	}
	if len(client.Actions()) != 1 {
		t.Fatal("repeated pages did not reuse history")
	}
	authorizer.denied["batch/jobs/list"] = authorization.DecisionDenied
	if _, complete := backend.olderJobRuns(t.Context(), binding, "a"); complete {
		t.Fatal("revoked access reused cached history")
	}
	delete(authorizer.denied, "batch/jobs/list")
	_, err := client.BatchV1().Jobs("a").Create(t.Context(), jobRun("a", "newest", "cron", 3), metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Second)
	older, complete := backend.olderJobRuns(t.Context(), binding, "a")
	if !complete || !older["a/latest"] {
		t.Fatal("five-second refresh missed the new execution")
	}
	binding.Generation = "next"
	client.ClearActions()
	if _, complete := backend.olderJobRuns(t.Context(), binding, "a"); !complete || len(client.Actions()) != 1 {
		t.Fatal("another generation reused old history")
	}
}

func TestJobRunCacheRejectsLatePublicationAfterInvalidation(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"generation", "close"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			client := kubefake.NewSimpleClientset()
			backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &allowResourceAuthorization{}}
			t.Cleanup(backend.Close)
			backend.OnGeneration("old")
			operationFinished := make(chan struct{})
			backend.clients = jobRunClientProviderFunc(func(ctx context.Context, _ namespaces.SelectionBinding) (context.Context, context.CancelFunc, resourceClientSet, error) {
				requestContext, cancel := context.WithCancel(ctx)
				return requestContext, func() { cancel(); close(operationFinished) }, resourceClientSet{kubernetes: client}, nil
			})
			started, finish, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(finish) }) })
			client.PrependReactor("list", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
				close(started)
				<-finish // Model a successful read crossing the cancellation boundary.
				close(returned)
				return true, &batchv1.JobList{Items: []batchv1.Job{*jobRun("a", "old", "cron", 1), *jobRun("a", "new", "cron", 2)}}, nil
			})
			result := make(chan bool, 1)
			go func() {
				_, complete := backend.olderJobRuns(t.Context(), namespaces.SelectionBinding{Generation: "old"}, "a")
				result <- complete
			}()
			<-started
			if change == "generation" {
				backend.OnGeneration("new")
			} else {
				backend.Close()
			}
			release.Do(func() { close(finish) })
			if <-result {
				t.Fatal("invalidated Job history was published")
			}
			<-returned
			<-operationFinished
			backend.jobRunsMu.Lock()
			retained := len(backend.jobRuns)
			backend.jobRunsMu.Unlock()
			if retained != 0 {
				t.Fatalf("invalidated cache retained %d namespaces", retained)
			}
		})
	}
}

func TestJobRunCacheReusesManySmallNamespaces(t *testing.T) {
	t.Parallel()
	objects := make([]runtime.Object, 0, 24)
	for i := range 12 {
		namespace := fmt.Sprintf("namespace-%d", i)
		objects = append(objects, jobRun(namespace, "old", "cron", 1), jobRun(namespace, "new", "cron", 2))
	}
	client := kubefake.NewSimpleClientset(objects...)
	backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &allowResourceAuthorization{}}
	t.Cleanup(backend.Close)
	for range 2 {
		for i := range 12 {
			namespace := fmt.Sprintf("namespace-%d", i)
			if older, complete := backend.olderJobRuns(t.Context(), namespaces.SelectionBinding{Generation: "gen"}, namespace); !complete || !older[namespace+"/old"] {
				t.Fatalf("history missing for %s", namespace)
			}
		}
	}
	if got := len(client.Actions()); got != 12 {
		t.Fatalf("second page should reuse each namespace history; LISTs=%d", got)
	}
}

func TestJobRunCacheCoalescesReadersAndKeepsRemainingWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := kubefake.NewSimpleClientset()
		finish := make(chan struct{})
		client.PrependReactor("list", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
			<-finish
			return true, &batchv1.JobList{Items: []batchv1.Job{*jobRun("a", "old", "cron", 1), *jobRun("a", "new", "cron", 2)}}, nil
		})
		backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &allowResourceAuthorization{}}
		defer backend.Close()
		binding := namespaces.SelectionBinding{Generation: "gen"}
		firstCtx, firstCancel := context.WithCancel(t.Context())
		secondCtx, secondCancel := context.WithCancel(t.Context())
		defer firstCancel()
		defer secondCancel()
		first, second := make(chan bool, 1), make(chan bool, 1)
		go func() { _, complete := backend.olderJobRuns(firstCtx, binding, "a"); first <- complete }()
		synctest.Wait()
		go func() { _, complete := backend.olderJobRuns(secondCtx, binding, "a"); second <- complete }()
		synctest.Wait()
		firstCancel()
		synctest.Wait()
		if <-first {
			t.Fatal("canceled waiter received Job history")
		}
		close(finish)
		synctest.Wait()
		if !<-second {
			t.Fatal("canceling one waiter canceled another reader")
		}
		if len(client.Actions()) != 1 {
			t.Fatal("concurrent readers duplicated the Jobs LIST")
		}
	})
}

func TestLatestJobRunPodsUnavailableHistoryAndCancellation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"denied", "unavailable", "cancelled", "incomplete"} {
		t.Run(name, func(t *testing.T) {
			client := kubefake.NewSimpleClientset()
			authorizer := &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if name == "denied" {
				authorizer.denied["batch/jobs/list"] = authorization.DecisionDenied
			}
			if name == "cancelled" {
				cancel()
			}
			client.PrependReactor("list", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
				if name == "incomplete" {
					return true, &batchv1.JobList{Items: make([]batchv1.Job, maximumCronJobHistoryScan+1)}, nil
				}
				return true, nil, errors.New("unavailable")
			})
			backend := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer}
			t.Cleanup(backend.Close)
			pods := []resources.PodDTO{{Namespace: "a", Name: "pod", Owner: &resources.OwnerDTO{APIVersion: "batch/v1", UID: "a/old", Kind: "Job", Name: "old"}}}
			got, partial := backend.latestJobRunPods(ctx, namespaces.SelectionBinding{Generation: "gen"}, pods)
			if !partial || !reflect.DeepEqual(got, pods) {
				t.Fatalf("pods=%v partial=%v", got, partial)
			}
			client.ClearActions()
			pods[0].Owner = nil
			_, partial = backend.latestJobRunPods(ctx, namespaces.SelectionBinding{Generation: "gen"}, pods)
			if partial || len(client.Actions()) != 0 {
				t.Fatal("ordinary Pods should never read Job history")
			}
		})
	}
}

func TestOlderCronJobRunsUsesOwnershipAndCreationOnly(t *testing.T) {
	t.Parallel()
	old, latest := jobRun("a", "z-old", "cron", 1), jobRun("a", "a-new", "cron", 2)
	old.Status.Failed, latest.Status.Succeeded = 1, 1
	unknown := jobRun("a", "unknown", "cron", 0)
	unknown.CreationTimestamp = metav1.Time{}
	noUID := jobRun("a", "no-uid", "", 1)
	manual := jobRun("a", "manual", "cron", 1)
	manual.OwnerReferences = nil
	recreated := jobRun("a", "recreated-cron", "new-uid", 3)
	jobs := []batchv1.Job{*latest, *old, *unknown, *noUID, *manual, *recreated}
	if got := olderCronJobRuns(jobs); !reflect.DeepEqual(got, map[string]bool{"a/z-old": true}) {
		t.Fatalf("older=%v", got)
	}
}

// BenchmarkOlderCronJobRuns10000 measures only grouping an already-loaded
// Job history, excluding cluster latency, decoding, and fixture construction.
func BenchmarkOlderCronJobRuns10000(b *testing.B) {
	const jobsPerCron, cronJobs = 100, 100
	jobs := make([]batchv1.Job, 0, jobsPerCron*cronJobs)
	for run := range jobsPerCron {
		for cron := range cronJobs {
			jobs = append(jobs, *jobRun("workloads", fmt.Sprintf("schedule-%d-run-%d", cron, run), fmt.Sprintf("cron-uid-%d", cron), int64(run+1)))
		}
	}
	b.ReportAllocs()
	var older map[string]bool
	for b.Loop() {
		older = olderCronJobRuns(jobs)
	}
	if len(older) != cronJobs*(jobsPerCron-1) {
		b.Fatalf("historical jobs=%d, want %d", len(older), cronJobs*(jobsPerCron-1))
	}
}
