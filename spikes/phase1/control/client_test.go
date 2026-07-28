package control

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatusRejectsAuthenticatedIdentityMismatch(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	instance := testInstance(t, listener.Addr().(*net.TCPAddr).Port)
	runtimeDir := t.TempDir()
	if err := WriteInstanceAtomic(runtimeDir, instance); err != nil {
		t.Fatalf("write state: %v", err)
	}

	fakeProof := instance.Public()
	fakeProof.InstanceID = "different-0123456789abcdef0123456789abcdef"
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(ControlTokenHeader) != instance.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fakeProof)
	})}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
		<-serveResult
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := Status(ctx, runtimeDir); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("Status error = %v, want ErrIdentityMismatch", err)
	}
	if _, err := LoadInstance(runtimeDir); err != nil {
		t.Fatalf("identity mismatch unexpectedly removed state: %v", err)
	}
}

func TestStatusRejectsTrailingIdentityProof(t *testing.T) {
	var instance Instance
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(ControlTokenHeader) != instance.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(instance.Public())
		_, _ = w.Write([]byte("{}\n"))
	}))
	defer server.Close()

	instance = testInstance(t, server.Listener.Addr().(*net.TCPAddr).Port)
	runtimeDir := t.TempDir()
	if err := WriteInstanceAtomic(runtimeDir, instance); err != nil {
		t.Fatalf("write state: %v", err)
	}
	lock, err := AcquireFileLock(LockPath(runtimeDir))
	if err != nil {
		t.Fatalf("acquire active-instance lock: %v", err)
	}
	defer lock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := Status(ctx, runtimeDir); err == nil ||
		!strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("Status error = %v; want trailing JSON rejection", err)
	}
}
