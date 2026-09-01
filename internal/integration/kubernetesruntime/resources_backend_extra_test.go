package kubernetesruntime

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

func TestNewResourceBackendRequiresDependenciesAndLifecycle(t *testing.T) {
	t.Parallel()
	if _, err := NewResourceBackend(nil, nil, nil); err == nil {
		t.Fatal("nil runtime and authorizer were accepted")
	}
	if _, err := NewResourceBackend(nil, &allowResourceAuthorization{}, nil); err == nil {
		t.Fatal("nil runtime was accepted")
	}
	if _, err := NewResourceBackend(&Runtime{}, nil, nil); err == nil {
		t.Fatal("nil authorizer was accepted")
	}
	backend, err := NewResourceBackend(&Runtime{}, &allowResourceAuthorization{}, nil)
	if err != nil || backend == nil || backend.watchManager == nil {
		t.Fatalf("valid construction failed: %v", err)
	}
	backend.OnGeneration("gen-1")
	if backend.watchGeneration != "gen-1" {
		t.Fatalf("watch generation = %q", backend.watchGeneration)
	}
	backend.watchBindings["gen-1"] = namespaces.SelectionBinding{Generation: "gen-1"}
	backend.OnGeneration("gen-2")
	if _, retained := backend.watchBindings["gen-1"]; retained {
		t.Fatal("previous generation binding was retained")
	}
	backend.Close()
	if backend.watchManager != nil {
		t.Fatal("close retained the watch manager")
	}
	backend.Close()

	empty := &ResourceBackend{}
	empty.OnGeneration("gen")
	empty.Close()
}

func TestResourceBackendListsNetworkAndStorageCollections(t *testing.T) {
	t.Parallel()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIPs: []string{"10.0.0.15"}}}
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web"}, Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "app.example.com"}}}}
	endpointSlice := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api-abc"}, AddressType: discoveryv1.AddressTypeIPv4}
	client := kubefake.NewSimpleClientset(service, ingress, endpointSlice)

	scheme := metadatafake.NewTestScheme()
	metav1.AddMetaToScheme(scheme)
	configMap := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}, ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "settings", UID: "cm-uid"}}
	secret := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "credentials", UID: "secret-uid"}}
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, configMap, secret)

	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
	backend := &ResourceBackend{
		clients:    fixedResourceClientProvider{set: resourceClientSet{kubernetes: client, metadata: metadataClient}},
		authorizer: &allowResourceAuthorization{}, now: time.Now,
	}
	ctx := context.Background()

	services, err := backend.ListServices(ctx, binding, resolution, resources.ListOptions{Limit: 10}, nil)
	if err != nil || len(services.Items) != 1 || services.Items[0].Name != "api" || services.Items[0].Type != "ClusterIP" {
		t.Fatalf("services = %#v err = %v", services.Items, err)
	}
	ingresses, err := backend.ListIngresses(ctx, binding, resolution, resources.ListOptions{Limit: 10}, nil)
	if err != nil || len(ingresses.Items) != 1 || len(ingresses.Items[0].Hosts) != 1 || ingresses.Items[0].Hosts[0] != "app.example.com" {
		t.Fatalf("ingresses = %#v err = %v", ingresses.Items, err)
	}
	slices, err := backend.ListEndpointSlices(ctx, binding, resolution, resources.ListOptions{Limit: 10}, nil)
	if err != nil || len(slices.Items) != 1 || slices.Items[0].AddressType != "IPv4" {
		t.Fatalf("endpoint slices = %#v err = %v", slices.Items, err)
	}
	configMaps, err := backend.ListConfigMaps(ctx, binding, resolution, resources.ListOptions{Limit: 10}, nil)
	if err != nil || len(configMaps.Items) != 1 || configMaps.Items[0].Name != "settings" {
		t.Fatalf("config maps = %#v err = %v", configMaps.Items, err)
	}
	secrets, err := backend.ListSecrets(ctx, binding, resolution, resources.ListOptions{Limit: 10}, nil)
	if err != nil || len(secrets.Items) != 1 || secrets.Items[0].Metadata.Name != "credentials" {
		t.Fatalf("secrets = %#v err = %v", secrets.Items, err)
	}
}

