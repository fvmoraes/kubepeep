package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	gingerconfig "github.com/fvmoraes/ginger/pkg/config"
	"github.com/fvmoraes/ginger/pkg/testhelper"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/logging"
	"github.com/fvmoraes/kubepeep/internal/securefs"
)

type staticSnapshots struct {
	snapshot api.Snapshot
	err      error
}

type fixedGeneration string

func (g fixedGeneration) Current() string { return string(g) }

func (s staticSnapshots) Snapshot(context.Context) (api.Snapshot, error) {
	return s.snapshot, s.err
}

func TestGingerRequestMiddlewareWritesLocalJSONL(t *testing.T) {
	logPath := filepath.Join(privateTestDirectory(t), "kubePeep.log")
	logger, sink, err := logging.New(logPath, &bytes.Buffer{}, logging.Options{})
	if err != nil {
		t.Fatal(err)
	}

	application, err := New(Options{
		Config: &gingerconfig.Config{
			App:  gingerconfig.AppConfig{Name: "kubePeep", Env: "test"},
			HTTP: gingerconfig.HTTPConfig{Host: "127.0.0.1", Port: 2748, ShutdownTimeout: 1},
			Log:  gingerconfig.LogConfig{Level: "info", Format: "json"},
		},
		Port:       2748,
		Build:      api.BuildInfo{Version: "0.1.0"},
		Snapshots:  staticSnapshots{snapshot: healthySnapshot()},
		SessionTTL: time.Hour,
		Frontend: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
		},
		Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}

	response := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/api/v1/status").
		WithHeader("X-Request-ID", "req_logging").Do()
	testhelper.AssertStatus(t, response, http.StatusOK)
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSONL entry: %v", err)
		}
		if entry["operation"] != "request_finished" {
			continue
		}
		found = true
		if entry["component"] != "http" || entry["request_id"] != "req_logging" || entry["duration"] == "" {
			t.Fatalf("incomplete HTTP observability fields: %#v", entry)
		}
		if _, leaked := entry["http.user_agent"]; leaked {
			t.Fatalf("non-allowlisted field leaked: %#v", entry)
		}
	}
	if !found {
		t.Fatalf("Ginger request middleware did not reach local sink: %s", content)
	}
}

func TestNewRequiresSanitizedLogger(t *testing.T) {
	options := testApplicationOptions(t, staticSnapshots{snapshot: healthySnapshot()})
	options.Logger = nil
	if _, err := New(options); err == nil || !strings.Contains(err.Error(), "logger") {
		t.Fatalf("New() error = %v, want required logger error", err)
	}
}

func TestReservedAPIFallbackUsesPublicErrorContract(t *testing.T) {
	application := newTestApplication(t, staticSnapshots{snapshot: healthySnapshot()})
	tests := []struct {
		name   string
		method string
		path   string
		status int
		code   string
		allow  string
	}{
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/missing", status: http.StatusNotFound, code: api.CodeNotFound},
		{name: "known route wrong method", method: http.MethodPost, path: "/api/v1/status", status: http.StatusMethodNotAllowed, code: api.CodeMethodNotAllowed, allow: "GET, HEAD"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := testhelper.NewRequest(t, application.Handler, test.method, "http://127.0.0.1:2748"+test.path).Do()
			testhelper.AssertStatus(t, response, test.status)
			testhelper.AssertHeader(t, response, "Cache-Control", "no-store")
			testhelper.AssertHeader(t, response, "Content-Type", "application/json")
			if got := response.Header().Get("Allow"); got != test.allow {
				t.Fatalf("Allow = %q, want %q", got, test.allow)
			}
			var body struct {
				Code      string `json:"code"`
				RequestID string `json:"requestId"`
			}
			testhelper.DecodeJSON(t, response, &body)
			if body.Code != test.code || body.RequestID == "" || body.RequestID != response.Header().Get("X-Request-ID") {
				t.Fatalf("unexpected API fallback envelope: %#v headers=%#v", body, response.Header())
			}
		})
	}
}

