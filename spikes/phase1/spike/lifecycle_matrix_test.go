package spike

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gingersse "github.com/fvmoraes/ginger/pkg/sse"
	"github.com/fvmoraes/ginger/pkg/ws"
)

func TestLifecycleMatrixCleansSSEWebSocketAndHijack(t *testing.T) {
	testCases := []struct {
		name      string
		terminate func(*Runtime, context.CancelFunc, <-chan error)
		assertErr func(*testing.T, error)
		stubborn  bool
	}{
		{
			name: "normal shutdown",
			terminate: func(_ *Runtime, cancel context.CancelFunc, _ <-chan error) {
				cancel()
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("normal shutdown returned %v", err)
				}
			},
		},
		{
			name: "server error",
			terminate: func(localRuntime *Runtime, _ context.CancelFunc, _ <-chan error) {
				_ = localRuntime.Listener.Close()
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("server error returned nil")
				}
			},
		},
		{
			name: "shutdown timeout",
			terminate: func(_ *Runtime, cancel context.CancelFunc, _ <-chan error) {
				cancel()
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("shutdown timeout returned %v", err)
				}
			},
			stubborn: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runLifecycleMatrixCase(t, testCase.stubborn, testCase.terminate, testCase.assertErr)
		})
	}
}

func runLifecycleMatrixCase(
	t *testing.T,
	stubborn bool,
	terminate func(*Runtime, context.CancelFunc, <-chan error),
	assertErr func(*testing.T, error),
) {
	t.Helper()
	application := newGingerApp(t)
	sessionContext, cancelSessions := context.WithCancel(context.Background())
	defer cancelSessions()

	sseStarted := make(chan struct{})
	wsStarted := make(chan struct{})
	hijackStarted := make(chan struct{})
	var sessions sync.WaitGroup
	sessions.Add(3)

	application.Router.HandleRaw("GET /matrix-sse", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defer sessions.Done()
		stream, err := gingersse.New(w)
		if err != nil {
			return
		}
		_ = stream.Send(gingersse.Event{Type: "ready", Data: "sse"})
		close(sseStarted)
		<-sessionContext.Done()
	}))
	application.Router.HandleRaw("GET /matrix-ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws.Handle(w, r, func(*ws.Conn) {
			defer sessions.Done()
			close(wsStarted)
			<-sessionContext.Done()
		})
	}))
	application.Router.HandleRaw("GET /matrix-hijack", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connection, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			sessions.Done()
			return
		}
		defer sessions.Done()
		defer connection.Close() //nolint:errcheck
		_, _ = buffer.WriteString(
			"HTTP/1.1 101 Switching Protocols\r\n" +
				"Connection: Upgrade\r\n" +
				"Upgrade: matrix\r\n\r\n",
		)
		_ = buffer.Flush()
		close(hijackStarted)
		<-sessionContext.Done()
	}))

	stubbornStarted := make(chan struct{})
	releaseStubborn := make(chan struct{})
	if stubborn {
		application.Router.HandleRaw("GET /matrix-stubborn", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			flusher := w.(http.Flusher)
			w.WriteHeader(http.StatusOK)
			flusher.Flush()
			close(stubbornStarted)
			<-releaseStubborn
		}))
	}

	var finalCleanup atomic.Bool
	localRuntime, err := NewRuntime(application, 2748, 50, func(context.Context) error {
		finalCleanup.Store(true)
		return nil
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	localRuntime.ShutdownTimeout = 100 * time.Millisecond
	sessionsClosed := make(chan struct{})
	go func() {
		sessions.Wait()
		close(sessionsClosed)
	}()
	localRuntime.AddSessionCleanup(func(ctx context.Context) error {
		cancelSessions()
		select {
		case <-sessionsClosed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	runContext, cancelRun := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- localRuntime.Run(runContext)
	}()
	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := localRuntime.WaitReady(readyContext); err != nil {
		t.Fatalf("wait ready: %v", err)
	}

	sseResponse, err := http.Get(localRuntime.URL() + "/matrix-sse")
	if err != nil {
		t.Fatalf("open SSE: %v", err)
	}
	defer sseResponse.Body.Close()
	waitForSignal(t, sseStarted, "SSE")

	wsConnection := dialLifecycleWebSocket(t, localRuntime)
	defer wsConnection.Close()
	waitForSignal(t, wsStarted, "WebSocket")

	hijacked := dialLifecycleHijack(t, localRuntime)
	defer hijacked.Close()
	waitForSignal(t, hijackStarted, "hijack")

	var stubbornResponse *http.Response
	if stubborn {
		responseResult := make(chan *http.Response, 1)
		responseErr := make(chan error, 1)
		go func() {
			response, err := http.Get(localRuntime.URL() + "/matrix-stubborn")
			if err != nil {
				responseErr <- err
				return
			}
			responseResult <- response
		}()
		select {
		case <-stubbornStarted:
		case err := <-responseErr:
			t.Fatalf("open stubborn stream: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("stubborn stream did not start")
		}
		select {
		case stubbornResponse = <-responseResult:
		default:
		}
	}

	terminate(localRuntime, cancelRun, result)
	select {
	case runErr := <-result:
		assertErr(t, runErr)
	case <-time.After(3 * time.Second):
		t.Fatal("runtime did not finish")
	}
	if stubborn {
		close(releaseStubborn)
		if stubbornResponse != nil {
			_ = stubbornResponse.Body.Close()
		}
	}
	if !finalCleanup.Load() {
		t.Fatal("final cleanup did not run")
	}
	select {
	case <-sessionsClosed:
	default:
		t.Fatal("session cleanup did not close SSE, WebSocket and hijack")
	}
}

func dialLifecycleWebSocket(t *testing.T, localRuntime *Runtime) net.Conn {
	t.Helper()
	connection, err := net.Dial("tcp4", localRuntime.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial lifecycle WebSocket: %v", err)
	}
	_, _ = io.WriteString(
		connection,
		"GET /matrix-ws HTTP/1.1\r\n"+
			"Host: "+localRuntime.Listener.Addr().String()+"\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: websocket\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n",
	)
	response, err := http.ReadResponse(
		bufio.NewReader(connection),
		&http.Request{Method: http.MethodGet},
	)
	if err != nil {
		_ = connection.Close()
		t.Fatalf("read lifecycle WebSocket handshake: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		_ = connection.Close()
		t.Fatalf("lifecycle WebSocket returned %d", response.StatusCode)
	}
	return connection
}

func dialLifecycleHijack(t *testing.T, localRuntime *Runtime) net.Conn {
	t.Helper()
	connection, err := net.Dial("tcp4", localRuntime.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial lifecycle hijack: %v", err)
	}
	_, _ = io.WriteString(
		connection,
		"GET /matrix-hijack HTTP/1.1\r\n"+
			"Host: "+localRuntime.Listener.Addr().String()+"\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: matrix\r\n\r\n",
	)
	response, err := http.ReadResponse(
		bufio.NewReader(connection),
		&http.Request{Method: http.MethodGet},
	)
	if err != nil {
		_ = connection.Close()
		t.Fatalf("read lifecycle hijack response: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		_ = connection.Close()
		t.Fatalf("lifecycle hijack returned %d", response.StatusCode)
	}
	return connection
}

func waitForSignal(t *testing.T, signal <-chan struct{}, resource string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not become active", resource)
	}
}
