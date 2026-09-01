package actions

import (
	"bytes"
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPortForwardTerminalPathsReleaseLoopbackPortAndAdapterGoroutine(t *testing.T) {
	for _, test := range []struct {
		name       string
		duration   time.Duration
		expected   PortForwardStatus
		generation string
		terminate  func(*testing.T, *PortForwardManager, *generationStub, *cleanupPortForwardHandle, PortForwardDTO)
	}{
		{
			name: "user close", duration: time.Hour, expected: PortForwardClosed, generation: "gen_1",
			terminate: func(t *testing.T, manager *PortForwardManager, _ *generationStub, _ *cleanupPortForwardHandle, dto PortForwardDTO) {
				if err := manager.Close(context.Background(), testBinding("gen_1"), dto.ID, PortForwardDeleteRequest{Confirmed: true, ExpectedGeneration: "gen_1"}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "absolute expiry", duration: 50 * time.Millisecond, expected: PortForwardExpired, generation: "gen_1",
			terminate: func(*testing.T, *PortForwardManager, *generationStub, *cleanupPortForwardHandle, PortForwardDTO) {},
		},
		{
			name: "upstream pod gone", duration: time.Hour, expected: PortForwardPodGone, generation: "gen_1",
			terminate: func(_ *testing.T, _ *PortForwardManager, _ *generationStub, handle *cleanupPortForwardHandle, _ PortForwardDTO) {
				handle.complete(ErrPortForwardPodGone)
			},
		},
		{
			name: "generation change", duration: time.Hour, generation: "gen_2",
			terminate: func(_ *testing.T, manager *PortForwardManager, generations *generationStub, _ *cleanupPortForwardHandle, _ PortForwardDTO) {
				generations.set("gen_2")
				manager.OnGeneration("gen_2")
			},
		},
		{
			name: "shutdown", duration: time.Hour, generation: "gen_1",
			terminate: func(_ *testing.T, manager *PortForwardManager, _ *generationStub, _ *cleanupPortForwardHandle, _ PortForwardDTO) {
				manager.Shutdown()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			generations := &generationStub{generation: "gen_1"}
			adapter := newCleanupPortForwardAdapter()
			manager, err := newPortForwardService(context.Background(),
				&authorizerStub{}, generations, adapter, netLoopbackBinder{}, NoopAuditSink{},
				systemClock{}, &identifierStub{}, test.duration, time.Minute, time.Second, time.Minute,
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(manager.Shutdown)
			binding := testBinding("gen_1")
			dto, replayed, err := manager.Create(
				context.Background(),
				binding,
				RouteTarget{Kind: "pods", Namespace: "payments", Name: "cleanup-pod"},
				"cleanup-port-forward-key",
				testPortForward("gen_1", "payments", "cleanup-pod", 8080),
			)
			if err != nil || replayed {
				t.Fatalf("create dto=%#v replayed=%v err=%v", dto, replayed, err)
			}
			handle := <-adapter.started
			client, err := net.DialTimeout("tcp", net.JoinHostPort(dto.LocalAddress, strconv.Itoa(dto.LocalPort)), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			select {
			case <-handle.accepted:
			case <-time.After(time.Second):
				t.Fatal("adapter did not accept the loopback connection")
			}

			test.terminate(t, manager, generations, handle, dto)
			select {
			case <-handle.exited:
			case <-time.After(2 * time.Second):
				t.Fatal("port-forward adapter goroutine survived termination")
			}
			if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := client.Read(make([]byte, 1)); err == nil {
				t.Fatal("accepted loopback connection survived termination")
			}
			var probe net.Listener
			deadline := time.Now().Add(2 * time.Second)
			for {
				probe, err = net.Listen("tcp", net.JoinHostPort(dto.LocalAddress, strconv.Itoa(dto.LocalPort)))
				if err == nil {
					_ = probe.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("loopback port remained allocated: %v", err)
				}
				time.Sleep(10 * time.Millisecond)
			}

			rows, err := manager.List(testBinding(test.generation))
			if err != nil {
				t.Fatal(err)
			}
			if test.expected == "" {
				if len(rows) != 0 {
					t.Fatalf("terminal port-forward session was retained: %#v", rows)
				}
			} else {
				if len(rows) != 1 || rows[0].Status != test.expected || rows[0].EndedAt == nil {
					t.Fatalf("terminal port-forward session rows=%#v", rows)
				}
			}
			if test.name == "shutdown" {
				_, _, err := manager.Create(
					context.Background(), binding,
					RouteTarget{Kind: "pods", Namespace: "payments", Name: "after-shutdown"},
					"after-shutdown-port-forward-key",
					testPortForward("gen_1", "payments", "after-shutdown", 8080),
				)
				requireCode(t, err, CodeServerShutdown)
			}
		})
	}
}

func TestExecTerminalPathsReleaseWaitGoroutineAndSession(t *testing.T) {
	for _, test := range []struct {
		name         string
		terminalType ExecTerminalType
		terminalCode ErrorCode
		exitReason   ExecExitReason
		closeCode    int
		terminate    func(*testing.T, *ExecManager, *generationStub, *cleanupRemoteExec, string)
	}{
		{
			name: "remote process exit", terminalType: ExecTerminalExit, exitReason: ExecExitCompleted, closeCode: 1000,
			terminate: func(_ *testing.T, _ *ExecManager, _ *generationStub, remote *cleanupRemoteExec, _ string) {
				zero := 0
				remote.finish(RemoteExecExit{ExitCode: &zero})
			},
		},
		{
			name: "user cancel", terminalType: ExecTerminalExit, exitReason: ExecExitCanceled, closeCode: 1000,
			terminate: func(t *testing.T, manager *ExecManager, _ *generationStub, _ *cleanupRemoteExec, sessionID string) {
				if err := manager.Cancel(sessionID); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "generation change", terminalType: ExecTerminalError, terminalCode: CodeGenerationChanged, closeCode: 1001,
			terminate: func(_ *testing.T, manager *ExecManager, generations *generationStub, _ *cleanupRemoteExec, _ string) {
				generations.set("gen_2")
				manager.OnGeneration("gen_2")
			},
		},
		{
			name: "shutdown", terminalType: ExecTerminalError, terminalCode: CodeServerShutdown, closeCode: 1001,
			terminate: func(_ *testing.T, manager *ExecManager, _ *generationStub, _ *cleanupRemoteExec, _ string) {
				manager.Shutdown()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			generations := &generationStub{generation: "gen_1"}
			adapter := newCleanupExecAdapter()
			manager, err := NewExecService(context.Background(),
				&authorizerStub{}, generations,
				&execInspectorStub{state: ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}},
				adapter, NoopAuditSink{},
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(manager.Shutdown)
			binding := testBinding("gen_1")
			route := RouteTarget{Kind: "pods", Namespace: "payments", Name: "cleanup-pod"}
			ticket, err := manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", "cleanup-pod", []string{"/bin/cleanup"}))
			if err != nil {
				t.Fatal(err)
			}
			grant, err := manager.AuthorizeUpgrade(context.Background(), binding, ticket.SessionID, ticket.Protocols)
			if err != nil {
				t.Fatal(err)
			}
			active, err := manager.Start(context.Background(), grant)
			if err != nil {
				t.Fatal(err)
			}
			remote := <-adapter.started

			test.terminate(t, manager, generations, remote, active.SessionID)
			select {
			case terminal := <-active.Terminal:
				if terminal.Type != test.terminalType || terminal.Code != test.terminalCode || terminal.Reason != test.exitReason || terminal.CloseCode != test.closeCode {
					t.Fatalf("terminal = %#v", terminal)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("exec terminal was not delivered")
			}
			select {
			case <-remote.waitExited:
			case <-time.After(2 * time.Second):
				t.Fatal("remote Wait goroutine survived termination")
			}
			requireCode(t, manager.Touch(active.SessionID), CodeSessionGone)

			if test.name == "shutdown" {
				_, err := manager.CreateTicket(context.Background(), binding, route, testExec("gen_1", "payments", "cleanup-pod", []string{"/bin/after-shutdown"}))
				requireCode(t, err, CodeServerShutdown)
			}
		})
	}
}

type cleanupPortForwardAdapter struct {
	started chan *cleanupPortForwardHandle
}

func newCleanupPortForwardAdapter() *cleanupPortForwardAdapter {
	return &cleanupPortForwardAdapter{started: make(chan *cleanupPortForwardHandle, 1)}
}

func (adapter *cleanupPortForwardAdapter) Start(_ context.Context, lifetime context.Context, _ PortForwardCommand, listener net.Listener) (PortForwardHandle, error) {
	handle := &cleanupPortForwardHandle{
		lifetime: lifetime,
		listener: listener,
		accepted: make(chan struct{}),
		done:     make(chan error, 1),
		exited:   make(chan struct{}),
	}
	go handle.run()
	adapter.started <- handle
	return handle, nil
}

type cleanupPortForwardHandle struct {
	lifetime context.Context
	listener net.Listener
	accepted chan struct{}
	done     chan error
	exited   chan struct{}
	once     sync.Once
	mu       sync.Mutex
	conn     net.Conn
}

func (handle *cleanupPortForwardHandle) run() {
	defer close(handle.exited)
	connection, err := handle.listener.Accept()
	if err != nil {
		handle.complete(err)
		return
	}
	handle.mu.Lock()
	handle.conn = connection
	handle.mu.Unlock()
	close(handle.accepted)
	<-handle.lifetime.Done()
	_ = connection.Close()
	handle.complete(handle.lifetime.Err())
}

func (handle *cleanupPortForwardHandle) Wait() error { return <-handle.done }

func (handle *cleanupPortForwardHandle) Close() error {
	_ = handle.listener.Close()
	handle.mu.Lock()
	connection := handle.conn
	handle.mu.Unlock()
	if connection != nil {
		_ = connection.Close()
	}
	handle.complete(context.Canceled)
	return nil
}

func (handle *cleanupPortForwardHandle) complete(err error) {
	handle.once.Do(func() { handle.done <- err })
}

type cleanupExecAdapter struct{ started chan *cleanupRemoteExec }

func newCleanupExecAdapter() *cleanupExecAdapter {
	return &cleanupExecAdapter{started: make(chan *cleanupRemoteExec, 1)}
}

func (adapter *cleanupExecAdapter) Start(_ context.Context, lifetime context.Context, _ ExecCommand) (RemoteExec, error) {
	remote := &cleanupRemoteExec{
		lifetime:   lifetime,
		closed:     make(chan struct{}),
		results:    make(chan RemoteExecExit, 1),
		waitExited: make(chan struct{}),
		stdout:     strings.NewReader(""),
		stderr:     strings.NewReader(""),
	}
	adapter.started <- remote
	return remote, nil
}

type cleanupRemoteExec struct {
	lifetime   context.Context
	closed     chan struct{}
	results    chan RemoteExecExit
	waitExited chan struct{}
	closeOnce  sync.Once
	stdin      bytes.Buffer
	stdout     io.Reader
	stderr     io.Reader
}

func (remote *cleanupRemoteExec) Stdin() io.WriteCloser { return nopWriteCloser{Writer: &remote.stdin} }
func (remote *cleanupRemoteExec) Stdout() io.Reader     { return remote.stdout }
func (remote *cleanupRemoteExec) Stderr() io.Reader     { return remote.stderr }
func (*cleanupRemoteExec) Resize(context.Context, uint16, uint16) error {
	return nil
}
func (remote *cleanupRemoteExec) Wait() RemoteExecExit {
	defer close(remote.waitExited)
	select {
	case result := <-remote.results:
		return result
	case <-remote.closed:
	case <-remote.lifetime.Done():
	}
	return RemoteExecExit{Err: context.Canceled}
}
func (remote *cleanupRemoteExec) Close() error {
	remote.closeOnce.Do(func() { close(remote.closed) })
	return nil
}
func (remote *cleanupRemoteExec) finish(result RemoteExecExit) {
	remote.results <- result
}

var _ PortForwardAdapter = (*cleanupPortForwardAdapter)(nil)
var _ PortForwardHandle = (*cleanupPortForwardHandle)(nil)
var _ ExecAdapter = (*cleanupExecAdapter)(nil)
var _ RemoteExec = (*cleanupRemoteExec)(nil)
