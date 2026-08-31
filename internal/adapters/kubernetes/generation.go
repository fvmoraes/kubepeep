package kubernetes

import (
	"context"
	"sync"
	"time"
)

var errStreamIdle = &SafeError{
	Code:      CodeRequestTimeout,
	Message:   "The Kubernetes stream was closed after being idle.",
	Retryable: true,
}

// Generation scopes every request, watch, log follow, exec, and port-forward
// to the active selection. Canceling it cancels all children, never siblings.
type Generation struct {
	id           uint64
	ctx          context.Context
	cancel       context.CancelCauseFunc
	unaryTimeout time.Duration
}

func newGeneration(parent context.Context, id uint64, unaryTimeout time.Duration) *Generation {
	ctx, cancel := context.WithCancelCause(parent)
	return &Generation{id: id, ctx: ctx, cancel: cancel, unaryTimeout: unaryTimeout}
}

func (generation *Generation) ID() uint64 {
	if generation == nil {
		return 0
	}
	return generation.id
}

func (generation *Generation) Context() context.Context {
	if generation == nil {
		return context.Background()
	}
	return generation.ctx
}

func (generation *Generation) cancelWith(cause error) {
	if generation != nil {
		generation.cancel(cause)
	}
}

// Unary derives a context with a finite timeout from both the caller and the
// generation. The returned cancel function must always be called.
func (generation *Generation) Unary(parent context.Context) (context.Context, context.CancelFunc, error) {
	if generation == nil || parent == nil || generation.unaryTimeout <= 0 {
		return nil, nil, safeError(CodeClientUnavailable, "A bounded Kubernetes request context is required.", false)
	}
	linked, cancelLinked, stopParent := linkContexts(generation.ctx, parent)
	ctx, cancelTimeout := context.WithTimeout(linked, generation.unaryTimeout)
	cancel := func() {
		cancelTimeout()
		stopParent()
		cancelLinked(context.Canceled)
	}
	return ctx, cancel, nil
}

// Stream derives a context without a global deadline. Its idle timer is reset
// explicitly through Activity, so useful long-running streams are not ended by
// the unary timeout.
func (generation *Generation) Stream(parent context.Context, idleTimeout time.Duration) (*StreamContext, error) {
	if generation == nil || parent == nil || idleTimeout <= 0 {
		return nil, safeError(CodeClientUnavailable, "A Kubernetes stream idle timeout is required.", false)
	}
	ctx, cancel, stopParent := linkContexts(generation.ctx, parent)
	stream := &StreamContext{
		ctx:         ctx,
		cancel:      cancel,
		stopParent:  stopParent,
		idleTimeout: idleTimeout,
	}
	stream.timer = time.AfterFunc(idleTimeout, func() {
		stream.closeWith(errStreamIdle)
	})
	return stream, nil
}

func linkContexts(generation, parent context.Context) (context.Context, context.CancelCauseFunc, func()) {
	ctx, cancel := context.WithCancelCause(generation)
	stopParent := context.AfterFunc(parent, func() {
		cancel(context.Cause(parent))
	})
	if err := parent.Err(); err != nil {
		cancel(context.Cause(parent))
	}
	return ctx, cancel, func() { stopParent() }
}

// StreamContext owns an activity-based idle deadline.
type StreamContext struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelCauseFunc
	stopParent  func()
	idleTimeout time.Duration
	timer       *time.Timer
	closed      bool
}

func (stream *StreamContext) Context() context.Context {
	if stream == nil {
		return context.Background()
	}
	return stream.ctx
}

// Activity resets the idle deadline and reports whether the stream is active.
// It stops the current timer before resetting it so that a timer firing
// concurrently with the heartbeat cannot close the stream after the reset.
func (stream *StreamContext) Activity() bool {
	if stream == nil {
		return false
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed || stream.ctx.Err() != nil {
		return false
	}
	if !stream.timer.Stop() {
		// The timer already fired; the stream is closing or closed.
		return false
	}
	stream.timer.Reset(stream.idleTimeout)
	return true
}

func (stream *StreamContext) Close() {
	if stream != nil {
		stream.closeWith(context.Canceled)
	}
}

func (stream *StreamContext) closeWith(cause error) {
	stream.mu.Lock()
	if stream.closed {
		stream.mu.Unlock()
		return
	}
	stream.closed = true
	if stream.timer != nil {
		stream.timer.Stop()
	}
	stopParent := stream.stopParent
	cancel := stream.cancel
	stream.mu.Unlock()
	stopParent()
	cancel(cause)
}