func TestResourceBackendCollectGuardrails(t *testing.T) {
	t.Parallel()
	client := kubefake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}})
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}, PreferGlobal: true}
	newBackend := func(authorizer resources.AuthorizationChecker) *ResourceBackend {
		return &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: authorizer, now: time.Now}
	}
	ctx := context.Background()

	mixedCursor := &resources.CompositeCursor[resources.PodDTO]{Version: 1, Origins: []resources.OriginCursor[resources.PodDTO]{
		{Origin: resources.Origin{Version: "v1", Resource: "pods"}},
		{Origin: resources.Origin{Namespace: "default", Version: "v1", Resource: "pods"}},
	}}
	if _, err := newBackend(&allowResourceAuthorization{}).ListPods(ctx, binding, resolution, resources.ListOptions{Limit: 10}, mixedCursor); resources.ErrorCodeOf(err) != resources.CodeValidationFailed {
		t.Fatalf("mixed cursor err = %v", err)
	}

	globalCursor := &resources.CompositeCursor[resources.PodDTO]{Version: 1, Origins: []resources.OriginCursor[resources.PodDTO]{
		{Origin: resources.Origin{Version: "v1", Resource: "pods"}},
	}}
	if _, err := newBackend(&namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionDenied}).ListPods(ctx, binding, resolution, resources.ListOptions{Limit: 10}, globalCursor); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("denied global cursor err = %v", err)
	}
	if _, err := newBackend(&namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionUnknown}).ListPods(ctx, binding, resolution, resources.ListOptions{Limit: 10}, globalCursor); resources.ErrorCodeOf(err) != resources.CodeAuthorizationUnavailable {
		t.Fatalf("unknown global cursor err = %v", err)
	}

	oversized := make([]string, resources.MaximumNamespaces+1)
	for index := range oversized {
		oversized[index] = "namespace-" + string(rune('a'+index%26)) + string(rune('a'+index/26))
	}
	if _, err := newBackend(&namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionDenied}).ListPods(ctx, binding, namespaces.ScopeResolution{ScopeName: "all", Namespaces: oversized, PreferGlobal: true}, resources.ListOptions{Limit: 10}, nil); resources.ErrorCodeOf(err) != resources.CodeLimitExceeded {
		t.Fatalf("oversized fanout err = %v", err)
	}

	if _, err := newBackend(&allowResourceAuthorization{}).ListPods(ctx, binding, resolution, resources.ListOptions{Limit: 10, Namespaces: []string{"other"}}, nil); resources.ErrorCodeOf(err) != resources.CodeValidationFailed {
		t.Fatalf("out of scope namespace err = %v", err)
	}

	if _, err := newBackend(&allowResourceAuthorization{}).ListPods(ctx, binding, resolution, resources.ListOptions{Limit: 10, Sort: "bogus"}, nil); resources.ErrorCodeOf(err) != resources.CodeValidationFailed {
		t.Fatalf("invalid sort err = %v", err)
	}
}

