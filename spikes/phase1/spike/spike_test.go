package spike

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/fvmoraes/ginger/pkg/app"
	"github.com/fvmoraes/ginger/pkg/config"
	gingerlogger "github.com/fvmoraes/ginger/pkg/logger"
	"github.com/fvmoraes/ginger/pkg/middleware"
	gingersse "github.com/fvmoraes/ginger/pkg/sse"
	"github.com/fvmoraes/ginger/pkg/ws"
	_ "modernc.org/sqlite"
)

func TestRootAndStartUseTheSameContract(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"--context", "development", "--namespace", "payments", "--no-browser", "--port", "3000"},
		{"start", "--context", "development", "--namespace", "payments", "--no-browser", "--port", "3000"},
	} {
		arguments := arguments
		t.Run(strings.Join(arguments, "_"), func(t *testing.T) {
			var received StartOptions
			command := NewRootCommand(func(_ context.Context, options StartOptions) error {
				received = options
				return nil
			})
			command.SetArgs(arguments)
			if err := command.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("execute command: %v", err)
			}
			if received.Context != "development" || received.Namespace != "payments" {
				t.Fatalf("unexpected options: %+v", received)
			}
			if !received.NoBrowser || received.Port != 3000 {
				t.Fatalf("unexpected local options: %+v", received)
			}
		})
	}
}

func TestCobraContextOwnsTheGingerRuntime(t *testing.T) {
	application := newGingerApp(t)
	started := make(chan *Runtime, 1)
	var cleaned atomic.Bool

	command := NewRootCommand(func(ctx context.Context, _ StartOptions) error {
		runtime, err := NewRuntime(application, 2748, 50, func(context.Context) error {
			cleaned.Store(true)
			return nil
		})
		if err != nil {
			return err
		}
		started <- runtime
		return runtime.Run(ctx)
	})
	command.SetArgs([]string{"start", "--no-browser"})

	commandContext, cancelCommand := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- command.ExecuteContext(commandContext)
	}()

	var runtime *Runtime
	select {
	case runtime = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Cobra did not start the Ginger runtime")
	}
	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := runtime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait ready: %v", err)
	}

	cancelCommand()
	if err := <-result; err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if !cleaned.Load() {
		t.Fatal("Cobra cancellation did not execute runtime cleanup")
	}
}

func TestBindLoopbackRetriesUsingTheRealListener(t *testing.T) {
	first, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve first port: %v", err)
	}
	defer first.Close()
	firstPort := first.Addr().(*net.TCPAddr).Port

	listener, selected, err := BindLoopback(firstPort, 2)
	if err != nil {
		t.Fatalf("bind with retry: %v", err)
	}
	defer listener.Close()
	if selected != firstPort+1 {
		t.Fatalf("selected port %d, want %d", selected, firstPort+1)
	}
	if !listener.Addr().(*net.TCPAddr).IP.IsLoopback() {
		t.Fatalf("listener is not loopback: %s", listener.Addr())
	}
}

