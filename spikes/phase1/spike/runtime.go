package spike

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/fvmoraes/ginger/pkg/app"
)

type CleanupFunc func(context.Context) error
type ListenFunc func(network, address string) (net.Listener, error)

type Runtime struct {
	App             *app.App
	Listener        net.Listener
	Server          *http.Server
	Port            int
	ShutdownTimeout time.Duration

	sessionCleanup []CleanupFunc
	cleanup        []CleanupFunc
	sessionOnce    sync.Once
	once           sync.Once
}

func NewRuntime(application *app.App, firstPort, attempts int, cleanup ...CleanupFunc) (*Runtime, error) {
	listener, port, err := BindLoopback(firstPort, attempts)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		App:      application,
		Listener: listener,
		Server: &http.Server{
			Handler:           HealthMux(application.Router),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      0,
			IdleTimeout:       60 * time.Second,
		},
		Port:            port,
		ShutdownTimeout: 2 * time.Second,
		cleanup:         append([]CleanupFunc(nil), cleanup...),
	}, nil
}

func BindLoopback(firstPort, attempts int) (net.Listener, int, error) {
	return bindLoopback(net.Listen, firstPort, attempts)
}

func bindLoopback(listen ListenFunc, firstPort, attempts int) (net.Listener, int, error) {
	if firstPort < 1 || firstPort > 65535 {
		return nil, 0, fmt.Errorf("invalid first port %d", firstPort)
	}
	if attempts < 1 {
		return nil, 0, fmt.Errorf("attempts must be positive")
	}
	if listen == nil {
		return nil, 0, fmt.Errorf("listen function is required")
	}

	var failures []error
	for offset := 0; offset < attempts && firstPort+offset <= 65535; offset++ {
		port := firstPort + offset
		listener, err := listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return listener, port, nil
		}
		bindErr := fmt.Errorf("bind port %d: %w", port, err)
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, 0, bindErr
		}
		failures = append(failures, bindErr)
	}
	return nil, 0, errors.Join(failures...)
}

func HealthMux(application http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(
			`{"data":{"status":"healthy","components":{"application":{"status":"healthy","code":"OK"},"sqlite":{"status":"healthy","code":"OK"},"kubeconfig":{"status":"unknown","code":"NOT_CHECKED"},"context":{"status":"unknown","code":"NOT_SELECTED"},"cluster":{"status":"unknown","code":"NOT_CHECKED"}}}}`,
		))
	})
	mux.Handle("/", application)
	return mux
}

func (r *Runtime) URL() string {
	return "http://" + r.Listener.Addr().String()
}

func (r *Runtime) WaitReady(ctx context.Context) error {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL()+"/health", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runtime) Run(ctx context.Context) error {
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- r.Server.Serve(r.Listener)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		runErr = errors.Join(runErr, r.runSessionCleanup())
		shutdownContext, cancel := context.WithTimeout(context.Background(), r.ShutdownTimeout)
		shutdownErr := r.Server.Shutdown(shutdownContext)
		cancel()
		if shutdownErr != nil {
			runErr = errors.Join(runErr, shutdownErr, r.Server.Close())
		}
	case err := <-serverErr:
		runErr = errors.Join(runErr, r.runSessionCleanup())
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = errors.Join(runErr, err, r.Server.Close())
		}
	}

	return errors.Join(runErr, r.runSessionCleanup(), r.runCleanup())
}

func (r *Runtime) AddSessionCleanup(cleanup ...CleanupFunc) {
	r.sessionCleanup = append(r.sessionCleanup, cleanup...)
}

func (r *Runtime) RunWithReady(
	ctx context.Context,
	readyTimeout time.Duration,
	onReady func(context.Context, string) error,
) error {
	runContext, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	result := make(chan error, 1)
	go func() {
		result <- r.Run(runContext)
	}()

	readyContext, cancelReady := context.WithTimeout(ctx, readyTimeout)
	readyErr := r.WaitReady(readyContext)
	cancelReady()
	if readyErr != nil {
		cancelRun()
		return errors.Join(readyErr, <-result)
	}
	if err := ctx.Err(); err != nil {
		cancelRun()
		return errors.Join(err, <-result)
	}
	if onReady != nil {
		if err := onReady(ctx, r.URL()); err != nil {
			cancelRun()
			return errors.Join(err, <-result)
		}
	}

	return <-result
}

func (r *Runtime) runCleanup() error {
	var cleanupErr error
	r.once.Do(func() {
		context, cancel := context.WithTimeout(context.Background(), r.ShutdownTimeout)
		defer cancel()

		for index := len(r.cleanup) - 1; index >= 0; index-- {
			cleanupErr = errors.Join(cleanupErr, r.cleanup[index](context))
		}
	})
	return cleanupErr
}

func (r *Runtime) runSessionCleanup() error {
	var cleanupErr error
	r.sessionOnce.Do(func() {
		cleanupContext, cancel := context.WithTimeout(
			context.Background(),
			r.ShutdownTimeout,
		)
		defer cancel()

		for index := len(r.sessionCleanup) - 1; index >= 0; index-- {
			cleanupErr = errors.Join(
				cleanupErr,
				r.sessionCleanup[index](cleanupContext),
			)
		}
	})
	return cleanupErr
}
