package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
)

func healthRequest(port int) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = fmt.Sprintf("127.0.0.1:%d", port)
	return request
}

func composeOptions(t *testing.T) Options {
	t.Helper()
	layout, err := userdirs.ForRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	return Options{
		Layout:    layout,
		Config:    productconfig.Default(),
		Port:      2757,
		LogOutput: &bytes.Buffer{},
	}
}

func TestComposeRejectsMissingLayoutRoot(t *testing.T) {
	options := composeOptions(t)
	options.Layout.Root = ""
	if _, err := Compose(context.Background(), options); err == nil {
		t.Fatal("Compose must reject an empty layout root")
	}
}

func TestComposeRejectsOutOfRangePorts(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		options := composeOptions(t)
		options.Port = port
		if _, err := Compose(context.Background(), options); err == nil {
			t.Fatalf("Compose must reject port %d", port)
		}
	}
}

func TestComposeBuildsServingPlatformAndCleanup(t *testing.T) {
	options := composeOptions(t)
	platform, err := Compose(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if platform.Handler == nil {
		t.Fatal("platform handler is nil")
	}
	if platform.Port != options.Port {
		t.Fatalf("platform port = %d, want %d", platform.Port, options.Port)
	}
	if platform.Origin == "" || platform.Sessions == nil || platform.Generation == nil {
		t.Fatal("platform origin, sessions, and generation are required")
	}
	if len(platform.Cleanups) == 0 {
		t.Fatal("platform must register cleanup hooks")
	}

	recorder := httptest.NewRecorder()
	platform.Handler.ServeHTTP(recorder, healthRequest(options.Port))
	if recorder.Code != http.StatusOK {
		t.Fatalf("/health status = %d", recorder.Code)
	}
	var payload struct {
		Data struct {
			Status     string `json:"status"`
			Components map[string]struct {
				Status string `json:"status"`
			} `json:"components"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("health payload: %v", err)
	}
	// The overall status legitimately degrades without an ambient kubeconfig
	// (cluster check), so assert on the local components Compose owns.
	if payload.Data.Components["application"].Status != "healthy" {
		t.Fatalf("application component = %q", payload.Data.Components["application"].Status)
	}
	if payload.Data.Components["sqlite"].Status != "healthy" {
		t.Fatalf("sqlite component = %q", payload.Data.Components["sqlite"].Status)
	}

	for _, cleanup := range platform.Cleanups {
		if err := cleanup.Func(context.Background()); err != nil {
			t.Fatalf("cleanup %s failed: %v", cleanup.Name, err)
		}
	}
}

func TestComposeWithoutKubeconfigStillServesLocalHealth(t *testing.T) {
	// A missing or invalid kubeconfig must not take the local application
	// down: the failure is published as a safe snapshot instead.
	options := composeOptions(t)
	options.Kubeconfig = filepath.Join(t.TempDir(), "missing.kubeconfig")
	options.KubeconfigSet = true
	platform, err := Compose(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	platform.Handler.ServeHTTP(recorder, healthRequest(options.Port))
	if recorder.Code != http.StatusOK {
		t.Fatalf("/health status = %d", recorder.Code)
	}
	for _, cleanup := range platform.Cleanups {
		_ = cleanup.Func(context.Background())
	}
}
