package observability

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/config"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestTracingExportsRealSafeOTLPSpans(t *testing.T) {
	var mu sync.Mutex
	var received collectortrace.ExportTraceServiceRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" || r.Header.Get("Content-Type") != "application/x-protobuf" {
			http.Error(w, "invalid OTLP request", 400)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read failed", 400)
			return
		}
		mu.Lock()
		err = proto.Unmarshal(body, &received)
		mu.Unlock()
		if err != nil {
			http.Error(w, "invalid protobuf", 400)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()
	endpoint := server.URL + "/v1/traces"
	var failures atomic.Int64
	tracing, err := NewTracing(context.Background(), config.OTelConfig{Enabled: true, Endpoint: &endpoint, Protocol: config.OTelHTTPProtobuf, Insecure: true}, NewRegistry(), func() { failures.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	ctx, end := StartSpan(WithTracing(context.Background(), tracing), "resources.list")
	_, endChild := StartSpan(ctx, "cursor.get")
	endChild(errors.New("sensitive-pod secret-token"))
	end(nil)
	_, unknown := StartSpan(ctx, "private-name-must-not-be-exported")
	unknown(nil)
	if err := tracing.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if failures.Load() != 0 {
		t.Fatal("export failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received.ResourceSpans) != 1 || len(received.ResourceSpans[0].ScopeSpans) != 1 {
		t.Fatalf("missing OTLP spans: %s", &received)
	}
	spans := received.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 2 {
		t.Fatalf("want 2 actual spans, got %d", len(spans))
	}
	if spans[0].Name != "cursor.get" || spans[1].Name != "resources.list" || string(spans[0].ParentSpanId) != string(spans[1].SpanId) {
		t.Fatal("span names or parent relationship changed")
	}
	for _, forbidden := range []string{"sensitive-pod", "secret-token", "private-name", "host.name", "process.command"} {
		if strings.Contains(received.String(), forbidden) {
			t.Fatalf("sensitive trace data: %s", forbidden)
		}
	}
}

func TestTracingDisabledAndExporterFailure(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	options := config.Default().Observability.OTel
	disabled, err := NewTracing(context.Background(), options, nil, nil)
	if err != nil || disabled != nil || calls.Load() != 0 {
		t.Fatal("disabled tracing allocated an exporter")
	}
	endpoint := server.URL + "/v1/traces"
	options.Enabled, options.Endpoint, options.Insecure = true, &endpoint, true
	registry := NewRegistry()
	var failures atomic.Int64
	tracing, err := NewTracing(context.Background(), options, registry, func() { failures.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	_, end := StartSpan(WithTracing(context.Background(), tracing), "resources.list")
	end(nil)
	if err := tracing.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || failures.Load() != 1 || !strings.Contains(registry.Render(), "kubepeep_trace_export_errors_total 1") {
		t.Fatal("export failure was hidden or retried")
	}
}
