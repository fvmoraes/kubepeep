package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func controlRequest(t *testing.T, state InstanceStateV1, method, path string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1:"+strconv.Itoa(state.Port)+path, nil)
	request.Host = "127.0.0.1:" + strconv.Itoa(state.Port)
	request.RemoteAddr = "127.0.0.1:41000"
	request.Header.Set(ControlTokenHeader, state.ControlToken)
	return request
}

func TestControlHandlerReturnsSixFieldProofWithoutToken(t *testing.T) {
	state := testState(t, DefaultFirstPort)
	handler := NewControlHandler(state, "127.0.0.1:2748", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, controlRequest(t, state, http.MethodGet, ControlStatusPath))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "control_token") || strings.Contains(recorder.Body.String(), state.ControlToken) {
		t.Fatal("control token leaked in response")
	}
	var identity ControlIdentityDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity != state.Identity() {
		t.Fatalf("identity = %#v, want %#v", identity, state.Identity())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("control security headers are missing")
	}
}

func TestControlStopCancelsOnceAfterProof(t *testing.T) {
	state := testState(t, DefaultFirstPort)
	var cancellations atomic.Int32
	handler := NewControlHandler(state, "127.0.0.1:2748", func() { cancellations.Add(1) })
	for range 2 {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, controlRequest(t, state, http.MethodPost, ControlStopPath))
		if recorder.Code != http.StatusOK || recorder.Body.Len() == 0 {
			t.Fatalf("stop response = %d %q", recorder.Code, recorder.Body.String())
		}
	}
	if cancellations.Load() != 1 {
		t.Fatalf("cancel called %d times", cancellations.Load())
	}
}

func TestControlHandlerEnforcesMethodAndGuards(t *testing.T) {
	state := testState(t, DefaultFirstPort)
	handler := NewControlHandler(state, "127.0.0.1:2748", nil)
	tests := []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{name: "method", mutate: func(request *http.Request) { request.Method = http.MethodPost }, status: http.StatusMethodNotAllowed},
		{name: "peer", mutate: func(request *http.Request) { request.RemoteAddr = "192.0.2.1:41000" }, status: http.StatusForbidden},
		{name: "host", mutate: func(request *http.Request) { request.Host = "localhost:2748" }, status: http.StatusForbidden},
		{name: "origin", mutate: func(request *http.Request) { request.Header.Set("Origin", "http://127.0.0.1:2748") }, status: http.StatusForbidden},
		{name: "empty origin header", mutate: func(request *http.Request) { request.Header["Origin"] = []string{""} }, status: http.StatusForbidden},
		{name: "query", mutate: func(request *http.Request) { request.URL.RawQuery = "x=1" }, status: http.StatusBadRequest},
		{name: "body", mutate: func(request *http.Request) { request.Body = ioNopCloser{"x"} }, status: http.StatusBadRequest},
		{name: "token", mutate: func(request *http.Request) { request.Header.Set(ControlTokenHeader, "wrong") }, status: http.StatusUnauthorized},
		{name: "duplicate token", mutate: func(request *http.Request) { request.Header.Add(ControlTokenHeader, state.ControlToken) }, status: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := controlRequest(t, state, http.MethodGet, ControlStatusPath)
			test.mutate(request)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
		})
	}
}

type ioNopCloser struct{ value string }

func (reader ioNopCloser) Read(buffer []byte) (int, error) {
	if reader.value == "" {
		return 0, errors.New("empty reader")
	}
	buffer[0] = reader.value[0]
	return 1, nil
}
func (ioNopCloser) Close() error { return nil }

func TestControlClientRoundTrip(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	state := testState(t, port)
	server := &http.Server{Handler: NewControlHandler(state, listener.Addr().String(), nil)}
	go server.Serve(listener)
	defer server.Close()

	identity, err := (ControlClient{}).Status(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if identity != state.Identity() {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestControlClientRejectsTrailingJSON(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	state := testState(t, port)
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		setControlHeaders(writer.Header())
		encoded, _ := json.Marshal(state.Identity())
		_, _ = writer.Write(append(encoded, []byte(" {}")...))
	})}
	go server.Serve(listener)
	defer server.Close()
	_, err = (ControlClient{}).Status(context.Background(), state)
	var protocolError *ControlProtocolError
	if !errors.As(err, &protocolError) {
		t.Fatalf("control error = %v, want protocol error", err)
	}
}

func TestControllerRemovesOnlyProvablyStaleState(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	if err := WriteInstanceStateAtomic(runtimeDirectory, testState(t, port)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, active, err := (Controller{}).Status(ctx, runtimeDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("stale instance reported active")
	}
	if _, err := os.Stat(InstancePath(runtimeDirectory)); !os.IsNotExist(err) {
		t.Fatalf("stale state was not removed: %v", err)
	}
}

func TestControllerFailsClosedOnIdentityMismatch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	state := testState(t, port)
	wrong := state.Identity()
	wrong.PID++
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setControlHeaders(writer.Header())
		_ = json.NewEncoder(writer).Encode(wrong)
	})}
	go server.Serve(listener)
	defer server.Close()
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	if err := WriteInstanceStateAtomic(runtimeDirectory, state); err != nil {
		t.Fatal(err)
	}
	_, _, err = (Controller{}).Status(context.Background(), runtimeDirectory)
	if !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("status error = %v, want identity mismatch", err)
	}
	if _, err := os.Stat(InstancePath(runtimeDirectory)); err != nil {
		t.Fatalf("mismatched state must remain fail-closed: %v", err)
	}
}

func TestControllerDoesNotRecoverStaleStateAfterCallerCancellation(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	if err := WriteInstanceStateAtomic(runtimeDirectory, testState(t, port)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = (Controller{}).Status(ctx, runtimeDirectory)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("status error = %v, want context canceled", err)
	}
	if _, err := os.Stat(InstancePath(runtimeDirectory)); err != nil {
		t.Fatalf("caller cancellation removed state: %v", err)
	}
}
