package resources

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	WatchTimeoutSeconds      = int64(300)
	MaximumStreams           = 8
	MaximumStreamEventBytes  = 64 << 10
	MaximumSnapshotItems     = 10000
	MaximumSnapshotBytes     = 10 << 20
	MaximumStreamQueueBytes  = 1 << 20
	MaximumStreamQueueEvents = 1000
	streamSSEEnvelopeReserve = 64
)

type Topic string

const (
	TopicPods           Topic = "pods"
	TopicEvents         Topic = "events"
	TopicWorkloads      Topic = "workloads"
	TopicServices       Topic = "services"
	TopicIngresses      Topic = "ingresses"
	TopicEndpointSlices Topic = "endpoint-slices"
	TopicConfigMaps     Topic = "configmaps"
)

var topicOrder = []Topic{TopicPods, TopicEvents, TopicWorkloads, TopicServices, TopicIngresses, TopicEndpointSlices, TopicConfigMaps}
var topicGVRs = map[Topic][]schema.GroupVersionResource{
	TopicPods: {{Group: "", Version: "v1", Resource: "pods"}}, TopicEvents: {{Group: "", Version: "v1", Resource: "events"}},
	TopicWorkloads: {{Group: "apps", Version: "v1", Resource: "deployments"}, {Group: "apps", Version: "v1", Resource: "statefulsets"}, {Group: "apps", Version: "v1", Resource: "daemonsets"}, {Group: "batch", Version: "v1", Resource: "jobs"}, {Group: "batch", Version: "v1", Resource: "cronjobs"}},
	TopicServices:  {{Group: "", Version: "v1", Resource: "services"}}, TopicIngresses: {{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}}, TopicEndpointSlices: {{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}}, TopicConfigMaps: {{Group: "", Version: "v1", Resource: "configmaps"}},
}

func ValidateTopics(values []Topic) ([]Topic, error) {
	if len(values) < 1 || len(values) > 7 {
		return nil, validationError("topic cardinality must be between 1 and 7")
	}
	seen := map[Topic]struct{}{}
	for _, value := range values {
		if _, ok := topicGVRs[value]; !ok {
			return nil, validationError("topic is not supported")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, validationError("topic must not contain duplicates")
		}
		seen[value] = struct{}{}
	}
	result := make([]Topic, 0, len(values))
	for _, value := range topicOrder {
		if _, ok := seen[value]; ok {
			result = append(result, value)
		}
	}
	return result, nil
}
func TopicGVRs(topic Topic) []schema.GroupVersionResource {
	return append([]schema.GroupVersionResource(nil), topicGVRs[topic]...)
}

// TopicObject is sealed to the seven allowlisted DTO families. SecretMetadataDTO
// intentionally cannot be placed in a watch snapshot or event.
type TopicObject interface{ resourceTopic() Topic }

func (PodDTO) resourceTopic() Topic           { return TopicPods }
func (EventDTO) resourceTopic() Topic         { return TopicEvents }
func (WorkloadDTO) resourceTopic() Topic      { return TopicWorkloads }
func (ServiceDTO) resourceTopic() Topic       { return TopicServices }
func (IngressDTO) resourceTopic() Topic       { return TopicIngresses }
func (EndpointSliceDTO) resourceTopic() Topic { return TopicEndpointSlices }
func (ConfigMapListDTO) resourceTopic() Topic { return TopicConfigMaps }

type WatchKey struct {
	Generation string
	Context    string
	Scope      string
	Topic      Topic
	GVR        schema.GroupVersionResource
	Namespace  string
	Selector   string
}

func (key WatchKey) identity() string {
	return key.Generation + "\x00" + key.Context + "\x00" + key.Scope + "\x00" + string(key.Topic) + "\x00" + key.GVR.String() + "\x00" + key.Namespace + "\x00" + key.Selector
}

type WatchSnapshot struct {
	ResourceVersion string
	Items           []TopicObject
}
type WatchChange struct {
	Type            string
	ResourceVersion string
	Object          TopicObject
	Deleted         *ResourceRef
	Err             error
}
type WatchStream interface {
	ResultChan() <-chan WatchChange
	Stop()
}
type WatchPort interface {
	List(context.Context, WatchKey) (WatchSnapshot, error)
	Watch(context.Context, WatchKey, string, int64, bool) (WatchStream, error)
}

