package dashboard

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNormalizeEventsUsesDocumentedTimestampOrder(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	coreEvent := corev1.Event{
		ObjectMeta:          metav1.ObjectMeta{Namespace: "ns", UID: "core", CreationTimestamp: metav1.NewTime(base.Add(-4 * time.Minute))},
		InvolvedObject:      corev1.ObjectReference{Kind: "Pod", Name: "pod", UID: "pod-uid"},
		EventTime:           metav1.NewMicroTime(base),
		Series:              &corev1.EventSeries{Count: 7, LastObservedTime: metav1.NewMicroTime(base.Add(-time.Minute))},
		LastTimestamp:       metav1.NewTime(base.Add(-2 * time.Minute)),
		Count:               3,
		ReportingController: "controller",
	}
	normalized := NormalizeCoreEvent(coreEvent)
	if !normalized.ObservedAt.Equal(base) || normalized.Count != 7 || normalized.Source != "controller" {
		t.Fatalf("unexpected core normalization: %+v", normalized)
	}

	event := eventsv1.Event{
		ObjectMeta:              metav1.ObjectMeta{Namespace: "ns", UID: "events", CreationTimestamp: metav1.NewTime(base.Add(-4 * time.Minute))},
		Regarding:               corev1.ObjectReference{Kind: "Pod", Name: "pod", UID: "pod-uid"},
		Series:                  &eventsv1.EventSeries{Count: 9, LastObservedTime: metav1.NewMicroTime(base.Add(-time.Minute))},
		DeprecatedLastTimestamp: metav1.NewTime(base.Add(-2 * time.Minute)),
		DeprecatedCount:         2,
		DeprecatedSource:        corev1.EventSource{Component: "legacy"},
	}
	normalized = NormalizeEventsV1(event)
	if !normalized.ObservedAt.Equal(base.Add(-time.Minute)) || normalized.Count != 9 || normalized.Source != "legacy" {
		t.Fatalf("unexpected events/v1 normalization: %+v", normalized)
	}
}

func TestGroupWarningEventsUsesClosedKeyAndPreservesCounts(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	events := []NormalizedEvent{
		{UID: "b", Namespace: "ns", RegardingKind: "Pod", RegardingName: "pod", RegardingUID: "uid", Type: "Warning", Reason: "BackOff", Message: "retry", Count: 2, Source: "kubelet", ObservedAt: now.Add(-time.Minute)},
		{UID: "a", Namespace: "ns", RegardingKind: "Pod", RegardingName: "pod", RegardingUID: "uid", Type: "warning", Reason: "BackOff", Message: "retry", Count: 3, Source: "kubelet", ObservedAt: now},
		{UID: "c", Namespace: "ns", RegardingKind: "Pod", RegardingName: "pod", RegardingUID: "uid", Type: "Warning", Reason: "BackOff", Message: "different", Count: 0, Source: "kubelet", ObservedAt: now.Add(-2 * time.Minute)},
		{UID: "normal", Namespace: "ns", Type: "Normal", Count: 99},
	}
	result := GroupWarningEvents(events)
	if len(result) != 2 {
		t.Fatalf("got %d groups, want 2", len(result))
	}
	if result[0].Count != 5 || result[0].Timestamp == nil || *result[0].Timestamp != now.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected aggregated warning: %+v", result[0])
	}
	if result[1].Count != 0 {
		t.Fatalf("zero count was not preserved: %+v", result[1])
	}
}

func TestGroupEventsNormalizesClosedTypesAndKeepsCursorIdentitiesDistinct(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	events := []NormalizedEvent{
		{UID: "normal-a", Namespace: "ns", RegardingKind: "Pod", RegardingName: "pod", RegardingUID: "pod-a", Type: "normal", Reason: "Pulled", Message: "image", Count: 1, Source: "kubelet", ObservedAt: now},
		{UID: "normal-b", Namespace: "ns", RegardingKind: "Pod", RegardingName: "pod", RegardingUID: "pod-b", Type: "Normal", Reason: "Pulled", Message: "image", Count: 1, Source: "kubelet", ObservedAt: now},
		{UID: "warning", Namespace: "ns", RegardingKind: "Pod", RegardingName: "pod", RegardingUID: "pod-c", Type: "WARNING", Reason: "BackOff", Message: "retry", Count: 2, Source: "kubelet", ObservedAt: now.Add(-time.Minute)},
		{UID: "custom", Namespace: "ns", RegardingKind: "Node", RegardingName: "node", RegardingUID: "node-a", Type: "Custom", Reason: "Odd", Message: "unknown", Count: 3, Source: "controller", ObservedAt: now.Add(-2 * time.Minute)},
	}
	result := GroupEvents(events)
	if len(result) != 4 {
		t.Fatalf("got %d groups, want 4", len(result))
	}
	if result[0].Type != "Normal" || result[1].Type != "Normal" || result[2].Type != "Warning" || result[3].Type != "Unknown" {
		t.Fatalf("event types were not normalized: %+v", result)
	}
	if EventCursorIdentity(result[0]) == EventCursorIdentity(result[1]) {
		t.Fatal("different Kubernetes UIDs collapsed to the same cursor identity")
	}
	for _, event := range result {
		if event.Type != "Normal" && event.Type != "Warning" && event.Type != "Unknown" {
			t.Fatalf("non-contract type escaped: %+v", event)
		}
	}
	if identity := EventCursorIdentity(EventDTO{Namespace: "ns", Message: strings.Repeat("x", 128<<10), Type: "Warning"}); len(identity) > 256 {
		t.Fatalf("cursor identity retained an unbounded event payload: %d bytes", len(identity))
	}
}