func TestIdentityComparatorsKeepCanonicalOrder(t *testing.T) {
	t.Parallel()
	first := time.Unix(10, 0).UTC().Format(time.RFC3339)
	second := time.Unix(20, 0).UTC().Format(time.RFC3339)

	workloads := []resources.WorkloadDTO{
		{Kind: "Deployment", Namespace: "a", Name: "b"},
		{Kind: "Deployment", Namespace: "a", Name: "a"},
		{Kind: "CronJob", Namespace: "z", Name: "a"},
	}
	if workloadIdentityLess(workloads[0], workloads[1]) || !workloadIdentityLess(workloads[1], workloads[0]) {
		t.Fatal("workload name ordering broken")
	}
	if workloadIdentityLess(workloads[2], workloads[0]) || !workloadIdentityLess(workloads[0], workloads[2]) {
		t.Fatal("workload kind ordering broken")
	}
	if workloadIdentityLess(workloads[0], workloads[0]) {
		t.Fatal("equal workloads compared less")
	}

	pods := []resources.PodDTO{{Namespace: "a", Name: "x"}, {Namespace: "b", Name: "a"}}
	if podIdentityLess(pods[1], pods[0]) || !podIdentityLess(pods[0], pods[1]) || podIdentityLess(pods[0], pods[0]) {
		t.Fatal("pod identity ordering broken")
	}

	oldTimestamp, newTimestamp := first, second
	events := []resources.EventDTO{
		{Timestamp: &newTimestamp, Namespace: "a", ObjectKind: "Pod", ObjectName: "x", Reason: "r"},
		{Timestamp: &oldTimestamp, Namespace: "a", ObjectKind: "Pod", ObjectName: "x", Reason: "r"},
	}
	if got := eventIdentityLess(events[0], events[1]); !got {
		t.Fatal("newer events must sort before older events")
	}
	tiedEvents := []resources.EventDTO{
		{Timestamp: &oldTimestamp, Namespace: "a", ObjectKind: "Pod", ObjectName: "x", Reason: "a"},
		{Timestamp: &oldTimestamp, Namespace: "a", ObjectKind: "Pod", ObjectName: "x", Reason: "b"},
	}
	if eventIdentityLess(tiedEvents[1], tiedEvents[0]) || !eventIdentityLess(tiedEvents[0], tiedEvents[1]) {
		t.Fatal("event tie-break ordering broken")
	}

	services := []resources.ServiceDTO{{Namespace: "a", Name: "x"}, {Namespace: "b", Name: "a"}}
	if serviceIdentityLess(services[1], services[0]) || !serviceIdentityLess(services[0], services[1]) {
		t.Fatal("service identity ordering broken")
	}
	ingresses := []resources.IngressDTO{{Namespace: "a", Name: "x"}, {Namespace: "b", Name: "a"}}
	if ingressIdentityLess(ingresses[1], ingresses[0]) || !ingressIdentityLess(ingresses[0], ingresses[1]) {
		t.Fatal("ingress identity ordering broken")
	}
	slices := []resources.EndpointSliceDTO{{Namespace: "a", Name: "x"}, {Namespace: "b", Name: "a"}}
	if endpointSliceIdentityLess(slices[1], slices[0]) || !endpointSliceIdentityLess(slices[0], slices[1]) {
		t.Fatal("endpoint slice identity ordering broken")
	}
	configMaps := []resources.ConfigMapListDTO{
		{Namespace: "a", Name: "x", UID: "1"},
		{Namespace: "a", Name: "x", UID: "2"},
	}
	if configMapIdentityLess(configMaps[1], configMaps[0]) || !configMapIdentityLess(configMaps[0], configMaps[1]) {
		t.Fatal("config map uid ordering broken")
	}
	secrets := []resources.SecretMetadataDTO{
		{Metadata: resources.SecretMetadataFieldsDTO{Name: "x", Namespace: "a", UID: "1"}},
		{Metadata: resources.SecretMetadataFieldsDTO{Name: "x", Namespace: "a", UID: "2"}},
	}
	if secretIdentityLess(secrets[1], secrets[0]) || !secretIdentityLess(secrets[0], secrets[1]) {
		t.Fatal("secret uid ordering broken")
	}
}

type fixedDecisionAuthorization struct {
	decision authorization.Decision
}

func (stub *fixedDecisionAuthorization) Check(context.Context, authorization.Key) authorization.Capability {
	return authorization.Capability{Decision: stub.decision}
}

func TestGlobalListDecisionHandlesNilUnknownAndInvalid(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	origins := []resources.Origin{{Version: "v1", Resource: "pods"}}
	if got := globalListDecision(ctx, nil, "gen", origins); got != authorization.DecisionUnknown {
		t.Fatalf("nil checker decision = %q", got)
	}
	if got := globalListDecision(ctx, &namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionUnknown}, "gen", origins); got != authorization.DecisionUnknown {
		t.Fatalf("unknown decision = %q", got)
	}
	if got := globalListDecision(ctx, &fixedDecisionAuthorization{decision: authorization.Decision("invalid")}, "gen", origins); got != authorization.DecisionUnknown {
		t.Fatalf("invalid decision = %q", got)
	}
	if got := globalListDecision(ctx, &allowResourceAuthorization{}, "gen", origins); got != authorization.DecisionAllowed {
		t.Fatalf("allowed decision = %q", got)
	}
	if got := globalListDecision(ctx, &namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionDenied}, "gen", origins); got != authorization.DecisionDenied {
		t.Fatalf("denied decision = %q", got)
	}
}

