package resources

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeWatchStream struct {
	channel chan WatchChange
	once    sync.Once
}

func TestOldWatchWorkerRemovalPreservesReplacement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := &WatchManager{workers: map[string]*watchWorker{}}
	key := WatchKey{Generation: "gen", Topic: TopicPods}
	subscription := newSubscription(ctx)
	old := &watchWorker{manager: manager, key: key, ctx: ctx, cancel: cancel, subscribers: map[*Subscription]struct{}{subscription: {}}}
	replacement := &watchWorker{manager: manager, key: key}
	manager.workers[key.identity()] = replacement
	old.remove(subscription)
	if manager.workers[key.identity()] != replacement {
		t.Fatal("closing an old subscription removed its replacement worker")
	}
	if ctx.Err() == nil {
		t.Fatal("old worker was not cancelled")
	}
}

func (stream *fakeWatchStream) ResultChan() <-chan WatchChange { return stream.channel }
func (stream *fakeWatchStream) Stop()                          { stream.once.Do(func() {}) }

type fakeWatchPort struct {
	mu             sync.Mutex
	lists          int
	watches        int
	snapshot       WatchSnapshot
	snapshots      []WatchSnapshot
	stream         *fakeWatchStream
	listErr        error
	watchErr       error
	watchErrs      []error
	watchVersions  []string
	initialWatches int
	initialStream  *fakeWatchStream
	initialErr     error
}

func (port *fakeWatchPort) List(context.Context, WatchKey) (WatchSnapshot, error) {
	port.mu.Lock()
	defer port.mu.Unlock()
	port.lists++
	if len(port.snapshots) > 0 {
		index := min(port.lists-1, len(port.snapshots)-1)
		return port.snapshots[index], port.listErr
	}
	return port.snapshot, port.listErr
}
func (port *fakeWatchPort) Watch(_ context.Context, _ WatchKey, resourceVersion string, _ int64, _ bool) (WatchStream, error) {
	port.mu.Lock()
	defer port.mu.Unlock()
	port.watches++
	port.watchVersions = append(port.watchVersions, resourceVersion)
	if len(port.watchErrs) >= port.watches && port.watchErrs[port.watches-1] != nil {
		return nil, port.watchErrs[port.watches-1]
	}
	if port.watchErr != nil {
		return nil, port.watchErr
	}
	return port.stream, nil
}

func (port *fakeWatchPort) WatchInitial(context.Context, WatchKey, int64) (WatchStream, error) {
	port.mu.Lock()
	defer port.mu.Unlock()
	port.initialWatches++
	if port.initialErr != nil {
		return nil, port.initialErr
	}
	return port.initialStream, nil
}

func TestStreamingListFastPathAndUnsupportedFallback(t *testing.T) {
	t.Run("fast path", func(t *testing.T) {
		stream := &fakeWatchStream{channel: make(chan WatchChange, 4)}
		stream.channel <- WatchChange{Type: "ADDED", Object: PodDTO{Namespace: "ns", Name: "api"}}
		stream.channel <- WatchChange{Type: "BOOKMARK", ResourceVersion: "7", InitialEventsEnd: true}
		port := &fakeWatchPort{initialStream: stream}
		manager := NewWatchManagerWithConfig(port, WatchManagerConfig{StreamingLists: true})
		defer manager.Close()
		subscription, err := manager.Subscribe(t.Context(), podWatchKey())
		if err != nil {
			t.Fatal(err)
		}
		defer subscription.Close()
		event, err := nextWithin(subscription)
		if err != nil || event.Event != "snapshot" || event.ResourceVersion != "7" || len(event.Items) != 1 {
			t.Fatalf("initial watch snapshot=%#v err=%v", event, err)
		}
		port.mu.Lock()
		defer port.mu.Unlock()
		if port.initialWatches != 1 || port.lists != 0 || port.watches != 0 {
			t.Fatalf("initial=%d lists=%d watches=%d", port.initialWatches, port.lists, port.watches)
		}
	})
	t.Run("unsupported falls back", func(t *testing.T) {
		port := &fakeWatchPort{initialErr: ErrStreamingListsUnsupported, snapshot: WatchSnapshot{ResourceVersion: "8"}, stream: &fakeWatchStream{channel: make(chan WatchChange, 1)}}
		manager := NewWatchManagerWithConfig(port, WatchManagerConfig{StreamingLists: true})
		defer manager.Close()
		subscription, err := manager.Subscribe(t.Context(), podWatchKey())
		if err != nil {
			t.Fatal(err)
		}
		defer subscription.Close()
		event, err := nextWithin(subscription)
		if err != nil || event.Event != "snapshot" || event.ResourceVersion != "8" {
			t.Fatalf("fallback snapshot=%#v err=%v", event, err)
		}
		port.mu.Lock()
		defer port.mu.Unlock()
		if port.initialWatches != 1 || port.lists != 1 || port.watches != 1 {
			t.Fatalf("initial=%d lists=%d watches=%d", port.initialWatches, port.lists, port.watches)
		}
	})
}

