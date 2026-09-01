package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	gingerconfig "github.com/fvmoraes/ginger/pkg/config"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/logging"
	"github.com/fvmoraes/kubepeep/internal/observability"
)

func metricsRequest() *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Host = "127.0.0.1:2749"
	request.URL.Path = "/metrics"
	return request
}

func healthRequest() *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = "127.0.0.1:2749"
	return request
}

func newMetricsTestApplication(t *testing.T, registry *observability.Registry) http.Handler {
	t.Helper()
	logPath := filepath.Join(privateTestDirectory(t), "kubePeep.log")
	logger, sink, err := logging.New(logPath, &bytes.Buffer{}, logging.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	application, err := New(Options{
		Config: &gingerconfig.Config{
			App:  gingerconfig.AppConfig{Name: "kubePeep", Env: "test"},
			HTTP: gingerconfig.HTTPConfig{Host: "127.0.0.1", Port: 2749, ShutdownTimeout: 1},
			Log:  gingerconfig.LogConfig{Level: "info", Format: "json"},
		},
		Port:       2749,
		Build:      api.BuildInfo{Version: "0.1.0"},
		Snapshots:  staticSnapshots{snapshot: healthySnapshot()},
		SessionTTL: time.Hour,
		Frontend: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
		},
		Logger:  logger,
		Metrics: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	return application.Handler
}

func TestMetricsEndpointDisabledByDefault(t *testing.T) {
	handler := newMetricsTestApplication(t, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, metricsRequest())
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("/metrics must stay unregistered without a registry, got %d", recorder.Code)
	}
}

func TestMetricsEndpointExposesCountersWhenEnabled(t *testing.T) {
	registry := observability.NewRegistry()
	handler := newMetricsTestApplication(t, registry)
	handler.ServeHTTP(httptest.NewRecorder(), healthRequest())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, metricsRequest())
	if recorder.Code != http.StatusOK {
		t.Fatalf("enabled /metrics returned %d", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("unexpected content type %q", contentType)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "kubepeep_requests_total") {
		t.Fatalf("metrics body missing request counter: %s", body)
	}
	if !strings.Contains(body, `route="GET /health"`) {
		t.Fatalf("metrics body missing health route series: %s", body)
	}
}
