package resources

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
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
	DefaultWatchIdleTimeout  = 45 * time.Second
	InitialWatchSyncTimeout  = 10 * time.Second
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
	// EffectiveOrigins contains the canonical scope resolution used to build
	// this watch. It prevents sharing when the named scope is unchanged but its
	// resolved namespace set differs.
	EffectiveOrigins []string
}

func (key WatchKey) identity() string {
	return key.Generation + "\x00" + key.Context + "\x00" + key.Scope + "\x00" + string(key.Topic) + "\x00" + key.GVR.String() + "\x00" + key.Namespace + "\x00" + key.Selector + "\x00" + strings.Join(canonicalCacheStrings(key.EffectiveOrigins), "\x1e")
}

type WatchSnapshot struct {
	ResourceVersion string
	Items           []TopicObject
}
type WatchChange struct {
	ReceivedAt       time.Time
	Type             string
	ResourceVersion  string
	Object           TopicObject
	Deleted          *ResourceRef
	InitialEventsEnd bool
	Err              error
}
type WatchStream interface {
	ResultChan() <-chan WatchChange
	Stop()
}
type WatchPort interface {
	List(context.Context, WatchKey) (WatchSnapshot, error)
	Watch(context.Context, WatchKey, string, int64, bool) (WatchStream, error)
}

// InitialWatchPort is optional. Clusters that do not implement streaming
// lists continue to use the classic LIST+WATCH path.
type InitialWatchPort interface {
	WatchInitial(context.Context, WatchKey, int64) (WatchStream, error)
}

var ErrStreamingListsUnsupported = errors.New("streaming lists unsupported")

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
	metrics              *observability.Registry
	port                 WatchPort
	cache                *ResourceCache
	idleTimeout          time.Duration
	streamingLists       bool
	streamingUnsupported map[string]bool
	onChange             func(WatchKey)
	mu                   sync.Mutex
	workers              map[string]*watchWorker
	closed               bool
}
type watchWorker struct {
	manager           *WatchManager
	key               WatchKey
	ctx               context.Context
	cancel            context.CancelFunc
	subscribers       map[*Subscription]struct{}
	initial           []StreamEvent
	snapshot          WatchSnapshot
	snapshotReady     bool
	connected         bool
	cacheFresh        bool
	cacheSubscription *CacheSubscription
	idleTimer         *time.Timer
	stopping          bool
}

type WatchManagerConfig struct {
	Metrics        *observability.Registry
	Cache          *ResourceCache
	IdleTimeout    time.Duration
	StreamingLists bool
	OnChange       func(WatchKey)
}

func NewWatchManager(port WatchPort) *WatchManager {
	return NewWatchManagerWithMetrics(port, nil)
}

func NewWatchManagerWithMetrics(port WatchPort, metrics *observability.Registry) *WatchManager {
	return NewWatchManagerWithConfig(port, WatchManagerConfig{Metrics: metrics})
}

