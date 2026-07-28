package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	statusPath         = "/_kubepeep/control/v1/status"
	stopPath           = "/_kubepeep/control/v1/stop"
	ControlTokenHeader = "X-KubePeep-Control-Token"
)

// ErrAlreadyRunning reports that the runtime lock is owned by another process.
var ErrAlreadyRunning = errors.New("control: another instance is already running")

// RunOptions configures the isolated foreground control probe.
type RunOptions struct {
	RuntimeDir      string
	FirstPort       int
	PortAttempts    int
	ShutdownTimeout time.Duration
	OnReady         func(PublicInstance)
}

// RunForeground acquires the single-instance lock, publishes authenticated
// state, and serves until ctx is canceled or the server fails.
func RunForeground(ctx context.Context, options RunOptions) (runErr error) {
	if ctx == nil {
		return errors.New("control: parent context is required")
	}
	if err := normalizeRunOptions(&options); err != nil {
		return err
	}
	if err := EnsureRuntimeDir(options.RuntimeDir); err != nil {
		return err
	}

	lock, err := AcquireFileLock(LockPath(options.RuntimeDir))
	if err != nil {
		if errors.Is(err, ErrLocked) {
			return ErrAlreadyRunning
		}
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, lock.Close())
	}()

	// Holding the lock proves that any previous state is stale. PID is never
	// inspected or signaled during this recovery.
	if err := RemoveInstance(options.RuntimeDir); err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, RemoveInstance(options.RuntimeDir))
	}()

	listener, port, err := bindLoopback(options.FirstPort, options.PortAttempts)
	if err != nil {
		return err
	}
	defer listener.Close()

	fingerprint, err := CurrentProcessFingerprint()
	if err != nil {
		return err
	}
	instanceID, err := randomSecret()
	if err != nil {
		return fmt.Errorf("control: generate instance ID: %w", err)
	}
	token, err := randomSecret()
	if err != nil {
		return fmt.Errorf("control: generate control token: %w", err)
	}
	instance := Instance{
		Schema:      SchemaVersion,
		InstanceID:  instanceID,
		Token:       token,
		PID:         os.Getpid(),
		Fingerprint: fingerprint,
		Port:        port,
		Protocol:    ProtocolVersion,
	}

	runContext, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	handler := newControlHandler(instance, listener.Addr().String(), cancelRun)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       15 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return runContext
		},
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	if err := WriteInstanceAtomic(options.RuntimeDir, instance); err != nil {
		cancelRun()
		_ = server.Close()
		<-serveResult
		return err
	}
	if options.OnReady != nil {
		options.OnReady(instance.Public())
	}

	select {
	case <-runContext.Done():
		shutdownContext, cancelShutdown := context.WithTimeout(
			context.Background(),
			options.ShutdownTimeout,
		)
		shutdownErr := server.Shutdown(shutdownContext)
		cancelShutdown()
		if shutdownErr != nil {
			runErr = errors.Join(shutdownErr, server.Close())
		}
		serveErr := <-serveResult
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			runErr = errors.Join(runErr, serveErr)
		}
	case serveErr := <-serveResult:
		cancelRun()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			runErr = serveErr
		}
	}
	return runErr
}

func normalizeRunOptions(options *RunOptions) error {
	if strings.TrimSpace(options.RuntimeDir) == "" {
		return errors.New("control: runtime directory is required")
	}
	switch {
	case options.FirstPort < 0 || options.FirstPort > 65535:
		return fmt.Errorf("control: invalid first port %d", options.FirstPort)
	case options.FirstPort == 0:
		options.PortAttempts = 1
	case options.PortAttempts <= 0:
		options.PortAttempts = 50
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = 2 * time.Second
	}
	return nil
}

func bindLoopback(firstPort, attempts int) (net.Listener, int, error) {
	if firstPort == 0 {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, 0, fmt.Errorf("control: bind ephemeral loopback port: %w", err)
		}
		return listener, listener.Addr().(*net.TCPAddr).Port, nil
	}

	var failures []error
	for offset := 0; offset < attempts && firstPort+offset <= 65535; offset++ {
		port := firstPort + offset
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return listener, port, nil
		}
		failures = append(failures, fmt.Errorf("port %d: %w", port, err))
	}
	return nil, 0, fmt.Errorf("control: bind loopback: %w", errors.Join(failures...))
}

func randomSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

type controlHandler struct {
	instance     Instance
	expectedHost string
	tokenHash    [sha256.Size]byte
	cancel       context.CancelFunc
	stopOnce     sync.Once
}

func newControlHandler(instance Instance, expectedHost string, cancel context.CancelFunc) http.Handler {
	handler := &controlHandler{
		instance:     instance,
		expectedHost: expectedHost,
		tokenHash:    sha256.Sum256([]byte(instance.Token)),
		cancel:       cancel,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+statusPath, handler.status)
	mux.HandleFunc("POST "+stopPath, handler.stop)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !handler.isLocalRequest(r) || r.Host != handler.expectedHost || r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		providedHash := sha256.Sum256([]byte(r.Header.Get(ControlTokenHeader)))
		if subtle.ConstantTimeCompare(providedHash[:], handler.tokenHash[:]) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (h *controlHandler) isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *controlHandler) status(w http.ResponseWriter, _ *http.Request) {
	writeControlJSON(w, h.instance.Public())
}

func (h *controlHandler) stop(w http.ResponseWriter, _ *http.Request) {
	writeControlJSON(w, h.instance.Public())
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	h.stopOnce.Do(h.cancel)
}

func writeControlJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(value)
}
