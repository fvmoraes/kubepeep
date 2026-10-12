package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestConvertEventPreservesTargetAndLatestOccurrence(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	latest := first.Add(time.Minute)
	value := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "warning-1", Namespace: "events-ns", UID: "event-uid"},
		InvolvedObject: corev1.ObjectReference{APIVersion: "sql.example.com/v1", Kind: "SQLInstance", Namespace: "db-ns", Name: "db"},
		Type:           corev1.EventTypeWarning, Count: 1, EventTime: metav1.NewMicroTime(first),
		Series: &corev1.EventSeries{Count: 4, LastObservedTime: metav1.NewMicroTime(latest)},
	}
	result := ConvertEvent(value, nil)
	if result.ObjectAPIVersion != "sql.example.com/v1" || result.ObjectNamespace != "db-ns" || result.UID != "event-uid" || result.Count != 4 || result.Timestamp == nil || *result.Timestamp != latest.Format(time.RFC3339) {
		t.Fatalf("lost target or occurrence: %+v", result)
	}
	value.Series = nil
	value.LastTimestamp = metav1.NewTime(latest)
	if got := ConvertEvent(value, nil); got.Timestamp == nil || *got.Timestamp != latest.Format(time.RFC3339) {
		t.Fatalf("legacy event did not use its latest occurrence: %+v", got)
	}
}