func TestRestartMatchesAndSortingHelpers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		filter resources.RestartFilter
		value  int64
		want   bool
	}{
		{filter: "", value: 0, want: true},
		{filter: resources.RestartAny, value: 42, want: true},
		{filter: resources.RestartGT0, value: 0, want: false},
		{filter: resources.RestartGT0, value: 1, want: true},
		{filter: resources.RestartGTE3, value: 2, want: false},
		{filter: resources.RestartGTE3, value: 3, want: true},
		{filter: resources.RestartGTE10, value: 9, want: false},
		{filter: resources.RestartGTE10, value: 10, want: true},
		{filter: "bogus", value: 10, want: false},
	}
	for _, test := range cases {
		if got := restartMatches(test.value, test.filter); got != test.want {
			t.Fatalf("restartMatches(%d, %q) = %v, want %v", test.value, test.filter, got, test.want)
		}
	}
	if workloadKindRank("ReplicaSet") != 5 {
		t.Fatal("unknown workload kind rank mismatch")
	}
	if int64Compare(1, 2) != -1 || int64Compare(2, 1) != 1 || int64Compare(2, 2) != 0 {
		t.Fatal("int64Compare broken")
	}
	if pointerString(nil) != "" || pointerString(stringPointer("v")) != "v" {
		t.Fatal("pointerString broken")
	}
	set := stringSet([]string{"a", "b", "a"})
	if !set["a"] || !set["b"] || len(set) != 2 {
		t.Fatalf("stringSet = %v", set)
	}
	selection := resourceSelection(namespaces.SelectionBinding{Generation: "gen", Context: "ctx"}, namespaces.ScopeResolution{ScopeSource: "context-source", Namespaces: []string{"a"}})
	if selection.Scope != "context-source" || selection.Generation != "gen" || len(selection.Namespaces) != 1 {
		t.Fatalf("resource selection = %+v", selection)
	}
}

func TestMatchesSearchCompoundQuerySemantics(t *testing.T) {
	t.Parallel()
	newPods := func() []resources.PodDTO {
		return []resources.PodDTO{
			{Namespace: "a", Name: "api-1", Node: stringPointer("node one")},
			{Namespace: "a", Name: "api-2", Node: stringPointer("node two")},
			{Namespace: "b", Name: "worker-1"},
		}
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Search: "api -worker"}); len(got) != 2 {
		t.Fatalf("include/exclude result = %#v", got)
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Search: "\"node one\" -\"node two\""}); len(got) != 1 || got[0].Name != "api-1" {
		t.Fatalf("phrase result = %#v", got)
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Search: "!worker"}); len(got) != 2 {
		t.Fatalf("bang exclude result = %#v", got)
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Search: "api -worker", SearchQuery: resources.SearchQuery{Include: []string{"node one"}}}); len(got) != 1 || got[0].Name != "api-1" {
		t.Fatalf("preparsed query must take precedence over raw search = %#v", got)
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10}); len(got) != 3 {
		t.Fatalf("empty search result = %#v", got)
	}

	events := []resources.EventDTO{
		{Namespace: "a", ObjectKind: "Pod", ObjectName: "api", Reason: "BackOff", Message: "container oomkilled"},
		{Namespace: "a", ObjectKind: "Pod", ObjectName: "web", Reason: "Scheduled"},
	}
	if got := filterSortEvents(events, resources.ListOptions{Limit: 10, Search: "oom"}); len(got) != 1 || got[0].ObjectName != "api" {
		t.Fatalf("event message search = %#v", got)
	}

	services := []resources.ServiceDTO{
		{Namespace: "a", Name: "api", Type: "ClusterIP", ClusterIPs: []string{"10.0.0.15"}},
		{Namespace: "a", Name: "web", Type: "NodePort", ClusterIPs: []string{"10.0.0.20"}},
	}
	if got := filterSortServices(services, resources.ListOptions{Limit: 10, Search: "10.0.0.15"}); len(got) != 1 || got[0].Name != "api" {
		t.Fatalf("service ip search = %#v", got)
	}
	if got := filterSortServices(services, resources.ListOptions{Limit: 10, Search: "nodeport"}); len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("service type search = %#v", got)
	}

	ingresses := []resources.IngressDTO{
		{Namespace: "a", Name: "web", ClassName: stringPointer("nginx"), Hosts: []string{"app.example.com"}},
		{Namespace: "a", Name: "admin", Hosts: []string{"admin.example.com"}},
	}
	if got := filterSortIngresses(ingresses, resources.ListOptions{Limit: 10, Search: "app.example"}); len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("ingress host search = %#v", got)
	}
	if got := filterSortIngresses(ingresses, resources.ListOptions{Limit: 10, Search: "nginx"}); len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("ingress class search = %#v", got)
	}

	slices := []resources.EndpointSliceDTO{
		{Namespace: "a", Name: "api-abc", Endpoints: []resources.EndpointDTO{{Addresses: []string{"10.1.1.5"}}}},
		{Namespace: "a", Name: "web-abc", Endpoints: []resources.EndpointDTO{{Addresses: []string{"10.1.1.9"}}}},
	}
	if got := filterSortEndpointSlices(slices, resources.ListOptions{Limit: 10, Search: "10.1.1.5"}); len(got) != 1 || got[0].Name != "api-abc" {
		t.Fatalf("endpoint address search = %#v", got)
	}

	configMaps := []resources.ConfigMapListDTO{
		{Namespace: "a", Name: "settings", UID: "1"},
		{Namespace: "a", Name: "features", UID: "2"},
	}
	if got := filterSortConfigMaps(configMaps, resources.ListOptions{Limit: 10, Search: "settings"}); len(got) != 1 || got[0].Name != "settings" {
		t.Fatalf("config map search = %#v", got)
	}

	secrets := []resources.SecretMetadataDTO{
		{Metadata: resources.SecretMetadataFieldsDTO{Name: "credentials", Namespace: "a", UID: "1"}},
		{Metadata: resources.SecretMetadataFieldsDTO{Name: "tokens", Namespace: "a", UID: "2"}},
	}
	if got := filterSortSecrets(secrets, resources.ListOptions{Limit: 10, Search: "credentials"}); len(got) != 1 || got[0].Metadata.Name != "credentials" {
		t.Fatalf("secret search = %#v", got)
	}
}

