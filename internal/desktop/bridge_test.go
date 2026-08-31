package desktop

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestInvokePathAllowlist(t *testing.T) {
	bridge := NewBridge(http.NotFoundHandler(), "http://127.0.0.1:2748", "http://127.0.0.1:2748")
	cases := []struct {
		method string
		path   string
		ok     bool
	}{
		{"GET", "/api/v1/status", true},
		{"POST", "/api/v1/contexts/select", true},
		{"GET", "/api/v1/pods?limit=10&continue=x", true},
		{"GET", "/api/v1/stream", false},
		{"GET", "/api/v1/pods/default/api/logs/stream", false},
		{"GET", "/api/v1/exec/sess/stream", false},
		{"GET", "/health", false},
		{"GET", "/api/v1/../secret", false},
		{"GET", "/api/v1/pods/..", false},
		{"PATCH", "/api/v1/status", false},
		{"GET", "", false},
	}
	for _, testCase := range cases {
		_, err := bridge.Invoke(testCase.method, testCase.path, nil, "")
		if testCase.ok && err != nil {
			t.Fatalf("%s %s: unexpected error: %v", testCase.method, testCase.path, err)
		}
		if !testCase.ok && err == nil {
			t.Fatalf("%s %s: expected rejection", testCase.method, testCase.path)
		}
	}
}

func TestInvokePreservesSecurityHeadersAndEnvelope(t *testing.T) {
	var observed *http.Request
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = r
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	bridge := NewBridge(handler, "http://127.0.0.1:2748", "http://127.0.0.1:2748")
	result, err := bridge.Invoke("POST", "/api/v1/contexts/select", map[string]string{
		"X-KubePeep-CSRF": "token",
		"Content-Type":    "application/json",
		"X-Request-ID":    "req_test",
		"X-Forwarded-For": "ignored",
		"Origin":          "https://evil.example",
	}, `{"context":"dev"}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if observed == nil {
		t.Fatal("handler was not called")
	}
	if observed.Host != "127.0.0.1:2748" {
		t.Fatalf("host=%q", observed.Host)
	}
	if observed.Header.Get("Origin") != "http://127.0.0.1:2748" {
		t.Fatalf("origin was spoofable: %q", observed.Header.Get("Origin"))
	}
	if observed.Header.Get("X-KubePeep-CSRF") != "token" {
		t.Fatalf("csrf header was not forwarded")
	}
	if observed.Header.Get("X-Forwarded-For") != "" {
		t.Fatalf("non-allowlisted header leaked: %q", observed.Header.Get("X-Forwarded-For"))
	}
	if result.Status != http.StatusOK || !strings.Contains(result.Body, `"ok":true`) {
		t.Fatalf("status=%d body=%s", result.Status, result.Body)
	}
}

func TestPlatformInfoIsSanitized(t *testing.T) {
	bridge := NewBridge(http.NotFoundHandler(), "http://127.0.0.1:2748", "http://127.0.0.1:2748")
	info := bridge.PlatformInfo()
	if info.Mode != "desktop" || info.StreamBase != "http://127.0.0.1:2748" {
		t.Fatalf("platform info: %+v", info)
	}
}

func TestLoopbackStreamCorsAndPreflight(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {}\n\n"))
	})
	loopback, err := NewLoopback(handler, DesktopExtraOrigins())
	if err != nil {
		t.Fatalf("loopback: %v", err)
	}
	defer loopback.Close(nilTimeout(t))
	client := &http.Client{Timeout: 5 * time.Second}

	streamRequest, err := http.NewRequest(http.MethodGet, loopback.Base()+"/api/v1/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamRequest.Header.Set("Origin", "null")
	response, err := client.Do(streamRequest)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != "null" {
		t.Fatalf("ACAO=%q", got)
	}

	plainRequest, _ := http.NewRequest(http.MethodGet, loopback.Base()+"/api/v1/status", nil)
	plainRequest.Header.Set("Origin", "null")
	response, err = client.Do(plainRequest)
	if err != nil {
		t.Fatalf("plain request: %v", err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("non-stream route received ACAO=%q", got)
	}

	preflight, _ := http.NewRequest(http.MethodOptions, loopback.Base()+"/api/v1/stream", nil)
	preflight.Header.Set("Origin", "wails://wails")
	response, err = client.Do(preflight)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status=%d", response.StatusCode)
	}
	if got := response.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-KubePeep-CSRF") {
		t.Fatalf("preflight headers=%q", got)
	}
}

func nilTimeout(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
