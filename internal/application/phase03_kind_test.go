//go:build phase03kind

package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	"github.com/fvmoraes/kubepeep/internal/application"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
)

// Run explicitly with -tags phase03kind against the dedicated Kind cluster.
// The app's SQLite/logs are isolated in t.TempDir; the browser is external.
func TestPhase03KindBrowser(t *testing.T) {
	if os.Getenv("KUBEPEEP_PHASE03_KIND") != "1" {
		t.Skip("opt-in Kind/browser performance probe")
	}
	root := t.TempDir()
	layout, err := userdirs.ForRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	platform, err := application.Compose(context.Background(), application.Options{
		Layout: layout, Config: productconfig.Default(), Port: port,
		Kubeconfig: filepath.Join(home, ".kube", "config"), KubeconfigSet: true,
		Context: "kind-kubepeep-f4", ContextSet: true, LogOutput: io.Discard,
	})
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	defer func() {
		for index := len(platform.Cleanups) - 1; index >= 0; index-- {
			_ = platform.Cleanups[index].Func(context.Background())
		}
	}()
	done := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/__phase03_done" && request.Method == http.MethodPost {
			close(done)
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		platform.Handler.ServeHTTP(writer, request)
	})}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	origin := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 30 * time.Second}
	call := func(method, path string, body any, csrf string) map[string]any {
		t.Helper()
		var input io.Reader
		if body != nil {
			encoded, marshalErr := json.Marshal(body)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			input = bytes.NewReader(encoded)
		}
		request, requestErr := http.NewRequest(method, origin+path, input)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		request.Header.Set("Origin", origin)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			request.Header.Set("X-KubePeep-CSRF", csrf)
		}
		response, responseErr := client.Do(request)
		if responseErr != nil {
			t.Fatal(responseErr)
		}
		defer response.Body.Close()
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if decodeErr := json.NewDecoder(response.Body).Decode(&envelope); decodeErr != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("%s %s: HTTP %d, decode=%v", method, path, response.StatusCode, decodeErr)
		}
		return envelope.Data
	}
	status := call(http.MethodGet, "/api/v1/status", nil, "")
	selection, ok := status["selection"].(map[string]any)
	if !ok {
		t.Fatal("Kind context did not activate")
	}
	session := call(http.MethodGet, "/api/v1/session", nil, "")
	csrf, ok := session["csrfToken"].(string)
	if !ok {
		t.Fatal("session has no CSRF token")
	}
	namespaces := make([]string, 50)
	for index := range namespaces {
		namespaces[index] = fmt.Sprintf("kp-bench-%04d", index)
	}
	scope := call(http.MethodPost, "/api/v1/namespace-scopes", map[string]any{
		"clusterProfileId": selection["clusterProfileId"], "context": selection["context"],
		"name": "Phase 03 isolated browser probe", "mode": "list", "namespaces": namespaces,
		"defaultNamespace": namespaces[0], "expectedGeneration": selection["generation"],
	}, csrf)
	call(http.MethodPost, fmt.Sprintf("/api/v1/namespace-scopes/%.0f/select", scope["id"]), map[string]any{"expectedGeneration": selection["generation"]}, csrf)
	t.Logf("phase03_real_origin=%s", origin)
	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		t.Fatal("browser probe did not signal completion")
	}
}