func TestFilterSortCollectionSortingModes(t *testing.T) {
	t.Parallel()
	workloads := []resources.WorkloadDTO{
		{Kind: "Deployment", Namespace: "a", Name: "api", Status: resources.WorkloadHealthy, AgeSeconds: 30},
		{Kind: "Deployment", Namespace: "a", Name: "web", Status: resources.WorkloadDegraded, AgeSeconds: 10},
	}
	if got := filterSortWorkloads(workloads, resources.ListOptions{Limit: 10, Sort: "age", Order: resources.OrderDescending}); len(got) != 2 || got[0].Name != "api" {
		t.Fatalf("workload age sort = %#v", got)
	}
	if got := filterSortWorkloads(workloads, resources.ListOptions{Limit: 10, Sort: "name"}); len(got) != 2 || got[0].Name != "api" {
		t.Fatalf("workload name sort = %#v", got)
	}
	if got := filterSortWorkloads(workloads, resources.ListOptions{Limit: 10, Sort: "status", Statuses: []string{"Degraded"}}); len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("workload status filter+sort = %#v", got)
	}

	pods := []resources.PodDTO{
		{Namespace: "a", Name: "api-1", Status: "Running", Restarts: 4, AgeSeconds: 30},
		{Namespace: "a", Name: "api-2", Status: "Failed", Restarts: 1, AgeSeconds: 10},
	}
	if got := filterSortPods(pods, resources.ListOptions{Limit: 10, Sort: "restarts", Order: resources.OrderDescending}); len(got) != 2 || got[0].Restarts != 4 {
		t.Fatalf("pod restart sort = %#v", got)
	}
	if got := filterSortPods(pods, resources.ListOptions{Limit: 10, Sort: "status", Statuses: []string{"Running"}}); len(got) != 1 || got[0].Name != "api-1" {
		t.Fatalf("pod status filter = %#v", got)
	}
	if got := filterSortPods(pods, resources.ListOptions{Limit: 10, Sort: "age", Order: resources.OrderDescending}); len(got) != 2 || got[0].Name != "api-1" {
		t.Fatalf("pod age sort = %#v", got)
	}

	events := []resources.EventDTO{
		{Namespace: "a", ObjectKind: "Pod", ObjectName: "api", Reason: "BackOff", Type: "Warning", Count: 5},
		{Namespace: "a", ObjectKind: "Pod", ObjectName: "web", Reason: "Scheduled", Type: "Normal", Count: 2},
	}
	if got := filterSortEvents(events, resources.ListOptions{Limit: 10, Sort: "count", Order: resources.OrderDescending}); len(got) != 2 || got[0].Count != 5 {
		t.Fatalf("event count sort = %#v", got)
	}
	if got := filterSortEvents(events, resources.ListOptions{Limit: 10, Sort: "identity", ObjectKind: "Pod", Reason: "BackOff"}); len(got) != 1 || got[0].ObjectName != "api" {
		t.Fatalf("event identity sort + filters = %#v", got)
	}

	services := []resources.ServiceDTO{
		{Namespace: "a", Name: "web", Type: "NodePort"},
		{Namespace: "a", Name: "api", Type: "ClusterIP"},
	}
	if got := filterSortServices(services, resources.ListOptions{Limit: 10, Sort: "name", Order: resources.OrderDescending}); len(got) != 2 || got[0].Name != "web" {
		t.Fatalf("service name desc = %#v", got)
	}
	if got := filterSortServices(services, resources.ListOptions{Limit: 10, Sort: "type"}); len(got) != 2 || got[0].Type != "ClusterIP" {
		t.Fatalf("service type sort = %#v", got)
	}

	slices := []resources.EndpointSliceDTO{
		{Namespace: "a", Name: "web-abc", AddressType: "IPv6"},
		{Namespace: "a", Name: "api-abc", AddressType: "IPv4"},
	}
	if got := filterSortEndpointSlices(slices, resources.ListOptions{Limit: 10, Sort: "addressType"}); len(got) != 2 || got[0].AddressType != "IPv4" {
		t.Fatalf("endpoint slice address type sort = %#v", got)
	}
	if got := filterSortEndpointSlices(slices, resources.ListOptions{Limit: 10, AddressType: "IPv4", Sort: "name"}); len(got) != 1 || got[0].Name != "api-abc" {
		t.Fatalf("endpoint slice address filter = %#v", got)
	}

	configMaps := []resources.ConfigMapListDTO{
		{Namespace: "a", Name: "older", UID: "1", CreationTimestamp: "2026-01-01T00:00:00Z"},
		{Namespace: "a", Name: "newer", UID: "2", CreationTimestamp: "2026-02-01T00:00:00Z"},
	}
	if got := filterSortConfigMaps(configMaps, resources.ListOptions{Limit: 10, Sort: "createdAt", Order: resources.OrderDescending}); len(got) != 2 || got[0].Name != "newer" {
		t.Fatalf("config map createdAt desc = %#v", got)
	}
	if got := filterSortConfigMaps(configMaps, resources.ListOptions{Limit: 10, Sort: "name"}); len(got) != 2 || got[0].Name != "newer" {
		t.Fatalf("config map name sort = %#v", got)
	}

	secrets := []resources.SecretMetadataDTO{
		{Metadata: resources.SecretMetadataFieldsDTO{Name: "older", Namespace: "a", UID: "1", CreationTimestamp: "2026-01-01T00:00:00Z"}},
		{Metadata: resources.SecretMetadataFieldsDTO{Name: "newer", Namespace: "a", UID: "2", CreationTimestamp: "2026-02-01T00:00:00Z"}},
	}
	if got := filterSortSecrets(secrets, resources.ListOptions{Limit: 10, Sort: "createdAt"}); len(got) != 2 || got[0].Metadata.Name != "older" {
		t.Fatalf("secret createdAt sort = %#v", got)
	}
	if got := filterSortSecrets(secrets, resources.ListOptions{Limit: 10, Sort: "name", Order: resources.OrderDescending}); len(got) != 2 || got[0].Metadata.Name != "older" {
		t.Fatalf("secret name desc = %#v", got)
	}

	ingresses := []resources.IngressDTO{
		{Namespace: "a", Name: "web"},
		{Namespace: "a", Name: "admin"},
	}
	if got := filterSortIngresses(ingresses, resources.ListOptions{Limit: 10, Sort: "name", Order: resources.OrderDescending}); len(got) != 2 || got[0].Name != "web" {
		t.Fatalf("ingress name desc = %#v", got)
	}
}

