package runtime

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func healthyFactory(cleaned *atomic.Bool) ServiceFactory {
	return ServiceFactoryFunc(func(context.Context, ServiceDependencies) (Service, error) {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusOK)
		})
		return Service{
			Handler: mux,
			Cleanups: []NamedCleanup{{Name: "test service", Func: func(context.Context) error {
				cleaned.Store(true)
				return nil
			}}},
		}, nil
	})
}

func TestRunForegroundPublishesAfterHealthAndStopsThroughControl(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	port := availablePort(t)
	ready := make(chan ControlIdentityDTO, 1)
	var cleaned atomic.Bool
	resultChannel := make(chan RunResult, 1)
	errorChannel := make(chan error, 1)
	go func() {
		result, err := RunForeground(context.Background(), RunOptions{
			DataRoot:         filepath.Dir(runtimeDirectory),
			RuntimeDirectory: runtimeDirectory,
			Port:             &port,
			Factory:          healthyFactory(&cleaned),
			OnReady:          func(identity ControlIdentityDTO) { ready <- identity },
		})
		resultChannel <- result
		errorChannel <- err
	}()

	var identity ControlIdentityDTO
	select {
	case identity = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not publish readiness")
	}
	if identity.Port != port {
		t.Fatalf("published port = %d, want %d", identity.Port, port)
	}
	if _, err := os.Stat(InstancePath(runtimeDirectory)); err != nil {
		t.Fatalf("instance state was not published: %v", err)
	}
	proved, active, err := (Controller{}).Stop(context.Background(), runtimeDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if !active || proved != identity {
		t.Fatalf("stop proof = %#v, active=%v", proved, active)
	}
	select {
	case result := <-resultChannel:
		if !result.Started || result.Existing || result.Identity != identity {
			t.Fatalf("run result = %#v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not stop")
	}
	if err := <-errorChannel; err != nil {
		t.Fatal(err)
	}
	if !cleaned.Load() {
		t.Fatal("service cleanup did not run")
	}
	if _, err := os.Stat(InstancePath(runtimeDirectory)); !os.IsNotExist(err) {
		t.Fatalf("instance state survived cleanup: %v", err)
	}
}

func TestRunForegroundDoesNotPublishBeforeHealth(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	port := availablePort(t)
	healthAllowed := make(chan struct{})
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	factory := ServiceFactoryFunc(func(context.Context, ServiceDependencies) (Service, error) {
		return Service{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/health" {
				http.NotFound(writer, request)
				return
			}
			select {
			case <-healthAllowed:
				writer.WriteHeader(http.StatusOK)
			default:
				writer.WriteHeader(http.StatusServiceUnavailable)
			}
		})}, nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := RunForeground(ctx, RunOptions{
			RuntimeDirectory: runtimeDirectory, Port: &port, Factory: factory,
			OnReady: func(ControlIdentityDTO) { close(ready) },
		})
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(InstancePath(runtimeDirectory)); !os.IsNotExist(err) {
		t.Fatalf("state was published before health: %v", err)
	}
	close(healthAllowed)
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not become ready")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRunForegroundReturnsExistingAuthenticatedInstance(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	port := availablePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan ControlIdentityDTO, 1)
	firstDone := make(chan error, 1)
	var cleaned atomic.Bool
	go func() {
		_, err := RunForeground(ctx, RunOptions{
			RuntimeDirectory: runtimeDirectory, Port: &port, Factory: healthyFactory(&cleaned),
			OnReady: func(identity ControlIdentityDTO) { ready <- identity },
		})
		firstDone <- err
	}()
	var want ControlIdentityDTO
	select {
	case want = <-ready:
	case err := <-firstDone:
		t.Fatalf("first runtime exited before readiness: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("first runtime did not publish readiness")
	}
	got, err := RunForeground(context.Background(), RunOptions{
		RuntimeDirectory: runtimeDirectory, Port: &port, Factory: healthyFactory(&atomic.Bool{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Existing || got.Started || got.Identity != want {
		t.Fatalf("second start result = %#v", got)
	}
	cancel()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("first runtime did not stop after cancellation")
	}
}

func TestRunForegroundRequiresRealServiceFactory(t *testing.T) {
	_, err := RunForeground(context.Background(), RunOptions{RuntimeDirectory: filepath.Join(t.TempDir(), "runtime")})
	if !errors.Is(err, ErrServiceFactoryUnavailable) {
		t.Fatalf("error = %v, want ErrServiceFactoryUnavailable", err)
	}
}

func TestCleanupStagesRunBeforeHTTPThenAfterHTTPAndAggregateFailures(t *testing.T) {
	var registry CleanupRegistry
	var order []string
	beforeErr := errors.New("before failed")
	shutdownErr := errors.New("shutdown timed out")
	afterErr := errors.New("after failed")
	cleanups := []NamedCleanup{
		{Name: "storage", Stage: CleanupAfterHTTP, Func: func(context.Context) error {
			order = append(order, "after")
			return afterErr
		}},
		{Name: "streams", Stage: CleanupBeforeHTTP, Func: func(context.Context) error {
			order = append(order, "before")
			return beforeErr
		}},
	}
	if err := registerServiceCleanups(&registry, cleanups, CleanupAfterHTTP); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("HTTP server", func(context.Context) error {
		order = append(order, "shutdown")
		return shutdownErr
	}); err != nil {
		t.Fatal(err)
	}
	if err := registerServiceCleanups(&registry, cleanups, CleanupBeforeHTTP); err != nil {
		t.Fatal(err)
	}
	err := registry.Run(context.Background())
	if !reflect.DeepEqual(order, []string{"before", "shutdown", "after"}) {
		t.Fatalf("cleanup order = %v", order)
	}
	for _, wanted := range []error{beforeErr, shutdownErr, afterErr} {
		if !errors.Is(err, wanted) {
			t.Fatalf("cleanup aggregate %v does not contain %v", err, wanted)
		}
	}
}

func TestShutdownHTTPServerForcesCloseAfterTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		enteredOnce.Do(func() { close(entered) })
		<-release
		writer.WriteHeader(http.StatusOK)
	})}
	serveDone := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(serveDone)
	}()
	requestDone := make(chan struct{})
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if requestErr == nil {
			_ = response.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("blocking request did not enter the handler")
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	err = shutdownHTTPServer(shutdownContext, server, serveDone)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("forced close did not release the client")
	}
	close(release)
}