type StreamEvent struct {
	Event           string        `json:"event"`
	Topic           Topic         `json:"topic,omitempty"`
	Generation      string        `json:"generation"`
	ResourceVersion string        `json:"resourceVersion,omitempty"`
	Items           []TopicObject `json:"items,omitempty"`
	Object          TopicObject   `json:"object,omitempty"`
	Deleted         *ResourceRef  `json:"deleted,omitempty"`
	Reason          string        `json:"reason,omitempty"`
	RefetchRequired bool          `json:"refetchRequired,omitempty"`
	Final           bool          `json:"final,omitempty"`
	Chunk           int           `json:"chunk,omitempty"`
	ObservedAt      string        `json:"observedAt,omitempty"`
}

type WatchManager struct {
	port    WatchPort
	mu      sync.Mutex
	workers map[string]*watchWorker
	closed  bool
}
type watchWorker struct {
	manager     *WatchManager
	key         WatchKey
	ctx         context.Context
	cancel      context.CancelFunc
	subscribers map[*Subscription]struct{}
	initial     []StreamEvent
}

func NewWatchManager(port WatchPort) *WatchManager {
	return &WatchManager{port: port, workers: map[string]*watchWorker{}}
}
func (manager *WatchManager) Subscribe(ctx context.Context, key WatchKey) (*Subscription, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if manager == nil || manager.port == nil {
		return nil, domainError(CodeFeatureUnavailable, "Resource watches are unavailable.", nil)
	}
	if key.Generation == "" || key.Context == "" || key.Scope == "" {
		return nil, validationError("watch binding is incomplete")
	}
	if _, ok := topicGVRs[key.Topic]; !ok {
		return nil, validationError("watch topic is invalid")
	}
	subscription := newSubscription(ctx)
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return nil, domainError(CodeFeatureUnavailable, "Resource watches are shutting down.", nil)
	}
	identity := key.identity()
	worker := manager.workers[identity]
	created := false
	if worker == nil {
		workerContext, cancel := context.WithCancel(context.Background())
		worker = &watchWorker{manager: manager, key: key, ctx: workerContext, cancel: cancel, subscribers: map[*Subscription]struct{}{}}
		manager.workers[identity] = worker
		created = true
	}
	worker.subscribers[subscription] = struct{}{}
	initial := append([]StreamEvent(nil), worker.initial...)
	subscription.closeFn = func() { worker.remove(subscription) }
	manager.mu.Unlock()
	if created {
		go worker.run()
	} else if len(initial) > 0 {
		go func() {
			for _, event := range initial {
				if !subscription.push(event) {
					subscription.forceTerminal(StreamEvent{Event: "reset", Topic: key.Topic, Generation: key.Generation, Reason: "slow_consumer", RefetchRequired: true})
					worker.remove(subscription)
					return
				}
			}
		}()
	}
	go func() {
		select {
		case <-ctx.Done():
			subscription.Close()
		case <-worker.ctx.Done():
		}
	}()
	return subscription, nil
}
func (manager *WatchManager) CancelGeneration(generation string) {
	manager.mu.Lock()
	workers := make([]*watchWorker, 0)
	for identity, worker := range manager.workers {
		if worker.key.Generation == generation {
			delete(manager.workers, identity)
			workers = append(workers, worker)
		}
	}
	manager.mu.Unlock()
	for _, worker := range workers {
		worker.terminal("generation_changed")
		worker.cancel()
	}
}
func (manager *WatchManager) Close() {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return
	}
	manager.closed = true
	workers := make([]*watchWorker, 0, len(manager.workers))
	for identity, worker := range manager.workers {
		delete(manager.workers, identity)
		workers = append(workers, worker)
	}
	manager.mu.Unlock()
	for _, worker := range workers {
		worker.terminal("server_shutdown")
		worker.cancel()
	}
}
func (manager *WatchManager) SharedWatchCount() int {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.workers)
}