func TestBindLoopbackDoesNotHideNonAddressInUseErrors(t *testing.T) {
	var calls atomic.Int32
	_, _, err := bindLoopback(func(_, _ string) (net.Listener, error) {
		calls.Add(1)
		return nil, syscall.EACCES
	}, 2748, 10)
	if !errors.Is(err, syscall.EACCES) {
		t.Fatalf("bind returned %v, want permission error", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("listen called %d times, want fail-fast after one call", calls.Load())
	}
}

func TestRuntimeUsesGingerAndAlwaysCleansUp(t *testing.T) {
	application := newGingerApp(t)
	var cleaned atomic.Bool
	runtime, err := NewRuntime(application, 2748, 50, func(context.Context) error {
		cleaned.Store(true)
		return nil
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- runtime.Run(runContext)
	}()

	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := runtime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait for readiness: %v", err)
	}

	cancelRun()
	if err := <-result; err != nil {
		t.Fatalf("run runtime: %v", err)
	}
	if !cleaned.Load() {
		t.Fatal("cleanup hook did not run")
	}
}

func TestOuterHealthMuxAndReadinessCallbackPrecedeBrowser(t *testing.T) {
	application := newGingerApp(t)
	localRuntime, err := NewRuntime(application, 2748, 50)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	var callbackCalled atomic.Bool
	err = localRuntime.RunWithReady(
		runContext,
		2*time.Second,
		func(_ context.Context, publishedURL string) error {
			if publishedURL != localRuntime.URL() {
				t.Fatalf("published URL %q, want %q", publishedURL, localRuntime.URL())
			}
			response, err := http.Get(publishedURL + "/health")
			if err != nil {
				return err
			}
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				return readErr
			}
			if response.StatusCode != http.StatusOK ||
				response.Header.Get("Cache-Control") != "no-store" ||
				!strings.Contains(string(body), `"components"`) ||
				!strings.Contains(string(body), `"kubeconfig"`) {
				t.Fatalf(
					"composed health not ready: status=%d cache=%q body=%q",
					response.StatusCode,
					response.Header.Get("Cache-Control"),
					body,
				)
			}
			callbackCalled.Store(true)
			cancelRun()
			return nil
		},
	)
	if err != nil {
		t.Fatalf("run with readiness: %v", err)
	}
	if !callbackCalled.Load() {
		t.Fatal("browser callback was not called after readiness")
	}
}

func TestRuntimeCleansUpAfterServeError(t *testing.T) {
	application := newGingerApp(t)
	var cleaned atomic.Bool
	runtime, err := NewRuntime(application, 2748, 50, func(context.Context) error {
		cleaned.Store(true)
		return nil
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	if err := runtime.Listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	err = runtime.Run(context.Background())
	if err == nil {
		t.Fatal("expected serve error")
	}
	if !cleaned.Load() {
		t.Fatal("cleanup hook did not run after serve error")
	}
}

func TestRuntimeForcesCloseAndCleansUpAfterShutdownTimeout(t *testing.T) {
	application := newGingerApp(t)
	started := make(chan struct{})
	release := make(chan struct{})
	application.Router.HandleRaw("GET /stuck-stream", RawMiddleware(application.Logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		close(started)
		<-release
	})))

	var cleaned atomic.Bool
	runtime, err := NewRuntime(application, 2748, 50, func(context.Context) error {
		cleaned.Store(true)
		return nil
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	runtime.ShutdownTimeout = 50 * time.Millisecond

	runContext, cancelRun := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runtime.Run(runContext) }()

	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := runtime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait ready: %v", err)
	}

	response := make(chan *http.Response, 1)
	requestErr := make(chan error, 1)
	go func() {
		res, err := http.Get(runtime.URL() + "/stuck-stream")
		if err != nil {
			requestErr <- err
			return
		}
		response <- res
	}()

	select {
	case <-started:
	case err := <-requestErr:
		t.Fatalf("start stuck stream: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("stuck stream did not start")
	}

	cancelRun()
	if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run returned %v, want shutdown deadline", err)
	}
	if !cleaned.Load() {
		t.Fatal("cleanup hook did not run after shutdown timeout")
	}
	close(release)

	select {
	case res := <-response:
		_ = res.Body.Close()
	case <-requestErr:
	case <-time.After(time.Second):
	}
}

func TestRuntimeCleanupOwnsHijackedConnections(t *testing.T) {
	application := newGingerApp(t)
	hijackedReady := make(chan struct{})
	release := make(chan struct{})
	var mutex sync.Mutex
	var hijacked net.Conn

	application.Router.HandleRaw("GET /hijack", RawMiddleware(application.Logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connection, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		mutex.Lock()
		hijacked = connection
		mutex.Unlock()
		_, _ = buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: probe\r\n\r\n")
		_ = buffer.Flush()
		close(hijackedReady)
		<-release
	})))

	var cleaned atomic.Bool
	runtime, err := NewRuntime(application, 2748, 50, func(context.Context) error {
		cleaned.Store(true)
		mutex.Lock()
		defer mutex.Unlock()
		if hijacked != nil {
			return hijacked.Close()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runtime.Run(runContext) }()

	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := runtime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait ready: %v", err)
	}

	client, err := net.Dial("tcp4", runtime.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial runtime: %v", err)
	}
	defer client.Close()
	_, _ = io.WriteString(client, "GET /hijack HTTP/1.1\r\nHost: "+runtime.Listener.Addr().String()+"\r\nConnection: Upgrade\r\nUpgrade: probe\r\n\r\n")

	select {
	case <-hijackedReady:
	case <-time.After(2 * time.Second):
		t.Fatal("connection was not hijacked")
	}

	cancelRun()
	if err := <-result; err != nil {
		t.Fatalf("run runtime: %v", err)
	}
	if !cleaned.Load() {
		t.Fatal("cleanup hook did not own the hijacked connection")
	}
	close(release)
}

