package dashboard

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NormalizedEvent removes the differences between core/v1.involvedObject and
// events.k8s.io/v1.regarding while retaining the exact identity needed for
// safe Pod correlation.
type NormalizedEvent struct {
	UID           types.UID
	Namespace     string
	RegardingKind string
	RegardingName string
	RegardingUID  types.UID
	Type          string
	Reason        string
	Message       string
	Count         int64
	Source        string
	ObservedAt    time.Time
}

func NormalizeCoreEvent(event corev1.Event) NormalizedEvent {
	observed := firstEventTime(
		microTime(event.EventTime),
		coreSeriesTime(event.Series),
		metaTime(event.LastTimestamp),
		metaTime(event.CreationTimestamp),
	)
	count := int64(event.Count)
	if event.Series != nil {
		count = int64(event.Series.Count)
	}
	if count < 0 {
		count = 0
	}
	source := event.ReportingController
	if source == "" {
		source = event.Source.Component
	}
	return NormalizedEvent{
		UID:           event.UID,
		Namespace:     event.Namespace,
		RegardingKind: event.InvolvedObject.Kind,
		RegardingName: event.InvolvedObject.Name,
		RegardingUID:  event.InvolvedObject.UID,
		Type:          event.Type,
		Reason:        event.Reason,
		Message:       event.Message,
		Count:         count,
		Source:        source,
		ObservedAt:    observed,
	}
}

func NormalizeEventsV1(event eventsv1.Event) NormalizedEvent {
	observed := firstEventTime(
		microTime(event.EventTime),
		eventsSeriesTime(event.Series),
		metaTime(event.DeprecatedLastTimestamp),
		metaTime(event.CreationTimestamp),
	)
	count := int64(event.DeprecatedCount)
	if event.Series != nil {
		count = int64(event.Series.Count)
	}
	if count < 0 {
		count = 0
	}
	source := event.ReportingController
	if source == "" {
		source = event.DeprecatedSource.Component
	}
	return NormalizedEvent{
		UID:           event.UID,
		Namespace:     event.Namespace,
		RegardingKind: event.Regarding.Kind,
		RegardingName: event.Regarding.Name,
		RegardingUID:  event.Regarding.UID,
		Type:          event.Type,
		Reason:        event.Reason,
		Message:       event.Note,
		Count:         count,
		Source:        source,
		ObservedAt:    observed,
	}
}

func microTime(value metav1.MicroTime) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.Time
}

func metaTime(value metav1.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.Time
}

func coreSeriesTime(series *corev1.EventSeries) time.Time {
	if series == nil {
		return time.Time{}
	}
	return microTime(series.LastObservedTime)
}

func eventsSeriesTime(series *eventsv1.EventSeries) time.Time {
	if series == nil {
		return time.Time{}
	}
	return microTime(series.LastObservedTime)
}

func firstEventTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}

// EventGroupKey is the documented grouping identity. Message is part of the
// key so distinct Kubernetes diagnoses are never collapsed.
type EventGroupKey struct {
	Namespace     string
	RegardingKind string
	RegardingName string
	RegardingUID  types.UID
	Type          string
	Reason        string
	Message       string
	Source        string
}

