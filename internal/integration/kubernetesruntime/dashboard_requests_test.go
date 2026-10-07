package kubernetesruntime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"

	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

// Done is observed by the coalescer after registering the waiter, providing a
// deterministic barrier without sleeps or depending on goroutine scheduling.
type dashboardWaiterContext struct {
	context.Context
	joined chan struct{}
	once   sync.Once
}

func (ctx *dashboardWaiterContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.joined) })
	return ctx.Context.Done()
}

func TestDashboardBlocksShareReadsWithoutSharingCancellationOrMutableData(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	client := kubefake.NewClientset()
	var calls atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	client.PrependReactor("list", "pods", func(kubetesting.Action) (bool, runtime.Object, error) {
		calls.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
			return true, nil, ctx.Err()
		}
		return true, &corev1.PodList{Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "api", Labels: map[string]string{"app": "api"}}}}}, nil
	})
	auth := &dashboardAuthorizationStub{}
	backend := newDashboardBackend(&fixedDashboardClients{set: dashboardClientSet{kubernetes: client}}, auth, dashboard.QueryBudget{})
	t.Cleanup(backend.requests.Close)
	type response struct {
		page dashboard.PodPage
		err  error
	}
	results := make([]chan response, 3)
	cancelFirst := func() {}
	for index := range results {
		parent := ctx
		if index == 0 {
			parent, cancelFirst = context.WithCancel(ctx)
		}
		waiter := &dashboardWaiterContext{Context: parent, joined: make(chan struct{})}
		_, adapter := backend.service(dashboardTestBinding())
		results[index] = make(chan response, 1)
		go func(index int) {
			page, err := adapter.ListPods(waiter, "payments", dashboard.PageRequest{Limit: 100})
			results[index] <- response{page, err}
		}(index)
		select {
		case <-waiter.joined:
		case <-ctx.Done():
			t.Fatal("waiter did not register")
		}
	}
	cancelFirst()
	if result := <-results[0]; !errors.Is(result.err, context.Canceled) {
		t.Fatalf("first waiter error = %v", result.err)
	}
	unblock()
	second, third := <-results[1], <-results[2]
	if second.err != nil || third.err != nil || calls.Load() != 1 {
		t.Fatalf("shared read: calls=%d errors=%v/%v", calls.Load(), second.err, third.err)
	}
	second.page.Items[0].Labels["app"] = "changed"
	if third.page.Items[0].Labels["app"] != "api" {
		t.Fatal("one block mutated another block's Kubernetes data")
	}
	_, adapter := backend.service(dashboardTestBinding())
	if _, err := adapter.ListPods(ctx, "payments", dashboard.PageRequest{Limit: 100}); err != nil || calls.Load() != 2 {
		t.Fatalf("completed data was cached: calls=%d err=%v", calls.Load(), err)
	}
	auth.denyResource = "pods"
	if _, err := adapter.ListPods(ctx, "payments", dashboard.PageRequest{Limit: 100}); err == nil || calls.Load() != 2 {
		t.Fatalf("revoked authorization did not stop a later LIST: calls=%d err=%v", calls.Load(), err)
	}
}

func TestDashboardPageIdentitySeparatesSelectionsAndPagination(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	coalescer := resourcecore.NewRequestCoalescer()
	defer coalescer.Close()
	binding := dashboardTestBinding()
	base := dashboardAdapter{binding: binding, requests: coalescer}
	profile, selectedContext, generation := base, base, base
	profile.binding.ClusterProfileID++
	selectedContext.binding.Context = "other"
	generation.binding.Generation = "new"
	cases := []struct {
		adapter         dashboardAdapter
		kind, namespace string
		page            dashboard.PageRequest
	}{
		{base, "pods", "payments", dashboard.PageRequest{Limit: 100}},
		{base, "events", "payments", dashboard.PageRequest{Limit: 100}},
		{base, "pods", "other", dashboard.PageRequest{Limit: 100}},
		{base, "pods", "payments", dashboard.PageRequest{Limit: 50}},
		{base, "pods", "payments", dashboard.PageRequest{Limit: 100, Continue: "next"}},
		{profile, "pods", "payments", dashboard.PageRequest{Limit: 100}},
		{selectedContext, "pods", "payments", dashboard.PageRequest{Limit: 100}},
		{generation, "pods", "payments", dashboard.PageRequest{Limit: 100}},
	}
	started := make(chan struct{}, len(cases))
	done := make(chan error, len(cases))
	for _, test := range cases {
		go func() {
			_, err := coalesceDashboardPage(ctx, &test.adapter, test.kind, test.namespace, test.page, func(shared context.Context, _ string, _ dashboard.PageRequest) (dashboard.PodPage, error) {
				started <- struct{}{}
				<-shared.Done()
				return dashboard.PodPage{}, shared.Err()
			})
			done <- err
		}()
	}
	for range cases {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("different page identities were incorrectly coalesced")
		}
	}
	cancel()
	for range cases {
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled collection = %v", err)
		}
	}
}

type blockedDashboardClients struct{}

func (blockedDashboardClients) Unary(ctx context.Context, _ namespaces.SelectionBinding) (context.Context, context.CancelFunc, dashboardClientSet, error) {
	<-ctx.Done()
	return nil, nil, dashboardClientSet{}, ctx.Err()
}

func TestDashboardProblemsBoundsAllCollectors(t *testing.T) {
	t.Parallel()
	backend := newDashboardBackend(blockedDashboardClients{}, &dashboardAuthorizationStub{}, dashboard.QueryBudget{Timeout: 20 * time.Millisecond})
	defer backend.requests.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	started := time.Now()
	block := backend.Problems(ctx, dashboardTestBinding(), namespaces.ScopeResolution{Namespaces: []string{"payments"}})
	if elapsed := time.Since(started); elapsed > time.Second || block.Complete || len(block.Errors) == 0 {
		t.Fatalf("collectors escaped the block budget: elapsed=%s complete=%v errors=%v", elapsed, block.Complete, block.Errors)
	}
}
