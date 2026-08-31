package kubernetesruntime

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/adapters/sqlite"
	"github.com/fvmoraes/kubepeep/internal/logging"
	"github.com/fvmoraes/kubepeep/internal/securefs"
	"github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func TestExecSecretsNeverReachAuditLogOrSQLiteArtifacts(t *testing.T) {
	const (
		commandNeedle = "COMMAND_MUST_NOT_PERSIST_7f2ab2b99e2d"
		stdinNeedle   = "STDIN_MUST_NOT_PERSIST_b6fb38eaa4c1"
		stdoutNeedle  = "STDOUT_MUST_NOT_PERSIST_323a02d54d81"
		stderrNeedle  = "STDERR_MUST_NOT_PERSIST_83c78ab63d0e"
	)
	directory := t.TempDir()
	if err := securefs.EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(directory, "kubePeep.db")
	logPath := filepath.Join(directory, "kubePeep.log")
	store, err := sqlite.Open(context.Background(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var stdout nonPersistenceBuffer
	logger, sink, err := logging.New(logPath, &stdout, logging.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	remote := newNonPersistenceRemote(stdoutNeedle, stderrNeedle)
	manager, err := actions.NewExecService(
		context.Background(),
		nonPersistenceAuthorizer{},
		nonPersistenceGeneration("gen_evidence"),
		nonPersistenceInspector{},
		&nonPersistenceExecAdapter{remote: remote},
		NewActionAuditSink(logger),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Shutdown)
	binding := namespaces.SelectionBinding{ClusterProfileID: 41, Context: "evidence", Generation: "gen_evidence"}
	route := actions.RouteTarget{Kind: "pods", Namespace: "payments", Name: "audit-evidence"}
	request := actions.ExecInit{
		Container: "api",
		Command:   []string{"/bin/evidence", "--opaque=" + commandNeedle},
		TTY:       false,
		Stdin:     true,
		Confirmation: actions.Confirmation{
			Confirmed:       true,
			Action:          actions.ActionExec,
			ConsequenceCode: actions.ConsequenceOpenInteractiveProcess,
			Target: actions.ActionTargetDTO{
				ClusterProfileID: 41,
				Context:          "evidence",
				Namespace:        "payments",
				Kind:             "Pod",
				Name:             "audit-evidence",
			},
			ExpectedGeneration: "gen_evidence",
		},
	}
	ticket, err := manager.CreateTicket(context.Background(), binding, route, request)
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
	if _, err := io.WriteString(active.Remote.Stdin(), stdinNeedle); err != nil {
		t.Fatal(err)
	}
	stdoutPayload, err := io.ReadAll(active.Remote.Stdout())
	if err != nil || string(stdoutPayload) != stdoutNeedle {
		t.Fatalf("stdout stream was not exercised: bytes=%d err=%v", len(stdoutPayload), err)
	}
	stderrPayload, err := io.ReadAll(active.Remote.Stderr())
	if err != nil || string(stderrPayload) != stderrNeedle {
		t.Fatalf("stderr stream was not exercised: bytes=%d err=%v", len(stderrPayload), err)
	}
	if remote.stdin.String() != stdinNeedle {
		t.Fatal("stdin stream was not exercised")
	}
	zero := 0
	remote.finish(actions.RemoteExecExit{ExitCode: &zero})
	select {
	case terminal := <-active.Terminal:
		if terminal.Type != actions.ExecTerminalExit || terminal.CloseCode != 1000 {
			t.Fatalf("terminal = %#v", terminal)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("exec did not finish")
	}
	waitForAuditOperation(t, &stdout, "exec_end")
	manager.Shutdown()
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	for _, operation := range []string{"exec_ticket_create", "exec_upgrade_authorize", "exec_start", "exec_end"} {
		if !strings.Contains(stdout.String(), operation) {
			t.Fatalf("audit evidence is missing operation %q", operation)
		}
	}
	needles := map[string]string{
		"command":         commandNeedle,
		"stdin":           stdinNeedle,
		"stdout":          stdoutNeedle,
		"stderr":          stderrNeedle,
		"ticket protocol": ticket.Protocols[1],
		"ticket token":    strings.TrimPrefix(ticket.Protocols[1], actions.ExecTicketPrefix),
	}
	assertNoExecNeedles(t, "sanitized stdout", stdout.Bytes(), needles)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	inspected := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		inspected++
		assertNoExecNeedles(t, entry.Name(), contents, needles)
	}
	if inspected < 2 {
		t.Fatalf("inspected %d artifacts, want at least SQLite and JSONL", inspected)
	}
}

type nonPersistenceGeneration string

func (generation nonPersistenceGeneration) CurrentGeneration() string { return string(generation) }

type nonPersistenceAuthorizer struct{}

func (nonPersistenceAuthorizer) Revalidate(ctx context.Context, _ authorization.Key, _ authorization.OperationKind) (authorization.Capability, error) {
	if err := ctx.Err(); err != nil {
		return authorization.Capability{}, err
	}
	return authorization.Capability{Decision: authorization.DecisionAllowed}, nil
}

func (authorizer nonPersistenceAuthorizer) Guard(ctx context.Context, key authorization.Key, kind authorization.OperationKind, operation authorization.Operation) (authorization.GuardResult, error) {
	capability, err := authorizer.Revalidate(ctx, key, kind)
	result := authorization.GuardResult{Capability: capability}
	if err != nil {
		return result, err
	}
	result.Executed = true
	return result, operation(ctx)
}

type nonPersistenceInspector struct{}

func (nonPersistenceInspector) InspectExecTarget(context.Context, actions.MutationTarget, string) (actions.ExecTargetState, error) {
	return actions.ExecTargetState{PodExists: true, ContainerExists: true, ContainerRunning: true}, nil
}

type nonPersistenceExecAdapter struct{ remote *nonPersistenceRemote }

func (adapter *nonPersistenceExecAdapter) Start(context.Context, context.Context, actions.ExecCommand) (actions.RemoteExec, error) {
	return adapter.remote, nil
}

type nonPersistenceRemote struct {
	stdin     bytes.Buffer
	stdout    *strings.Reader
	stderr    *strings.Reader
	done      chan actions.RemoteExecExit
	closeOnce sync.Once
}

func newNonPersistenceRemote(stdout, stderr string) *nonPersistenceRemote {
	return &nonPersistenceRemote{
		stdout: strings.NewReader(stdout),
		stderr: strings.NewReader(stderr),
		done:   make(chan actions.RemoteExecExit, 1),
	}
}

func (remote *nonPersistenceRemote) Stdin() io.WriteCloser {
	return nonPersistenceWriteCloser{Writer: &remote.stdin}
}
func (remote *nonPersistenceRemote) Stdout() io.Reader { return remote.stdout }
func (remote *nonPersistenceRemote) Stderr() io.Reader { return remote.stderr }
func (*nonPersistenceRemote) Resize(context.Context, uint16, uint16) error {
	return nil
}
func (remote *nonPersistenceRemote) Wait() actions.RemoteExecExit { return <-remote.done }
func (remote *nonPersistenceRemote) Close() error {
	remote.closeOnce.Do(func() { remote.done <- actions.RemoteExecExit{Err: context.Canceled} })
	return nil
}
func (remote *nonPersistenceRemote) finish(exit actions.RemoteExecExit) {
	remote.closeOnce.Do(func() { remote.done <- exit })
}

type nonPersistenceWriteCloser struct{ io.Writer }

func (nonPersistenceWriteCloser) Close() error { return nil }

type nonPersistenceBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (buffer *nonPersistenceBuffer) Write(payload []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.Write(payload)
}

func (buffer *nonPersistenceBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.String()
}

func (buffer *nonPersistenceBuffer) Bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return append([]byte(nil), buffer.Buffer.Bytes()...)
}

func waitForAuditOperation(t *testing.T, stdout *nonPersistenceBuffer, operation string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stdout.String(), operation) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("audit operation %q was not emitted", operation)
}

func assertNoExecNeedles(t *testing.T, artifact string, contents []byte, needles map[string]string) {
	t.Helper()
	for label, needle := range needles {
		if needle != "" && bytes.Contains(contents, []byte(needle)) {
			t.Fatalf("%s persisted %s bytes", artifact, label)
		}
	}
}

var _ actions.AuthorizationService = nonPersistenceAuthorizer{}
var _ actions.GenerationReader = nonPersistenceGeneration("")
var _ actions.ExecTargetInspector = nonPersistenceInspector{}
var _ actions.ExecAdapter = (*nonPersistenceExecAdapter)(nil)
var _ actions.RemoteExec = (*nonPersistenceRemote)(nil)
