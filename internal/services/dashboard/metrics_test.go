package dashboard

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func TestBuildMetricsNormalizesQuantitiesAndRanksDeterministically(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	values := []metricsv1beta1.PodMetrics{
		podMetric("ns", "b", time.Minute, containerMetric("sidecar", "500m", "1Gi"), containerMetric("app", "1", "512Mi")),
		podMetric("ns", "a", time.Minute, containerMetric("app", "1500m", "1536Mi")),
		podMetric("other", "c", time.Minute, containerMetric("app", "250u", "1024Ki")),
	}
	result, err := BuildMetrics(values, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.WindowSeconds != 60 || len(result.Pods) != 3 {
		t.Fatalf("unexpected metrics: %+v", result)
	}
	if result.Pods[0].Pod != "a" || result.Pods[0].CPUMillicores != 1500 || result.Pods[0].MemoryBytes != 1536*1024*1024 {
		t.Fatalf("quantity conversion mismatch: %+v", result.Pods[0])
	}
	if result.Pods[1].Containers[0].Name != "app" || result.Pods[1].CPUMillicores != 1500 {
		t.Fatalf("container sorting/sum mismatch: %+v", result.Pods[1])
	}
	if result.TopCPU[0].Pod != "a" || result.TopCPU[1].Pod != "b" {
		t.Fatalf("CPU tie was not deterministic: %+v", result.TopCPU)
	}
	if result.TopMemory[0].Pod != "a" || result.TopMemory[1].Pod != "b" {
		t.Fatalf("memory tie was not deterministic: %+v", result.TopMemory)
	}
}

func TestBuildMetricsRejectsNegativeMismatchedAndMissingWindow(t *testing.T) {
	t.Parallel()
	now := time.Now()
	negative := podMetric("ns", "pod", time.Minute, containerMetric("app", "-1m", "1Mi"))
	if _, err := BuildMetrics([]metricsv1beta1.PodMetrics{negative}, now); err == nil {
		t.Fatal("negative quantity accepted")
	}
	if _, err := BuildMetrics([]metricsv1beta1.PodMetrics{podMetric("ns", "a", time.Minute, containerMetric("a", "1m", "1Mi")), podMetric("ns", "b", 2*time.Minute, containerMetric("b", "1m", "1Mi"))}, now); err == nil {
		t.Fatal("mismatched windows accepted")
	}
	if _, err := BuildMetrics(nil, now); err == nil {
		t.Fatal("missing positive window accepted")
	}
}

func TestMetricsServiceOptionalUnavailableDeniedAndAvailable(t *testing.T) {
	t.Parallel()
	clock := fixedClock{time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	selection := Selection{Namespaces: []string{"allowed", "denied"}}

	unavailable := NewMetricsService(&fakeMetricsPort{available: false}, allowMetrics{}, clock, QueryBudget{})
	block := unavailable.Collect(context.Background(), selection)
	if block.Complete || len(block.Errors) != 1 || block.Errors[0].Code != CodeFeatureUnavailable {
		t.Fatalf("unexpected unavailable state: %+v", block)
	}

	port := &fakeMetricsPort{available: true, pages: map[string]MetricsPage{
		"allowed": {Items: []metricsv1beta1.PodMetrics{podMetric("allowed", "pod", time.Minute, containerMetric("app", "10m", "2Mi"))}},
	}}
	service := NewMetricsService(port, selectiveMetricsAuth{}, clock, QueryBudget{})
	block = service.Collect(context.Background(), selection)
	if block.Complete || block.Coverage.CompletedNamespaces != 1 || len(block.Coverage.DeniedNamespaces) != 1 || len(block.Value.Pods) != 1 {
		t.Fatalf("unexpected partial metrics state: %+v", block)
	}
	if port.calls["denied"] != 0 {
		t.Fatal("metrics were queried before/after denied authorization")
	}
}

func TestMetricsServiceRepresentsAuthoritativeEmptyCollection(t *testing.T) {
	t.Parallel()
	port := &fakeMetricsPort{available: true, pages: map[string]MetricsPage{
		"empty": {Window: time.Minute},
	}}
	block := NewMetricsService(port, allowMetrics{}, fixedClock{time.Now()}, QueryBudget{}).Collect(context.Background(), Selection{Namespaces: []string{"empty"}})
	if !block.Complete || block.Value.WindowSeconds != 60 || block.Value.Pods == nil || len(block.Value.Pods) != 0 {
		t.Fatalf("authoritative zero was not represented: %+v", block)
	}
}

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

func containerMetric(name, cpu, memory string) metricsv1beta1.ContainerMetrics {
	return metricsv1beta1.ContainerMetrics{Name: name, Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse(memory)}}
}

func podMetric(namespace, name string, window time.Duration, containers ...metricsv1beta1.ContainerMetrics) metricsv1beta1.PodMetrics {
	return metricsv1beta1.PodMetrics{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}, Window: metav1.Duration{Duration: window}, Containers: containers}
}

type fakeMetricsPort struct {
	available bool
	pages     map[string]MetricsPage
	calls     map[string]int
}

func (port *fakeMetricsPort) Available(context.Context) (bool, error) { return port.available, nil }

func (port *fakeMetricsPort) ListPodMetrics(_ context.Context, namespace string, _ PageRequest) (MetricsPage, error) {
	if port.calls == nil {
		port.calls = make(map[string]int)
	}
	port.calls[namespace]++
	return port.pages[namespace], nil
}

type allowMetrics struct{}

func (allowMetrics) CanListPodMetrics(context.Context, string) (PermissionDecision, error) {
	return PermissionAllowed, nil
}

type selectiveMetricsAuth struct{}

func (selectiveMetricsAuth) CanListPodMetrics(_ context.Context, namespace string) (PermissionDecision, error) {
	if namespace == "denied" {
		return PermissionDenied, nil
	}
	return PermissionAllowed, nil
}