func TestStatusUsesOnlyTheCurrentGeneration(t *testing.T) {
	selectedSnapshot := func(generation string) api.Snapshot {
		snapshot := healthySnapshot()
		snapshot.Selection = &api.SelectionSummary{
			ClusterProfileID: 1,
			Context:          "development",
			Cluster:          "development-cluster",
			ScopeSource:      "none",
			NamespaceCount:   0,
			Generation:       generation,
		}
		return snapshot
	}

	t.Run("fills empty generation", func(t *testing.T) {
		options := testApplicationOptions(t, staticSnapshots{snapshot: selectedSnapshot("")})
		options.Generation = fixedGeneration("gen_current")
		application, err := New(options)
		if err != nil {
			t.Fatal(err)
		}
		response := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/api/v1/status").Do()
		testhelper.AssertStatus(t, response, http.StatusOK)
		var body struct {
			Data struct {
				Selection *api.SelectionSummary `json:"selection"`
			} `json:"data"`
		}
		testhelper.DecodeJSON(t, response, &body)
		if body.Data.Selection == nil || body.Data.Selection.Generation != "gen_current" {
			t.Fatalf("status selection = %#v", body.Data.Selection)
		}
	})

	t.Run("rejects stale generation", func(t *testing.T) {
		options := testApplicationOptions(t, staticSnapshots{snapshot: selectedSnapshot("gen_stale")})
		options.Generation = fixedGeneration("gen_current")
		application, err := New(options)
		if err != nil {
			t.Fatal(err)
		}
		response := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/api/v1/status").Do()
		testhelper.AssertStatus(t, response, http.StatusInternalServerError)
		testhelper.AssertHeader(t, response, "Cache-Control", "no-store")
		var body struct {
			Code string `json:"code"`
		}
		testhelper.DecodeJSON(t, response, &body)
		if body.Code != api.CodeInternal {
			t.Fatalf("status error code = %q", body.Code)
		}
	})
}

func TestHealthAndStatusApplyDegradedPolicyAndFixedDTOs(t *testing.T) {
	snapshot := healthySnapshot()
	snapshot.Components.Cluster = component(api.StatusDegraded, "CLUSTER_UNAVAILABLE", "The cluster is temporarily unavailable.")
	application := newTestApplication(t, staticSnapshots{snapshot: snapshot})

	health := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/health").Do()
	testhelper.AssertStatus(t, health, http.StatusOK)
	testhelper.AssertHeader(t, health, "Cache-Control", "no-store")
	var healthBody struct {
		Data struct {
			Status     string                    `json:"status"`
			Components map[string]map[string]any `json:"components"`
		} `json:"data"`
	}
	testhelper.DecodeJSON(t, health, &healthBody)
	if healthBody.Data.Status != "degraded" || len(healthBody.Data.Components) != 5 {
		t.Fatalf("unexpected health response: %#v", healthBody)
	}
	if _, exists := healthBody.Data.Components["metrics"]; exists {
		t.Fatal("health unexpectedly includes metrics")
	}

	status := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/api/v1/status").
		WithHeader("Origin", "http://127.0.0.1:2748").Do()
	testhelper.AssertStatus(t, status, http.StatusOK)
	testhelper.AssertHeader(t, status, "Cache-Control", "no-store")
	var statusBody struct {
		Data struct {
			Version    string                    `json:"version"`
			Commit     string                    `json:"commit"`
			BuildDate  string                    `json:"buildDate"`
			Port       int                       `json:"port"`
			Components map[string]map[string]any `json:"components"`
			Selection  any                       `json:"selection"`
		} `json:"data"`
	}
	testhelper.DecodeJSON(t, status, &statusBody)
	if statusBody.Data.Port != 2748 || len(statusBody.Data.Components) != 6 || statusBody.Data.Selection != nil {
		t.Fatalf("unexpected status response: %#v", statusBody.Data)
	}
	if statusBody.Data.Commit != "unknown" || statusBody.Data.BuildDate != "unknown" {
		t.Fatalf("empty build metadata was not normalized: %#v", statusBody.Data)
	}
}

