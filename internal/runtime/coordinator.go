package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	defaultShutdownTimeout = 10 * time.Second
	defaultHealthTimeout   = 5 * time.Second
)

var (
	ErrServiceFactoryUnavailable = errors.New("runtime: application service factory is not configured")
	ErrAlreadyRunningUnverified  = errors.New("runtime: another instance holds the lock but did not provide a valid identity")
)

// ServiceDependencies are the local values known only after the definitive
// listener has been acquired.
type ServiceDependencies struct {
	DataRoot string
	Port     int
}

// CleanupStage positions a hook relative to graceful HTTP shutdown. Storage
// normally closes after HTTP; hijacked streams and session registries close
// before it so Shutdown cannot wait on them indefinitely.
type CleanupStage uint8

const (
	CleanupAfterHTTP CleanupStage = iota
	CleanupBeforeHTTP
)

// NamedCleanup is returned in resource acquisition order within its stage.
type NamedCleanup struct {
	Name  string
	Stage CleanupStage
	Func  CleanupFunc
}

// Service is the HTTP application composed by the API/storage layer.
type Service struct {
	Handler  http.Handler
	Cleanups []NamedCleanup
}

// ServiceFactory keeps runtime independent from the still-evolving API and
// storage composition.
type ServiceFactory interface {
	Build(context.Context, ServiceDependencies) (Service, error)
}

type ServiceFactoryFunc func(context.Context, ServiceDependencies) (Service, error)

func (factory ServiceFactoryFunc) Build(ctx context.Context, dependencies ServiceDependencies) (Service, error) {
	return factory(ctx, dependencies)
}

// RunOptions contains already-resolved operational settings. A nil Port means
// the exact automatic range; a non-nil Port is attempted once.
type RunOptions struct {
	DataRoot         string
	RuntimeDirectory string
	Port             *int
	ShutdownTimeout  time.Duration
	HealthTimeout    time.Duration
	Factory          ServiceFactory
	OnReady          func(ControlIdentityDTO)
}

// RunResult distinguishes a newly served process from an already-running
// authenticated instance.
type RunResult struct {
	Identity ControlIdentityDTO
	Started  bool
	Existing bool
}

