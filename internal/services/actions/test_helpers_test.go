package actions

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type generationStub struct {
	mu         sync.Mutex
	generation string
}

func (s *generationStub) CurrentGeneration() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}

func (s *generationStub) set(value string) {
	s.mu.Lock()
	s.generation = value
	s.mu.Unlock()
}

type authorizationCall struct {
	key  authorization.Key
	kind authorization.OperationKind
}

type authorizerStub struct {
	mu    sync.Mutex
	calls []authorizationCall
	err   error
}

func (s *authorizerStub) Revalidate(ctx context.Context, key authorization.Key, kind authorization.OperationKind) (authorization.Capability, error) {
	s.mu.Lock()
	s.calls = append(s.calls, authorizationCall{key: key, kind: kind})
	err := s.err
	s.mu.Unlock()
	if err != nil {
		return authorization.Capability{}, err
	}
	if err := ctx.Err(); err != nil {
		return authorization.Capability{}, err
	}
	return authorization.Capability{Decision: authorization.DecisionAllowed}, nil
}

func (s *authorizerStub) Guard(ctx context.Context, key authorization.Key, kind authorization.OperationKind, operation authorization.Operation) (authorization.GuardResult, error) {
	capability, err := s.Revalidate(ctx, key, kind)
	result := authorization.GuardResult{Capability: capability}
	if err != nil {
		return result, err
	}
	result.Executed = true
	return result, operation(ctx)
}

func (s *authorizerStub) snapshot() []authorizationCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]authorizationCall(nil), s.calls...)
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

type identifierStub struct {
	next atomic.Int64
}

func (s *identifierStub) NewID(prefix string) (string, error) {
	value := s.next.Add(1)
	return prefix + "aaaaaaaa" + string(rune('a'+value%20)), nil
}

func (s *identifierStub) NewToken() (string, error) {
	value := s.next.Add(1)
	return "ticket_token_aaaaaaaa_" + string(rune('a'+value%20)), nil
}

type auditStub struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (s *auditStub) Record(_ context.Context, event AuditEvent) {
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
}

func (s *auditStub) snapshot() []AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AuditEvent(nil), s.events...)
}

func testBinding(generation string) namespaces.SelectionBinding {
	return namespaces.SelectionBinding{ClusterProfileID: 7, Context: "development", Generation: generation}
}

func testTarget(kind, namespace, name string) ActionTargetDTO {
	return ActionTargetDTO{ClusterProfileID: 7, Context: "development", Namespace: namespace, Kind: kind, Name: name}
}

func testRestart(generation, namespace, name string) RestartRequest {
	return RestartRequest{
		Confirmation: Confirmation{
			Confirmed:          true,
			Action:             ActionRestart,
			ConsequenceCode:    ConsequenceRecreateWorkloadPods,
			Target:             testTarget("Deployment", namespace, name),
			ExpectedGeneration: generation,
		},
		ExpectedResourceVersion: "123",
	}
}

func testScale(generation, routeKind, namespace, name string, replicas int64) ScaleRequest {
	kind := "Deployment"
	if routeKind == "statefulsets" {
		kind = "StatefulSet"
	}
	return ScaleRequest{
		Replicas: replicas,
		Confirmation: Confirmation{
			Confirmed:          true,
			Action:             ActionScale,
			ConsequenceCode:    ConsequenceChangeReplicaCount,
			Target:             testTarget(kind, namespace, name),
			ExpectedGeneration: generation,
		},
		ExpectedResourceVersion: "124",
	}
}

func testDelete(generation, namespace, name string) PodDeleteRequest {
	return PodDeleteRequest{
		Confirmation: Confirmation{
			Confirmed:          true,
			Action:             ActionDeletePod,
			ConsequenceCode:    ConsequenceDeletePod,
			Target:             testTarget("Pod", namespace, name),
			ExpectedGeneration: generation,
		},
		ExpectedUID:             "uid-1",
		ExpectedResourceVersion: "125",
	}
}

func testPortForward(generation, namespace, name string, remotePort int) PortForwardCreateRequest {
	return PortForwardCreateRequest{
		RemotePort: remotePort,
		Confirmation: Confirmation{
			Confirmed:          true,
			Action:             ActionPortForward,
			ConsequenceCode:    ConsequenceExposePodPortLocally,
			Target:             testTarget("Pod", namespace, name),
			ExpectedGeneration: generation,
		},
	}
}

func testExec(generation, namespace, name string, command []string) ExecInit {
	return ExecInit{
		Container: "api",
		Command:   command,
		TTY:       true,
		Stdin:     true,
		Confirmation: Confirmation{
			Confirmed:          true,
			Action:             ActionExec,
			ConsequenceCode:    ConsequenceOpenInteractiveProcess,
			Target:             testTarget("Pod", namespace, name),
			ExpectedGeneration: generation,
		},
	}
}