func GroupWarningEvents(events []NormalizedEvent) []EventDTO {
	type aggregate struct {
		key      EventGroupKey
		uid      types.UID
		observed time.Time
		count    int64
	}
	groups := make(map[EventGroupKey]aggregate)
	for _, event := range events {
		if !strings.EqualFold(event.Type, "Warning") {
			continue
		}
		key := EventGroupKey{
			Namespace:     event.Namespace,
			RegardingKind: event.RegardingKind,
			RegardingName: event.RegardingName,
			RegardingUID:  event.RegardingUID,
			Type:          "Warning",
			Reason:        event.Reason,
			Message:       event.Message,
			Source:        event.Source,
		}
		current := groups[key]
		current.key = key
		if event.ObservedAt.After(current.observed) || (event.ObservedAt.Equal(current.observed) && string(event.UID) < string(current.uid)) {
			current.observed = event.ObservedAt
			current.uid = event.UID
		}
		if event.Count > 0 {
			if current.count > math.MaxInt64-event.Count {
				current.count = math.MaxInt64
			} else {
				current.count += event.Count
			}
		}
		groups[key] = current
	}
	result := make([]EventDTO, 0, len(groups))
	ordering := make([]aggregate, 0, len(groups))
	for _, item := range groups {
		ordering = append(ordering, item)
	}
	sort.Slice(ordering, func(left, right int) bool {
		if !ordering[left].observed.Equal(ordering[right].observed) {
			return ordering[left].observed.After(ordering[right].observed)
		}
		if ordering[left].key.Namespace != ordering[right].key.Namespace {
			return ordering[left].key.Namespace < ordering[right].key.Namespace
		}
		return string(ordering[left].uid) < string(ordering[right].uid)
	})
	for _, item := range ordering {
		var timestamp *string
		if !item.observed.IsZero() {
			formatted := item.observed.UTC().Format(time.RFC3339Nano)
			timestamp = &formatted
		}
		result = append(result, EventDTO{
			Timestamp:  timestamp,
			Namespace:  item.key.Namespace,
			ObjectKind: sanitizeText(item.key.RegardingKind, MaximumStatusBytes),
			ObjectName: sanitizeText(item.key.RegardingName, MaximumStatusBytes),
			Reason:     sanitizeText(item.key.Reason, MaximumStatusBytes),
			Message:    sanitizeText(item.key.Message, MaximumProblemText),
			Count:      item.count,
			Source:     stringPointer(sanitizeText(item.key.Source, MaximumStatusBytes)),
			Type:       "Warning",
		})
	}
	return result
}

type EventService struct {
	port   EventPort
	budget QueryBudget
}

func NewEventService(port EventPort, budget QueryBudget) *EventService {
	return &EventService{port: port, budget: budget.normalized()}
}

func (s *EventService) Warnings(ctx context.Context, selection Selection) DashboardBlockDTO[[]EventDTO] {
	namespaces := canonicalNamespaces(selection.Namespaces)
	block := blockWithValue(make([]EventDTO, 0), emptyCoverage(len(namespaces)))
	if s == nil || s.port == nil {
		addBlockError(&block, "", NewFeatureUnavailableError())
		return block
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	all := make([]NormalizedEvent, 0)
	for _, namespace := range namespaces {
		remaining := s.budget.MaxItems - len(all)
		if remaining <= 0 {
			block.Truncated = true
			block.Complete = false
			break
		}
		items, complete, truncated, err := s.loadNamespace(requestContext, namespace, remaining)
		all = append(all, items...)
		if err != nil {
			addBlockError(&block, namespace, err)
			continue
		}
		if complete {
			block.Coverage.CompletedNamespaces++
		}
		if truncated {
			block.Truncated = true
			block.Complete = false
		}
	}
	block.Value = GroupWarningEvents(all)
	return block
}

func (s *EventService) loadNamespace(ctx context.Context, namespace string, maximumItems int) ([]NormalizedEvent, bool, bool, error) {
	result := make([]NormalizedEvent, 0)
	continuation := ""
	for page := 0; page < s.budget.MaxPages; page++ {
		if err := ctx.Err(); err != nil {
			return result, false, false, err
		}
		response, err := s.port.ListEvents(ctx, namespace, PageRequest{Limit: s.budget.PageSize, Continue: continuation})
		if err != nil {
			return result, false, false, err
		}
		remaining := maximumItems - len(result)
		if remaining <= 0 {
			return result, false, true, nil
		}
		if len(response.Items) > remaining {
			result = append(result, response.Items[:remaining]...)
			return result, false, true, nil
		}
		result = append(result, response.Items...)
		continuation = response.Continue
		if continuation == "" {
			return result, true, false, nil
		}
	}
	return result, false, continuation != "", nil
}
