package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestControlHandlerRequiresLoopbackHostOriginAndToken(t *testing.T) {
	instance := testInstance(t, 2748)
	var cancelCalls atomic.Int32
	handler := newControlHandler(instance, "127.0.0.1:2748", func() {
		cancelCalls.Add(1)
	})

	request := func(method, path, token, host, origin, remote string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://127.0.0.1:2748"+path, nil)
		req.Host = host
		req.RemoteAddr = remote
		req.Header.Set(ControlTokenHeader, token)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder
	}

	valid := request(
		http.MethodGet,
		statusPath,
		instance.Token,
		"127.0.0.1:2748",
		"",
		"127.0.0.1:45000",
	)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid status = %d, body=%q", valid.Code, valid.Body.String())
	}
	if valid.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", valid.Header().Get("Cache-Control"))
	}
	if containsToken(valid.Body.Bytes(), instance.Token) {
		t.Fatal("status response exposed the control token")
	}

	tests := []struct {
		name   string
		token  string
		host   string
		origin string
		remote string
		want   int
	}{
		{
			name:   "wrong token",
			token:  "wrong-token",
			host:   "127.0.0.1:2748",
			remote: "127.0.0.1:45000",
			want:   http.StatusUnauthorized,
		},
		{
			name:   "foreign host",
			token:  instance.Token,
			host:   "attacker.example",
			remote: "127.0.0.1:45000",
			want:   http.StatusForbidden,
		},
		{
			name:   "browser origin",
			token:  instance.Token,
			host:   "127.0.0.1:2748",
			origin: "https://attacker.example",
			remote: "127.0.0.1:45000",
			want:   http.StatusForbidden,
		},
		{
			name:   "non-loopback peer",
			token:  instance.Token,
			host:   "127.0.0.1:2748",
			remote: "192.0.2.10:45000",
			want:   http.StatusForbidden,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := request(
				http.MethodGet,
				statusPath,
				test.token,
				test.host,
				test.origin,
				test.remote,
			)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}

	for range 2 {
		recorder := request(
			http.MethodPost,
			stopPath,
			instance.Token,
			"127.0.0.1:2748",
			"",
			"127.0.0.1:45000",
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("stop status = %d", recorder.Code)
		}
	}
	if got := cancelCalls.Load(); got != 1 {
		t.Fatalf("cancel calls = %d, want 1", got)
	}
}

func TestForegroundStatusAndStopUseAuthenticatedIdentity(t *testing.T) {
	runtimeDir := t.TempDir()
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	ready := make(chan PublicInstance, 1)
	runResult := make(chan error, 1)
	go func() {
		runResult <- RunForeground(parent, RunOptions{
			RuntimeDir:      runtimeDir,
			FirstPort:       0,
			PortAttempts:    1,
			ShutdownTimeout: time.Second,
			OnReady: func(instance PublicInstance) {
				ready <- instance
			},
		})
	}()

	var expected PublicInstance
	select {
	case expected = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("foreground runtime did not become ready")
	}

	requestContext, cancelRequest := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelRequest()
	status, err := Status(requestContext, runtimeDir)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status != expected {
		t.Fatalf("status = %#v, want %#v", status, expected)
	}

	stopped, running, err := Stop(requestContext, runtimeDir)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !running || stopped != expected {
		t.Fatalf("stop = %#v running=%v, want %#v", stopped, running, expected)
	}

	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("foreground result: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("foreground runtime did not stop")
	}

	if _, err := LoadInstance(runtimeDir); err != ErrNotRunning {
		t.Fatalf("state after stop error = %v, want ErrNotRunning", err)
	}
	_, running, err = Stop(requestContext, runtimeDir)
	if err != nil || running {
		t.Fatalf("idempotent stop: running=%v err=%v", running, err)
	}
}

func TestForegroundParentCancellationCleansStateAndReleasesLock(t *testing.T) {
	runtimeDir := t.TempDir()
	parent, cancelParent := context.WithCancel(context.Background())

	ready := make(chan PublicInstance, 1)
	runResult := make(chan error, 1)
	go func() {
		runResult <- RunForeground(parent, RunOptions{
			RuntimeDir:      runtimeDir,
			FirstPort:       0,
			ShutdownTimeout: time.Second,
			OnReady: func(instance PublicInstance) {
				ready <- instance
			},
		})
	}()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("foreground runtime did not become ready")
	}

	cancelParent()
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("foreground cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("foreground runtime did not stop after parent cancellation")
	}
	if _, err := LoadInstance(runtimeDir); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("state after cancellation error = %v, want ErrNotRunning", err)
	}
	lock, err := AcquireFileLock(LockPath(runtimeDir))
	if err != nil {
		t.Fatalf("reacquire lock after cancellation: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("release reacquired lock: %v", err)
	}
}

func TestRandomSecretsAreIndependent(t *testing.T) {
	first, err := randomSecret()
	if err != nil {
		t.Fatalf("first random secret: %v", err)
	}
	second, err := randomSecret()
	if err != nil {
		t.Fatalf("second random secret: %v", err)
	}
	if len(first) < 32 || len(second) < 32 || first == second {
		t.Fatalf("unexpected random secrets: first_len=%d second_len=%d equal=%v", len(first), len(second), first == second)
	}
}

func containsToken(data []byte, token string) bool {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	if value, ok := raw["token"].(string); ok && value == token {
		return true
	}
	for _, value := range raw {
		if text, ok := value.(string); ok && text == token {
			return true
		}
	}
	return false
}
