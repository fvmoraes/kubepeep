package kubernetes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/observability"
	"k8s.io/client-go/rest"
)

func TestClientMetricsCounts429WithoutSensitiveLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(429) }))
	defer server.Close()
	registry := observability.NewRegistry()
	config := &rest.Config{Host: server.URL, QPS: 10, Burst: 20}
	observeTransport(config, registry, "unary")
	client, err := rest.HTTPClientFor(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL + "/sensitive-pod?token=private")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	rendered := registry.Render()
	if !strings.Contains(rendered, `kubepeep_kubernetes_requests_total{status="429",traffic="unary"} 1`) {
		t.Fatalf("missing 429: %s", rendered)
	}
	if strings.Contains(rendered, "sensitive") || strings.Contains(rendered, "private") || strings.Contains(rendered, server.URL) {
		t.Fatal("sensitive transport attributes were recorded")
	}
}

func TestMeasuredRateLimiterPreservesFamilyBudgetsAndCancellation(t *testing.T) {
	registry := observability.NewRegistry()
	config := &rest.Config{QPS: 1, Burst: 1}
	first := observeRateLimit(config, registry, "unary")
	second := observeRateLimit(config, registry, "unary")
	if config.RateLimiter != nil {
		t.Fatal("source config was modified")
	}
	if !first.RateLimiter.TryAccept() || !second.RateLimiter.TryAccept() {
		t.Fatal("independent family budgets were merged")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := first.RateLimiter.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was lost: %v", err)
	}
	if first.RateLimiter.QPS() != 1 {
		t.Fatal("QPS changed")
	}
	if !strings.Contains(registry.Render(), `kubepeep_client_throttle_total{traffic="unary"} 1`) {
		t.Fatalf("wait not measured: %s", registry.Render())
	}
	if observeRateLimit(config, nil, "unary") != config {
		t.Fatal("disabled metrics changed config")
	}
}