func (worker *watchWorker) run() {
	defer worker.finish()
	snapshot, err := worker.manager.port.List(worker.ctx, worker.key)
	if err != nil {
		worker.fail(err)
		return
	}
	events, err := SnapshotEvents(worker.key.Generation, worker.key.Topic, snapshot)
	if err != nil {
		worker.terminal("snapshot_too_large")
		return
	}
	worker.manager.mu.Lock()
	worker.initial = append([]StreamEvent(nil), events...)
	worker.manager.mu.Unlock()
	for _, event := range events {
		if !worker.broadcast(event) {
			return
		}
	}
	rv := snapshot.ResourceVersion
	backoff := 250 * time.Millisecond
	for {
		stream, watchErr := worker.manager.port.Watch(worker.ctx, worker.key, rv, WatchTimeoutSeconds, true)
		if watchErr != nil {
			if errors.Is(watchErr, ErrResourceExpired) {
				// A 410 at watch creation means this resourceVersion can never
				// succeed. End with reset so the next connection performs a fresh
				// LIST instead of backing off around an obsolete RV forever.
				worker.terminal("resource_version_expired")
				return
			}
			code := ErrorCodeOf(sanitizePortError(watchErr))
			if code == CodeForbidden || code == CodeAuthorizationUnavailable {
				worker.fail(watchErr)
				return
			}
			if !worker.wait(jitterDuration(backoff)) {
				return
			}
			backoff = min(backoff*2, 10*time.Second)
			continue
		}
		stable := time.NewTimer(60 * time.Second)
		for {
			select {
			case <-worker.ctx.Done():
				stable.Stop()
				stream.Stop()
				return
			case <-stable.C:
				backoff = 250 * time.Millisecond
				stable.Reset(60 * time.Second)
			case change, ok := <-stream.ResultChan():
				if !ok {
					stable.Stop()
					stream.Stop()
					if !worker.wait(jitterDuration(backoff)) {
						return
					}
					backoff = min(backoff*2, 10*time.Second)
					goto reconnect
				}
				if errors.Is(change.Err, ErrResourceExpired) {
					stable.Stop()
					stream.Stop()
					worker.terminal("resource_version_expired")
					return
				}
				if change.Err != nil {
					stable.Stop()
					stream.Stop()
					worker.fail(change.Err)
					return
				}
				event, convertErr := watchChangeEvent(worker.key, change)
				if convertErr != nil {
					stable.Stop()
					stream.Stop()
					worker.terminal("event_too_large")
					return
				}
				if change.ResourceVersion != "" {
					rv = change.ResourceVersion
				}
				if !worker.broadcast(event) {
					stable.Stop()
					stream.Stop()
					return
				}
			}
		}
	reconnect:
	}
}
func (worker *watchWorker) wait(duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-worker.ctx.Done():
		return false
	}
}
func (worker *watchWorker) broadcast(event StreamEvent) bool {
	worker.manager.mu.Lock()
	subscribers := make([]*Subscription, 0, len(worker.subscribers))
	for subscription := range worker.subscribers {
		subscribers = append(subscribers, subscription)
	}
	worker.manager.mu.Unlock()
	if len(subscribers) == 0 {
		return false
	}
	for _, subscription := range subscribers {
		if !subscription.push(event) {
			subscription.forceTerminal(StreamEvent{Event: "reset", Topic: worker.key.Topic, Generation: worker.key.Generation, Reason: "slow_consumer", RefetchRequired: true})
			worker.remove(subscription)
		}
	}
	return true
}
func (worker *watchWorker) terminal(reason string) {
	worker.broadcast(StreamEvent{Event: "reset", Topic: worker.key.Topic, Generation: worker.key.Generation, Reason: reason, RefetchRequired: true})
}
func (worker *watchWorker) fail(err error) {
	code := ErrorCodeOf(sanitizePortError(err))
	worker.broadcast(StreamEvent{Event: "error", Topic: worker.key.Topic, Generation: worker.key.Generation, Reason: string(code), RefetchRequired: true})
}
func (worker *watchWorker) remove(subscription *Subscription) {
	worker.manager.mu.Lock()
	delete(worker.subscribers, subscription)
	empty := len(worker.subscribers) == 0
	if empty {
		delete(worker.manager.workers, worker.key.identity())
	}
	worker.manager.mu.Unlock()
	if empty {
		worker.cancel()
	}
}
func (worker *watchWorker) finish() {
	worker.manager.mu.Lock()
	if worker.manager.workers[worker.key.identity()] == worker {
		delete(worker.manager.workers, worker.key.identity())
	}
	subscribers := make([]*Subscription, 0, len(worker.subscribers))
	for subscription := range worker.subscribers {
		subscribers = append(subscribers, subscription)
	}
	worker.subscribers = map[*Subscription]struct{}{}
	worker.manager.mu.Unlock()
	for _, subscription := range subscribers {
		subscription.Close()
	}
}

