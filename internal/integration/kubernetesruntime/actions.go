package kubernetesruntime

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

// ActionBackends is the generation-aware client-go boundary used to compose
// the three action services. Separate values are necessary because ExecAdapter
// and PortForwardAdapter intentionally have distinct Start signatures.
type ActionBackends struct {
	Mutations   *MutationBackend
	Exec        *ExecBackend
	PortForward *PortForwardBackend
}

func NewActionBackends(runtime *Runtime) (ActionBackends, error) {
	if runtime == nil {
		return ActionBackends{}, errors.New("kubernetes runtime actions: runtime is required")
	}
	return ActionBackends{
		Mutations:   &MutationBackend{runtime: runtime},
		Exec:        &ExecBackend{runtime: runtime},
		PortForward: &PortForwardBackend{runtime: runtime},
	}, nil
}

type MutationBackend struct{ runtime *Runtime }

func (backend *MutationBackend) RestartDeployment(ctx context.Context, command actions.RestartDeploymentCommand) (actions.MutationResult, error) {
	requestContext, cancel, client, err := backend.unary(ctx, command.Target)
	if err != nil {
		return actions.MutationResult{}, err
	}
	defer cancel()
	return client.RestartDeployment(requestContext, command)
}

func (backend *MutationBackend) UpdateScale(ctx context.Context, command actions.ScaleCommand) (actions.MutationResult, error) {
	requestContext, cancel, client, err := backend.unary(ctx, command.Target)
	if err != nil {
		return actions.MutationResult{}, err
	}
	defer cancel()
	return client.UpdateScale(requestContext, command)
}

func (backend *MutationBackend) DeletePod(ctx context.Context, command actions.DeletePodCommand) (actions.MutationResult, error) {
	requestContext, cancel, client, err := backend.unary(ctx, command.Target)
	if err != nil {
		return actions.MutationResult{}, err
	}
	defer cancel()
	return client.DeletePod(requestContext, command)
}

func (backend *MutationBackend) InspectExecTarget(ctx context.Context, target actions.MutationTarget, container string) (actions.ExecTargetState, error) {
	requestContext, cancel, client, err := backend.unary(ctx, target)
	if err != nil {
		return actions.ExecTargetState{}, err
	}
	defer cancel()
	return client.InspectExecTarget(requestContext, target, container)
}

func (backend *MutationBackend) unary(ctx context.Context, target actions.MutationTarget) (context.Context, context.CancelFunc, *kubernetes.ActionClient, error) {
	if backend == nil || backend.runtime == nil {
		return nil, nil, nil, errors.New("kubernetes runtime actions: mutation backend is unavailable")
	}
	lease, err := backend.runtime.leaseFor(ctx, bindingForActionTarget(target))
	if err != nil {
		return nil, nil, nil, err
	}
	requestContext, cancel, err := lease.Generation.Unary(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	client, err := lease.Clients.ActionClient()
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	return requestContext, cancel, client, nil
}

type ExecBackend struct{ runtime *Runtime }

func (backend *ExecBackend) Start(setup context.Context, lifetime context.Context, command actions.ExecCommand) (actions.RemoteExec, error) {
	if backend == nil || backend.runtime == nil {
		return nil, errors.New("kubernetes runtime actions: exec backend is unavailable")
	}
	lease, err := backend.runtime.leaseFor(setup, bindingForActionTarget(command.Target))
	if err != nil {
		return nil, err
	}
	client, err := lease.Clients.ActionClient()
	if err != nil {
		return nil, err
	}
	streamContext, release := generationLifetime(lifetime, lease.Generation.Context())
	remote, err := client.Start(setup, streamContext, command)
	if err != nil {
		release()
		return nil, err
	}
	return &generationRemoteExec{RemoteExec: remote, release: release}, nil
}

type generationRemoteExec struct {
	actions.RemoteExec
	release context.CancelFunc
	once    sync.Once
}

func (remote *generationRemoteExec) Wait() actions.RemoteExecExit {
	result := remote.RemoteExec.Wait()
	remote.once.Do(remote.release)
	return result
}

func (remote *generationRemoteExec) Close() error {
	remote.once.Do(remote.release)
	return remote.RemoteExec.Close()
}

type PortForwardBackend struct{ runtime *Runtime }

func (backend *PortForwardBackend) Start(setup context.Context, lifetime context.Context, command actions.PortForwardCommand, listener net.Listener) (actions.PortForwardHandle, error) {
	if backend == nil || backend.runtime == nil {
		return nil, errors.New("kubernetes runtime actions: port-forward backend is unavailable")
	}
	lease, err := backend.runtime.leaseFor(setup, bindingForActionTarget(command.Target))
	if err != nil {
		return nil, err
	}
	client, err := lease.Clients.ActionClient()
	if err != nil {
		return nil, err
	}
	streamContext, release := generationLifetime(lifetime, lease.Generation.Context())
	handle, err := client.StartPortForward(setup, streamContext, command, listener)
	if err != nil {
		release()
		return nil, err
	}
	return &generationPortForward{PortForwardHandle: handle, release: release}, nil
}

type generationPortForward struct {
	actions.PortForwardHandle
	release context.CancelFunc
	once    sync.Once
}

func (handle *generationPortForward) Wait() error {
	err := handle.PortForwardHandle.Wait()
	handle.once.Do(handle.release)
	return err
}

func (handle *generationPortForward) Close() error {
	handle.once.Do(handle.release)
	return handle.PortForwardHandle.Close()
}

func generationLifetime(lifetime, generation context.Context) (context.Context, context.CancelFunc) {
	if lifetime == nil {
		lifetime = context.Background()
	}
	if generation == nil {
		generation = context.Background()
	}
	ctx, cancel := context.WithCancel(lifetime)
	stopGeneration := context.AfterFunc(generation, cancel)
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			stopGeneration()
			cancel()
		})
	}
}

func bindingForActionTarget(target actions.MutationTarget) namespaces.SelectionBinding {
	return namespaces.SelectionBinding{
		ClusterProfileID: target.ClusterProfileID,
		Context:          target.Context,
		Generation:       target.Generation,
	}
}

// CurrentGeneration makes Runtime the shared generation fence for all action
// services without exposing the active binding itself.
func (runtime *Runtime) CurrentGeneration() string {
	if runtime == nil {
		return ""
	}
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtime.binding.Generation
}

var (
	_ actions.KubernetesActions   = (*MutationBackend)(nil)
	_ actions.ExecTargetInspector = (*MutationBackend)(nil)
	_ actions.ExecAdapter         = (*ExecBackend)(nil)
	_ actions.PortForwardAdapter  = (*PortForwardBackend)(nil)
	_ actions.GenerationReader    = (*Runtime)(nil)
)
