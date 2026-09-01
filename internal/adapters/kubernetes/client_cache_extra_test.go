package kubernetes

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type failingBuilder struct {
	err error
}

func (builder failingBuilder) Build(context.Context, *Resolution) (*Clients, error) {
	return nil, builder.err
}

type blockingBuilder struct {
	started    chan struct{}
	startedOne sync.Once
	release    chan struct{}
}

func (builder *blockingBuilder) Build(ctx context.Context, _ *Resolution) (*Clients, error) {
	builder.startedOne.Do(func() { close(builder.started) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-builder.release:
		return &Clients{unary: &clientGroup{httpClient: &http.Client{}}}, nil
	}
}

func TestNewClientCacheValidatesDependencies(t *testing.T) {
	if _, err := NewClientCache(nil, failingBuilder{}, time.Second); err == nil {
		t.Fatal("nil parent was accepted")
	}
	if _, err := NewClientCache(context.Background(), nil, time.Second); err == nil {
		t.Fatal("nil builder was accepted")
	}
	if _, err := NewClientCache(context.Background(), failingBuilder{}, -time.Second); err == nil {
		t.Fatal("negative timeout was accepted")
	}
	cache, err := NewClientCache(context.Background(), failingBuilder{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if cache.unaryTimeout != DefaultUnaryTimeout {
		t.Fatalf("default timeout = %s", cache.unaryTimeout)
	}
}

func TestClientCacheActivateGuardsInputsAndClosedState(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	cache, err := NewClientCache(context.Background(), failingBuilder{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Activate(nil, resolution); err == nil {
		t.Fatal("nil context was accepted")
	}
	if _, err := cache.Activate(context.Background(), nil); err == nil {
		t.Fatal("nil resolution was accepted")
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = cache.Activate(context.Background(), resolution)
	if code := safeCode(t, err); code != CodeClientUnavailable {
		t.Fatalf("closed cache code = %q", code)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientCachePropagatesBuilderFailureClass(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	builderFailure := safeError(CodeClusterUnavailable, "The Kubernetes cluster is temporarily unavailable.", true)
	cache, err := NewClientCache(context.Background(), failingBuilder{err: builderFailure}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	_, err = cache.Activate(context.Background(), resolution)
	if !errors.Is(err, builderFailure) {
		t.Fatalf("activate err = %v", err)
	}
	if IsGenerationChanged(err) {
		t.Fatal("builder failure was classified as generation change")
	}
}

func TestClientCacheWaitingCallerSeesItsOwnCancellation(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	builder := &blockingBuilder{started: make(chan struct{}), release: make(chan struct{})}
	cache, err := NewClientCache(context.Background(), builder, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	builderError := make(chan error, 1)
	go func() {
		_, err := cache.Activate(context.Background(), resolution)
		builderError <- err
	}()
	select {
	case <-builder.started:
	case <-time.After(time.Second):
		t.Fatal("first build did not start")
	}

	waiterError := make(chan error, 1)
	waiterContext, cancelWaiter := context.WithCancel(context.Background())
	go func() {
		_, err := cache.Activate(waiterContext, resolution)
		waiterError <- err
	}()
	cancelWaiter()
	select {
	case err := <-waiterError:
		if code := safeCode(t, err); code != CodeRequestCanceled {
			t.Fatalf("waiter code = %q", code)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter was not released")
	}

	close(builder.release)
	select {
	case err := <-builderError:
		if !IsGenerationChanged(err) {
			t.Fatalf("superseded builder err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked builder was not released")
	}
}

func TestClientCacheWaitingCallerSeesClosedCache(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	builder := &blockingBuilder{started: make(chan struct{}), release: make(chan struct{})}
	cache, err := NewClientCache(context.Background(), builder, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	builderDone := make(chan error, 1)
	go func() {
		_, err := cache.Activate(context.Background(), resolution)
		builderDone <- err
	}()
	select {
	case <-builder.started:
	case <-time.After(time.Second):
		t.Fatal("first build did not start")
	}

	waiterDone := make(chan error, 1)
	go func() {
		_, err := cache.Activate(context.Background(), resolution)
		waiterDone <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waiterDone:
		if code := safeCode(t, err); code != CodeClientUnavailable {
			t.Fatalf("waiter code = %q", code)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter was not released by cache close")
	}
	close(builder.release)
	select {
	case err := <-builderDone:
		if code := safeCode(t, err); code != CodeClientUnavailable {
			t.Fatalf("builder code = %q", code)
		}
	case <-time.After(time.Second):
		t.Fatal("builder was not released by cache close")
	}
}

func TestClientCacheCloseReleasesActiveAndEntries(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	var builds atomic.Int32
	builder := builderFunc(func(context.Context, *Resolution) (*Clients, error) {
		builds.Add(1)
		return &Clients{unary: &clientGroup{httpClient: &http.Client{}}}, nil
	})
	cache, err := NewClientCache(context.Background(), builder, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := cache.Activate(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lease.Generation.Context().Done():
	default:
		t.Fatal("close did not cancel the active generation")
	}
}

type builderFunc func(context.Context, *Resolution) (*Clients, error)

func (fn builderFunc) Build(ctx context.Context, resolution *Resolution) (*Clients, error) {
	return fn(ctx, resolution)
}

func TestIsGenerationChangedIgnoresOtherErrors(t *testing.T) {
	if IsGenerationChanged(nil) {
		t.Fatal("nil error classified as generation change")
	}
	if IsGenerationChanged(context.Canceled) {
		t.Fatal("plain error classified as generation change")
	}
	if IsGenerationChanged(safeError(CodeRequestCanceled, "canceled", true)) {
		t.Fatal("other safe error classified as generation change")
	}
	if !IsGenerationChanged(errGenerationChanged) {
		t.Fatal("generation changed error not recognized")
	}
}
