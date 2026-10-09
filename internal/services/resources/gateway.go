package resources

import (
	"sort"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func init() {
	for _, resource := range resourcecatalog.GatewayResources() {
		collection := Collection(resource.Collection)
		columnCollectionIDs = append(columnCollectionIDs, resource.Collection)
		rulesByCollection[collection] = collectionRules{sorts: []string{"identity", "name"}, defaultSort: "identity", defaultOrder: OrderAscending}
		origin := Origin{APIGroup: resource.Group, Version: resource.Version, Resource: resource.Resource}
		if resource.Namespaced {
			collectionGVR[collection] = origin
		} else {
			clusterGVR[collection] = origin
		}
	}
}

const maximumGatewayEntries = 128

// GatewayDTO keeps inventories and their warm cache free from arbitrary
// annotations and spec payloads. The explicit YAML reader owns full documents.
type GatewayDTO struct {
	Name            string         `json:"name"`
	Namespace       string         `json:"namespace"`
	UID             string         `json:"uid"`
	ResourceVersion string         `json:"resourceVersion"`
	APIVersion      string         `json:"apiVersion"`
	Kind            string         `json:"kind"`
	Status          string         `json:"status"`
	ClassName       string         `json:"className"`
	ControllerName  string         `json:"controllerName"`
	Hosts           []string       `json:"hosts"`
	Addresses       []string       `json:"addresses"`
	Listeners       int            `json:"listeners"`
	Rules           int            `json:"rules"`
	AgeSeconds      int64          `json:"ageSeconds"`
	Conditions      []ConditionDTO `json:"conditions"`
	Related         []ResourceRef  `json:"related"`
	Truncated       bool           `json:"truncated"`
}

func (GatewayDTO) resourceListItem()   {}
func (GatewayDTO) resourceDetailItem() {}

func ConvertGateway(value *unstructured.Unstructured, now time.Time) GatewayDTO {
	out := GatewayDTO{Name: value.GetName(), Namespace: value.GetNamespace(), UID: string(value.GetUID()), ResourceVersion: value.GetResourceVersion(), APIVersion: value.GetAPIVersion(), Kind: value.GetKind(), Status: "Unknown", Hosts: []string{}, Addresses: []string{}, Conditions: []ConditionDTO{}, Related: []ResourceRef{}}
	created := value.GetCreationTimestamp()
	if !created.IsZero() {
		out.AgeSeconds = max(0, int64(now.Sub(created.Time).Seconds()))
	}
	out.ClassName, _, _ = unstructured.NestedString(value.Object, "spec", "gatewayClassName")
	out.ControllerName, _, _ = unstructured.NestedString(value.Object, "spec", "controllerName")
	out.Hosts, _, _ = unstructured.NestedStringSlice(value.Object, "spec", "hostnames")
	if out.Hosts == nil {
		out.Hosts = []string{}
	}
	if len(out.Hosts) > maximumGatewayEntries {
		out.Hosts = out.Hosts[:maximumGatewayEntries]
		out.Truncated = true
	}
	listeners, _, _ := unstructured.NestedSlice(value.Object, "spec", "listeners")
	out.Listeners = len(listeners)
	rules, _, _ := unstructured.NestedSlice(value.Object, "spec", "rules")
	out.Rules = len(rules)
	addresses, _, _ := unstructured.NestedSlice(value.Object, "status", "addresses")
	for _, item := range addresses {
		if len(out.Addresses) == maximumGatewayEntries {
			out.Truncated = true
			break
		}
		if address, ok := item.(map[string]any); ok {
			if v, ok := address["value"].(string); ok {
				out.Addresses = append(out.Addresses, v)
			}
		}
	}
	seen := map[string]bool{}
	addRef := func(group, kind, namespace, name string) {
		if name == "" {
			return
		}
		key := group + "/" + kind + "/" + namespace + "/" + name
		if seen[key] {
			return
		}
		seen[key] = true
		if len(out.Related) == maximumGatewayEntries {
			out.Truncated = true
			return
		}
		out.Related = append(out.Related, ResourceRef{Kind: kind, Namespace: namespace, Name: name, APIGroup: group})
	}
	addRef(resourcecatalog.GatewayGroup, "GatewayClass", "", out.ClassName)
	var visit func(any, string, int)
	visit = func(node any, field string, depth int) {
		if depth > 12 {
			out.Truncated = true
			return
		}
		switch node := node.(type) {
		case []any:
			for _, child := range node {
				visit(child, field, depth+1)
			}
		case map[string]any:
			defaultKind := map[string]string{"parentRefs": "Gateway", "parentRef": "Gateway", "ancestorRef": "Gateway", "backendRefs": "Service", "backendRef": "Service", "certificateRefs": "Secret", "caCertificateRefs": "ConfigMap", "targetRefs": "Service"}[field]
			if field == "parametersRef" || field == "to" && value.GetKind() == "ReferenceGrant" {
				defaultKind, _ = node["kind"].(string)
			}
			if defaultKind != "" {
				name, _ := node["name"].(string)
				kind, _ := node["kind"].(string)
				group, explicitGroup := node["group"].(string)
				namespace, _ := node["namespace"].(string)
				if kind == "" {
					kind = defaultKind
				}
				if namespace == "" {
					namespace = value.GetNamespace()
				}
				if !explicitGroup && (field == "parentRefs" || field == "parentRef" || field == "ancestorRef") {
					group = resourcecatalog.GatewayGroup
				}
				addRef(group, kind, namespace, name)
			}
			keys := make([]string, 0, len(node))
			for k := range node {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				visit(node[k], k, depth+1)
			}
		}
	}
	visit(value.Object["spec"], "spec", 0)
	positive, pending, failed := false, false, false
	appendConditions := func(list []any, prefix string) {
		for _, item := range list {
			condition, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text := func(key string) string { v, _ := condition[key].(string); return v }
			reason, message, since := text("reason"), text("message"), text("lastTransitionTime")
			typeName, status := text("type"), text("status")
			observed, _, _ := unstructured.NestedInt64(condition, "observedGeneration")
			stale := observed < value.GetGeneration()
			if stale {
				status, reason, message = "Unknown", "AwaitingReconciliation", "The controller has not observed this object's current generation."
			}
			if len(out.Conditions) < maximumGatewayEntries {
				out.Conditions = append(out.Conditions, ConditionDTO{Type: prefix + typeName, Status: status, Reason: &reason, Message: &message, LastTransitionTime: &since})
			} else {
				out.Truncated = true
			}
			switch typeName {
			case "Accepted", "Programmed", "ResolvedRefs":
				if status == "True" {
					positive = positive || typeName == "Accepted" || typeName == "Programmed"
				} else if status == "False" && reason != "Pending" {
					failed = true
				} else {
					pending = true
				}
			case "Conflicted":
				failed = failed || status == "True"
			}
		}
	}
	conditions, _, _ := unstructured.NestedSlice(value.Object, "status", "conditions")
	appendConditions(conditions, "")
	if value.GetKind() == "Gateway" || value.GetKind() == "ListenerSet" {
		programmed := false
		for _, condition := range out.Conditions {
			programmed = programmed || condition.Type == "Programmed" && condition.Status == "True"
		}
		pending = pending || !programmed
	}
	for _, group := range []string{"parents", "listeners", "ancestors"} {
		values, _, _ := unstructured.NestedSlice(value.Object, "status", group)
		for _, item := range values {
			if m, ok := item.(map[string]any); ok {
				for _, field := range []string{"parentRef", "ancestorRef"} {
					if ref, ok := m[field].(map[string]any); ok {
						visit(ref, field, 0)
					}
				}
				if conditions, ok := m["conditions"].([]any); ok {
					name, _ := m["name"].(string)
					if name == "" {
						for _, field := range []string{"parentRef", "ancestorRef"} {
							if ref, ok := m[field].(map[string]any); ok {
								name, _ = ref["name"].(string)
							}
						}
					}
					appendConditions(conditions, group+"/"+name+"/")
				}
			}
		}
	}
	if len(out.Conditions) > 0 {
		out.Status = "Pending"
	}
	if positive && !pending {
		out.Status = "Ready"
	}
	if failed {
		out.Status = "Degraded"
	}
	// Restrict status messages to a bounded inventory payload.
	for i := range out.Conditions {
		message := out.Conditions[i].Message
		if message != nil && len(*message) > 2048 {
			bounded := string([]rune(*message)[:min(len([]rune(*message)), 512)])
			out.Conditions[i].Message = &bounded
			out.Truncated = true
		}
	}
	return out
}
