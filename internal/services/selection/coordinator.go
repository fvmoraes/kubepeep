// Package selection serializes every mutation that can change the active
// Kubernetes context or namespace scope. It owns intent cancellation,
// generation publication, CSRF rotation, and generation-scoped work.
package selection

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/fvmoraes/kubepeep/internal/api"
)

var (
	ErrSuperseded        = errors.New("selection intent was superseded")
	ErrGenerationChanged = errors.New("selection generation changed")
)

type InvalidateHook func(generation string)

type Coordinator struct {
	mu sync.Mutex

	generation *api.GenerationStore
	sessions   *api.SessionStore
	sequence   uint64
	intentStop context.CancelFunc
	workCtx    context.Context
	workStop   context.CancelFunc
	hooks      []InvalidateHook
}

func NewCoordinator(generation *api.GenerationStore, sessions *api.SessionStore, hooks ...InvalidateHook) (*Coordinator, error) {
	if generation == nil || generation.Current() == "" {
		return nil, errors.New("selection: generation store is required")
	}
	if sessions == nil {
		return nil, errors.New("selection: session store is required")
	}
	workCtx, workStop := context.WithCancel(context.Background())
	return &Coordinator{generation: generation, sessions: sessions, workCtx: workCtx, workStop: workStop, hooks: hooks}, nil
}

// Begin records a newer user intent and immediately cancels validation or
// bootstrap work belonging to its predecessor. The returned intent must be
// committed or canceled by the caller.
func (c *Coordinator) Begin(parent context.Context) *Intent {
	if parent == nil {
		parent = context.Background()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.intentStop != nil {
		c.intentStop()
	}
	c.sequence++
	intentCtx, stop := context.WithCancel(parent)
	c.intentStop = stop
	return &Intent{coordinator: c, sequence: c.sequence, ctx: intentCtx, stop: stop}
}

// WorkContext is canceled only after a successful selection transaction and
// replaced with the context for the newly published generation.
func (c *Coordinator) WorkContext() (context.Context, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.workCtx, c.generation.Current()
}

func (c *Coordinator) CurrentGeneration() string { return c.generation.Current() }

func (c *Coordinator) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.intentStop != nil {
		c.intentStop()
		c.intentStop = nil
	}
	if c.workStop != nil {
		c.workStop()
		c.workStop = nil
	}
}

type Intent struct {
	coordinator *Coordinator
	sequence    uint64
	ctx         context.Context
	stop        context.CancelFunc
	once        sync.Once
}

func (i *Intent) Context() context.Context { return i.ctx }

func (i *Intent) Cancel() {
	if i == nil {
		return
	}
	i.once.Do(i.stop)
}

// Validate checks that this intent still owns the current epoch and that its
// generation precondition still holds. Callers use it before remote
// preparation; CommitConditional performs the same check at the commit fence.
func (i *Intent) Validate(expectedGeneration string) error {
	if i == nil || i.coordinator == nil {
		return errors.New("selection: valid intent is required")
	}
	c := i.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	if i.sequence != c.sequence || errors.Is(i.ctx.Err(), context.Canceled) {
		return ErrSuperseded
	}
	if expectedGeneration == "" || expectedGeneration != c.generation.Current() {
		return ErrGenerationChanged
	}
	return nil
}

// Commit runs the supplied local transaction while holding the shared
// selection lock. A failed transaction does not rotate generation, CSRF, or
// generation-scoped work. Cluster connectivity belongs after Commit and must
// not be folded into transaction, so offline state never rolls back a valid
// local selection.
func (i *Intent) Commit(expectedGeneration string, transaction func(context.Context) error) (string, error) {
	return i.CommitConditional(expectedGeneration, func(ctx context.Context) (bool, error) {
		if err := transaction(ctx); err != nil {
			return false, err
		}
		return true, nil
	}, nil)
}

// CommitConditional lets storage-only mutations preserve the current
// generation. The transaction callback must contain local-only work: remote
// preparation has already completed on the cancelable intent context.
// afterPublish runs under the coordinator lock after any required
// generation/CSRF publication and before a newer intent may begin.
func (i *Intent) CommitConditional(
	expectedGeneration string,
	transaction func(context.Context) (publishGeneration bool, err error),
	afterPublish func(generation string),
) (string, error) {
	if i == nil || i.coordinator == nil || transaction == nil {
		return "", errors.New("selection: valid intent and transaction are required")
	}
	c := i.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	defer i.Cancel()

	if i.sequence != c.sequence || errors.Is(i.ctx.Err(), context.Canceled) {
		return "", ErrSuperseded
	}
	current := c.generation.Current()
	if expectedGeneration == "" || expectedGeneration != current {
		return "", ErrGenerationChanged
	}
	publish, err := transaction(i.ctx)
	if err != nil {
		return "", err
	}
	if !publish {
		if afterPublish != nil {
			afterPublish(current)
		}
		return current, nil
	}

	next, err := c.generation.Rotate()
	if err != nil {
		return "", &PublishedStateError{Stage: "generation", Cause: err}
	}
	if c.workStop != nil {
		c.workStop()
	}
	c.workCtx, c.workStop = context.WithCancel(context.Background())
	for _, hook := range c.hooks {
		if hook != nil {
			hook(next)
		}
	}
	// The durable transaction and generation are already published. Keep the
	// in-memory selection on that same generation even if nonce generation
	// fails; /session can retry nonce creation without reverting state.
	if afterPublish != nil {
		afterPublish(next)
	}
	if err := c.sessions.Rotate(next); err != nil {
		return next, &PublishedStateError{Stage: "csrf", Cause: err}
	}
	return next, nil
}

// PublishedStateError means the database transaction committed, so callers
// must not report or attempt rollback even though in-memory publication could
// not be completed. A process restart safely reconstructs this state.
type PublishedStateError struct {
	Stage string
	Cause error
}

func (e *PublishedStateError) Error() string {
	return fmt.Sprintf("selection: local state committed but %s publication failed", e.Stage)
}

func (e *PublishedStateError) Unwrap() error { return e.Cause }