func TestSPAFallbackDoesNotCaptureHealthOrAPI(t *testing.T) {
	frontend, err := FrontendFS()
	if err != nil {
		t.Fatalf("frontend fs: %v", err)
	}
	handler := SPAHandler(frontend)

	for _, path := range []string{
		"/health",
		"/health/details",
		"/api",
		"/api/v1/status",
		"/api/v1/missing",
	} {
		request, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		recorder := newRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.status != http.StatusNotFound {
			t.Fatalf("%s returned %d, want 404", path, recorder.status)
		}
	}

	request, _ := http.NewRequest(http.MethodGet, "/workloads/example", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	recorder := newRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.status != http.StatusOK || !strings.Contains(recorder.body.String(), "embedded frontend") {
		t.Fatalf("SPA fallback failed: status=%d body=%q", recorder.status, recorder.body.String())
	}

	request, _ = http.NewRequest(http.MethodGet, "/workloads/example", nil)
	request.Header.Set("Accept", "application/json")
	recorder = newRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.status != http.StatusNotFound {
		t.Fatalf("non-HTML fallback returned %d, want 404", recorder.status)
	}
}

func TestFrontendAndMigrationsAreEmbeddedTogether(t *testing.T) {
	frontend, err := FrontendFS()
	if err != nil {
		t.Fatalf("frontend fs: %v", err)
	}
	if _, err := io.ReadAll(mustOpen(t, frontend, "index.html")); err != nil {
		t.Fatalf("read frontend: %v", err)
	}
	migrations, err := MigrationFS()
	if err != nil {
		t.Fatalf("migration fs: %v", err)
	}
	content, err := io.ReadAll(mustOpen(t, migrations, "001_initial.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if !strings.Contains(string(content), "CREATE TABLE phase1_probe") {
		t.Fatalf("unexpected migration: %s", content)
	}

	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "phase1.db"))
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := ApplyEmbeddedMigrations(context.Background(), database); err != nil {
		t.Fatalf("apply embedded migration: %v", err)
	}
	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'phase1_probe'`,
	).Scan(&count); err != nil {
		t.Fatalf("query migrated schema: %v", err)
	}
	if count != 1 {
		t.Fatalf("migrated table count = %d, want 1", count)
	}
}

func TestRawChainPreservesStreamingInterfacesAndRejectsForeignOrigin(t *testing.T) {
	application := newGingerApp(t)
	var logOutput bytes.Buffer
	application.Logger = &gingerlogger.Logger{
		Logger: slog.New(slog.NewJSONHandler(&logOutput, nil)),
	}
	var requestIDObserved atomic.Bool
	application.Router.HandleRaw("GET /stream", RawMiddleware(application.Logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := RequireStreamingInterfaces(w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		requestIDObserved.Store(middleware.RequestIDFromContext(r.Context()) != "")
		w.WriteHeader(http.StatusNoContent)
	})))
	application.Router.HandleRaw("GET /panic", RawMiddleware(application.Logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("synthetic panic")
	})))

	runtime, err := NewRuntime(application, 2748, 50)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	runContext, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runtime.Run(runContext) }()
	defer func() {
		cancel()
		<-result
	}()

	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := runtime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait ready: %v", err)
	}

	response, err := http.Get(runtime.URL() + "/stream")
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("stream returned %d", response.StatusCode)
	}
	if response.Header.Get("X-Request-ID") == "" || !requestIDObserved.Load() {
		t.Fatal("raw chain did not propagate request ID to context and response")
	}
	if !strings.Contains(logOutput.String(), "raw_request_finished") ||
		!strings.Contains(logOutput.String(), "request_id") {
		t.Fatalf("raw chain did not emit structured completion log: %s", logOutput.String())
	}

	request, _ := http.NewRequest(http.MethodGet, runtime.URL()+"/stream", nil)
	request.Header.Set("Origin", "https://attacker.invalid")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("foreign origin request: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin returned %d, want 403", response.StatusCode)
	}

	response, err = http.Get(runtime.URL() + "/panic")
	if err != nil {
		t.Fatalf("panic request: %v", err)
	}
	panicBody, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatalf("read panic response: %v", readErr)
	}
	if response.StatusCode != http.StatusInternalServerError ||
		!strings.Contains(string(panicBody), `"code":"INTERNAL"`) ||
		response.Header.Get("X-Request-ID") == "" {
		t.Fatalf(
			"panic recovery failed: status=%d requestID=%q body=%q",
			response.StatusCode,
			response.Header.Get("X-Request-ID"),
			panicBody,
		)
	}
	if !strings.Contains(logOutput.String(), "panic_recovered") {
		t.Fatalf("panic was not logged by raw recovery: %s", logOutput.String())
	}
}

func TestGingerWebSocketAloneAcceptsForeignOriginAndUnmaskedClientFrame(t *testing.T) {
	application := newGingerApp(t)
	received := make(chan map[string]string, 1)
	application.Router.HandleRaw("GET /ws-gap", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws.Handle(w, r, func(connection *ws.Conn) {
			var message map[string]string
			if err := connection.Recv(&message); err == nil {
				received <- message
			}
		})
	}))

	runtime, err := NewRuntime(application, 2748, 50)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	runContext, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runtime.Run(runContext) }()
	defer func() {
		cancel()
		<-result
	}()

	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := runtime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait ready: %v", err)
	}

	client, err := net.Dial("tcp4", runtime.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close()

	_, _ = io.WriteString(client,
		"GET /ws-gap HTTP/1.1\r\n"+
			"Host: "+runtime.Listener.Addr().String()+"\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: websocket\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
			"Origin: https://attacker.invalid\r\n\r\n",
	)
	reader := bufio.NewReader(client)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read websocket handshake: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("foreign Origin returned %d, want evidence of the 101 gap", response.StatusCode)
	}

	payload := []byte(`{"type":"unmasked"}`)
	frame := append([]byte{0x81, byte(len(payload))}, payload...)
	if _, err := client.Write(frame); err != nil {
		t.Fatalf("write unmasked frame: %v", err)
	}

	select {
	case message := <-received:
		if message["type"] != "unmasked" {
			t.Fatalf("unexpected message: %+v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ginger did not accept the unmasked client frame as expected by the documented gap")
	}
}

func TestGingerSSEStopsWhenTheRequestContextIsCanceled(t *testing.T) {
	application := newGingerApp(t)
	canceled := make(chan struct{})
	application.Router.HandleRaw("GET /cancel-stream", RawMiddleware(application.Logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream, err := gingersse.New(w)
		if err != nil {
			return
		}
		_ = stream.Send(gingersse.Event{Type: "probe", Data: "ready"})
		<-r.Context().Done()
		close(canceled)
	})))

	runtime, err := NewRuntime(application, 2748, 50)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	runContext, cancelRun := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runtime.Run(runContext) }()
	defer func() {
		cancelRun()
		<-result
	}()

	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := runtime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait ready: %v", err)
	}

	requestContext, cancelRequest := context.WithCancel(context.Background())
	request, _ := http.NewRequestWithContext(requestContext, http.MethodGet, runtime.URL()+"/cancel-stream", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	cancelRequest()
	_ = response.Body.Close()

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not observe request cancellation")
	}
}

func TestCompoundCursorRejectsTamperingQueryChangesAndExpiry(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	codec, err := NewCursorCodec([]byte("0123456789abcdef0123456789abcdef"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("new cursor codec: %v", err)
	}
	cursor := Cursor{
		ContextGeneration: "generation-7",
		QueryHash:         "query-a",
		Positions: []CursorPosition{{
			Namespace: "payments",
			Kind:      "Pod",
			Continue:  "next-1",
		}},
		ExpiresAt: now.Add(time.Minute),
	}
	token, err := codec.Encode(cursor)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := codec.Decode(token, "generation-7", "query-a")
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if len(decoded.Positions) != 1 ||
		decoded.Positions[0].Namespace != "payments" ||
		decoded.Positions[0].Kind != "Pod" ||
		decoded.Positions[0].Continue != "next-1" {
		t.Fatalf("unexpected cursor: %+v", decoded)
	}
	if _, err := codec.Decode(token, "generation-8", "query-a"); !errors.Is(err, ErrCursorContext) {
		t.Fatalf("generation mismatch returned %v", err)
	}
	if _, err := codec.Decode(token, "generation-7", "query-b"); !errors.Is(err, ErrCursorQuery) {
		t.Fatalf("query mismatch returned %v", err)
	}
	if _, err := codec.Decode(token+"x", "generation-7", "query-a"); !errors.Is(err, ErrCursorInvalid) {
		t.Fatalf("tampered cursor returned %v", err)
	}

	expiredCodec, _ := NewCursorCodec(
		[]byte("0123456789abcdef0123456789abcdef"),
		func() time.Time { return now.Add(2 * time.Minute) },
	)
	if _, err := expiredCodec.Decode(token, "generation-7", "query-a"); !errors.Is(err, ErrCursorExpired) {
		t.Fatalf("expired cursor returned %v", err)
	}

	_, err = codec.Encode(Cursor{
		ContextGeneration: "generation-7",
		QueryHash:         "query-a",
		Positions: []CursorPosition{
			{Namespace: "payments", Kind: "Pod", Continue: "next-1"},
			{Namespace: "payments", Kind: "Pod", Continue: "next-2"},
		},
		ExpiresAt: now.Add(time.Minute),
	})
	if !errors.Is(err, ErrCursorState) {
		t.Fatalf("duplicate structural position returned %v", err)
	}
}

func TestSSESurvivesLongerThanGingerRunWriteTimeout(t *testing.T) {
	if os.Getenv("KUBEPEEP_LONG_TEST") != "1" {
		t.Skip("set KUBEPEEP_LONG_TEST=1 to execute the 16-second stream proof")
	}

	application := newGingerApp(t)
	application.Router.HandleRaw("GET /long-stream", RawMiddleware(application.Logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream, err := gingersse.New(w)
		if err != nil {
			http.Error(w, "stream unavailable", http.StatusInternalServerError)
			return
		}
		if err := stream.Send(gingersse.Event{Type: "probe", Data: "start"}); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(16 * time.Second):
		}
		_ = stream.Send(gingersse.Event{Type: "probe", Data: "end"})
	})))

	runtime, err := NewRuntime(application, 2748, 50)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	runContext, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runtime.Run(runContext) }()
	defer func() {
		cancel()
		<-result
	}()

	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := runtime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait ready: %v", err)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Get(runtime.URL() + "/long-stream")
	if err != nil {
		t.Fatalf("long stream request: %v", err)
	}
	content, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read long stream: %v", err)
	}
	if !strings.Contains(string(content), "data: end") {
		t.Fatalf("stream ended early: %q", content)
	}
}

func newGingerApp(t *testing.T) *app.App {
	t.Helper()
	application := app.New(&config.Config{
		App: config.AppConfig{Name: "phase1-spike", Env: "test"},
		HTTP: config.HTTPConfig{
			Host:            "127.0.0.1",
			Port:            2748,
			ShutdownTimeout: 1,
		},
		Log: config.LogConfig{Level: "error", Format: "json"},
	})
	return application
}

type responseRecorder struct {
	header http.Header
	body   strings.Builder
	status int
}

func newRecorder() *responseRecorder {
	return &responseRecorder{header: make(http.Header), status: http.StatusOK}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(payload []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(payload)
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
}

func mustOpen(t *testing.T, filesystem interface {
	Open(string) (fs.File, error)
}, name string) fs.File {
	t.Helper()
	file, err := filesystem.Open(name)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}
