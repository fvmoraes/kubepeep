package resources

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type RestartFilter string

const (
	RestartAny   RestartFilter = "any"
	RestartGT0   RestartFilter = "gt0"
	RestartGTE3  RestartFilter = "gte3"
	RestartGTE10 RestartFilter = "gte10"
)

// ListOptions is the normalized domain representation of the common query.
// Continue is the externally authenticated token and is never persisted.
type ListOptions struct {
	Limit        int
	Continue     string
	Search       string
	SearchQuery  SearchQuery
	Namespaces   []string
	Statuses     []string
	Kinds        []WorkloadKind
	Sort         string
	Order        SortOrder
	Workload     string
	Node         string
	Restarts     RestartFilter
	Problematic  *bool
	ObjectKind   string
	Reason       string
	AddressType  string
}

type collectionRules struct {
	statuses     []string
	sorts        []string
	defaultSort  string
	defaultOrder SortOrder
}

var rulesByCollection = map[Collection]collectionRules{
	CollectionWorkloads:      {statuses: []string{"Healthy", "Progressing", "Degraded", "Suspended", "Completed", "Failed", "Unknown"}, sorts: []string{"identity", "name", "age", "status"}, defaultSort: "identity", defaultOrder: OrderAscending},
	CollectionPods:           {statuses: []string{"Running", "Pending", "Succeeded", "Failed", "Unknown"}, sorts: []string{"identity", "name", "age", "restarts", "status"}, defaultSort: "identity", defaultOrder: OrderAscending},
	CollectionEvents:         {statuses: []string{"Normal", "Warning", "Unknown"}, sorts: []string{"timestamp", "count", "identity"}, defaultSort: "timestamp", defaultOrder: OrderDescending},
	CollectionServices:       {sorts: []string{"identity", "name", "type"}, defaultSort: "identity", defaultOrder: OrderAscending},
	CollectionIngresses:      {sorts: []string{"identity", "name"}, defaultSort: "identity", defaultOrder: OrderAscending},
	CollectionEndpointSlices: {sorts: []string{"identity", "name", "addressType"}, defaultSort: "identity", defaultOrder: OrderAscending},
	CollectionConfigMaps:     {sorts: []string{"identity", "name", "createdAt"}, defaultSort: "identity", defaultOrder: OrderAscending},
	CollectionSecrets:        {sorts: []string{"identity", "name", "createdAt"}, defaultSort: "identity", defaultOrder: OrderAscending},
}

// NormalizeListOptions applies all bounded, endpoint-specific defaults and
// canonicalizes repeated enums. Scope intersection is performed separately by
// ResolveNamespaces because it needs the active selection.
func NormalizeListOptions(collection Collection, options ListOptions) (ListOptions, error) {
	rules, ok := rulesByCollection[collection]
	if !ok {
		return ListOptions{}, validationError("collection is not supported")
	}
	if options.Limit == 0 {
		options.Limit = DefaultListLimit
	}
	if options.Limit < 1 || options.Limit > MaximumListLimit {
		return ListOptions{}, validationError("limit must be between 1 and 500")
	}
	if len(options.Continue) > MaximumCursorBytes {
		return ListOptions{}, validationError("continue exceeds 16 KiB")
	}
	if len(options.Search) > MaximumSearchBytes || !utf8.ValidString(options.Search) {
		return ListOptions{}, validationError("search must be valid UTF-8 up to 256 bytes")
	}
	options.SearchQuery = ParseSearch(options.Search)
	var err error
	options.Namespaces, err = canonicalStrings(options.Namespaces, MaximumNamespaces, nil, "namespace")
	if err != nil {
		return ListOptions{}, err
	}
	options.Statuses, err = canonicalStrings(options.Statuses, len(rules.statuses), rules.statuses, "status")
	if err != nil {
		return ListOptions{}, err
	}
	if collection == CollectionWorkloads {
		options.Kinds, err = canonicalKinds(options.Kinds)
		if err != nil {
			return ListOptions{}, err
		}
	} else if len(options.Kinds) > 0 {
		return ListOptions{}, validationError("kind is not supported by this collection")
	}
	if options.Sort == "" {
		options.Sort = rules.defaultSort
	}
	if !contains(rules.sorts, options.Sort) {
		return ListOptions{}, validationError("sort is not supported by this collection")
	}
	if options.Order == "" {
		options.Order = rules.defaultOrder
	}
	if options.Order != OrderAscending && options.Order != OrderDescending {
		return ListOptions{}, validationError("order must be asc or desc")
	}
	if collection != CollectionPods && (options.Workload != "" || options.Node != "" || options.Restarts != "" || options.Problematic != nil) {
		return ListOptions{}, validationError("pod filters are not supported by this collection")
	}
	if options.Restarts != "" && options.Restarts != RestartAny && options.Restarts != RestartGT0 && options.Restarts != RestartGTE3 && options.Restarts != RestartGTE10 {
		return ListOptions{}, validationError("restarts has an invalid value")
	}
	if collection != CollectionEvents && (options.ObjectKind != "" || options.Reason != "") {
		return ListOptions{}, validationError("event filters are not supported by this collection")
	}
	if collection != CollectionEndpointSlices && options.AddressType != "" {
		return ListOptions{}, validationError("addressType is not supported by this collection")
	}
	if options.AddressType != "" && !contains([]string{"IPv4", "IPv6", "FQDN", "Unknown"}, options.AddressType) {
		return ListOptions{}, validationError("addressType has an invalid value")
	}
	return options, nil
}

