package observability

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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

func TestGaugesTrackConcurrentActivityAndLastValue(t *testing.T) {
	registry := NewRegistry()
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() { registry.AddGauge(WatchActiveName, nil, 1) })
	}
	workers.Wait()
	registry.AddGauge(WatchActiveName, nil, -1)
	registry.SetGauge(CursorBytesName, nil, 12)
	registry.SetGauge(CursorBytesName, nil, 7)
	for _, want := range []string{"# TYPE kubepeep_watch_active gauge", "kubepeep_watch_active 19", "kubepeep_cursor_bytes 7"} {
		if !strings.Contains(registry.Render(), want) {
			t.Fatalf("missing %q: %s", want, registry.Render())
		}
	}
}

func TestRegistryBoundsSeriesCardinality(t *testing.T) {
	registry := NewRegistry()
	for index := range 2048 {
		labels := map[string]string{"resource": fmt.Sprint(index)}
		registry.IncCounter(ResourceListsTotalName, labels)
		registry.SetGauge(WatchActiveName, labels, 1)
	}
	if got := strings.Count(registry.Render(), "{resource="); got != 2048 {
		t.Fatalf("want 1024 series per metric, got %d", got)
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

func TestRequestsMiddlewarePreservesStreaming(t *testing.T) {
	registry := NewRegistry()
	server := httptest.NewServer(RequestsMiddleware(registry)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, canFlush := w.(http.Flusher)
		_, canHijack := w.(http.Hijacker)
		if !canFlush || !canHijack {
			http.Error(w, "streaming interfaces missing", 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: ready\n\n")
	})))
	defer server.Close()
	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || string(body) != "data: ready\n\n" {
		t.Fatalf("stream response %d: %s", response.StatusCode, body)
	}
}

func TestRequestsMiddlewareCountsFirstFinalStatus(t *testing.T) {
	registry := NewRegistry()
	handler := RequestsMiddleware(registry)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
		w.WriteHeader(http.StatusInternalServerError)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(registry.Render(), `status="200"} 1`) {
		t.Fatalf("wrong wire status: %s", registry.Render())
	}
}