type Subscription struct {
	ctx     context.Context
	mu      sync.Mutex
	signal  chan struct{}
	queue   []queuedEvent
	bytes   int
	closed  bool
	closeFn func()
}
type queuedEvent struct {
	event StreamEvent
	bytes int
}

func newSubscription(ctx context.Context) *Subscription {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Subscription{ctx: ctx, signal: make(chan struct{}, 1), queue: []queuedEvent{}}
}
func (subscription *Subscription) Next(ctx context.Context) (StreamEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		subscription.mu.Lock()
		if len(subscription.queue) > 0 {
			item := subscription.queue[0]
			subscription.queue = subscription.queue[1:]
			subscription.bytes -= item.bytes
			subscription.mu.Unlock()
			return item.event, nil
		}
		closed := subscription.closed
		subscription.mu.Unlock()
		if closed {
			return StreamEvent{}, io.EOF
		}
		select {
		case <-ctx.Done():
			return StreamEvent{}, ctx.Err()
		case <-subscription.ctx.Done():
			subscription.Close()
			return StreamEvent{}, subscription.ctx.Err()
		case <-subscription.signal:
		}
	}
}
func (subscription *Subscription) push(event StreamEvent) bool {
	encoded, _ := json.Marshal(event)
	wireBytes := len(encoded) + streamSSEEnvelopeReserve
	if wireBytes > MaximumStreamEventBytes {
		return false
	}
	subscription.mu.Lock()
	defer subscription.mu.Unlock()
	if subscription.closed || len(subscription.queue) >= MaximumStreamQueueEvents || subscription.bytes+wireBytes > MaximumStreamQueueBytes {
		return false
	}
	subscription.queue = append(subscription.queue, queuedEvent{event: event, bytes: wireBytes})
	subscription.bytes += wireBytes
	select {
	case subscription.signal <- struct{}{}:
	default:
	}
	return true
}
func (subscription *Subscription) forceTerminal(event StreamEvent) {
	encoded, _ := json.Marshal(event)
	wireBytes := len(encoded) + streamSSEEnvelopeReserve
	subscription.mu.Lock()
	if subscription.closed {
		subscription.mu.Unlock()
		return
	}
	subscription.queue = []queuedEvent{{event: event, bytes: wireBytes}}
	subscription.bytes = wireBytes
	subscription.closed = true
	select {
	case subscription.signal <- struct{}{}:
	default:
	}
	subscription.mu.Unlock()
}
func (subscription *Subscription) Close() {
	subscription.mu.Lock()
	if subscription.closed {
		subscription.mu.Unlock()
		return
	}
	subscription.closed = true
	closeFn := subscription.closeFn
	subscription.closeFn = nil
	select {
	case subscription.signal <- struct{}{}:
	default:
	}
	subscription.mu.Unlock()
	if closeFn != nil {
		closeFn()
	}
}