func TestWatchCreationResourceExpiredRelistsWithoutDroppingSubscription(t *testing.T) {
	stream := &fakeWatchStream{channel: make(chan WatchChange, 1)}
	port := &fakeWatchPort{
		snapshots: []WatchSnapshot{
			{ResourceVersion: "1", Items: []TopicObject{PodDTO{Namespace: "payments", Name: "old"}}},
			{ResourceVersion: "2", Items: []TopicObject{PodDTO{Namespace: "payments", Name: "fresh"}}},
		},
		watchErrs: []error{ErrResourceExpired, nil},
		stream:    stream,
	}
	manager := NewWatchManager(port)
	defer manager.Close()

	first, err := manager.Subscribe(context.Background(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	if event, nextErr := nextWithin(first); nextErr != nil || event.Event != "snapshot" || event.ResourceVersion != "1" {
		t.Fatalf("first snapshot=%#v err=%v", event, nextErr)
	}
	if event, nextErr := nextWithin(first); nextErr != nil || event.Event != "refreshed" || event.Reason != "resource_version_expired" || !event.RefetchRequired {
		t.Fatalf("expiry notice=%#v err=%v", event, nextErr)
	}
	if event, nextErr := nextWithin(first); nextErr != nil || event.Event != "snapshot" || event.ResourceVersion != "2" {
		t.Fatalf("relisted snapshot=%#v err=%v", event, nextErr)
	}
	if manager.SharedWatchCount() != 1 {
		t.Fatal("recovery dropped the shared watch")
	}
	port.mu.Lock()
	defer port.mu.Unlock()
	if port.lists != 2 || port.watches != 2 || len(port.watchVersions) != 2 || port.watchVersions[0] != "1" || port.watchVersions[1] != "2" {
		t.Fatalf("lists=%d watches=%d versions=%v", port.lists, port.watches, port.watchVersions)
	}
}

func TestWatchForbiddenInvalidatesCachedSnapshotBeforeNextSubscription(t *testing.T) {
	stream := &fakeWatchStream{channel: make(chan WatchChange, 1)}
	port := &fakeWatchPort{
		snapshots: []WatchSnapshot{
			{ResourceVersion: "1", Items: []TopicObject{PodDTO{Namespace: "payments", Name: "old"}}},
			{ResourceVersion: "2", Items: []TopicObject{PodDTO{Namespace: "payments", Name: "current"}}},
		},
		watchErrs: []error{domainError(CodeForbidden, "Resource watch is forbidden.", nil), nil},
		stream:    stream,
	}
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 4})
	manager := NewWatchManagerWithConfig(port, WatchManagerConfig{Cache: cache})
	defer manager.Close()
	first, err := manager.Subscribe(t.Context(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	if event, nextErr := nextWithin(first); nextErr != nil || event.Event != "snapshot" || event.ResourceVersion != "1" {
		t.Fatalf("first snapshot=%#v err=%v", event, nextErr)
	}
	if event, nextErr := nextWithin(first); nextErr != nil || event.Event != "error" || event.Reason != string(CodeForbidden) {
		t.Fatalf("forbidden event=%#v err=%v", event, nextErr)
	}
	deadline := time.Now().Add(time.Second)
	for manager.SharedWatchCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stats := cache.Stats(); stats.Entries != 0 {
		t.Fatalf("forbidden snapshot retained: %#v", stats)
	}
	second, err := manager.Subscribe(t.Context(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if event, nextErr := nextWithin(second); nextErr != nil || event.Event != "snapshot" || event.ResourceVersion != "2" || event.Items[0].(PodDTO).Name != "current" {
		t.Fatalf("revalidated snapshot=%#v err=%v", event, nextErr)
	}
}

func TestWatchBookmarkAdvancesReconnectCheckpointWithoutUIEvent(t *testing.T) {
	stream := &fakeWatchStream{channel: make(chan WatchChange, 2)}
	port := &fakeWatchPort{snapshot: WatchSnapshot{ResourceVersion: "1"}, stream: stream}
	manager := NewWatchManager(port)
	defer manager.Close()
	subscription, err := manager.Subscribe(t.Context(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	if event, nextErr := nextWithin(subscription); nextErr != nil || event.Event != "snapshot" {
		t.Fatalf("initial snapshot=%#v err=%v", event, nextErr)
	}
	stream.channel <- WatchChange{Type: "BOOKMARK", ResourceVersion: "9"}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if event, nextErr := subscription.Next(ctx); !errors.Is(nextErr, context.DeadlineExceeded) {
		t.Fatalf("bookmark leaked to UI: %#v, %v", event, nextErr)
	}
	close(stream.channel)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		port.mu.Lock()
		versions := append([]string(nil), port.watchVersions...)
		port.mu.Unlock()
		if len(versions) >= 2 {
			if versions[0] != "1" || versions[1] != "9" {
				t.Fatalf("watch reconnect versions = %v", versions)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("watch did not reconnect from bookmark checkpoint")
}

func TestWatchCoverageRequiresConnectedStream(t *testing.T) {
	stream := &fakeWatchStream{channel: make(chan WatchChange)}
	port := &fakeWatchPort{snapshot: WatchSnapshot{ResourceVersion: "1"}, stream: stream}
	cache := NewResourceCache(ResourceCacheConfig{})
	manager := NewWatchManagerWithConfig(port, WatchManagerConfig{Cache: cache})
	defer manager.Close()
	subscription, err := manager.Subscribe(t.Context(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	if event, nextErr := nextWithin(subscription); nextErr != nil || event.Event != "snapshot" {
		t.Fatalf("initial snapshot=%#v err=%v", event, nextErr)
	}
	origins := []Origin{{Namespace: "payments", Version: "v1", Resource: "pods"}}
	deadline := time.Now().Add(time.Second)
	selection := Selection{Generation: "gen", Context: "ctx", Scope: "scope"}
	for !manager.Covers(selection, TopicPods, origins) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !manager.Covers(selection, TopicPods, origins) {
		t.Fatal("healthy watch did not cover its origin")
	}
	if manager.Covers(Selection{Generation: "gen", Context: "another", Scope: "scope"}, TopicPods, origins) {
		t.Fatal("watch from another context covered an HTTP page")
	}
	port.mu.Lock()
	port.watchErr = errors.New("watch temporarily unavailable")
	port.mu.Unlock()
	close(stream.channel)
	deadline = time.Now().Add(time.Second)
	for manager.Covers(selection, TopicPods, origins) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.Covers(selection, TopicPods, origins) {
		t.Fatal("disconnected watch still covered an HTTP page")
	}
	probe, err := cache.Subscribe(t.Context(), ResourceCacheKey{WatchKey: podWatchKey()})
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if cached, ok := probe.Load(t.Context()); !ok || cached.State != CacheStateStale {
		t.Fatalf("disconnected watch cache state = %#v, found=%t", cached.State, ok)
	}
}
func (port *fakeWatchPort) counts() (int, int) {
	port.mu.Lock()
	defer port.mu.Unlock()
	return port.lists, port.watches
}
func podWatchKey() WatchKey {
	return WatchKey{Generation: "gen", Context: "ctx", Scope: "scope", Topic: TopicPods, GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespace: "payments"}
}

func TestValidateTopicsCanonicalizesExactAllowlistAndForbidsDuplicates(t *testing.T) {
	topics, err := ValidateTopics([]Topic{TopicConfigMaps, TopicPods, TopicEvents})
	if err != nil {
		t.Fatal(err)
	}
	if got := []Topic{topics[0], topics[1], topics[2]}; got[0] != TopicPods || got[1] != TopicEvents || got[2] != TopicConfigMaps {
		t.Fatalf("topics = %#v", topics)
	}
	for _, invalid := range [][]Topic{{}, {TopicPods, TopicPods}, {"secrets"}, {TopicPods, TopicEvents, TopicWorkloads, TopicServices, TopicIngresses, TopicEndpointSlices, TopicConfigMaps, "extra"}} {
		if _, err = ValidateTopics(invalid); ErrorCodeOf(err) != CodeValidationFailed {
			t.Fatalf("invalid %#v: %v", invalid, err)
		}
	}
	if len(TopicGVRs(TopicWorkloads)) != 5 {
		t.Fatal("workloads must map to exactly five GVRs")
	}
}

func TestSnapshotEventsChunksAtomicallyAndRejectsOversize(t *testing.T) {
	items := make([]TopicObject, 100)
	for index := range items {
		items[index] = PodDTO{Namespace: "payments", Name: strings.Repeat("x", 1000)}
	}
	events, err := SnapshotEvents("gen", TopicPods, WatchSnapshot{ResourceVersion: "10", Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 || !events[len(events)-1].Final {
		t.Fatalf("events = %#v", events)
	}
	for index, event := range events {
		if event.Final != (index == len(events)-1) {
			t.Fatalf("final flag at %d", index)
		}
	}
	_, err = SnapshotEvents("gen", TopicPods, WatchSnapshot{Items: []TopicObject{PodDTO{Name: strings.Repeat("x", MaximumStreamEventBytes)}}})
	if ErrorCodeOf(err) != CodeLimitExceeded {
		t.Fatalf("oversize = %v", err)
	}
	_, err = SnapshotEvents("gen", TopicPods, WatchSnapshot{Items: []TopicObject{EventDTO{}}})
	if ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("topic mismatch = %v", err)
	}
}

func TestWatchManagerSharesSourceAndCancelsGeneration(t *testing.T) {
	stream := &fakeWatchStream{channel: make(chan WatchChange, 4)}
	port := &fakeWatchPort{snapshot: WatchSnapshot{ResourceVersion: "1", Items: []TopicObject{PodDTO{Namespace: "payments", Name: "api"}}}, stream: stream}
	manager := NewWatchManager(port)
	defer manager.Close()
	first, err := manager.Subscribe(context.Background(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	event, err := nextWithin(first)
	if err != nil || event.Event != "snapshot" {
		t.Fatalf("first snapshot=%#v err=%v", event, err)
	}
	second, err := manager.Subscribe(context.Background(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	event, err = nextWithin(second)
	if err != nil || event.Event != "snapshot" {
		t.Fatalf("second snapshot=%#v err=%v", event, err)
	}
	lists, watches := port.counts()
	if lists != 1 || watches != 1 || manager.SharedWatchCount() != 1 {
		t.Fatalf("lists=%d watches=%d shared=%d", lists, watches, manager.SharedWatchCount())
	}
	stream.channel <- WatchChange{Type: "MODIFIED", ResourceVersion: "2", Object: PodDTO{Namespace: "payments", Name: "api"}}
	for _, subscription := range []*Subscription{first, second} {
		event, err = nextWithin(subscription)
		if err != nil || event.Event != "modified" || event.ResourceVersion != "2" {
			t.Fatalf("change=%#v err=%v", event, err)
		}
	}
	manager.CancelGeneration("gen")
	for _, subscription := range []*Subscription{first, second} {
		event, err = nextWithin(subscription)
		if err != nil || event.Event != "reset" || event.Reason != "generation_changed" {
			t.Fatalf("terminal=%#v err=%v", event, err)
		}
	}
}

func TestWatchManagerKeepsIdleSourceAndReplaysItsCurrentSnapshot(t *testing.T) {
	stream := &fakeWatchStream{channel: make(chan WatchChange, 8)}
	port := &fakeWatchPort{
		snapshot: WatchSnapshot{ResourceVersion: "1", Items: []TopicObject{PodDTO{Namespace: "payments", Name: "api", Status: "Pending"}}},
		stream:   stream,
	}
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 8})
	manager := NewWatchManagerWithConfig(port, WatchManagerConfig{Cache: cache, IdleTimeout: 100 * time.Millisecond})
	defer manager.Close()

	first, err := manager.Subscribe(t.Context(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	if event, nextErr := nextWithin(first); nextErr != nil || event.Event != "snapshot" {
		t.Fatalf("first snapshot=%#v err=%v", event, nextErr)
	}
	stream.channel <- WatchChange{
		Type: "MODIFIED", ResourceVersion: "2",
		Object: PodDTO{Namespace: "payments", Name: "api", Status: "Running"},
	}
	if event, nextErr := nextWithin(first); nextErr != nil || event.Event != "modified" {
		t.Fatalf("first delta=%#v err=%v", event, nextErr)
	}
	first.Close()
	if manager.SharedWatchCount() != 1 {
		t.Fatalf("idle worker count = %d", manager.SharedWatchCount())
	}

	second, err := manager.Subscribe(t.Context(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	event, err := nextWithin(second)
	if err != nil || event.Event != "snapshot" || len(event.Items) != 1 {
		t.Fatalf("idle replay=%#v err=%v", event, err)
	}
	pod, ok := event.Items[0].(PodDTO)
	if !ok || pod.Status != "Running" || event.ResourceVersion != "2" {
		t.Fatalf("idle replay pod=%#v event=%#v", event.Items[0], event)
	}
	if lists, watches := port.counts(); lists != 1 || watches != 1 {
		t.Fatalf("idle reuse lists=%d watches=%d", lists, watches)
	}
	second.Close()
	deadline := time.Now().Add(time.Second)
	for manager.SharedWatchCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.SharedWatchCount() != 0 {
		t.Fatal("idle worker was not released")
	}

	third, err := manager.Subscribe(t.Context(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	event, err = nextWithin(third)
	if err != nil || event.Event != "snapshot" || event.ResourceVersion != "2" {
		t.Fatalf("cache replay=%#v err=%v", event, err)
	}
	pod, ok = event.Items[0].(PodDTO)
	if !ok || pod.Status != "Running" {
		t.Fatalf("cache replay pod=%#v", event.Items[0])
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if lists, watches := port.counts(); lists == 1 && watches == 2 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if lists, watches := port.counts(); lists != 1 || watches != 2 {
		t.Fatalf("cached watch reopened with LIST: lists=%d watches=%d", lists, watches)
	}
}

func TestSubscriptionCoalescesResourceBurstButPreservesEvents(t *testing.T) {
	t.Parallel()
	subscription := newSubscription(t.Context())
	for index := range 10000 {
		if !subscription.push(StreamEvent{Event: "modified", Topic: TopicPods, Generation: "gen", Object: PodDTO{Namespace: "ns", Name: "pod", Status: strconv.Itoa(index)}}) {
			t.Fatalf("resource burst rejected at %d", index)
		}
	}
	if len(subscription.queue) != 1 || subscription.bytes > MaximumStreamQueueBytes {
		t.Fatalf("coalesced queue len=%d bytes=%d", len(subscription.queue), subscription.bytes)
	}
	event, err := subscription.Next(t.Context())
	if err != nil || event.Object.(PodDTO).Status != "9999" {
		t.Fatalf("latest state=%#v err=%v", event, err)
	}
	for index := range 2 {
		if !subscription.push(StreamEvent{Event: "modified", Topic: TopicEvents, Generation: "gen", Object: EventDTO{Name: "event", Count: int64(index)}}) {
			t.Fatal("chronological event rejected")
		}
	}
	if len(subscription.queue) != 2 {
		t.Fatalf("events were coalesced: %d", len(subscription.queue))
	}
}

func TestWatchManagerDoesNotShareDifferentEffectiveOrigins(t *testing.T) {
	stream := &fakeWatchStream{channel: make(chan WatchChange, 4)}
	port := &fakeWatchPort{snapshot: WatchSnapshot{ResourceVersion: "1"}, stream: stream}
	manager := NewWatchManager(port)
	defer manager.Close()
	firstKey := podWatchKey()
	firstKey.EffectiveOrigins = []string{"payments", "platform"}
	secondKey := podWatchKey()
	secondKey.EffectiveOrigins = []string{"payments", "restricted"}
	first, err := manager.Subscribe(t.Context(), firstKey)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := manager.Subscribe(t.Context(), secondKey)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err = nextWithin(first); err != nil {
		t.Fatal(err)
	}
	if _, err = nextWithin(second); err != nil {
		t.Fatal(err)
	}
	if lists, watches := port.counts(); lists != 2 || watches != 2 || manager.SharedWatchCount() != 2 {
		t.Fatalf("lists=%d watches=%d shared=%d", lists, watches, manager.SharedWatchCount())
	}
}

func TestWatchManagerRejectsTopicGVRMismatch(t *testing.T) {
	t.Parallel()
	manager := NewWatchManager(&fakeWatchPort{})
	defer manager.Close()
	key := podWatchKey()
	key.GVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	if _, err := manager.Subscribe(t.Context(), key); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("secret resource accepted for Pods watch: %v", err)
	}
}

func TestApplyStreamEventToSnapshotMaintainsLevelState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		start WatchSnapshot
		event StreamEvent
		want  []string
	}{
		{
			name: "add", start: WatchSnapshot{Items: []TopicObject{PodDTO{Namespace: "ns", Name: "one"}}},
			event: StreamEvent{Event: "added", ResourceVersion: "2", Object: PodDTO{Namespace: "ns", Name: "two"}},
			want:  []string{"one", "two"},
		},
		{
			name: "modify", start: WatchSnapshot{Items: []TopicObject{PodDTO{Namespace: "ns", Name: "one", Status: "Pending"}}},
			event: StreamEvent{Event: "modified", ResourceVersion: "2", Object: PodDTO{Namespace: "ns", Name: "one", Status: "Running"}},
			want:  []string{"one:Running"},
		},
		{
			name: "delete", start: WatchSnapshot{Items: []TopicObject{PodDTO{Namespace: "ns", Name: "one"}, PodDTO{Namespace: "ns", Name: "two"}}},
			event: StreamEvent{Event: "deleted", ResourceVersion: "2", Deleted: &ResourceRef{Kind: "Pod", Namespace: "ns", Name: "one"}},
			want:  []string{"two"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := applyStreamEventToSnapshot(TopicPods, test.start, test.event)
			if got.ResourceVersion != "2" {
				t.Fatalf("resource version = %q", got.ResourceVersion)
			}
			values := make([]string, 0, len(got.Items))
			for _, item := range got.Items {
				pod := item.(PodDTO)
				value := pod.Name
				if pod.Status != "" {
					value += ":" + pod.Status
				}
				values = append(values, value)
			}
			if strings.Join(values, ",") != strings.Join(test.want, ",") {
				t.Fatalf("items = %v, want %v", values, test.want)
			}
		})
	}
}

func TestApplyStreamEventToSnapshotDeletesExactEvent(t *testing.T) {
	t.Parallel()
	snapshot := WatchSnapshot{Items: []TopicObject{
		EventDTO{Name: "one", Namespace: "ns", ObjectKind: "Pod", ObjectName: "api"},
		EventDTO{Name: "two", Namespace: "ns", ObjectKind: "Pod", ObjectName: "api"},
	}}
	result := applyStreamEventToSnapshot(TopicEvents, snapshot, StreamEvent{
		Event: "deleted", Topic: TopicEvents, Deleted: &ResourceRef{Kind: "Event", Namespace: "ns", Name: "one"},
	})
	if len(result.Items) != 1 || result.Items[0].(EventDTO).Name != "two" {
		t.Fatalf("event deletion removed the wrong item: %#v", result.Items)
	}
}

func nextWithin(subscription *Subscription) (StreamEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return subscription.Next(ctx)
}

func TestSubscriptionQueueHasExactCountAndByteBackpressure(t *testing.T) {
	subscription := newSubscription(context.Background())
	small := StreamEvent{Event: "modified", Generation: "gen", Object: PodDTO{Name: "api"}}
	for index := 0; index < MaximumStreamQueueEvents; index++ {
		if !subscription.push(small) {
			t.Fatalf("queue filled at %d", index)
		}
	}
	if subscription.push(small) {
		t.Fatal("queue accepted event above count cap")
	}
	subscription.forceTerminal(StreamEvent{Event: "reset", Generation: "gen", Reason: "slow_consumer"})
	event, err := nextWithin(subscription)
	if err != nil || event.Reason != "slow_consumer" {
		t.Fatalf("terminal=%#v err=%v", event, err)
	}
	largeSubscription := newSubscription(context.Background())
	large := StreamEvent{Event: "modified", Generation: "gen", Object: PodDTO{Name: strings.Repeat("x", 60000)}}
	accepted := 0
	for largeSubscription.push(large) {
		accepted++
	}
	if accepted < 1 || accepted >= MaximumStreamQueueEvents {
		t.Fatalf("byte cap accepted %d", accepted)
	}
}

func TestReplayRingUsesOpaqueBoundIDsAndExcludesHeartbeat(t *testing.T) {
	ring, err := NewReplayRing("instance", "gen", []Topic{TopicPods})
	if err != nil {
		t.Fatal(err)
	}
	first, err := ring.Append(StreamEvent{Event: "added", Generation: "gen", Object: PodDTO{Name: "one"}})
	if err != nil || !strings.HasPrefix(first, "kpse1.") {
		t.Fatalf("first=%q err=%v", first, err)
	}
	second, _ := ring.Append(StreamEvent{Event: "modified", Generation: "gen", Object: PodDTO{Name: "two"}})
	if heartbeat, _ := ring.Append(StreamEvent{Event: "heartbeat", Generation: "gen"}); heartbeat != "" {
		t.Fatalf("heartbeat id = %q", heartbeat)
	}
	for _, terminal := range []string{"reset", "error"} {
		if id, appendErr := ring.Append(StreamEvent{Event: terminal, Generation: "gen"}); appendErr != nil || id != "" {
			t.Fatalf("terminal %s id=%q err=%v", terminal, id, appendErr)
		}
	}
	replayed, err := ring.Replay(first, ring.Binding())
	if err != nil || len(replayed) != 1 || replayed[0].Event != "modified" {
		t.Fatalf("replay=%#v err=%v second=%q", replayed, err, second)
	}
	if _, err = ring.Replay("malformed", ring.Binding()); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("malformed = %v", err)
	}
	other, _ := NewReplayRing("instance", "gen", []Topic{TopicPods})
	otherID, _ := other.Append(StreamEvent{Event: "added", Generation: "gen"})
	if _, err = ring.Replay(otherID, ring.Binding()); !errors.Is(err, ErrResumeUnavailable) {
		t.Fatalf("foreign id = %v", err)
	}
	if _, err = ring.Replay(first, "other-binding"); !errors.Is(err, ErrResumeUnavailable) {
		t.Fatalf("binding = %v", err)
	}
	entries, err := ring.ReplayEntries(first, ring.Binding())
	if err != nil || len(entries) != 1 || entries[0].ID != second || entries[0].Event.Event != "modified" {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
	if binding, bindErr := ReplayBinding("instance", "gen", []Topic{TopicPods}); bindErr != nil || binding != ring.Binding() || ring.Epoch() == "" {
		t.Fatalf("binding=%q epoch=%q err=%v", binding, ring.Epoch(), bindErr)
	}
}

func TestReplayRingEvictsOldIDsByExactCountWindow(t *testing.T) {
	ring, err := NewReplayRing("instance", "gen", []Topic{TopicPods})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := ring.Append(StreamEvent{Event: "added", Generation: "gen", Object: PodDTO{Name: "first"}})
	for index := 0; index < MaximumStreamQueueEvents; index++ {
		if _, err = ring.Append(StreamEvent{Event: "modified", Generation: "gen", Object: PodDTO{Name: strconv.Itoa(index)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = ring.ReplayEntries(first, ring.Binding()); !errors.Is(err, ErrResumeUnavailable) {
		t.Fatalf("evicted ID remained resumable: %v", err)
	}
}

func TestReplayRingEvictsOldIDsByByteWindow(t *testing.T) {
	ring, err := NewReplayRing("instance", "gen", []Topic{TopicPods})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := ring.Append(StreamEvent{Event: "added", Generation: "gen", Object: PodDTO{Name: "first"}})
	for index := 0; index < 24; index++ {
		if _, err = ring.Append(StreamEvent{Event: "modified", Generation: "gen", Object: PodDTO{Name: strings.Repeat("x", 60000) + strconv.Itoa(index)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = ring.ReplayEntries(first, ring.Binding()); !errors.Is(err, ErrResumeUnavailable) {
		t.Fatalf("byte-evicted ID remained resumable: %v", err)
	}
}

func TestTopicAuthorizationIsAllOrNothingForListAndWatch(t *testing.T) {
	selection := Selection{Generation: "gen", Namespaces: []string{"payments"}}
	allowed := &fakeAuthorization{decisions: map[string]authorization.Decision{}}
	if err := AuthorizeTopics(context.Background(), allowed, selection, []Topic{TopicWorkloads}); err != nil {
		t.Fatal(err)
	}
	if len(allowed.keys) != 10 {
		t.Fatalf("checks = %d", len(allowed.keys))
	}
	denied := &fakeAuthorization{decisions: map[string]authorization.Decision{"payments": authorization.DecisionDenied}}
	if err := AuthorizeTopics(context.Background(), denied, selection, []Topic{TopicPods}); ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("denied = %v", err)
	}
	unknown := &fakeAuthorization{decisions: map[string]authorization.Decision{"payments": authorization.DecisionUnknown}}
	if err := AuthorizeTopics(context.Background(), unknown, selection, []Topic{TopicPods}); ErrorCodeOf(err) != CodeAuthorizationUnavailable {
		t.Fatalf("unknown = %v", err)
	}
}

type refreshingAuthorization struct {
	checks    int
	refreshes int
	decision  authorization.Decision
}

func (stub *refreshingAuthorization) Check(context.Context, authorization.Key) authorization.Capability {
	stub.checks++
	return authorization.Capability{Decision: authorization.DecisionAllowed}
}
func (stub *refreshingAuthorization) Refresh(context.Context, authorization.Key) authorization.Capability {
	stub.refreshes++
	return authorization.Capability{Decision: stub.decision}
}

func TestReauthorizeTopicsBypassesCacheAndStopsOnRevocation(t *testing.T) {
	stub := &refreshingAuthorization{decision: authorization.DecisionDenied}
	err := ReauthorizeTopics(context.Background(), stub, Selection{Generation: "gen", Namespaces: []string{"payments"}}, []Topic{TopicPods})
	if ErrorCodeOf(err) != CodeForbidden || stub.refreshes != 1 || stub.checks != 0 {
		t.Fatalf("err=%v refreshes=%d checks=%d", err, stub.refreshes, stub.checks)
	}
}

func TestStreamRegistryCapsEightAndReleasesIdempotently(t *testing.T) {
	registry := &StreamRegistry{}
	releases := make([]func(), 0, MaximumStreams)
	for index := 0; index < MaximumStreams; index++ {
		release, err := registry.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	if _, err := registry.Acquire(); ErrorCodeOf(err) != CodeLimitExceeded {
		t.Fatalf("ninth stream = %v", err)
	}
	releases[0]()
	releases[0]()
	if _, err := registry.Acquire(); err != nil {
		t.Fatalf("released slot unavailable: %v", err)
	}
}
