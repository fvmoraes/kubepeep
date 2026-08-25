package resources

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

type EventDTO struct {
	Timestamp  *string `json:"timestamp"`
	Namespace  string  `json:"namespace"`
	ObjectKind string  `json:"objectKind"`
	ObjectName string  `json:"objectName"`
	Reason     string  `json:"reason"`
	Message    string  `json:"message"`
	Count      int64   `json:"count"`
	Source     *string `json:"source"`
	Type       string  `json:"type"`
}

func (EventDTO) resourceListItem() {}

func ConvertEvent(value *corev1.Event, redactor TextRedactor) EventDTO {
	message := value.Message
	if redactor != nil {
		message = redactor.Redact(message)
	}
	message = sanitizeText(message, maximumMessageBytes)
	reason := sanitizeText(value.Reason, maximumMessageBytes)
	count := int64(value.Count)
	if count < 0 {
		count = 0
	}
	typeName := value.Type
	if typeName != corev1.EventTypeNormal && typeName != corev1.EventTypeWarning {
		typeName = "Unknown"
	}
	return EventDTO{
		Timestamp: eventTimestamp(value), Namespace: value.Namespace,
		ObjectKind: value.InvolvedObject.Kind, ObjectName: value.InvolvedObject.Name,
		Reason: reason, Message: message, Count: count,
		Source: eventSource(value), Type: typeName,
	}
}

func eventTimestamp(value *corev1.Event) *string {
	var timestamp time.Time
	switch {
	case !value.EventTime.IsZero():
		timestamp = value.EventTime.Time
	case value.Series != nil && !value.Series.LastObservedTime.IsZero():
		timestamp = value.Series.LastObservedTime.Time
	case !value.LastTimestamp.IsZero():
		timestamp = value.LastTimestamp.Time
	case !value.FirstTimestamp.IsZero():
		timestamp = value.FirstTimestamp.Time
	case !value.CreationTimestamp.IsZero():
		timestamp = value.CreationTimestamp.Time
	default:
		return nil
	}
	formatted := timestamp.UTC().Format(time.RFC3339)
	return &formatted
}

func eventSource(value *corev1.Event) *string {
	source := value.ReportingController
	if source == "" {
		source = value.Source.Component
	}
	return nullableSanitized(source, maximumMessageBytes)
}