type actionAdapterStub struct {
	mu         sync.Mutex
	restarts   []RestartDeploymentCommand
	scales     []ScaleCommand
	deletes    []DeletePodCommand
	restartErr error
	scaleErr   error
	deleteErr  error
	started    chan struct{}
	release    chan struct{}
}

func (s *actionAdapterStub) RestartDeployment(ctx context.Context, command RestartDeploymentCommand) (MutationResult, error) {
	s.mu.Lock()
	s.restarts = append(s.restarts, command)
	started, release, resultErr := s.started, s.release, s.restartErr
	s.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return MutationResult{}, ctx.Err()
		}
	}
	if resultErr != nil {
		return MutationResult{}, resultErr
	}
	return MutationResult{ResourceVersion: "200"}, nil
}

func (s *actionAdapterStub) UpdateScale(_ context.Context, command ScaleCommand) (MutationResult, error) {
	s.mu.Lock()
	s.scales = append(s.scales, command)
	err := s.scaleErr
	s.mu.Unlock()
	return MutationResult{ResourceVersion: "201"}, err
}

func (s *actionAdapterStub) DeletePod(_ context.Context, command DeletePodCommand) (MutationResult, error) {
	s.mu.Lock()
	s.deletes = append(s.deletes, command)
	err := s.deleteErr
	s.mu.Unlock()
	return MutationResult{}, err
}

type portForwardHandleStub struct {
	done chan error
	once sync.Once
}

func newPortForwardHandleStub() *portForwardHandleStub {
	return &portForwardHandleStub{done: make(chan error, 1)}
}

func (h *portForwardHandleStub) Wait() error { return <-h.done }

func (h *portForwardHandleStub) Close() error {
	h.once.Do(func() { h.done <- context.Canceled })
	return nil
}

func (h *portForwardHandleStub) finish(err error) { h.once.Do(func() { h.done <- err }) }

type portForwardAdapterStub struct {
	mu        sync.Mutex
	commands  []PortForwardCommand
	listeners []net.Listener
	handles   []*portForwardHandleStub
	started   chan struct{}
	release   chan struct{}
	err       error
}

func (s *portForwardAdapterStub) Start(setup context.Context, _ context.Context, command PortForwardCommand, listener net.Listener) (PortForwardHandle, error) {
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-setup.Done():
			return nil, setup.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	handle := newPortForwardHandleStub()
	s.mu.Lock()
	s.commands = append(s.commands, command)
	s.listeners = append(s.listeners, listener)
	s.handles = append(s.handles, handle)
	s.mu.Unlock()
	return handle, nil
}

type execInspectorStub struct {
	mu      sync.Mutex
	calls   int
	state   ExecTargetState
	err     error
	targets []MutationTarget
}

func (s *execInspectorStub) InspectExecTarget(_ context.Context, target MutationTarget, _ string) (ExecTargetState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.targets = append(s.targets, target)
	return s.state, s.err
}

type remoteExecStub struct {
	stdin  bytes.Buffer
	stdout *bytes.Reader
	stderr *bytes.Reader
	done   chan RemoteExecExit
	once   sync.Once
}

func newRemoteExecStub() *remoteExecStub {
	return &remoteExecStub{stdout: bytes.NewReader(nil), stderr: bytes.NewReader(nil), done: make(chan RemoteExecExit, 1)}
}

func (r *remoteExecStub) Stdin() io.WriteCloser                        { return nopWriteCloser{Writer: &r.stdin} }
func (r *remoteExecStub) Stdout() io.Reader                            { return r.stdout }
func (r *remoteExecStub) Stderr() io.Reader                            { return r.stderr }
func (r *remoteExecStub) Resize(context.Context, uint16, uint16) error { return nil }
func (r *remoteExecStub) Wait() RemoteExecExit                         { return <-r.done }
func (r *remoteExecStub) Close() error {
	r.once.Do(func() { r.done <- RemoteExecExit{Err: context.Canceled} })
	return nil
}
func (r *remoteExecStub) finish(exit RemoteExecExit) { r.once.Do(func() { r.done <- exit }) }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type execAdapterStub struct {
	mu       sync.Mutex
	commands []ExecCommand
	remotes  []*remoteExecStub
	err      error
}

func (s *execAdapterStub) Start(_ context.Context, _ context.Context, command ExecCommand) (RemoteExec, error) {
	if s.err != nil {
		return nil, s.err
	}
	remote := newRemoteExecStub()
	s.mu.Lock()
	s.commands = append(s.commands, command)
	s.remotes = append(s.remotes, remote)
	s.mu.Unlock()
	return remote, nil
}

func requireCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if ErrorCodeOf(err) != code {
		t.Fatalf("expected error code %s, got %s (%v)", code, ErrorCodeOf(err), err)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not reached")
}

var _ = errors.Is
