package observability

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIncCounterIgnoresUnknownNamesAndLabels(t *testing.T) {
	registry := NewRegistry()
	registry.IncCounter("unknown_metric", map[string]string{"method": "GET"})
	registry.IncCounter(RequestsTotalName, map[string]string{"bogus": "x", "method": "GET", "status": "200"})
	rendered := registry.Render()
	if strings.Contains(rendered, "unknown_metric") || strings.Contains(rendered, "bogus") {
		t.Fatalf("registry recorded unallowed series: %s", rendered)
	}
	if !strings.Contains(rendered, `kubepeep_requests_total{method="GET",status="200"} 1`) {
		t.Fatalf("missing expected series: %s", rendered)
	}
}

func TestIncCounterAggregatesSameLabelSet(t *testing.T) {
	registry := NewRegistry()
	for range 3 {
		registry.IncCounter(RequestsTotalName, map[string]string{"method": "GET", "route": "GET /health", "status": "200"})
	}
	registry.IncCounter(RequestsTotalName, map[string]string{"method": "GET", "route": "GET /health", "status": "500"})
	rendered := registry.Render()
	if !strings.Contains(rendered, `kubepeep_requests_total{method="GET",route="GET /health",status="200"} 3`) {
		t.Fatalf("counter did not aggregate: %s", rendered)
	}
	if !strings.Contains(rendered, `status="500"} 1`) {
		t.Fatalf("separate label sets must not merge: %s", rendered)
	}
}

func TestRenderEscapesLabelValues(t *testing.T) {
	registry := NewRegistry()
	registry.IncCounter(RequestsTotalName, map[string]string{"method": "G\"E\nT", "route": "back\\slash", "status": "200"})
	rendered := registry.Render()
	if !strings.Contains(rendered, `method="G\"E\nT"`) || !strings.Contains(rendered, `route="back\\slash"`) {
		t.Fatalf("label values were not escaped: %s", rendered)
	}
}

func TestRenderIsDeterministicAndSorted(t *testing.T) {
	registry := NewRegistry()
	registry.IncCounter(RequestsTotalName, map[string]string{"method": "POST", "route": "r2", "status": "204"})
	registry.IncCounter(RequestsTotalName, map[string]string{"method": "GET", "route": "r1", "status": "200"})
	first := registry.Render()
	second := registry.Render()
	if first != second {
		t.Fatalf("render is not deterministic:\n%s\n---\n%s", first, second)
	}
	getIndex := strings.Index(first, `method="GET"`)
	postIndex := strings.Index(first, `method="POST"`)
	if getIndex == -1 || postIndex == -1 || getIndex > postIndex {
		t.Fatalf("series are not sorted: %s", first)
	}
}

func TestSetGaugeIsCurrentlyIgnored(t *testing.T) {
	registry := NewRegistry()
	registry.SetGauge("kubepeep_active_streams", nil, 3)
	if rendered := registry.Render(); rendered != "" {
		t.Fatalf("gauges should not render yet: %s", rendered)
	}
}

func TestRequestsMiddlewareCountsStatuses(t *testing.T) {
	registry := NewRegistry()
	mux := http.NewServeMux()
	mux.Handle("GET /health", http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	mux.Handle("GET /boom", http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Error(response, "nope", http.StatusTeapot)
	}))
	handler := RequestsMiddleware(registry)(mux)
	request := func(method, path string) *http.Request {
		return &http.Request{Method: method, URL: &url.URL{Path: path}, Header: http.Header{}}
	}
	handler.ServeHTTP(httptest.NewRecorder(), request(http.MethodGet, "/health"))
	handler.ServeHTTP(httptest.NewRecorder(), request(http.MethodGet, "/boom"))
	rendered := registry.Render()
	if !strings.Contains(rendered, `route="GET /health",status="200"} 1`) {
		t.Fatalf("ok request not counted with route pattern: %s", rendered)
	}
	if !strings.Contains(rendered, `route="GET /boom",status="418"} 1`) {
		t.Fatalf("error status not captured: %s", rendered)
	}
}
