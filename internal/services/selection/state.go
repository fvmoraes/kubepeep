package selection

import (
	"context"
	"errors"
	"sync"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

// State is the in-memory active selection. Durable context/scope rows remain
// in their repositories; only the current binding and resolved namespace set
// are held here.
type State struct {
	coordinator *Coordinator
	mu          sync.RWMutex
	binding     namespaces.SelectionBinding
	resolution  namespaces.ScopeResolution
}

func NewState(coordinator *Coordinator, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) (*State, error) {
	if coordinator == nil {
		return nil, errors.New("selection: coordinator is required")
	}
	if binding.Generation == "" {
		binding.Generation = coordinator.CurrentGeneration()
	}
	if binding.Generation != coordinator.CurrentGeneration() {
		return nil, ErrGenerationChanged
	}
	return &State{coordinator: coordinator, binding: binding, resolution: copyResolution(resolution)}, nil
}

func (s *State) Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.binding, copyResolution(s.resolution)
}

// ActiveProfileID returns the currently bound profile without exposing the
// mutable selection snapshot to consumers that only need profile identity.
func (s *State) ActiveProfileID() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.binding.ClusterProfileID
}

// IfCurrent executes fn while the supplied binding is still the active
// selection. fn must be short and must not call back into State.
func (s *State) IfCurrent(binding namespaces.SelectionBinding, fn func()) bool {
	if fn == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.binding.ClusterProfileID != binding.ClusterProfileID ||
		s.binding.Context != binding.Context ||
		s.binding.Generation != binding.Generation ||
		s.binding.ActiveScopeID != binding.ActiveScopeID {
		return false
	}
	fn()
	return true
}

