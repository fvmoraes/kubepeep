package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strconv"
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

// GroupEvents normalizes the Kubernetes event type to the closed public enum
// and groups only events with the complete documented identity. The private
// cursor identity retains the UIDs which are deliberately omitted from the
// public DTO, so two otherwise equal events remain distinct while paginating.
func GroupEvents(events []NormalizedEvent) []EventDTO {
	return groupEvents(events, false)
}

func GroupWarningEvents(events []NormalizedEvent) []EventDTO {
	return groupEvents(events, true)
}

func groupEvents(events []NormalizedEvent, warningsOnly bool) []EventDTO {
	type aggregate struct {
		key      EventGroupKey
		uid      types.UID
		observed time.Time
		count    int64
	}
	groups := make(map[EventGroupKey]aggregate)
	for _, event := range events {
		eventType := normalizeEventType(event.Type)
		if warningsOnly && eventType != "Warning" {
			continue
		}
		key := EventGroupKey{
			Namespace:     event.Namespace,
			RegardingKind: event.RegardingKind,
			RegardingName: event.RegardingName,
			RegardingUID:  event.RegardingUID,
			Type:          eventType,
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
		cursorTimestamp := ""
		if !item.observed.IsZero() {
			formatted := item.observed.UTC().Format(time.RFC3339Nano)
			timestamp = &formatted
			cursorTimestamp = formatted
		}
		identityHash := sha256.Sum256([]byte(strings.Join([]string{
			item.key.RegardingKind,
			item.key.RegardingName,
			string(item.key.RegardingUID),
			item.key.Type,
			item.key.Reason,
			item.key.Message,
			item.key.Source,
		}, "\x00")))
		result = append(result, EventDTO{
			Timestamp:  timestamp,
			Namespace:  item.key.Namespace,
			ObjectKind: sanitizeText(item.key.RegardingKind, MaximumStatusBytes),
			ObjectName: sanitizeText(item.key.RegardingName, MaximumStatusBytes),
			Reason:     sanitizeText(item.key.Reason, MaximumStatusBytes),
			Message:    sanitizeText(item.key.Message, MaximumProblemText),
			Count:      item.count,
			Source:     stringPointer(sanitizeText(item.key.Source, MaximumStatusBytes)),
			Type:       item.key.Type,
			cursorIdentity: strings.Join([]string{
				cursorTimestamp, item.key.Namespace, string(item.uid), hex.EncodeToString(identityHash[:]),
			}, "\x00"),
		})
	}
	return result
}

func normalizeEventType(value string) string {
	switch {
	case strings.EqualFold(value, "Normal"):
		return "Normal"
	case strings.EqualFold(value, "Warning"):
		return "Warning"
	default:
		return "Unknown"
	}
}

// EventCursorIdentity returns a stable internal merge key. UIDs never enter
// the EventDTO JSON, but remain authenticated inside the opaque cursor.
func EventCursorIdentity(value EventDTO) string {
	if value.cursorIdentity != "" {
		return value.cursorIdentity
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{
		value.ObjectKind, value.ObjectName, value.Type, value.Reason, value.Message,
		optionalEventString(value.Source), strconv.FormatInt(value.Count, 10),
	}, "\x00")))
	return strings.Join([]string{
		optionalEventString(value.Timestamp), value.Namespace, hex.EncodeToString(digest[:]),
	}, "\x00")
}

func optionalEventString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type EventService struct {
	port   EventPort
	budget QueryBudget
}

func NewEventService(port EventPort, budget QueryBudget) *EventService {
	return &EventService{port: port, budget: budget.Normalized()}
}

func (s *EventService) Warnings(ctx context.Context, selection Selection) DashboardBlockDTO[[]EventDTO] {
	return s.collect(ctx, selection, true)
}

// All returns all three public event types. Dashboard summary deliberately
// continues to call Warnings, while the event endpoint applies its own
// default-Warning or explicit Normal/Warning/Unknown filter.
func (s *EventService) All(ctx context.Context, selection Selection) DashboardBlockDTO[[]EventDTO] {
	return s.collect(ctx, selection, false)
}

func (s *EventService) collect(ctx context.Context, selection Selection, warningsOnly bool) DashboardBlockDTO[[]EventDTO] {
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
	if warningsOnly {
		block.Value = GroupWarningEvents(all)
	} else {
		block.Value = GroupEvents(all)
	}
	return block
}

func (s *EventService) loadNamespace(ctx context.Context, namespace string, maximumItems int) ([]NormalizedEvent, bool, bool, error) {
	result := make([]NormalizedEvent, 0)
	continuation := ""
	for page := 0; page < s.budget.MaxPages; page++ {
		remaining := maximumItems - len(result)
		if remaining <= 0 {
			return result, false, true, nil
		}
		if err := ctx.Err(); err != nil {
			return result, false, false, err
		}
		response, err := s.port.ListEvents(ctx, namespace, PageRequest{Limit: s.budget.PageSize, Continue: continuation})
		if err != nil {
			return result, false, false, err
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