// RunForeground owns the lock, listener, HTTP server, readiness publication
// and LIFO cleanup. It never daemonizes and never installs signal handlers.
func RunForeground(ctx context.Context, options RunOptions) (result RunResult, runErr error) {
	if ctx == nil {
		return RunResult{}, errors.New("runtime: parent context is required")
	}
	if options.Factory == nil {
		return RunResult{}, ErrServiceFactoryUnavailable
	}
	if options.RuntimeDirectory == "" {
		return RunResult{}, errors.New("runtime: runtime directory is required")
	}
	if options.ShutdownTimeout == 0 {
		options.ShutdownTimeout = defaultShutdownTimeout
	}
	if options.ShutdownTimeout < time.Second || options.ShutdownTimeout > 30*time.Second {
		return RunResult{}, errors.New("runtime: shutdown timeout must be between 1s and 30s")
	}
	if options.HealthTimeout == 0 {
		options.HealthTimeout = defaultHealthTimeout
	}
	if options.HealthTimeout <= 0 {
		return RunResult{}, errors.New("runtime: health timeout must be positive")
	}
	if err := EnsureRuntimeDirectory(options.RuntimeDirectory); err != nil {
		return RunResult{}, err
	}

	lock, err := AcquireFileLock(LockPath(options.RuntimeDirectory))
	if errors.Is(err, ErrLocked) {
		identity, active, statusErr := (Controller{}).Status(ctx, options.RuntimeDirectory)
		if statusErr != nil {
			return RunResult{}, errors.Join(ErrAlreadyRunningUnverified, statusErr)
		}
		if !active {
			return RunResult{}, ErrAlreadyRunningUnverified
		}
		if options.OnReady != nil {
			options.OnReady(identity)
		}
		return RunResult{Identity: identity, Existing: true}, nil
	}
	if err != nil {
		return RunResult{}, err
	}

	var cleanups CleanupRegistry
	if err := cleanups.Add("instance lock", func(context.Context) error { return lock.Close() }); err != nil {
		_ = lock.Close()
		return RunResult{}, err
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), options.ShutdownTimeout)
		defer cancel()
		runErr = errors.Join(runErr, cleanups.Run(shutdownContext))
	}()

	// Holding the lock proves there is no live owner. A valid previous state is
	// stale; unsafe/corrupt state remains fail-closed and is not removed.
	previous, stateErr := LoadInstanceState(options.RuntimeDirectory)
	if stateErr == nil {
		if err := RemoveInstanceState(options.RuntimeDirectory); err != nil {
			return RunResult{}, err
		}
		_ = previous
	} else if !errors.Is(stateErr, ErrNotRunning) {
		return RunResult{}, stateErr
	}
	if err := cleanups.Add("instance state", func(context.Context) error {
		return RemoveInstanceState(options.RuntimeDirectory)
	}); err != nil {
		return RunResult{}, err
	}

	listener, port, err := BindLoopback(options.Port)
	if err != nil {
		return RunResult{}, err
	}
	if err := cleanups.Add("loopback listener", func(context.Context) error {
		err := listener.Close()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}); err != nil {
		_ = listener.Close()
		return RunResult{}, err
	}

	fingerprint, err := CurrentProcessFingerprint()
	if err != nil {
		return RunResult{}, err
	}
	state, err := NewInstanceState(os.Getpid(), port, fingerprint)
	if err != nil {
		return RunResult{}, err
	}
	runContext, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	service, err := options.Factory.Build(runContext, ServiceDependencies{DataRoot: options.DataRoot, Port: port})
	if err != nil {
		return RunResult{}, fmt.Errorf("runtime: build application service: %w", err)
	}
	if service.Handler == nil {
		return RunResult{}, errors.New("runtime: application service returned a nil HTTP handler")
	}
	if err := registerServiceCleanups(&cleanups, service.Cleanups, CleanupAfterHTTP); err != nil {
		return RunResult{}, err
	}

	publishedHost := listener.Addr().String()
	handler := combineControlAndApplication(NewControlHandler(state, publishedHost, cancelRun), service.Handler)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       30 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return runContext
		},
	}
	serveDone := make(chan struct{})
	var serveMutex sync.Mutex
	var serveErr error
	go func() {
		err := server.Serve(listener)
		serveMutex.Lock()
		serveErr = err
		serveMutex.Unlock()
		close(serveDone)
	}()
	if err := cleanups.Add("HTTP server", func(shutdownContext context.Context) error {
		return shutdownHTTPServer(shutdownContext, server, serveDone)
	}); err != nil {
		_ = server.Close()
		return RunResult{}, err
	}
	if err := registerServiceCleanups(&cleanups, service.Cleanups, CleanupBeforeHTTP); err != nil {
		_ = server.Close()
		return RunResult{}, err
	}

	if err := waitForHealth(runContext, publishedHost, options.HealthTimeout); err != nil {
		cancelRun()
		return RunResult{}, err
	}
	if err := WriteInstanceStateAtomic(options.RuntimeDirectory, state); err != nil {
		cancelRun()
		return RunResult{}, err
	}
	identity := state.Identity()
	result = RunResult{Identity: identity, Started: true}
	if options.OnReady != nil {
		options.OnReady(identity)
	}

	select {
	case <-runContext.Done():
	case <-serveDone:
		cancelRun()
	}
	serveMutex.Lock()
	serverFailure := serveErr
	serveMutex.Unlock()
	if serverFailure != nil && !errors.Is(serverFailure, http.ErrServerClosed) {
		runErr = errors.Join(runErr, fmt.Errorf("runtime: HTTP server failed: %w", serverFailure))
	}
	return result, runErr
}

func shutdownHTTPServer(ctx context.Context, server *http.Server, serveDone <-chan struct{}) error {
	shutdownErr := server.Shutdown(ctx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	<-serveDone
	return shutdownErr
}

func registerServiceCleanups(registry *CleanupRegistry, cleanups []NamedCleanup, stage CleanupStage) error {
	for index, cleanup := range cleanups {
		if cleanup.Stage != CleanupAfterHTTP && cleanup.Stage != CleanupBeforeHTTP {
			return errors.New("runtime: application cleanup has an invalid stage")
		}
		if cleanup.Stage != stage {
			continue
		}
		name := cleanup.Name
		if name == "" {
			name = "application hook " + strconv.Itoa(index+1)
		}
		if err := registry.Add(name, cleanup.Func); err != nil {
			return err
		}
	}
	return nil
}

func combineControlAndApplication(control, application http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == ControlStatusPath || request.URL.Path == ControlStopPath ||
			len(request.URL.Path) >= len("/_kubepeep/control/") && request.URL.Path[:len("/_kubepeep/control/")] == "/_kubepeep/control/" {
			control.ServeHTTP(writer, request)
			return
		}
		application.ServeHTTP(writer, request)
	})
}

func waitForHealth(parent context.Context, host string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	dialer := &net.Dialer{}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != host {
				return nil, errors.New("health dial target changed")
			}
			return dialer.DialContext(ctx, network, address)
		},
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+host+"/health", nil)
		if err != nil {
			return err
		}
		request.Host = host
		response, requestErr := client.Do(request)
		if requestErr == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, MaxStateBytes))
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("runtime: application did not become healthy: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