// ResolveNamespaces computes an intersection. A query can never expand the
// active scope. The returned order is stable and lexical.
func ResolveNamespaces(active, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return canonicalStrings(active, MaximumNamespaces, nil, "active namespace")
	}
	requestedCanonical, err := canonicalStrings(requested, MaximumNamespaces, nil, "namespace")
	if err != nil {
		return nil, err
	}
	// An all-namespaces scope may legitimately contain more namespaces than the
	// fan-out ceiling. A bounded explicit subset can still be served safely.
	activeCanonical, err := canonicalStrings(active, max(MaximumNamespaces, len(active)), nil, "active namespace")
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(activeCanonical))
	for _, namespace := range activeCanonical {
		allowed[namespace] = struct{}{}
	}
	for _, namespace := range requestedCanonical {
		if _, ok := allowed[namespace]; !ok {
			return nil, validationError("namespace is outside the active scope")
		}
	}
	return requestedCanonical, nil
}

func canonicalStrings(values []string, maximum int, order []string, field string) ([]string, error) {
	if len(values) > maximum {
		return nil, validationError(fmt.Sprintf("%s exceeds its cardinality limit", field))
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || !utf8.ValidString(value) {
			return nil, validationError(field + " must not be empty")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, validationError(field + " must not contain duplicates")
		}
		if order != nil && !contains(order, value) {
			return nil, validationError(field + " has an invalid value")
		}
		seen[value] = struct{}{}
	}
	result := append([]string(nil), values...)
	if order == nil {
		sort.Strings(result)
		return result, nil
	}
	result = result[:0]
	for _, candidate := range order {
		if _, ok := seen[candidate]; ok {
			result = append(result, candidate)
		}
	}
	return result, nil
}

func canonicalKinds(values []WorkloadKind) ([]WorkloadKind, error) {
	if len(values) == 0 {
		return append([]WorkloadKind(nil), canonicalWorkloadKinds...), nil
	}
	stringsIn := make([]string, len(values))
	order := make([]string, len(canonicalWorkloadKinds))
	for index := range values {
		stringsIn[index] = string(values[index])
	}
	for index := range canonicalWorkloadKinds {
		order[index] = string(canonicalWorkloadKinds[index])
	}
	canonical, err := canonicalStrings(stringsIn, len(order), order, "kind")
	if err != nil {
		return nil, err
	}
	result := make([]WorkloadKind, len(canonical))
	for index := range canonical {
		result[index] = WorkloadKind(canonical[index])
	}
	return result, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validationError(message string) error {
	return domainError(CodeValidationFailed, message, nil)
}

// ContainsFolded implements Unicode simple case folding without regex or
// locale-dependent behavior.
func ContainsFolded(value, search string) bool {
	return strings.Contains(foldSimple(value), foldSimple(search))
}

func foldSimple(value string) string {
	var builder strings.Builder
	for _, current := range value {
		canonical := current
		for candidate := unicode.SimpleFold(current); candidate != current; candidate = unicode.SimpleFold(candidate) {
			if candidate < canonical {
				canonical = candidate
			}
		}
		builder.WriteRune(canonical)
	}
	return builder.String()
}