func TestHealthReturns503ForSQLiteAnd500ForSnapshotFailure(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		snapshot := healthySnapshot()
		snapshot.Components.SQLite = component(api.StatusUnhealthy, "SQLITE_UNAVAILABLE", "SQLite is unavailable.")
		application := newTestApplication(t, staticSnapshots{snapshot: snapshot})
		response := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/health").Do()
		testhelper.AssertStatus(t, response, http.StatusServiceUnavailable)
	})

	t.Run("provider", func(t *testing.T) {
		application := newTestApplication(t, staticSnapshots{err: errors.New("sensitive storage failure")})
		response := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/health").Do()
		testhelper.AssertStatus(t, response, http.StatusInternalServerError)
		if response.Body.String() == "" || contains(response.Body.String(), "sensitive") {
			t.Fatalf("unexpected error body: %q", response.Body.String())
		}
	})
}

func TestSessionIsNoStoreAndHostOriginAreExact(t *testing.T) {
	application := newTestApplication(t, staticSnapshots{snapshot: healthySnapshot()})
	session := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/api/v1/session").
		WithHeader("Origin", "http://127.0.0.1:2748").Do()
	testhelper.AssertStatus(t, session, http.StatusOK)
	testhelper.AssertHeader(t, session, "Cache-Control", "no-store")
	var body struct {
		Data api.SessionData `json:"data"`
	}
	testhelper.DecodeJSON(t, session, &body)
	if body.Data.CSRFToken == "" || body.Data.Generation == "" || body.Data.Origin != "http://127.0.0.1:2748" {
		t.Fatalf("invalid session: %#v", body.Data)
	}

	wrongHost := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://localhost:2748/api/v1/session").Do()
	testhelper.AssertStatus(t, wrongHost, http.StatusForbidden)
	foreignOrigin := testhelper.NewRequest(t, application.Handler, http.MethodGet, "http://127.0.0.1:2748/api/v1/session").
		WithHeader("Origin", "http://evil.invalid").Do()
	testhelper.AssertStatus(t, foreignOrigin, http.StatusForbidden)
	if wrongHost.Header().Get("X-Request-ID") == "" || foreignOrigin.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("security response headers are incomplete or CORS was enabled")
	}
}

func newTestApplication(t *testing.T, snapshots api.SnapshotProvider) *Application {
	t.Helper()
	application, err := New(testApplicationOptions(t, snapshots))
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func testApplicationOptions(t *testing.T, snapshots api.SnapshotProvider) Options {
	t.Helper()
	logger, sink, err := logging.New(filepath.Join(privateTestDirectory(t), "kubePeep.log"), io.Discard, logging.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	return Options{
		Config: &gingerconfig.Config{
			App:  gingerconfig.AppConfig{Name: "kubePeep", Env: "test"},
			HTTP: gingerconfig.HTTPConfig{Host: "127.0.0.1", Port: 2748, ShutdownTimeout: 1},
			Log:  gingerconfig.LogConfig{Level: "error", Format: "json"},
		},
		Port:       2748,
		Build:      api.BuildInfo{Version: "0.1.0"},
		Snapshots:  snapshots,
		SessionTTL: time.Hour,
		Frontend: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
		},
		Logger: logger,
	}
}

func privateTestDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := securefs.EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	return directory
}

func healthySnapshot() api.Snapshot {
	return api.Snapshot{Components: api.StatusComponents{
		Application: component(api.StatusHealthy, "OK", "Application is ready."),
		SQLite:      component(api.StatusHealthy, "OK", "SQLite is available."),
		Kubeconfig:  component(api.StatusUnknown, "NOT_CHECKED", "Kubeconfig has not been checked."),
		Context:     component(api.StatusUnknown, "NOT_SELECTED", "No context is selected."),
		Cluster:     component(api.StatusUnknown, "NOT_CHECKED", "The cluster has not been checked."),
		Metrics:     component(api.StatusUnknown, "NOT_CHECKED", "Metrics API has not been checked."),
	}}
}

func component(status api.ComponentStatus, code, message string) api.ComponentState {
	checkedAt := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	if status == api.StatusUnknown {
		return api.ComponentState{Status: status, Code: code, Message: message}
	}
	return api.ComponentState{Status: status, Code: code, Message: message, CheckedAt: &checkedAt}
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
