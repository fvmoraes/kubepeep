package dashboard

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func canonicalNamespaces(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func appendUnique(values []string, value string) []string {
	for _, item := range values {
		if item == value {
			return values
		}
	}
	return append(values, value)
}

func ageSeconds(created metav1.Time, now time.Time) int64 {
	if created.IsZero() || created.Time.After(now) {
		return 0
	}
	return int64(now.Sub(created.Time) / time.Second)
}

func truncateUTF8(value string, maximum int) (string, bool) {
	if maximum < 0 {
		maximum = 0
	}
	if len(value) <= maximum {
		return value, false
	}
	value = value[:maximum]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func sanitizeText(value string, maximum int) string {
	value = strings.ToValidUTF8(value, "�")
	var builder strings.Builder
	builder.Grow(len(value))
	for _, current := range value {
		if current < 0x20 && current != '\n' && current != '\r' && current != '\t' {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(current)
	}
	result, _ := truncateUTF8(builder.String(), maximum)
	return result
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func containerTypePointer(value ContainerType) *ContainerType {
	copy := value
	return &copy
}

func apiGroup(apiVersion string) string {
	if slash := strings.IndexByte(apiVersion, '/'); slash >= 0 {
		return apiVersion[:slash]
	}
	return ""
}

func resourceRef(apiVersion, kind, namespace, name string, uid types.UID) *ResourceRef {
	if kind == "" || name == "" {
		return nil
	}
	return &ResourceRef{
		APIGroup:  apiGroup(apiVersion),
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		UID:       string(uid),
	}
}

func int32Pointer(value int32) *int64 {
	converted := int64(value)
	return &converted
}
