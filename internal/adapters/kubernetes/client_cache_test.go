package kubernetes

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type countingBuilder struct {
	count   atomic.Int32
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

type stagedBuilder struct {
	count    atomic.Int32
	started  chan int
	releases []chan struct{}
}

func (builder *stagedBuilder) Build(ctx context.Context, _ *Resolution) (*Clients, error) {
	position := int(builder.count.Add(1)) - 1
	builder.started <- position
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-builder.releases[position]:
		return &Clients{}, nil
	}
}

func (builder *countingBuilder) Build(ctx context.Context, _ *Resolution) (*Clients, error) {
	builder.count.Add(1)
	if builder.started != nil {
		builder.once.Do(func() { close(builder.started) })
	}
	if builder.release != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-builder.release:
		}
	}
	return &Clients{}, nil
}

func cacheResolution(t *testing.T, path, contextName, server string) *Resolution {
	t.Helper()
	if server != "" {
		writeTestKubeconfig(t, path, testKubeconfig(server, "current"))
	}
	loader := NewLoader(LoaderOptions{})
	resolution, err := loader.Resolve(context.Background(), ResolveRequest{
		ExplicitPath:    &path,
		ExplicitContext: &contextName,
		FirstReconcile:  true,
	})
	if err != nil {
		t.Fatalf("resolve cache fixture: %v", err)
	}
	return resolution
}

func TestClientCacheDeduplicatesConcurrentBuildsByLogicalKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	builder := &countingBuilder{started: make(chan struct{}), release: make(chan struct{})}
	cache, err := NewClientCache(context.Background(), builder, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	const callers = 12
	start := make(chan struct{})
	results := make(chan *Lease, callers)
	errors := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			lease, err := cache.Activate(context.Background(), resolution)
			results <- lease
			errors <- err
		}()
	}
	close(start)
	select {
	case <-builder.started:
	case <-time.After(time.Second):
		t.Fatal("client build did not start")
	}
	close(builder.release)
	group.Wait()
	close(results)
	close(errors)
	if got := builder.count.Load(); got != 1 {
		t.Fatalf("build count = %d, want 1", got)
	}
	var first *Lease
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent activate: %v", err)
		}
	}
	for lease := range results {
		if lease == nil {
			t.Fatal("nil lease")
		}
		if first == nil {
			first = lease
		} else if lease != first || lease.Generation != first.Generation || lease.Clients != first.Clients {
			t.Fatal("concurrent callers did not share the active cache lease")
		}
	}
}

func TestClientCacheContextSwitchCancelsPreviousGenerationRequestsAndStreams(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeTestKubeconfig(t, path, testKubeconfig("https://cluster.invalid", "current"))
	current := cacheResolution(t, path, "current", "")
	explicit := cacheResolution(t, path, "explicit", "")
	builder := &countingBuilder{}
	cache, err := NewClientCache(context.Background(), builder, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	first, err := cache.Activate(context.Background(), current)
	if err != nil {
		t.Fatal(err)
	}
	unary, cancelUnary, err := first.Generation.Unary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelUnary()
	stream, err := first.Generation.Stream(context.Background(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	second, err := cache.Activate(context.Background(), explicit)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation.ID() == second.Generation.ID() {
		t.Fatal("context switch reused a generation")
	}
	for name, done := range map[string]<-chan struct{}{
		"generation": first.Generation.Context().Done(),
		"unary":      unary.Done(),
		"stream":     stream.Context().Done(),
	} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("previous %s was not canceled", name)
		}
	}
	if second.Generation.Context().Err() != nil {
		t.Fatal("new generation was canceled")
	}
}

func TestClientCacheInvalidatesChangedFilesAndAuthenticationFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	builder := &countingBuilder{}
	cache, err := NewClientCache(context.Background(), builder, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	first, err := cache.Activate(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	writeTestKubeconfig(t, path, testKubeconfig("https://changed.invalid", "current"))
	if _, err := cache.Activate(context.Background(), resolution); !IsGenerationChanged(err) {
		t.Fatalf("stale resolution error = %v", err)
	}
	select {
	case <-first.Generation.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("file change did not cancel active generation")
	}
	fresh := cacheResolution(t, path, "current", "")
	second, err := cache.Activate(context.Background(), fresh)
	if err != nil {
		t.Fatal(err)
	}
	if !cache.InvalidateOnError(fresh.Descriptor(), apierrors.NewUnauthorized("AUTH_MARKER_SHOULD_NOT_LEAK")) {
		t.Fatal("unauthorized error did not invalidate cache")
	}
	select {
	case <-second.Generation.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("authentication invalidation did not cancel generation")
	}
	if cache.InvalidateOnError(fresh.Descriptor(), context.DeadlineExceeded) {
		t.Fatal("non-authentication error invalidated the cache")
	}
	if _, err := cache.Activate(context.Background(), fresh); err != nil {
		t.Fatal(err)
	}
	if got := builder.count.Load(); got != 3 {
		t.Fatalf("build count = %d, want 3", got)
	}
}

func TestClientCacheInvalidationFencesInflightBuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	builder := &stagedBuilder{
		started:  make(chan int, 2),
		releases: []chan struct{}{make(chan struct{}), make(chan struct{})},
	}
	cache, err := NewClientCache(context.Background(), builder, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	firstResult := make(chan error, 1)
	go func() {
		_, activateErr := cache.Activate(context.Background(), resolution)
		firstResult <- activateErr
	}()
	if position := <-builder.started; position != 0 {
		t.Fatalf("first build position = %d", position)
	}
	cache.Invalidate(resolution.Descriptor())

	secondLease := make(chan *Lease, 1)
	secondError := make(chan error, 1)
	go func() {
		lease, activateErr := cache.Activate(context.Background(), resolution)
		secondLease <- lease
		secondError <- activateErr
	}()
	if position := <-builder.started; position != 1 {
		t.Fatalf("second build position = %d", position)
	}
	close(builder.releases[0])
	if err := <-firstResult; !IsGenerationChanged(err) {
		t.Fatalf("superseded build error = %v", err)
	}
	close(builder.releases[1])
	lease := <-secondLease
	if err := <-secondError; err != nil || lease == nil {
		t.Fatalf("replacement lease=%#v err=%v", lease, err)
	}
	reused, err := cache.Activate(context.Background(), resolution)
	if err != nil || reused != lease || builder.count.Load() != 2 {
		t.Fatalf("reused=%#v lease=%#v builds=%d err=%v", reused, lease, builder.count.Load(), err)
	}
}

func TestClientCacheParentCancellationStopsActiveGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	resolution := cacheResolution(t, path, "current", "https://cluster.invalid")
	parent, cancel := context.WithCancel(context.Background())
	cache, err := NewClientCache(parent, &countingBuilder{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	lease, err := cache.Activate(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-lease.Generation.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not reach active generation")
	}
}