func TestFilterSortPodsPodOnlyFilters(t *testing.T) {
	t.Parallel()
	newPods := func() []resources.PodDTO {
		return []resources.PodDTO{
			{Namespace: "a", Name: "api-1", Node: stringPointer("node-a"), Owner: &resources.OwnerDTO{Name: "api"}, Problematic: true, Restarts: 4},
			{Namespace: "a", Name: "api-2", Restarts: 0, Status: "Running"},
		}
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Node: "node-a"}); len(got) != 1 || got[0].Name != "api-1" {
		t.Fatalf("node filter = %#v", got)
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Workload: "api"}); len(got) != 1 || got[0].Name != "api-1" {
		t.Fatalf("workload filter = %#v", got)
	}
	problematic := false
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Problematic: &problematic}); len(got) != 1 || got[0].Name != "api-2" {
		t.Fatalf("problematic filter = %#v", got)
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Restarts: resources.RestartGT0}); len(got) != 1 || got[0].Name != "api-1" {
		t.Fatalf("restart filter = %#v", got)
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10, Restarts: "bogus"}); len(got) != 0 {
		t.Fatalf("invalid restart filter = %#v", got)
	}
	if got := filterSortPods(newPods(), resources.ListOptions{Limit: 10}); len(got) != 2 {
		t.Fatalf("unfiltered = %#v", got)
	}
}