func NewWatchManagerWithConfig(port WatchPort, config WatchManagerConfig) *WatchManager {
	if config.Cache == nil {
		config.Cache = NewResourceCache(ResourceCacheConfig{Metrics: config.Metrics})
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = DefaultWatchIdleTimeout
	}
	return &WatchManager{
		port: port, workers: map[string]*watchWorker{}, metrics: config.Metrics,
		cache: config.Cache, idleTimeout: config.IdleTimeout,
		streamingLists: config.StreamingLists, streamingUnsupported: make(map[string]bool),
		onChange: config.OnChange,
	}
}
func (manager *WatchManager) Subscribe(ctx context.Context, key WatchKey) (*Subscription, error) {
	if ctx == nil {
		ctx = context.TODO()
	}
	if manager == nil || manager.port == nil {
		return nil, domainError(CodeFeatureUnavailable, "Resource watches are unavailable.", nil)
	}
	if key.Generation == "" || key.Context == "" || key.Scope == "" {
		return nil, validationError("watch binding is incomplete")
	}
	if !slices.Contains(topicGVRs[key.Topic], key.GVR) {
		return nil, validationError("watch topic and resource do not match")
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
		workerContext, cancel := context.WithCancel(observability.DetachedTracing(ctx))
		cacheSubscription, cacheErr := manager.cache.Subscribe(workerContext, ResourceCacheKey{WatchKey: key})
		if cacheErr != nil {
			cancel()
			manager.mu.Unlock()
			return nil, cacheErr
		}
		worker = &watchWorker{
			manager: manager, key: key, ctx: workerContext, cancel: cancel,
			subscribers: map[*Subscription]struct{}{}, cacheSubscription: cacheSubscription,
		}
		if cached, ok := cacheSubscription.Load(ctx); ok && cached.State != CacheStateExpired {
			events, snapshotErr := SnapshotEvents(key.Generation, key.Topic, cached.Snapshot)
			if snapshotErr == nil {
				worker.snapshot = cached.Snapshot
				worker.snapshotReady = true
				worker.cacheFresh = cached.State == CacheStateFresh
				worker.initial = events
			} else {
				if invalidateErr := manager.cache.Invalidate(ResourceCacheKey{WatchKey: key}); invalidateErr != nil {
					cacheSubscription.Close()
					cancel()
					manager.mu.Unlock()
					return nil, invalidateErr
				}
			}
		}
		manager.workers[identity] = worker
		created = true
	}
	if worker.idleTimer != nil {
		worker.idleTimer.Stop()
		worker.idleTimer = nil
	}
	if worker.snapshotReady && len(worker.initial) == 0 {
		events, snapshotErr := SnapshotEvents(key.Generation, key.Topic, worker.snapshot)
		if snapshotErr != nil {
			manager.mu.Unlock()
			return nil, snapshotErr
		}
		worker.initial = events
	}
	worker.subscribers[subscription] = struct{}{}
	initial := append([]StreamEvent(nil), worker.initial...)
	subscription.closeFn = func() { worker.remove(subscription) }
	for _, event := range initial {
		if !subscription.push(event) {
			subscription.forceTerminal(StreamEvent{Event: "reset", Topic: key.Topic, Generation: key.Generation, Reason: "slow_consumer", RefetchRequired: true})
			manager.mu.Unlock()
			worker.remove(subscription)
			return subscription, nil
		}
	}
	manager.mu.Unlock()
	if created {
		go worker.run()
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
	manager.cache.InvalidateGeneration(generation)
	manager.mu.Lock()
	for key := range manager.streamingUnsupported {
		if strings.HasPrefix(key, generation+"\x00") {
			delete(manager.streamingUnsupported, key)
		}
	}
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

// Covers reports whether every Kubernetes origin of a page has a live local
// snapshot. Pages without this coverage must revalidate through LIST so a
// manual refresh cannot be answered by an unwatched stale page cache.
func (manager *WatchManager) Covers(selection Selection, topic Topic, origins []Origin) bool {
	if manager == nil || len(origins) == 0 {
		return false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for _, origin := range origins {
		if manager.coveringWorkerLocked(selection, topic, origin) == nil {
			return false
		}
	}
	return true
}

// SnapshotsFor returns one current, connected snapshot for every origin under
// the exact resolved selection. Callers must still reauthorize every origin;
// watch coverage alone never grants LIST permission.
func (manager *WatchManager) SnapshotsFor(selection Selection, topic Topic, origins []Origin) (map[string]WatchSnapshot, bool) {
	if manager == nil || len(origins) == 0 {
		return nil, false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	snapshots := make(map[string]WatchSnapshot, len(origins))
	for _, origin := range origins {
		worker := manager.coveringWorkerLocked(selection, topic, origin)
		if worker == nil {
			return nil, false
		}
		snapshots[origin.Key()] = WatchSnapshot{ResourceVersion: worker.snapshot.ResourceVersion, Items: append([]TopicObject(nil), worker.snapshot.Items...)}
	}
	return snapshots, true
}

func (manager *WatchManager) coveringWorkerLocked(selection Selection, topic Topic, origin Origin) *watchWorker {
	for _, worker := range manager.workers {
		key := worker.key
		if worker.stopping || !worker.connected || !worker.snapshotReady || key.Selector != "" || key.Generation != selection.Generation || key.Context != selection.Context || key.Scope != selection.Scope || key.Topic != topic || !slices.Equal(canonicalCacheStrings(key.EffectiveOrigins), canonicalCacheStrings(selection.Namespaces)) {
			continue
		}
		if key.GVR.Group == origin.APIGroup && key.GVR.Version == origin.Version && key.GVR.Resource == origin.Resource && key.Namespace == origin.Namespace {
			return worker
		}
	}
	return nil
}

func (worker *watchWorker) run() {
	defer worker.finish()
	labels := map[string]string{"resource": string(worker.key.Topic)}
	metrics := worker.manager.metrics
	metrics.AddGauge(observability.WatchActiveName, labels, 1)
	defer metrics.AddGauge(observability.WatchActiveName, labels, -1)
	rv := worker.snapshot.ResourceVersion
	var initialStream WatchStream
	if !worker.snapshotReady {
		initialStream, rv = worker.tryInitialWatch()
	}
	if !worker.cacheFresh || rv == "" {
		if initialStream == nil {
			var err error
			rv, err = worker.relist()
			if err != nil {
				worker.fail(err)
				return
			}
		}
	}
	backoff := 250 * time.Millisecond
	attempted := false
	for {
		spanName := "watch.connect"
		if attempted {
			spanName = "watch.reconnect"
			metrics.IncCounter(observability.WatchReconnectsTotalName, labels)
		}
		attempted = true
		var stream WatchStream
		var watchErr error
		if initialStream != nil {
			stream, initialStream = initialStream, nil
		} else {
			_, endConnect := observability.StartSpan(worker.ctx, spanName)
			stream, watchErr = worker.manager.port.Watch(worker.ctx, worker.key, rv, WatchTimeoutSeconds, true)
			endConnect(watchErr)
		}
		if watchErr != nil {
			if errors.Is(watchErr, ErrResourceExpired) {
				metrics.IncCounter(observability.WatchExpiredTotalName, labels)
				if invalidateErr := worker.invalidateCache(); invalidateErr != nil {
					worker.fail(invalidateErr)
					return
				}
				worker.broadcast(StreamEvent{Event: "refreshed", Topic: worker.key.Topic, Generation: worker.key.Generation, Reason: "resource_version_expired", RefetchRequired: true})
				rv, watchErr = worker.relist()
				if watchErr != nil {
					worker.fail(watchErr)
					return
				}
				backoff = 250 * time.Millisecond
				continue
			}
			code := ErrorCodeOf(sanitizePortError(watchErr))
			if code == CodeForbidden || code == CodeAuthorizationUnavailable {
				if invalidateErr := worker.invalidateCache(); invalidateErr != nil {
					worker.fail(invalidateErr)
					return
				}
				worker.fail(watchErr)
				return
			}
			if !worker.wait(jitterDuration(backoff)) {
				return
			}
			backoff = min(backoff*2, 10*time.Second)
			continue
		}
		worker.setConnected(true)
		stable := time.NewTimer(60 * time.Second)
		for {
			select {
			case <-worker.ctx.Done():
				worker.setConnected(false)
				stable.Stop()
				stream.Stop()
				return
			case <-stable.C:
				backoff = 250 * time.Millisecond
				stable.Reset(60 * time.Second)
			case change, ok := <-stream.ResultChan():
				if !ok {
					worker.setConnected(false)
					stable.Stop()
					stream.Stop()
					if !worker.wait(jitterDuration(backoff)) {
						return
					}
					backoff = min(backoff*2, 10*time.Second)
					goto reconnect
				}
				if errors.Is(change.Err, ErrResourceExpired) {
					worker.setConnected(false)
					metrics.IncCounter(observability.WatchExpiredTotalName, labels)
					stable.Stop()
					stream.Stop()
					if invalidateErr := worker.invalidateCache(); invalidateErr != nil {
						worker.fail(invalidateErr)
						return
					}
					worker.broadcast(StreamEvent{Event: "refreshed", Topic: worker.key.Topic, Generation: worker.key.Generation, Reason: "resource_version_expired", RefetchRequired: true})
					var relistErr error
					rv, relistErr = worker.relist()
					if relistErr != nil {
						worker.fail(relistErr)
						return
					}
					backoff = 250 * time.Millisecond
					goto reconnect
				}
				if change.Err != nil {
					worker.setConnected(false)
					stable.Stop()
					stream.Stop()
					code := ErrorCodeOf(sanitizePortError(change.Err))
					if code == CodeForbidden || code == CodeAuthorizationUnavailable {
						if invalidateErr := worker.invalidateCache(); invalidateErr != nil {
							worker.fail(invalidateErr)
							return
						}
					}
					worker.fail(change.Err)
					return
				}
				if strings.EqualFold(change.Type, "BOOKMARK") {
					if change.ResourceVersion != "" {
						rv = change.ResourceVersion
						worker.manager.mu.Lock()
						if worker.snapshotReady {
							worker.snapshot.ResourceVersion = rv
							worker.initial = nil
						}
						worker.manager.mu.Unlock()
					}
					continue
				}
				event, convertErr := watchChangeEvent(worker.key, change)
				if convertErr != nil {
					worker.setConnected(false)
					stable.Stop()
					stream.Stop()
					worker.terminal("event_too_large")
					return
				}
				if change.ResourceVersion != "" {
					rv = change.ResourceVersion
				}
				if !worker.updateSnapshot(event) {
					worker.setConnected(false)
					stable.Stop()
					stream.Stop()
					return
				}
				worker.cacheSubscription.MarkStale()
				if worker.manager.onChange != nil {
					worker.manager.onChange(worker.key)
				}
				metrics.IncCounter(observability.WatchEventsTotalName, labels)
				if !change.ReceivedAt.IsZero() {
					metrics.SetGauge(observability.WatchLagMillisecondsName, labels, max(0, time.Since(change.ReceivedAt).Milliseconds()))
				}
				if !worker.broadcast(event) {
					worker.setConnected(false)
					stable.Stop()
					stream.Stop()
					return
				}
			}
		}
	reconnect:
	}
}

func (worker *watchWorker) setConnected(connected bool) {
	worker.manager.mu.Lock()
	worker.connected = connected
	worker.manager.mu.Unlock()
	if !connected && worker.cacheSubscription != nil {
		worker.cacheSubscription.MarkStale()
	}
}

func (worker *watchWorker) tryInitialWatch() (WatchStream, string) {
	port, ok := worker.manager.port.(InitialWatchPort)
	if !ok || !worker.manager.streamingLists {
		return nil, ""
	}
	capabilityKey := worker.key.Generation + "\x00" + worker.key.GVR.String()
	worker.manager.mu.Lock()
	unsupported := worker.manager.streamingUnsupported[capabilityKey]
	worker.manager.mu.Unlock()
	if unsupported {
		return nil, ""
	}
	stream, err := port.WatchInitial(worker.ctx, worker.key, WatchTimeoutSeconds)
	if err != nil {
		worker.markInitialUnsupported(capabilityKey, err)
		return nil, ""
	}
	ready := false
	defer func() {
		if !ready {
			stream.Stop()
		}
	}()
	timer := time.NewTimer(InitialWatchSyncTimeout)
	defer timer.Stop()
	snapshot := WatchSnapshot{Items: []TopicObject{}}
	bytes := 0
	for {
		select {
		case <-worker.ctx.Done():
			return nil, ""
		case <-timer.C:
			return nil, ""
		case change, open := <-stream.ResultChan():
			if !open {
				return nil, ""
			}
			if change.Err != nil {
				worker.markInitialUnsupported(capabilityKey, change.Err)
				return nil, ""
			}
			if strings.EqualFold(change.Type, "BOOKMARK") && change.InitialEventsEnd {
				snapshot.ResourceVersion = change.ResourceVersion
				if snapshot.ResourceVersion == "" {
					return nil, ""
				}
				events, snapshotErr := SnapshotEvents(worker.key.Generation, worker.key.Topic, snapshot)
				if snapshotErr != nil {
					return nil, ""
				}
				token, tokenErr := worker.cacheSubscription.BeginRefresh(worker.ctx)
				if tokenErr == nil {
					if commitErr := worker.cacheSubscription.Commit(worker.ctx, token, snapshot, false); commitErr != nil {
						worker.cacheSubscription.AbortRefresh(token)
					}
				}
				worker.manager.mu.Lock()
				worker.snapshot = snapshot
				worker.snapshotReady = true
				worker.initial = append([]StreamEvent(nil), events...)
				worker.manager.mu.Unlock()
				if worker.manager.onChange != nil {
					worker.manager.onChange(worker.key)
				}
				for _, event := range events {
					if !worker.broadcast(event) {
						return nil, ""
					}
				}
				ready = true
				return stream, snapshot.ResourceVersion
			}
			if !strings.EqualFold(change.Type, "ADDED") || change.Object == nil || change.Object.resourceTopic() != worker.key.Topic {
				return nil, ""
			}
			encoded, encodeErr := json.Marshal(change.Object)
			if encodeErr != nil || len(snapshot.Items) >= MaximumSnapshotItems || bytes+len(encoded) > MaximumSnapshotBytes {
				return nil, ""
			}
			bytes += len(encoded)
			snapshot.Items = append(snapshot.Items, change.Object)
		}
	}
}

func (worker *watchWorker) markInitialUnsupported(key string, err error) {
	if !errors.Is(err, ErrStreamingListsUnsupported) {
		return
	}
	worker.manager.mu.Lock()
	worker.manager.streamingUnsupported[key] = true
	worker.manager.mu.Unlock()
}

// relist creates a new consistent LIST checkpoint after a cold start or a
// resourceVersion expiration. It never publishes a partial list.
func (worker *watchWorker) relist() (string, error) {
	token, err := worker.cacheSubscription.BeginRefresh(worker.ctx)
	if err != nil {
		return "", err
	}
	snapshot, err := worker.manager.port.List(worker.ctx, worker.key)
	if err != nil {
		worker.cacheSubscription.AbortRefresh(token)
		code := ErrorCodeOf(sanitizePortError(err))
		if code == CodeForbidden || code == CodeAuthorizationUnavailable {
			if invalidateErr := worker.invalidateCache(); invalidateErr != nil {
				return "", errors.Join(err, invalidateErr)
			}
		}
		return "", err
	}
	events, err := SnapshotEvents(worker.key.Generation, worker.key.Topic, snapshot)
	if err != nil {
		worker.cacheSubscription.AbortRefresh(token)
		return "", err
	}
	if cacheErr := worker.cacheSubscription.Commit(worker.ctx, token, snapshot, false); cacheErr != nil {
		worker.cacheSubscription.AbortRefresh(token)
		if errors.Is(cacheErr, ErrCacheWriteFenced) {
			return "", cacheErr
		}
		// Cache pressure must not break the established LIST+WATCH path.
		if invalidateErr := worker.invalidateCache(); invalidateErr != nil {
			return "", errors.Join(cacheErr, invalidateErr)
		}
	}
	worker.manager.mu.Lock()
	worker.snapshot = snapshot
	worker.snapshotReady = true
	worker.initial = append([]StreamEvent(nil), events...)
	worker.manager.mu.Unlock()
	if worker.manager.onChange != nil {
		worker.manager.onChange(worker.key)
	}
	for _, event := range events {
		if !worker.broadcast(event) {
			return "", context.Canceled
		}
	}
	return snapshot.ResourceVersion, nil
}

func (worker *watchWorker) updateSnapshot(event StreamEvent) bool {
	worker.manager.mu.Lock()
	defer worker.manager.mu.Unlock()
	if worker.stopping || !worker.snapshotReady {
		return !worker.stopping
	}
	_, end := observability.StartSpan(worker.ctx, "cache.apply_event")
	defer end(nil)
	worker.snapshot = applyStreamEventToSnapshot(worker.key.Topic, worker.snapshot, event)
	worker.initial = nil
	return true
}

func (worker *watchWorker) invalidateCache() error {
	err := worker.manager.cache.Invalidate(ResourceCacheKey{WatchKey: worker.key})
	if worker.manager.onChange != nil {
		worker.manager.onChange(worker.key)
	}
	return err
}

func applyStreamEventToSnapshot(topic Topic, snapshot WatchSnapshot, event StreamEvent) WatchSnapshot {
	result := WatchSnapshot{ResourceVersion: event.ResourceVersion, Items: append([]TopicObject(nil), snapshot.Items...)}
	if result.ResourceVersion == "" {
		result.ResourceVersion = snapshot.ResourceVersion
	}
	switch event.Event {
	case "added", "modified":
		identity, ok := topicObjectIdentity(event.Object)
		if !ok {
			return result
		}
		for index, item := range result.Items {
			if candidate, candidateOK := topicObjectIdentity(item); candidateOK && candidate == identity {
				result.Items[index] = event.Object
				return result
			}
		}
		result.Items = append(result.Items, event.Object)
	case "deleted":
		identity, ok := resourceRefIdentity(topic, event.Deleted)
		if !ok {
			return result
		}
		for index, item := range result.Items {
			if candidate, candidateOK := topicObjectIdentity(item); candidateOK && candidate == identity {
				result.Items = append(result.Items[:index], result.Items[index+1:]...)
				return result
			}
		}
	}
	return result
}

func topicObjectIdentity(object TopicObject) (string, bool) {
	switch value := object.(type) {
	case PodDTO:
		return objectIdentity("pod", value.Namespace, value.Name), true
	case *PodDTO:
		if value != nil {
			return objectIdentity("pod", value.Namespace, value.Name), true
		}
	case EventDTO:
		return eventObjectIdentity(value), true
	case *EventDTO:
		if value != nil {
			return eventObjectIdentity(*value), true
		}
	case WorkloadDTO:
		return objectIdentity(value.Kind, value.Namespace, value.Name), true
	case *WorkloadDTO:
		if value != nil {
			return objectIdentity(value.Kind, value.Namespace, value.Name), true
		}
	case ServiceDTO:
		return objectIdentity("service", value.Namespace, value.Name), true
	case *ServiceDTO:
		if value != nil {
			return objectIdentity("service", value.Namespace, value.Name), true
		}
	case IngressDTO:
		return objectIdentity("ingress", value.Namespace, value.Name), true
	case *IngressDTO:
		if value != nil {
			return objectIdentity("ingress", value.Namespace, value.Name), true
		}
	case EndpointSliceDTO:
		return objectIdentity("endpointslice", value.Namespace, value.Name), true
	case *EndpointSliceDTO:
		if value != nil {
			return objectIdentity("endpointslice", value.Namespace, value.Name), true
		}
	case ConfigMapListDTO:
		return objectIdentity("configmap", value.Namespace, value.Name), true
	case *ConfigMapListDTO:
		if value != nil {
			return objectIdentity("configmap", value.Namespace, value.Name), true
		}
	}
	return "", false
}

func eventObjectIdentity(event EventDTO) string {
	if event.Name != "" {
		return objectIdentity("event", event.Namespace, event.Name)
	}
	timestamp := ""
	if event.Timestamp != nil {
		timestamp = *event.Timestamp
	}
	return strings.Join([]string{"event", event.Namespace, event.ObjectKind, event.ObjectName, event.Reason, timestamp}, "\x00")
}

func resourceRefIdentity(topic Topic, ref *ResourceRef) (string, bool) {
	if ref == nil {
		return "", false
	}
	kind := ref.Kind
	if kind == "" {
		switch topic {
		case TopicPods:
			kind = "pod"
		case TopicServices:
			kind = "service"
		case TopicIngresses:
			kind = "ingress"
		case TopicEndpointSlices:
			kind = "endpointslice"
		case TopicConfigMaps:
			kind = "configmap"
		}
	}
	return objectIdentity(kind, ref.Namespace, ref.Name), true
}

func objectIdentity(kind, namespace, name string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "\x00" + namespace + "\x00" + name
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
	stopping := worker.stopping
	worker.manager.mu.Unlock()
	if len(subscribers) == 0 {
		return !stopping
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
	immediate := false
	if empty && worker.idleTimer == nil {
		current := worker.manager.workers[worker.key.identity()] == worker
		if !current || worker.manager.idleTimeout <= 0 {
			if current {
				delete(worker.manager.workers, worker.key.identity())
			}
			worker.stopping = true
			immediate = true
		} else {
			worker.idleTimer = time.AfterFunc(worker.manager.idleTimeout, worker.stopIfIdle)
		}
	}
	worker.manager.mu.Unlock()
	if immediate {
		if err := worker.flushCache(); err != nil && worker.cacheSubscription != nil {
			worker.cacheSubscription.MarkStale()
		}
		worker.cancel()
	}
}

func (worker *watchWorker) stopIfIdle() {
	worker.manager.mu.Lock()
	if worker.manager.workers[worker.key.identity()] != worker || len(worker.subscribers) != 0 {
		worker.idleTimer = nil
		worker.manager.mu.Unlock()
		return
	}
	worker.stopping = true
	err := worker.flushSnapshot(worker.snapshot, worker.snapshotReady)
	if (err != nil || !worker.connected) && worker.cacheSubscription != nil {
		worker.cacheSubscription.MarkStale()
	}
	delete(worker.manager.workers, worker.key.identity())
	worker.idleTimer = nil
	worker.manager.mu.Unlock()
	worker.cancel()
}

func (worker *watchWorker) flushCache() error {
	if worker.cacheSubscription == nil {
		return nil
	}
	worker.manager.mu.Lock()
	if !worker.snapshotReady {
		worker.manager.mu.Unlock()
		return nil
	}
	snapshot := worker.snapshot
	ready := worker.snapshotReady
	connected := worker.connected
	worker.manager.mu.Unlock()
	err := worker.flushSnapshot(snapshot, ready)
	if (err != nil || !connected) && worker.cacheSubscription != nil {
		worker.cacheSubscription.MarkStale()
	}
	return err
}

func (worker *watchWorker) flushSnapshot(snapshot WatchSnapshot, ready bool) error {
	if worker.cacheSubscription == nil || !ready {
		return nil
	}
	token, err := worker.cacheSubscription.BeginRefresh(worker.ctx)
	if err != nil {
		return err
	}
	if err = worker.cacheSubscription.Commit(worker.ctx, token, snapshot, false); err != nil {
		worker.cacheSubscription.AbortRefresh(token)
		return err
	}
	return nil
}

func (worker *watchWorker) finish() {
	worker.cancel()
	worker.manager.mu.Lock()
	if worker.manager.workers[worker.key.identity()] == worker {
		delete(worker.manager.workers, worker.key.identity())
	}
	worker.stopping = true
	if worker.idleTimer != nil {
		worker.idleTimer.Stop()
		worker.idleTimer = nil
	}
	subscribers := make([]*Subscription, 0, len(worker.subscribers))
	for subscription := range worker.subscribers {
		subscribers = append(subscribers, subscription)
	}
	worker.subscribers = map[*Subscription]struct{}{}
	worker.manager.mu.Unlock()
	if worker.cacheSubscription != nil {
		worker.cacheSubscription.Close()
	}
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
	if subscription.closed {
		return false
	}
	// Resource lists are level-driven: replace an undelivered delta for the
	// same object with its latest state. Event streams remain chronological.
	if identity, ok := streamEventIdentity(event); ok {
		for index := len(subscription.queue) - 1; index >= 0; index-- {
			if subscription.queue[index].event.Event == "snapshot" && subscription.queue[index].event.Topic == event.Topic {
				break
			}
			if previous, previousOK := streamEventIdentity(subscription.queue[index].event); previousOK && previous == identity {
				if subscription.bytes-subscription.queue[index].bytes+wireBytes > MaximumStreamQueueBytes {
					return false
				}
				subscription.bytes += wireBytes - subscription.queue[index].bytes
				subscription.queue[index] = queuedEvent{event: event, bytes: wireBytes}
				return true
			}
		}
	}
	if len(subscription.queue) >= MaximumStreamQueueEvents || subscription.bytes+wireBytes > MaximumStreamQueueBytes {
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

func streamEventIdentity(event StreamEvent) (string, bool) {
	if event.Topic == "" || event.Topic == TopicEvents {
		return "", false
	}
	var identity string
	var ok bool
	switch event.Event {
	case "added", "modified":
		identity, ok = topicObjectIdentity(event.Object)
	case "deleted":
		identity, ok = resourceRefIdentity(event.Topic, event.Deleted)
	}
	if !ok {
		return "", false
	}
	return string(event.Topic) + "\x00" + identity, true
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
