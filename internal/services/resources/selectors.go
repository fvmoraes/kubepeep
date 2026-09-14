package resources

import (
	"strings"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

const MaximumSelectorBytes = 1024

// SelectorSupport is the conservative pushdown matrix. Label selectors are
// supported by Kubernetes LIST for every resource collection. Field selectors
// are restricted to fields the API server documents for the relevant kind;
// unsupported application filters continue to run against the bounded page.
type SelectorSupport struct {
	Label  bool
	Fields map[string]struct{}
}

func SelectorSupportFor(collection Collection) SelectorSupport {
	support := SelectorSupport{Label: true, Fields: map[string]struct{}{"metadata.name": {}}}
	if !isClusterScoped(collection) {
		support.Fields["metadata.namespace"] = struct{}{}
	}
	switch collection {
	case CollectionPods:
		support.Fields["spec.nodeName"] = struct{}{}
		support.Fields["status.phase"] = struct{}{}
	case CollectionEvents:
		for _, field := range []string{
			"involvedObject.kind", "involvedObject.name", "involvedObject.namespace",
			"involvedObject.uid", "reason", "reportingComponent", "source", "type",
		} {
			support.Fields[field] = struct{}{}
		}
	}
	return support
}

func normalizeSelectors(collection Collection, options ListOptions) (ListOptions, error) {
	if len(options.LabelSelector) > MaximumSelectorBytes || !utf8.ValidString(options.LabelSelector) {
		return ListOptions{}, validationError("labelSelector must be valid UTF-8 up to 1 KiB")
	}
	if len(options.FieldSelector) > MaximumSelectorBytes || !utf8.ValidString(options.FieldSelector) {
		return ListOptions{}, validationError("fieldSelector must be valid UTF-8 up to 1 KiB")
	}
	support := SelectorSupportFor(collection)
	if options.LabelSelector != "" {
		selector, err := labels.Parse(options.LabelSelector)
		if err != nil {
			return ListOptions{}, validationError("labelSelector is invalid")
		}
		options.LabelSelector = selector.String()
	}
	if options.FieldSelector != "" {
		selector, err := fields.ParseSelector(options.FieldSelector)
		if err != nil {
			return ListOptions{}, validationError("fieldSelector is invalid")
		}
		for _, requirement := range selector.Requirements() {
			if _, ok := support.Fields[requirement.Field]; !ok {
				return ListOptions{}, validationError("fieldSelector contains a field unsupported by this collection")
			}
		}
		options.FieldSelector = selector.String()
	}

	// Existing exact filters are pushed down when Kubernetes supports the same
	// semantics. They are still checked locally, preserving behavior on test
	// adapters and protecting against incomplete upstream implementations.
	derived := make([]string, 0, 2)
	if collection == CollectionPods && options.Node != "" {
		derived = append(derived, fields.OneTermEqualSelector("spec.nodeName", options.Node).String())
	}
	if collection == CollectionEvents {
		if options.ObjectKind != "" {
			derived = append(derived, fields.OneTermEqualSelector("involvedObject.kind", options.ObjectKind).String())
		}
		if options.Reason != "" {
			derived = append(derived, fields.OneTermEqualSelector("reason", options.Reason).String())
		}
	}
	if options.FieldSelector != "" {
		derived = append([]string{options.FieldSelector}, derived...)
	}
	options.FieldSelector = strings.Join(derived, ",")
	return options, nil
}