func TestListCursorModeRejectsMixedOrigins(t *testing.T) {
	t.Parallel()
	if _, _, err := listCursorMode[resources.PodDTO](nil); err != nil {
		t.Fatalf("nil cursor err = %v", err)
	}
	global, namespaced, err := listCursorMode(&resources.CompositeCursor[resources.PodDTO]{Origins: []resources.OriginCursor[resources.PodDTO]{
		{Origin: resources.Origin{Version: "v1", Resource: "pods"}},
	}})
	if err != nil || !global || namespaced {
		t.Fatalf("global cursor = %v/%v/%v", global, namespaced, err)
	}
	namespacedCursor := &resources.CompositeCursor[resources.PodDTO]{Origins: []resources.OriginCursor[resources.PodDTO]{
		{Origin: resources.Origin{Namespace: "default", Version: "v1", Resource: "pods"}},
	}}
	global, namespaced, err = listCursorMode(namespacedCursor)
	if err != nil || global || !namespaced {
		t.Fatalf("namespaced cursor = %v/%v/%v", global, namespaced, err)
	}
	mixed := &resources.CompositeCursor[resources.PodDTO]{Origins: []resources.OriginCursor[resources.PodDTO]{
		{Origin: resources.Origin{Version: "v1", Resource: "pods"}},
		{Origin: resources.Origin{Namespace: "default", Version: "v1", Resource: "pods"}},
	}}
	if _, _, err := listCursorMode(mixed); err == nil {
		t.Fatal("mixed origins were accepted")
	}
}

func TestResourceBackendMapErrorHelpers(t *testing.T) {
	t.Parallel()
	if got := mapMetadataError(apierrors.NewGenericServerResponse(406, "", schema.GroupResource{}, "", "", 0, false), "Secret metadata is unavailable."); resources.ErrorCodeOf(got) != resources.CodeFeatureUnavailable {
		t.Fatalf("not acceptable mapping = %v", got)
	}
	if got := mapMetadataError(apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "x"), "Secret metadata is unavailable."); resources.ErrorCodeOf(got) != resources.CodeNotFound {
		t.Fatalf("not found metadata mapping = %v", got)
	}
	if got := mapOptionalResourceError(apierrors.NewNotFound(schema.GroupResource{Resource: "endpointslices"}, "x")); resources.ErrorCodeOf(got) != resources.CodeFeatureUnavailable {
		t.Fatalf("optional not found mapping = %v", got)
	}
	if got := mapOptionalResourceError(apierrors.NewTimeoutError("timeout", 1)); resources.ErrorCodeOf(got) != resources.CodeUpstreamTimeout {
		t.Fatalf("optional timeout mapping = %v", got)
	}
	if mapResourceError(nil) != nil {
		t.Fatal("nil error was mapped")
	}
}