// Initialize restores the startup selection without publishing a second
// generation. It is intentionally one-shot: after a profile has been bound,
// every change must go through ReplaceContext or Mutate.
func (s *State) Initialize(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) error {
	if binding.ClusterProfileID <= 0 || binding.Context == "" {
		return errors.New("selection: initial profile and context are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.binding.ClusterProfileID != 0 || s.binding.Context != "" {
		return errors.New("selection: initial state is already bound")
	}
	binding.Generation = s.coordinator.CurrentGeneration()
	binding.ActiveScopeID = resolution.ScopeID
	s.binding = binding
	s.resolution = copyResolution(resolution)
	return nil
}

func (s *State) Mutate(
	ctx context.Context,
	expectedGeneration string,
	prepare namespaces.SelectionPreparation,
) (namespaces.SelectionResult, error) {
	if prepare == nil {
		return namespaces.SelectionResult{}, errors.New("selection: preparation callback is required")
	}
	intent := s.coordinator.Begin(ctx)
	defer intent.Cancel()
	if err := intent.Validate(expectedGeneration); err != nil {
		return namespaces.SelectionResult{}, mapMutationError(err)
	}

	s.mu.RLock()
	preparedBinding := s.binding
	s.mu.RUnlock()
	if preparedBinding.Generation != expectedGeneration {
		return namespaces.SelectionResult{}, namespaces.ErrGenerationChanged
	}
	if err := intent.Validate(expectedGeneration); err != nil {
		return namespaces.SelectionResult{}, mapMutationError(err)
	}
	commit, err := prepare(intent.Context(), preparedBinding)
	if err != nil {
		if validationErr := intent.Validate(expectedGeneration); validationErr != nil {
			return namespaces.SelectionResult{}, mapMutationError(validationErr)
		}
		return namespaces.SelectionResult{}, err
	}
	if err := intent.Validate(expectedGeneration); err != nil {
		return namespaces.SelectionResult{}, mapMutationError(err)
	}
	if commit == nil {
		return namespaces.SelectionResult{}, errors.New("selection: local commit callback is required")
	}

	var mutation namespaces.SelectionMutation
	var result namespaces.SelectionResult
	next, err := intent.CommitConditional(expectedGeneration, func(intentContext context.Context) (bool, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		binding := s.binding
		if binding.Generation != expectedGeneration {
			return false, ErrGenerationChanged
		}
		mutation, err = commit(intentContext, binding)
		if err != nil {
			return false, err
		}
		if mutation.PublishGeneration && mutation.Activation == nil {
			return false, errors.New("selection: published scope mutation requires an activation")
		}
		if !mutation.PublishGeneration && mutation.Activation != nil {
			return false, errors.New("selection: inactive scope mutation cannot activate a resolution")
		}
		if mutation.Activation != nil {
			copied := copyResolution(*mutation.Activation)
			mutation.Activation = &copied
		}
		return mutation.PublishGeneration, nil
	}, func(generation string) {
		s.mu.Lock()
		if mutation.PublishGeneration && mutation.Activation != nil {
			s.binding.ActiveScopeID = mutation.Activation.ScopeID
			s.binding.Generation = generation
			s.resolution = copyResolution(*mutation.Activation)
		}
		result = namespaces.SelectionResult{
			Generation: generation,
			Binding:    s.binding,
			Resolution: copyResolution(s.resolution),
			Changed:    mutation.PublishGeneration,
		}
		s.mu.Unlock()
	})
	if err != nil {
		return namespaces.SelectionResult{}, mapMutationError(err)
	}
	if result.Generation == "" {
		return namespaces.SelectionResult{}, errors.New("selection: commit completed without a result snapshot")
	}
	result.Generation = next
	return result, nil
}

// ReplaceContextPrepared records the selection intent before resolving the
// target context. prepare may perform remote work on the cancelable intent
// context and must return a local-only transaction for the commit fence.
func (s *State) ReplaceContextPrepared(
	ctx context.Context,
	expectedGeneration string,
	prepare func(context.Context) (namespaces.SelectionBinding, namespaces.ScopeResolution, func(context.Context) error, error),
) (namespaces.SelectionResult, error) {
	if prepare == nil {
		return namespaces.SelectionResult{}, errors.New("selection: context preparation callback is required")
	}
	intent := s.coordinator.Begin(ctx)
	defer intent.Cancel()
	if err := intent.Validate(expectedGeneration); err != nil {
		return namespaces.SelectionResult{}, mapMutationError(err)
	}
	s.mu.RLock()
	currentGeneration := s.binding.Generation
	s.mu.RUnlock()
	if currentGeneration != expectedGeneration {
		return namespaces.SelectionResult{}, namespaces.ErrGenerationChanged
	}
	if err := intent.Validate(expectedGeneration); err != nil {
		return namespaces.SelectionResult{}, mapMutationError(err)
	}

	binding, resolution, transaction, err := prepare(intent.Context())
	if err != nil {
		if validationErr := intent.Validate(expectedGeneration); validationErr != nil {
			return namespaces.SelectionResult{}, mapMutationError(validationErr)
		}
		return namespaces.SelectionResult{}, err
	}
	if err := intent.Validate(expectedGeneration); err != nil {
		return namespaces.SelectionResult{}, mapMutationError(err)
	}
	if binding.ClusterProfileID <= 0 || binding.Context == "" {
		return namespaces.SelectionResult{}, errors.New("selection: prepared profile and context are required")
	}
	if transaction == nil {
		return namespaces.SelectionResult{}, errors.New("selection: prepared context transaction is required")
	}
	resolution = copyResolution(resolution)

	var result namespaces.SelectionResult
	next, err := intent.CommitConditional(expectedGeneration, func(intentContext context.Context) (bool, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.binding.Generation != expectedGeneration {
			return false, ErrGenerationChanged
		}
		if err := transaction(intentContext); err != nil {
			return false, err
		}
		return true, nil
	}, func(generation string) {
		s.mu.Lock()
		binding.Generation = generation
		binding.ActiveScopeID = resolution.ScopeID
		s.binding = binding
		s.resolution = copyResolution(resolution)
		result = namespaces.SelectionResult{
			Generation: generation,
			Binding:    s.binding,
			Resolution: copyResolution(s.resolution),
			Changed:    true,
		}
		s.mu.Unlock()
	})
	if err != nil {
		return namespaces.SelectionResult{}, mapMutationError(err)
	}
	result.Generation = next
	return result, nil
}

// ReplaceContext publishes a prevalidated, already-committed profile/context
// selection and resets or restores its scope binding atomically in memory.
func (s *State) ReplaceContext(ctx context.Context, expectedGeneration string, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, transaction func(context.Context) error) (string, error) {
	intent := s.coordinator.Begin(ctx)
	defer intent.Cancel()
	next, err := intent.CommitConditional(expectedGeneration, func(intentContext context.Context) (bool, error) {
		if transaction == nil {
			return false, errors.New("selection: context transaction is required")
		}
		if err := transaction(intentContext); err != nil {
			return false, err
		}
		return true, nil
	}, func(generation string) {
		binding.Generation = generation
		binding.ActiveScopeID = resolution.ScopeID
		s.mu.Lock()
		s.binding = binding
		s.resolution = copyResolution(resolution)
		s.mu.Unlock()
	})
	return next, err
}

func copyResolution(value namespaces.ScopeResolution) namespaces.ScopeResolution {
	value.Namespaces = append([]string(nil), value.Namespaces...)
	if value.DefaultNamespace != nil {
		copied := *value.DefaultNamespace
		value.DefaultNamespace = &copied
	}
	return value
}

func mapMutationError(err error) error {
	if errors.Is(err, ErrGenerationChanged) || errors.Is(err, ErrSuperseded) {
		return namespaces.ErrGenerationChanged
	}
	return err
}

var _ namespaces.SelectionCoordinator = (*State)(nil)