func SnapshotEvents(generation string, topic Topic, snapshot WatchSnapshot) ([]StreamEvent, error) {
	if len(snapshot.Items) > MaximumSnapshotItems {
		return nil, domainError(CodeLimitExceeded, "The initial snapshot is too large.", nil)
	}
	for _, item := range snapshot.Items {
		if item == nil || item.resourceTopic() != topic {
			return nil, validationError("snapshot contains an object for another topic")
		}
	}
	whole, _ := json.Marshal(snapshot.Items)
	if len(whole) > MaximumSnapshotBytes {
		return nil, domainError(CodeLimitExceeded, "The initial snapshot is too large.", nil)
	}
	events := []StreamEvent{}
	chunk := []TopicObject{}
	for _, item := range snapshot.Items {
		candidate := append(append([]TopicObject(nil), chunk...), item)
		event := StreamEvent{Event: "snapshot", Topic: topic, Generation: generation, ResourceVersion: snapshot.ResourceVersion, Items: candidate, Chunk: len(events)}
		encoded, _ := json.Marshal(event)
		if len(encoded) > MaximumStreamEventBytes-512 {
			if len(chunk) == 0 {
				return nil, domainError(CodeLimitExceeded, "A snapshot object exceeds the event limit.", nil)
			}
			events = append(events, StreamEvent{Event: "snapshot", Topic: topic, Generation: generation, ResourceVersion: snapshot.ResourceVersion, Items: chunk, Chunk: len(events)})
			chunk = []TopicObject{item}
			single := StreamEvent{Event: "snapshot", Topic: topic, Generation: generation, ResourceVersion: snapshot.ResourceVersion, Items: chunk, Chunk: len(events), Final: true}
			singleBytes, _ := json.Marshal(single)
			if len(singleBytes)+streamSSEEnvelopeReserve > MaximumStreamEventBytes {
				return nil, domainError(CodeLimitExceeded, "A snapshot object exceeds the event limit.", nil)
			}
			continue
		}
		chunk = candidate
	}
	events = append(events, StreamEvent{Event: "snapshot", Topic: topic, Generation: generation, ResourceVersion: snapshot.ResourceVersion, Items: chunk, Chunk: len(events), Final: true})
	return events, nil
}
func watchChangeEvent(key WatchKey, change WatchChange) (StreamEvent, error) {
	eventType := strings.ToLower(change.Type)
	if eventType != "added" && eventType != "modified" && eventType != "deleted" {
		return StreamEvent{}, validationError("watch event type is invalid")
	}
	event := StreamEvent{Event: eventType, Topic: key.Topic, Generation: key.Generation, ResourceVersion: change.ResourceVersion, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if eventType == "deleted" {
		if change.Deleted == nil {
			return StreamEvent{}, validationError("deleted watch event requires a reference")
		}
		event.Deleted = change.Deleted
	} else {
		if change.Object == nil || change.Object.resourceTopic() != key.Topic {
			return StreamEvent{}, validationError("watch object does not match its topic")
		}
		event.Object = change.Object
	}
	encoded, _ := json.Marshal(event)
	if len(encoded)+streamSSEEnvelopeReserve > MaximumStreamEventBytes {
		return StreamEvent{}, domainError(CodeLimitExceeded, "The watch event exceeds the event limit.", nil)
	}
	return event, nil
}

func AuthorizeTopics(ctx context.Context, checker AuthorizationChecker, selection Selection, topics []Topic) error {
	return authorizeTopics(ctx, checker, selection, topics, false)
}

// ReauthorizeTopics repeats the all-or-nothing stream capability matrix and,
// when available, bypasses the authorization cache through Refresh.
func ReauthorizeTopics(ctx context.Context, checker AuthorizationChecker, selection Selection, topics []Topic) error {
	return authorizeTopics(ctx, checker, selection, topics, true)
}

func authorizeTopics(ctx context.Context, checker AuthorizationChecker, selection Selection, topics []Topic, refresh bool) error {
	canonical, err := ValidateTopics(topics)
	if err != nil {
		return err
	}
	if checker == nil {
		return domainError(CodeFeatureUnavailable, "Resource authorization is unavailable.", nil)
	}
	if selection.Generation == "" || len(selection.Namespaces) == 0 {
		return validationError("stream selection is incomplete")
	}
	for _, topic := range canonical {
		for _, gvr := range topicGVRs[topic] {
			for _, namespace := range selection.Namespaces {
				for _, verb := range []string{"list", "watch"} {
					capability := authorizationCapability(ctx, checker, authorization.Key{Generation: selection.Generation, Namespace: namespace, APIGroup: gvr.Group, Resource: gvr.Resource, Verb: verb}, refresh)
					switch capability.Decision {
					case authorization.DecisionAllowed:
					case authorization.DecisionDenied:
						return domainError(CodeForbidden, "Access to the requested resource stream was denied.", nil)
					default:
						return domainError(CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
					}
				}
			}
		}
	}
	return nil
}

type StreamRegistry struct {
	mu     sync.Mutex
	active int
}

func (registry *StreamRegistry) Acquire() (func(), error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.active >= MaximumStreams {
		return nil, domainError(CodeLimitExceeded, "The stream limit was reached.", nil)
	}
	registry.active++
	once := sync.Once{}
	return func() { once.Do(func() { registry.mu.Lock(); registry.active--; registry.mu.Unlock() }) }, nil
}

type ReplayRing struct {
	mu       sync.Mutex
	epoch    string
	binding  string
	sequence uint64
	events   []ringEvent
	bytes    int
}
type ringEvent struct {
	id    string
	event StreamEvent
	bytes int
}

// ReplayEntry preserves the opaque wire identifier alongside the event so an
// HTTP reconnect can replay the exact resumable sequence. The byte accounting
// remains private to ReplayRing.
type ReplayEntry struct {
	ID    string
	Event StreamEvent
}

var ErrResumeUnavailable = errors.New("resources: SSE resume unavailable")

func NewReplayRing(instance, generation string, topics []Topic) (*ReplayRing, error) {
	binding, err := ReplayBinding(instance, generation, topics)
	if err != nil {
		return nil, err
	}
	epochBytes := make([]byte, 16)
	if _, err = rand.Read(epochBytes); err != nil {
		return nil, fmt.Errorf("resources: generate stream epoch: %w", err)
	}
	return &ReplayRing{epoch: base64.RawURLEncoding.EncodeToString(epochBytes), binding: binding}, nil
}

func ReplayBinding(instance, generation string, topics []Topic) (string, error) {
	if instance == "" || generation == "" {
		return "", validationError("stream replay binding is incomplete")
	}
	canonical, err := ValidateTopics(topics)
	if err != nil {
		return "", err
	}
	names := make([]string, len(canonical))
	for i := range canonical {
		names[i] = string(canonical[i])
	}
	return instance + "\x00" + generation + "\x00" + strings.Join(names, ","), nil
}

func (ring *ReplayRing) Append(event StreamEvent) (string, error) {
	if event.Event == "heartbeat" || event.Event == "reset" || event.Event == "error" {
		return "", nil
	}
	encoded, _ := json.Marshal(event)
	if len(encoded)+streamSSEEnvelopeReserve > MaximumStreamEventBytes {
		return "", domainError(CodeLimitExceeded, "The stream event exceeds the event limit.", nil)
	}
	ring.mu.Lock()
	defer ring.mu.Unlock()
	ring.sequence++
	id := "kpse1." + ring.epoch + "." + strconv.FormatUint(ring.sequence, 36)
	wireBytes := len(encoded) + streamSSEEnvelopeReserve
	ring.events = append(ring.events, ringEvent{id: id, event: event, bytes: wireBytes})
	ring.bytes += wireBytes
	for len(ring.events) > MaximumStreamQueueEvents || ring.bytes > MaximumStreamQueueBytes {
		ring.bytes -= ring.events[0].bytes
		ring.events = ring.events[1:]
	}
	return id, nil
}
func (ring *ReplayRing) Replay(lastID, binding string) ([]StreamEvent, error) {
	entries, err := ring.ReplayEntries(lastID, binding)
	if err != nil {
		return nil, err
	}
	result := make([]StreamEvent, len(entries))
	for index := range entries {
		result[index] = entries[index].Event
	}
	return result, nil
}

func (ring *ReplayRing) ReplayEntries(lastID, binding string) ([]ReplayEntry, error) {
	parts := strings.Split(lastID, ".")
	if len(parts) != 3 || parts[0] != "kpse1" {
		return nil, validationError("Last-Event-ID is malformed")
	}
	epoch, decodeErr := base64.RawURLEncoding.DecodeString(parts[1])
	if decodeErr != nil || len(epoch) != 16 {
		return nil, validationError("Last-Event-ID is malformed")
	}
	if _, err := strconv.ParseUint(parts[2], 36, 64); err != nil {
		return nil, validationError("Last-Event-ID is malformed")
	}
	if parts[1] != ring.epoch {
		return nil, ErrResumeUnavailable
	}
	ring.mu.Lock()
	defer ring.mu.Unlock()
	if binding != ring.binding {
		return nil, ErrResumeUnavailable
	}
	index := -1
	for i := range ring.events {
		if ring.events[i].id == lastID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, ErrResumeUnavailable
	}
	result := make([]ReplayEntry, 0, len(ring.events)-index-1)
	for _, item := range ring.events[index+1:] {
		result = append(result, ReplayEntry{ID: item.id, Event: item.event})
	}
	return result, nil
}
func (ring *ReplayRing) Binding() string { return ring.binding }
func (ring *ReplayRing) Epoch() string   { return ring.epoch }

func jitterDuration(duration time.Duration) time.Duration {
	var sample [1]byte
	if _, err := rand.Read(sample[:]); err != nil {
		return duration
	}
	// Uniformly map 0..255 to 80%..120%.
	factor := 0.8 + (float64(sample[0])/255.0)*0.4
	return time.Duration(float64(duration) * factor)
}
