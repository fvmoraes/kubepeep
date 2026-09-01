package kubernetesruntime

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func TestNewActionBackendsRequireRuntime(t *testing.T) {
	t.Parallel()
	if _, err := NewActionBackends(nil); err == nil {
		t.Fatal("nil runtime was accepted")
	}
	backs, err := NewActionBackends(&Runtime{})
	if err != nil || backs.Mutations == nil || backs.Exec == nil || backs.PortForward == nil {
		t.Fatalf("action backends = %+v err = %v", backs, err)
	}
}

func actionTestTarget() actions.MutationTarget {
	return actions.MutationTarget{ClusterProfileID: 1, Context: "ctx", Generation: "gen", Namespace: "payments", Kind: "Pod", Name: "api"}
}

func TestMutationBackendRejectsUnavailableRuntime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	target := actionTestTarget()

	if _, err := (&MutationBackend{}).RestartDeployment(ctx, actions.RestartDeploymentCommand{Target: target}); err == nil {
		t.Fatal("nil runtime restart deployment was accepted")
	}
	if _, err := (&MutationBackend{}).UpdateScale(ctx, actions.ScaleCommand{Target: target}); err == nil {
		t.Fatal("nil runtime scale was accepted")
	}
	if _, err := (&MutationBackend{}).DeletePod(ctx, actions.DeletePodCommand{Target: target}); err == nil {
		t.Fatal("nil runtime delete pod was accepted")
	}
	if _, err := (&MutationBackend{}).InspectExecTarget(ctx, target, "api"); err == nil {
		t.Fatal("nil runtime inspect was accepted")
	}

	stale := &MutationBackend{runtime: &Runtime{}}
	if _, err := stale.RestartDeployment(ctx, actions.RestartDeploymentCommand{Target: target}); err == nil {
		t.Fatalf("stale restart deployment err = %v", err)
	}
	if _, err := stale.UpdateScale(ctx, actions.ScaleCommand{Target: target}); err == nil {
		t.Fatal("stale scale was accepted")
	}
	if _, err := stale.DeletePod(ctx, actions.DeletePodCommand{Target: target}); err == nil {
		t.Fatal("stale delete pod was accepted")
	}
	if _, err := stale.InspectExecTarget(ctx, target, "api"); err == nil {
		t.Fatal("stale inspect was accepted")
	}

	var nilBackend *MutationBackend
	if _, _, _, err := nilBackend.unary(ctx, target); err == nil {
		t.Fatalf("nil backend unary err = %v", err)
	}
}

func TestExecAndPortForwardBackendsRejectUnavailableRuntime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	command := actions.ExecCommand{Target: actionTestTarget(), Container: "api", Command: []string{"sh"}}
	for name, backend := range map[string]*ExecBackend{"nil runtime": {}, "stale runtime": {runtime: &Runtime{}}} {
		if _, err := backend.Start(ctx, ctx, command); err == nil {
			t.Fatalf("%s exec start was accepted", name)
		}
	}
	var nilExec *ExecBackend
	if _, err := nilExec.Start(ctx, ctx, command); err == nil {
		t.Fatal("nil exec backend was accepted")
	}

	forward := actions.PortForwardCommand{Target: actionTestTarget(), RemotePort: 8080}
	for name, backend := range map[string]*PortForwardBackend{"nil runtime": {}, "stale runtime": {runtime: &Runtime{}}} {
		if _, err := backend.Start(ctx, ctx, forward, nil); err == nil {
			t.Fatalf("%s port forward start was accepted", name)
		}
	}
	var nilForward *PortForwardBackend
	if _, err := nilForward.Start(ctx, ctx, forward, nil); err == nil {
		t.Fatal("nil port forward backend was accepted")
	}
}

func TestGenerationLifetimeIsIdempotentAndFollowsGeneration(t *testing.T) {
	t.Parallel()
	lifetime, lifetimeCancel := context.WithCancel(context.Background())
	defer lifetimeCancel()
	generation, generationCancel := context.WithCancel(context.Background())
	defer generationCancel()

	streamContext, release := generationLifetime(lifetime, generation)
	release()
	release()
	if err := streamContext.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("stream context after release = %v", err)
	}
	if err := lifetime.Err(); err != nil {
		t.Fatalf("release leaked into lifetime: %v", err)
	}
	if err := generation.Err(); err != nil {
		t.Fatalf("release leaked into generation: %v", err)
	}

	second, secondRelease := generationLifetime(lifetime, generation)
	generationCancel()
	select {
	case <-second.Done():
	case <-context.Background().Done():
		t.Fatal("generation cancel did not propagate to the stream context")
	}
	secondRelease()

	nilContexts, nilRelease := generationLifetime(nil, nil)
	nilRelease()
	if err := nilContexts.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("nil lifetime context = %v", err)
	}
}

func TestGenerationRemoteExecReleasesExactlyOnce(t *testing.T) {
	t.Parallel()
	remote := newNonPersistenceRemote("out", "err")
	zero := 0
	remote.finish(actions.RemoteExecExit{ExitCode: &zero})

	releases := 0
	var mu sync.Mutex
	release := func() {
		mu.Lock()
		defer mu.Unlock()
		releases++
	}
	wrapped := &generationRemoteExec{RemoteExec: remote, release: release}
	exit := wrapped.Wait()
	if exit.ExitCode == nil || *exit.ExitCode != 0 {
		t.Fatalf("exit = %+v", exit)
	}
	if err := wrapped.Close(); err != nil {
		t.Fatalf("close = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if releases != 1 {
		t.Fatalf("releases = %d, want 1", releases)
	}
}

type stubPortForwardHandle struct {
	waitErr error
	closed  int
}

func (handle *stubPortForwardHandle) Wait() error { return handle.waitErr }
func (handle *stubPortForwardHandle) Close() error {
	handle.closed++
	return nil
}

func TestGenerationPortForwardReleasesExactlyOnce(t *testing.T) {
	t.Parallel()
	handle := &stubPortForwardHandle{waitErr: context.Canceled}
	releases := 0
	var mu sync.Mutex
	release := func() {
		mu.Lock()
		defer mu.Unlock()
		releases++
	}
	wrapped := &generationPortForward{PortForwardHandle: handle, release: release}
	if err := wrapped.Close(); err != nil {
		t.Fatalf("close = %v", err)
	}
	if err := wrapped.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if releases != 1 || handle.closed != 1 {
		t.Fatalf("releases = %d closed = %d", releases, handle.closed)
	}
}

func TestBindingForActionTargetMapsSelectionFields(t *testing.T) {
	t.Parallel()
	target := actionTestTarget()
	binding := bindingForActionTarget(target)
	expected := namespaces.SelectionBinding{ClusterProfileID: target.ClusterProfileID, Context: target.Context, Generation: target.Generation}
	if binding != expected {
		t.Fatalf("binding = %+v, want %+v", binding, expected)
	}
}

func TestCurrentGenerationReadsActiveBinding(t *testing.T) {
	t.Parallel()
	var nilRuntime *Runtime
	if got := nilRuntime.CurrentGeneration(); got != "" {
		t.Fatalf("nil runtime generation = %q", got)
	}
	runtime := &Runtime{binding: namespaces.SelectionBinding{Generation: "gen-7"}}
	if got := runtime.CurrentGeneration(); got != "gen-7" {
		t.Fatalf("generation = %q", got)
	}
}
