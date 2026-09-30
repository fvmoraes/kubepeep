package resources

import (
	"context"
	"errors"
	"sync"
)

var ErrCoalescerClosed = errors.New("request coalescer is closed")

type coalescedCall struct {
	key      string
	ctx      context.Context
	cancel   context.CancelCauseFunc
	done     chan struct{}
	waiters  int
	finished bool
	result   any
	err      error
}

// RequestCoalescer deduplicates only overlapping work. It intentionally does
// not cache completed Kubernetes data. Each waiter keeps its own cancellation;
// shared work is canceled as soon as the last waiter leaves.
type RequestCoalescer struct {
	mu     sync.Mutex
	calls  map[string]*coalescedCall
	closed bool
}

func NewRequestCoalescer() *RequestCoalescer {
	return &RequestCoalescer{calls: make(map[string]*coalescedCall)}
}

func (coalescer *RequestCoalescer) Do(ctx context.Context, key string, operation func(context.Context) (any, error)) (any, error) {
	if coalescer == nil || operation == nil || key == "" {
		return nil, validationError("coalesced request is invalid")
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	coalescer.mu.Lock()
	if coalescer.closed {
		coalescer.mu.Unlock()
		return nil, ErrCoalescerClosed
	}
	call, ok := coalescer.calls[key]
	if ok {
		call.waiters++
	} else {
		shared, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
		call = &coalescedCall{key: key, ctx: shared, cancel: cancel, done: make(chan struct{}), waiters: 1}
		coalescer.calls[key] = call
		go coalescer.run(key, call, operation)
	}
	coalescer.mu.Unlock()

	select {
	case <-call.done:
		coalescer.leave(call, false)
		return call.result, call.err
	case <-ctx.Done():
		coalescer.leave(call, true)
		return nil, ctx.Err()
	}
}

func (coalescer *RequestCoalescer) run(key string, call *coalescedCall, operation func(context.Context) (any, error)) {
	result, err := operation(call.ctx)
	coalescer.mu.Lock()
	call.result, call.err, call.finished = result, err, true
	if coalescer.calls[key] == call {
		delete(coalescer.calls, key)
	}
	close(call.done)
	call.cancel(nil)
	coalescer.mu.Unlock()
}

func (coalescer *RequestCoalescer) leave(call *coalescedCall, canceled bool) {
	coalescer.mu.Lock()
	defer coalescer.mu.Unlock()
	if call.waiters > 0 {
		call.waiters--
	}
	if canceled && call.waiters == 0 && !call.finished {
		if coalescer.calls[call.key] == call {
			delete(coalescer.calls, call.key)
		}
		call.cancel(context.Canceled)
	}
}

func (coalescer *RequestCoalescer) Close() {
	if coalescer == nil {
		return
	}
	coalescer.mu.Lock()
	defer coalescer.mu.Unlock()
	if coalescer.closed {
		return
	}
	coalescer.closed = true
	for _, call := range coalescer.calls {
		call.cancel(ErrCoalescerClosed)
	}
}
