package resources

import (
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type IndexCoverage struct {
	Topic            Topic      `json:"topic"`
	State            CacheState `json:"state"`
	Complete         bool       `json:"complete"`
	LoadedNamespaces []string   `json:"loadedNamespaces"`
}

type IndexedResource struct {
	APIGroup  string `json:"apiGroup"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status,omitempty"`
}

type NamespaceIndexCounts struct {
	Namespace string         `json:"namespace"`
	Resources map[string]int `json:"resources"`
}

type LocalIndexSnapshot struct {
	Generation     string                     `json:"generation"`
	CollectedAt    time.Time                  `json:"collectedAt"`
	Coverage       []IndexCoverage            `json:"coverage"`
	Resources      []IndexedResource          `json:"resources"`
	NamespaceCount []NamespaceIndexCounts     `json:"namespaceCounts"`
	Pods           []PodDTO                   `json:"-"`
	Workloads      []WorkloadDTO              `json:"-"`
	Services       []ServiceDTO               `json:"-"`
	EndpointSlices []EndpointSliceDTO         `json:"-"`
	ConfigMaps     []ConfigMapListDTO         `json:"-"`
	PVCs           []PersistentVolumeClaimDTO `json:"-"`
	Events         []EventDTO                 `json:"-"`
}

type Investigation struct {
	Target         IndexedResource   `json:"target"`
	OwnerChain     []IndexedResource `json:"ownerChain"`
	Pods           []IndexedResource `json:"pods"`
	Services       []IndexedResource `json:"services"`
	EndpointSlices []IndexedResource `json:"endpointSlices"`
	ConfigMaps     []IndexedResource `json:"configMaps"`
	PVCs           []IndexedResource `json:"pvcs"`
	Events         []IndexedResource `json:"events"`
	Coverage       []IndexCoverage   `json:"coverage"`
}

// LocalIndex builds bounded secondary indexes from generation-scoped cache
// snapshots. It never performs Kubernetes I/O and never treats missing topics
// as authoritative absence.
func (cache *ResourceCache) LocalIndex(generation string, expectedNamespaces ...string) LocalIndexSnapshot {
	result := LocalIndexSnapshot{Generation: generation, CollectedAt: time.Now().UTC()}
	if cache == nil || generation == "" {
		return result
	}
	type coverageState struct {
		state      CacheState
		complete   bool
		namespaces map[string]struct{}
		origins    map[string]map[schema.GroupVersionResource]struct{}
	}
	coverage := make(map[Topic]*coverageState)
	seen := make(map[string]struct{})
	cache.mu.Lock()
	now := cache.now().UTC()
	for _, entry := range cache.entries {
		if entry.key.Generation != generation || entry.key.Selector != "" || len(entry.key.Query.Filters) > 0 || entry.key.Query.Sort != "" || entry.key.Query.Order != "" || entry.key.Query.PageSize != 0 || entry.key.Query.Cursor != "" {
			continue
		}
		state := entry.state(now)
		if state == CacheStateExpired {
			continue
		}
		itemCoverage := coverage[entry.key.Topic]
		if itemCoverage == nil {
			itemCoverage = &coverageState{state: state, complete: true, namespaces: make(map[string]struct{}), origins: make(map[string]map[schema.GroupVersionResource]struct{})}
			coverage[entry.key.Topic] = itemCoverage
		}
		if entry.partial || state == CacheStatePartial || state == CacheStateRefreshing {
			itemCoverage.complete = false
		}
		if cacheStateRank(state) > cacheStateRank(itemCoverage.state) {
			itemCoverage.state = state
		}
		if entry.key.Namespace != "" {
			itemCoverage.namespaces[entry.key.Namespace] = struct{}{}
		}
		if itemCoverage.origins[entry.key.Namespace] == nil {
			itemCoverage.origins[entry.key.Namespace] = make(map[schema.GroupVersionResource]struct{})
		}
		itemCoverage.origins[entry.key.Namespace][entry.key.GVR] = struct{}{}
		for _, object := range entry.snapshot.Items {
			identity, ok := topicObjectIdentity(object)
			if !ok {
				continue
			}
			identity = string(entry.key.Topic) + "\x00" + identity
			if _, duplicate := seen[identity]; duplicate {
				continue
			}
			seen[identity] = struct{}{}
			result.addObject(object)
		}
	}
	cache.mu.Unlock()
	for _, topic := range topicOrder {
		item := coverage[topic]
		if item == nil {
			result.Coverage = append(result.Coverage, IndexCoverage{Topic: topic, State: CacheStateExpired, Complete: false, LoadedNamespaces: []string{}})
			continue
		}
		namespaces := make([]string, 0, len(item.namespaces))
		for namespace := range item.namespaces {
			namespaces = append(namespaces, namespace)
		}
		sort.Strings(namespaces)
		complete := item.complete && indexOriginsComplete(topic, item.origins, expectedNamespaces)
		result.Coverage = append(result.Coverage, IndexCoverage{Topic: topic, State: item.state, Complete: complete, LoadedNamespaces: namespaces})
	}
	result.finalize()
	return result
}

func indexOriginsComplete(topic Topic, origins map[string]map[schema.GroupVersionResource]struct{}, expectedNamespaces []string) bool {
	expectedGVRs := TopicGVRs(topic)
	containsAll := func(values map[schema.GroupVersionResource]struct{}) bool {
		if len(values) < len(expectedGVRs) {
			return false
		}
		for _, gvr := range expectedGVRs {
			if _, ok := values[gvr]; !ok {
				return false
			}
		}
		return true
	}
	if containsAll(origins[""]) {
		return true
	}
	expected := make(map[string]struct{}, len(expectedNamespaces))
	for _, namespace := range expectedNamespaces {
		namespace = strings.TrimSpace(namespace)
		if namespace != "" {
			expected[namespace] = struct{}{}
		}
	}
	if len(expected) == 0 {
		return false
	}
	for namespace := range expected {
		if !containsAll(origins[namespace]) {
			return false
		}
	}
	return true
}

func cacheStateRank(state CacheState) int {
	switch state {
	case CacheStateFresh:
		return 0
	case CacheStateStale:
		return 1
	case CacheStateRefreshing:
		return 2
	case CacheStatePartial:
		return 3
	default:
		return 4
	}
}

func (index *LocalIndexSnapshot) addObject(object TopicObject) {
	switch value := object.(type) {
	case PodDTO:
		index.Pods = append(index.Pods, value)
		index.Resources = append(index.Resources, IndexedResource{Kind: "Pod", Namespace: value.Namespace, Name: value.Name, Status: value.Status})
	case WorkloadDTO:
		index.Workloads = append(index.Workloads, value)
		index.Resources = append(index.Resources, IndexedResource{APIGroup: workloadAPIGroup(value.Kind), Kind: value.Kind, Namespace: value.Namespace, Name: value.Name, Status: string(value.Status)})
	case ServiceDTO:
		index.Services = append(index.Services, value)
		index.Resources = append(index.Resources, IndexedResource{Kind: "Service", Namespace: value.Namespace, Name: value.Name})
	case EndpointSliceDTO:
		index.EndpointSlices = append(index.EndpointSlices, value)
		index.Resources = append(index.Resources, IndexedResource{APIGroup: "discovery.k8s.io", Kind: "EndpointSlice", Namespace: value.Namespace, Name: value.Name})
	case ConfigMapListDTO:
		index.ConfigMaps = append(index.ConfigMaps, value)
		index.Resources = append(index.Resources, IndexedResource{Kind: "ConfigMap", Namespace: value.Namespace, Name: value.Name})
	case PersistentVolumeClaimDTO:
		index.PVCs = append(index.PVCs, value)
		index.Resources = append(index.Resources, IndexedResource{Kind: "PersistentVolumeClaim", Namespace: value.Namespace, Name: value.Name, Status: value.Status})
	case EventDTO:
		index.Events = append(index.Events, value)
		index.Resources = append(index.Resources, IndexedResource{Kind: "Event", Namespace: value.Namespace, Name: value.Name, Status: value.Type})
	case IngressDTO:
		index.Resources = append(index.Resources, IndexedResource{APIGroup: "networking.k8s.io", Kind: "Ingress", Namespace: value.Namespace, Name: value.Name})
	}
}

func (index *LocalIndexSnapshot) finalize() {
	sort.Slice(index.Resources, func(left, right int) bool {
		if index.Resources[left].Kind != index.Resources[right].Kind {
			return index.Resources[left].Kind < index.Resources[right].Kind
		}
		if index.Resources[left].Namespace != index.Resources[right].Namespace {
			return index.Resources[left].Namespace < index.Resources[right].Namespace
		}
		return index.Resources[left].Name < index.Resources[right].Name
	})
	counts := make(map[string]map[string]int)
	for _, resource := range index.Resources {
		if resource.Namespace == "" || resource.Kind == "Event" {
			continue
		}
		if counts[resource.Namespace] == nil {
			counts[resource.Namespace] = make(map[string]int)
		}
		counts[resource.Namespace][resource.Kind]++
	}
	for namespace, resources := range counts {
		index.NamespaceCount = append(index.NamespaceCount, NamespaceIndexCounts{Namespace: namespace, Resources: resources})
	}
	sort.Slice(index.NamespaceCount, func(left, right int) bool {
		return index.NamespaceCount[left].Namespace < index.NamespaceCount[right].Namespace
	})
}

func (index LocalIndexSnapshot) Search(term string, limit int) []IndexedResource {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return []IndexedResource{}
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	result := make([]IndexedResource, 0, min(limit, len(index.Resources)))
	for _, resource := range index.Resources {
		haystack := strings.ToLower(resource.Kind + " " + resource.Namespace + " " + resource.Name + " " + resource.Status)
		if strings.Contains(haystack, term) {
			result = append(result, resource)
			if len(result) == limit {
				break
			}
		}
	}
	return result
}

func (index LocalIndexSnapshot) Investigate(kind, namespace, name string) Investigation {
	target := IndexedResource{Kind: kind, Namespace: namespace, Name: name}
	for _, resource := range index.Resources {
		if strings.EqualFold(resource.Kind, kind) && resource.Namespace == namespace && resource.Name == name {
			target = resource
			break
		}
	}
	result := Investigation{Target: target, OwnerChain: []IndexedResource{}, Pods: []IndexedResource{}, Services: []IndexedResource{}, EndpointSlices: []IndexedResource{}, ConfigMaps: []IndexedResource{}, PVCs: []IndexedResource{}, Events: []IndexedResource{}, Coverage: append([]IndexCoverage(nil), index.Coverage...)}
	pods := make([]PodDTO, 0)
	configNames, pvcNames := make(map[string]struct{}), make(map[string]struct{})
	if strings.EqualFold(kind, "Pod") {
		for _, pod := range index.Pods {
			if pod.Namespace == namespace && pod.Name == name {
				pods = append(pods, pod)
				result.followOwner(index.Workloads, namespace, pod.Owner)
				addNames(configNames, pod.ConfigMaps)
				addNames(pvcNames, pod.PVCs)
				break
			}
		}
	} else {
		for _, workload := range index.Workloads {
			if workload.Namespace == namespace && strings.EqualFold(workload.Kind, kind) && workload.Name == name {
				result.followOwner(index.Workloads, namespace, workload.Owner)
				addNames(configNames, workload.ConfigMaps)
				addNames(pvcNames, workload.PVCs)
			}
		}
		for _, pod := range index.Pods {
			if pod.Namespace != namespace || pod.Owner == nil {
				continue
			}
			if strings.EqualFold(pod.Owner.Kind, kind) && pod.Owner.Name == name || ownerLeadsTo(index.Workloads, namespace, pod.Owner, kind, name) {
				pods = append(pods, pod)
				addNames(configNames, pod.ConfigMaps)
				addNames(pvcNames, pod.PVCs)
			}
		}
	}
	for _, pod := range pods {
		result.Pods = append(result.Pods, IndexedResource{Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, Status: pod.Status})
	}
	for _, service := range index.Services {
		if service.Namespace == namespace && anyPodMatches(service.Selector, pods) {
			result.Services = append(result.Services, IndexedResource{Kind: "Service", Namespace: namespace, Name: service.Name})
		}
	}
	serviceNames := make(map[string]struct{})
	for _, service := range result.Services {
		serviceNames[service.Name] = struct{}{}
	}
	for _, slice := range index.EndpointSlices {
		if slice.Namespace == namespace {
			if _, ok := serviceNames[slice.ServiceName]; ok {
				result.EndpointSlices = append(result.EndpointSlices, IndexedResource{APIGroup: "discovery.k8s.io", Kind: "EndpointSlice", Namespace: namespace, Name: slice.Name})
			}
		}
	}
	for _, config := range index.ConfigMaps {
		if config.Namespace == namespace {
			if _, ok := configNames[config.Name]; ok {
				result.ConfigMaps = append(result.ConfigMaps, IndexedResource{Kind: "ConfigMap", Namespace: namespace, Name: config.Name})
			}
		}
	}
	for _, pvc := range index.PVCs {
		if pvc.Namespace == namespace {
			if _, ok := pvcNames[pvc.Name]; ok {
				result.PVCs = append(result.PVCs, IndexedResource{Kind: "PersistentVolumeClaim", Namespace: namespace, Name: pvc.Name, Status: pvc.Status})
			}
		}
	}
	podNames := make(map[string]struct{})
	for _, pod := range pods {
		podNames[pod.Name] = struct{}{}
	}
	for _, event := range index.Events {
		if event.Namespace == namespace && ((strings.EqualFold(event.ObjectKind, kind) && event.ObjectName == name) || strings.EqualFold(event.ObjectKind, "Pod") && containsName(podNames, event.ObjectName)) {
			result.Events = append(result.Events, IndexedResource{Kind: "Event", Namespace: namespace, Name: event.Name, Status: event.Reason})
		}
	}
	return result
}

func (result *Investigation) followOwner(workloads []WorkloadDTO, namespace string, owner *OwnerDTO) {
	seen := make(map[string]struct{})
	for owner != nil && len(result.OwnerChain) < 8 {
		key := strings.ToLower(owner.Kind) + "\x00" + owner.Name
		if _, ok := seen[key]; ok {
			break
		}
		seen[key] = struct{}{}
		result.OwnerChain = append(result.OwnerChain, IndexedResource{APIGroup: workloadAPIGroup(owner.Kind), Kind: owner.Kind, Namespace: namespace, Name: owner.Name})
		var next *OwnerDTO
		for _, workload := range workloads {
			if workload.Namespace == namespace && strings.EqualFold(workload.Kind, owner.Kind) && workload.Name == owner.Name {
				next = workload.Owner
				break
			}
		}
		owner = next
	}
}

func ownerLeadsTo(workloads []WorkloadDTO, namespace string, owner *OwnerDTO, kind, name string) bool {
	probe := Investigation{}
	probe.followOwner(workloads, namespace, owner)
	for _, candidate := range probe.OwnerChain {
		if strings.EqualFold(candidate.Kind, kind) && candidate.Name == name {
			return true
		}
	}
	return false
}
func addNames(target map[string]struct{}, values []string) {
	for _, value := range values {
		if value != "" {
			target[value] = struct{}{}
		}
	}
}
func containsName(values map[string]struct{}, value string) bool { _, ok := values[value]; return ok }
func anyPodMatches(selector map[string]string, pods []PodDTO) bool {
	if len(selector) == 0 {
		return false
	}
	for _, pod := range pods {
		matched := true
		for key, value := range selector {
			if pod.Labels[key] != value {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
func workloadAPIGroup(kind string) string {
	if strings.EqualFold(kind, "Job") || strings.EqualFold(kind, "CronJob") {
		return "batch"
	}
	return "apps"
}
