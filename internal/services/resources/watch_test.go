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

func (stream *fakeWatchStream) ResultChan() <-chan WatchChange { return stream.channel }
func (stream *fakeWatchStream) Stop()                          { stream.once.Do(func() {}) }

type fakeWatchPort struct {
	mu            sync.Mutex
	lists         int
	watches       int
	snapshot      WatchSnapshot
	snapshots     []WatchSnapshot
	stream        *fakeWatchStream
	listErr       error
	watchErr      error
	watchErrs     []error
	watchVersions []string
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

func TestWatchCreationResourceExpiredResetsAndNextConnectionRelists(t *testing.T) {
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
	if event, nextErr := nextWithin(first); nextErr != nil || event.Event != "reset" || event.Reason != "resource_version_expired" || !event.RefetchRequired {
		t.Fatalf("expiry terminal=%#v err=%v", event, nextErr)
	}
	deadline := time.Now().Add(time.Second)
	for manager.SharedWatchCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	second, err := manager.Subscribe(context.Background(), podWatchKey())
	if err != nil {
		t.Fatal(err)
	}
	if event, nextErr := nextWithin(second); nextErr != nil || event.Event != "snapshot" || event.ResourceVersion != "2" {
		t.Fatalf("relisted snapshot=%#v err=%v", event, nextErr)
	}
	port.mu.Lock()
	defer port.mu.Unlock()
	if port.lists != 2 || port.watches != 2 || len(port.watchVersions) != 2 || port.watchVersions[0] != "1" || port.watchVersions[1] != "2" {
		t.Fatalf("lists=%d watches=%d versions=%v", port.lists, port.watches, port.watchVersions)
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
