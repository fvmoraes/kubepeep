package runtime

import (
	"context"
	"errors"
	"fmt"
)

// Controller implements status/stop without ever signaling a PID.
type Controller struct {
	Client ControlClient
}

// Status returns the identity and true when the authenticated instance is
// active. Missing or provably stale state returns false without an error.
func (controller Controller) Status(ctx context.Context, runtimeDirectory string) (ControlIdentityDTO, bool, error) {
	return controller.request(ctx, runtimeDirectory, false)
}

// Stop requests idempotent cancellation. Missing or provably stale state is a
// successful no-op represented by active=false.
func (controller Controller) Stop(ctx context.Context, runtimeDirectory string) (ControlIdentityDTO, bool, error) {
	return controller.request(ctx, runtimeDirectory, true)
}

func (controller Controller) request(ctx context.Context, runtimeDirectory string, stop bool) (ControlIdentityDTO, bool, error) {
	state, err := LoadInstanceState(runtimeDirectory)
	if errors.Is(err, ErrNotRunning) {
		return ControlIdentityDTO{}, false, nil
	}
	if err != nil {
		return ControlIdentityDTO{}, false, err
	}
	var identity ControlIdentityDTO
	if stop {
		identity, err = controller.Client.Stop(ctx, state)
	} else {
		identity, err = controller.Client.Status(ctx, state)
	}
	if err == nil {
		return identity, true, nil
	}
	var transportError *ControlTransportError
	if !errors.As(err, &transportError) {
		return ControlIdentityDTO{}, false, err
	}
	if ctx.Err() != nil {
		return ControlIdentityDTO{}, false, ctx.Err()
	}
	stale, recoveryErr := recoverStaleState(runtimeDirectory, state)
	if recoveryErr != nil {
		return ControlIdentityDTO{}, false, errors.Join(ErrUnverifiedRuntime, recoveryErr)
	}
	if !stale {
		return ControlIdentityDTO{}, false, fmt.Errorf("%w: instance lock is held", ErrUnverifiedRuntime)
	}
	return ControlIdentityDTO{}, false, nil
}

func recoverStaleState(runtimeDirectory string, expected InstanceStateV1) (bool, error) {
	lock, err := AcquireFileLock(LockPath(runtimeDirectory))
	if errors.Is(err, ErrLocked) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer lock.Close()
	current, err := LoadInstanceState(runtimeDirectory)
	if errors.Is(err, ErrNotRunning) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if current != expected {
		return false, errors.New("runtime: instance state changed during stale recovery")
	}
	if err := RemoveInstanceState(runtimeDirectory); err != nil {
		return false, err
	}
	return true, nil
}
