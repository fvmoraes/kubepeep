package resources

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	maximumLabels       = 64
	maximumLabelBytes   = 16 << 10
	maximumConditions   = 64
	maximumContainers   = 128
	maximumRelated      = 256
	maximumMessageBytes = 4 << 10
)

func ConvertMetadata(value metav1.Object) ResourceMetadataDTO {
	if value == nil {
		return ResourceMetadataDTO{Labels: map[string]string{}}
	}
	created := ""
	createdAt := value.GetCreationTimestamp()
	if !createdAt.IsZero() {
		created = createdAt.UTC().Format(time.RFC3339)
	}
	return ResourceMetadataDTO{
		Namespace:         value.GetNamespace(),
		Name:              value.GetName(),
		UID:               string(value.GetUID()),
		ResourceVersion:   value.GetResourceVersion(),
		CreationTimestamp: created,
		Labels:            limitedStringMap(value.GetLabels()),
	}
}

func limitedStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]string, min(len(keys), maximumLabels))
	used := 0
	for _, key := range keys {
		if len(result) == maximumLabels {
			break
		}
		value := values[key]
		if used+len(key)+len(value) > maximumLabelBytes {
			break
		}
		result[key] = value
		used += len(key) + len(value)
	}
	return result
}

func ContainerSpecs(values []corev1.Container) []ContainerSpecDTO {
	if len(values) > maximumContainers {
		values = values[:maximumContainers]
	}
	result := make([]ContainerSpecDTO, 0, len(values))
	for _, value := range values {
		ports := make([]ContainerPortDTO, 0, len(value.Ports))
		for _, port := range value.Ports {
			if port.ContainerPort < 1 || port.ContainerPort > 65535 {
				continue
			}
			name := nullableString(port.Name)
			ports = append(ports, ContainerPortDTO{Name: name, ContainerPort: port.ContainerPort, Protocol: normalizeProtocol(port.Protocol)})
		}
		result = append(result, ContainerSpecDTO{Name: value.Name, Image: value.Image, Ports: ports})
	}
	return result
}

func normalizeProtocol(protocol corev1.Protocol) string {
	switch protocol {
	case corev1.ProtocolUDP:
		return "UDP"
	case corev1.ProtocolSCTP:
		return "SCTP"
	default:
		return "TCP"
	}
}

func normalizeConditionStatus(status corev1.ConditionStatus) string {
	switch status {
	case corev1.ConditionTrue:
		return "True"
	case corev1.ConditionFalse:
		return "False"
	default:
		return "Unknown"
	}
}

func conditionDTO(kind, status, reason, message string, transitioned metav1.Time) ConditionDTO {
	result := ConditionDTO{
		Type:    kind,
		Status:  normalizeConditionStatus(corev1.ConditionStatus(status)),
		Reason:  nullableSanitized(reason, maximumMessageBytes),
		Message: nullableSanitized(message, maximumMessageBytes),
	}
	if !transitioned.IsZero() {
		formatted := transitioned.UTC().Format(time.RFC3339)
		result.LastTransitionTime = &formatted
	}
	return result
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func nullableSanitized(value string, maximum int) *string {
	if value == "" {
		return nil
	}
	clean := sanitizeText(value, maximum)
	return &clean
}

func sanitizeText(value string, maximum int) string {
	value = strings.ToValidUTF8(value, "�")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r >= 0x20 {
			return r
		}
		return '�'
	}, value)
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func ageSeconds(created time.Time, now time.Time) int64 {
	if created.IsZero() || now.Before(created) {
		return 0
	}
	return int64(now.Sub(created) / time.Second)
}

func copyInt64(value int32) *int64 {
	converted := int64(value)
	if converted < 0 {
		converted = 0
	}
	return &converted
}

func selectorMap(selector *metav1.LabelSelector) map[string]string {
	if selector == nil || len(selector.MatchExpressions) > 0 {
		return nil
	}
	return limitedStringMap(selector.MatchLabels)
}