func TestRunForegroundRawTimeoutAndHookFailureStillCleanAllState(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	port := availablePort(t)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan ControlIdentityDTO, 1)
	rawEntered := make(chan struct{})
	rawRelease := make(chan struct{})
	rawExitedDone := make(chan struct{})
	requestDone := make(chan struct{})
	var enteredOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(rawRelease) }) })
	var rawExited atomic.Bool
	var beforeRan atomic.Bool
	var afterRan atomic.Bool
	hookFailure := errors.New("synthetic stream cleanup failure")

	factory := ServiceFactoryFunc(func(context.Context, ServiceDependencies) (Service, error) {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusOK)
		})
		mux.HandleFunc("/raw", func(writer http.ResponseWriter, _ *http.Request) {
			flusher, ok := writer.(http.Flusher)
			if !ok {
				http.Error(writer, "streaming unavailable", http.StatusInternalServerError)
				return
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.WriteHeader(http.StatusOK)
			flusher.Flush()
			enteredOnce.Do(func() { close(rawEntered) })
			<-rawRelease
			rawExited.Store(true)
			close(rawExitedDone)
		})
		return Service{
			Handler: mux,
			Cleanups: []NamedCleanup{
				{Name: "raw streams", Stage: CleanupBeforeHTTP, Func: func(context.Context) error {
					beforeRan.Store(true)
					return hookFailure
				}},
				{Name: "storage", Stage: CleanupAfterHTTP, Func: func(context.Context) error {
					afterRan.Store(true)
					releaseOnce.Do(func() { close(rawRelease) })
					return nil
				}},
			},
		}, nil
	})
	type outcome struct {
		result RunResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := RunForeground(parent, RunOptions{
			RuntimeDirectory: runtimeDirectory,
			Port:             &port,
			Factory:          factory,
			ShutdownTimeout:  time.Second,
			OnReady:          func(identity ControlIdentityDTO) { ready <- identity },
		})
		finished <- outcome{result: result, err: err}
	}()

	identity := <-ready
	go func() {
		response, err := http.Get(identity.URL() + "/raw")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-rawEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("raw route did not become active")
	}
	cancel()

	var got outcome
	select {
	case got = <-finished:
	case <-time.After(4 * time.Second):
		t.Fatal("runtime did not finish after forced shutdown")
	}
	if !got.result.Started || got.result.Identity != identity {
		t.Fatalf("run result lost the published identity: %#v", got.result)
	}
	if !errors.Is(got.err, context.DeadlineExceeded) || !errors.Is(got.err, hookFailure) {
		t.Fatalf("shutdown error does not expose timeout and hook failure: %v", got.err)
	}
	select {
	case <-rawExitedDone:
	case <-time.After(time.Second):
		t.Fatal("after-HTTP cleanup did not release the non-cooperative raw handler")
	}
	if !beforeRan.Load() || !afterRan.Load() || !rawExited.Load() {
		t.Fatalf("cleanup incomplete: before=%v after=%v rawExited=%v", beforeRan.Load(), afterRan.Load(), rawExited.Load())
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("forced close left the raw client blocked")
	}
	if _, err := os.Stat(InstancePath(runtimeDirectory)); !os.IsNotExist(err) {
		t.Fatalf("instance state survived failed shutdown: %v", err)
	}
	lock, err := AcquireFileLock(LockPath(runtimeDirectory))
	if err != nil {
		t.Fatalf("instance lock survived failed shutdown: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
}
