package kubernetesruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kwatch "k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

func watchTestBinding() namespaces.SelectionBinding {
	return namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
}

func watchTestResolution() namespaces.ScopeResolution {
	return namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
}

func TestAuthorizeTopicsFallsBackWhenGlobalDecisionUnknown(t *testing.T) {
	t.Parallel()
	backend := &ResourceBackend{authorizer: &namespaceAwareResourceAuthorization{globalDecision: authorization.DecisionUnknown}}
	effective, err := backend.AuthorizeTopics(context.Background(), watchTestBinding(), namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"a", "b"}, PreferGlobal: true}, []resources.Topic{resources.TopicPods})
	if err != nil {
		t.Fatal(err)
	}
	if effective.PreferGlobal || len(effective.Namespaces) != 2 {
		t.Fatalf("effective = %#v", effective)
	}

	if err := (&ResourceBackend{authorizer: &allowResourceAuthorization{}}).ReauthorizeTopics(context.Background(), watchTestBinding(), namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{metav1.NamespaceAll}, PreferGlobal: true}, []resources.Topic{resources.TopicPods}); err != nil {
		t.Fatalf("global reauthorization err = %v", err)
	}

	names := make([]string, resources.MaximumNamespaces+1)
	for index := range names {
		names[index] = "namespace-" + string(rune('a'+index%26)) + string(rune('a'+index/26))
	}
	if err := backend.ReauthorizeTopics(context.Background(), watchTestBinding(), namespaces.ScopeResolution{ScopeName: "scope", Namespaces: names}, []resources.Topic{resources.TopicPods}); resources.ErrorCodeOf(err) != resources.CodeLimitExceeded {
		t.Fatalf("oversized reauthorization err = %v", err)
	}
}

func TestSubscribeRequiresWatchManagerAndRegistersBinding(t *testing.T) {
	t.Parallel()
	backend := &ResourceBackend{watchBindings: map[string]namespaces.SelectionBinding{}}
	if _, err := backend.Subscribe(context.Background(), watchTestBinding(), watchTestResolution(), resources.TopicPods, schema.GroupVersionResource{Version: "v1", Resource: "pods"}, "default"); resources.ErrorCodeOf(err) != resources.CodeFeatureUnavailable {
		t.Fatalf("missing manager err = %v", err)
	}

	backend.watchManager = resources.NewWatchManager(&resourceWatchPort{backend: backend})
	subscription, err := backend.Subscribe(context.Background(), watchTestBinding(), watchTestResolution(), resources.TopicPods, schema.GroupVersionResource{Version: "v1", Resource: "pods"}, "default")
	if err != nil || subscription == nil {
		t.Fatalf("subscribe = %v err = %v", subscription, err)
	}
	backend.watchMu.Lock()
	binding, registered := backend.watchBindings["gen"]
	backend.watchMu.Unlock()
	if !registered || binding.Generation != "gen" || backend.watchGeneration != "gen" {
		t.Fatalf("binding registration failed: %+v %q", binding, backend.watchGeneration)
	}
	subscription.Close()
	backend.watchManager.Close()
}

func TestResourceWatchPortBindingValidation(t *testing.T) {
	t.Parallel()
	var nilPort *resourceWatchPort
	if _, err := nilPort.binding("gen"); resources.ErrorCodeOf(err) != resources.CodeFeatureUnavailable {
		t.Fatalf("nil port err = %v", err)
	}
	orphanPort := &resourceWatchPort{}
	if _, err := orphanPort.binding("gen"); resources.ErrorCodeOf(err) != resources.CodeFeatureUnavailable {
		t.Fatalf("nil backend err = %v", err)
	}
	backend := &ResourceBackend{watchBindings: map[string]namespaces.SelectionBinding{"gen": watchTestBinding()}}
	port := &resourceWatchPort{backend: backend}
	if _, err := port.binding("gen"); err != nil {
		t.Fatalf("registered binding err = %v", err)
	}
	if _, err := port.binding("missing"); resources.ErrorCodeOf(err) != resources.CodeGenerationChanged {
		t.Fatalf("unknown generation err = %v", err)
	}
	backend.watchBindings["mismatch"] = namespaces.SelectionBinding{Generation: "other"}
	if _, err := port.binding("mismatch"); resources.ErrorCodeOf(err) != resources.CodeGenerationChanged {
		t.Fatalf("mismatched generation err = %v", err)
	}
}

func TestResourceWatchPortListValidatesClientsAndSnapshots(t *testing.T) {
	t.Parallel()
	binding := watchTestBinding()
	key := resources.WatchKey{Generation: "gen", Context: "ctx", Scope: "scope", Topic: resources.TopicPods, GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespace: "default"}

	backend := &ResourceBackend{
		clients:    fixedResourceClientProvider{set: resourceClientSet{kubernetes: kubefake.NewSimpleClientset()}},
		authorizer: &allowResourceAuthorization{}, now: time.Now,
		watchBindings: map[string]namespaces.SelectionBinding{"gen": binding},
	}
	port := &resourceWatchPort{backend: backend}
	if _, err := port.List(context.Background(), key); resources.ErrorCodeOf(err) != resources.CodeFeatureUnavailable {
		t.Fatalf("missing dynamic client err = %v", err)
	}

	missingBindingKey := key
	missingBindingKey.Generation = "unknown"
	if _, err := port.List(context.Background(), missingBindingKey); resources.ErrorCodeOf(err) != resources.CodeGenerationChanged {
		t.Fatalf("unregistered binding err = %v", err)
	}

	podObject := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", UID: "pod-uid", ResourceVersion: "3"}}
	listScheme := runtime.NewScheme()
	if err := corev1.AddToScheme(listScheme); err != nil {
		t.Fatal(err)
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(listScheme, map[schema.GroupVersionResource]string{{Version: "v1", Resource: "pods"}: "PodList"}, podObject)
	backend.clients = fixedResourceClientProvider{set: resourceClientSet{kubernetes: kubefake.NewSimpleClientset(), dynamic: dynamicClient}}
	snapshot, err := port.List(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("snapshot items = %#v", snapshot.Items)
	}
	if podDTO, ok := snapshot.Items[0].(resources.PodDTO); !ok || podDTO.Name != "api" {
		t.Fatalf("snapshot item = %#v", snapshot.Items[0])
	}

	configMapKey := resources.WatchKey{Generation: "gen", Context: "ctx", Scope: "scope", Topic: resources.TopicConfigMaps, GVR: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, Namespace: "default"}
	metadataScheme := metadatafake.NewTestScheme()
	metav1.AddMetaToScheme(metadataScheme)
	metadataClient := metadatafake.NewSimpleMetadataClient(metadataScheme, &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "settings", UID: "cm-uid"},
	})
	backend.clients = fixedResourceClientProvider{set: resourceClientSet{kubernetes: kubefake.NewSimpleClientset(), metadata: metadataClient}}
	configMapSnapshot, err := port.List(context.Background(), configMapKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(configMapSnapshot.Items) != 1 {
		t.Fatalf("config map snapshot = %#v", configMapSnapshot.Items)
	}
	if _, ok := configMapSnapshot.Items[0].(resources.ConfigMapListDTO); !ok {
		t.Fatalf("config map item type = %T", configMapSnapshot.Items[0])
	}
}

func TestWatchRejectsStaleBindingOrMissingLease(t *testing.T) {
	t.Parallel()
	binding := watchTestBinding()
	key := resources.WatchKey{Generation: "gen", Context: "ctx", Scope: "scope", Topic: resources.TopicPods, GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespace: "default"}
	port := &resourceWatchPort{backend: &ResourceBackend{runtime: &Runtime{}}}
	if _, err := port.Watch(context.Background(), key, "1", 30, false); resources.ErrorCodeOf(err) != resources.CodeGenerationChanged {
		t.Fatalf("unregistered binding err = %v", err)
	}
	port.backend.watchBindings = map[string]namespaces.SelectionBinding{"gen": binding}
	if _, err := port.Watch(context.Background(), key, "1", 30, false); err == nil || resources.ErrorCodeOf(err) == resources.CodeGenerationChanged {
		t.Fatalf("lease failure err = %v", err)
	}
}

func unstructuredPod(name, uid string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"namespace": "default", "name": name, "uid": uid, "resourceVersion": "7"},
	}}
}

func collectWatchChanges(t *testing.T, stream *resourceWatchStream) []resources.WatchChange {
	t.Helper()
	changes := []resources.WatchChange{}
	for change := range stream.ResultChan() {
		changes = append(changes, change)
	}
	return changes
}

func TestWatchStreamRunProcessesAddsDeletesAndBookmarks(t *testing.T) {
	t.Parallel()
	backend := &ResourceBackend{authorizer: &allowResourceAuthorization{}, now: time.Now}
	port := &resourceWatchPort{backend: backend}
	key := resources.WatchKey{Generation: "gen", Context: "ctx", Scope: "scope", Topic: resources.TopicPods, GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespace: "default"}
	source := kwatch.NewFake()
	stream := &resourceWatchStream{ctx: context.Background(), source: source, cancel: func() {}, results: make(chan resources.WatchChange, 16)}
	done := make(chan []resources.WatchChange, 1)
	go func() {
		stream.run(key, port)
		done <- collectWatchChanges(t, stream)
	}()

	source.Add(unstructuredPod("api", "uid-1"))
	source.Modify(unstructuredPod("api", "uid-1"))
	source.Delete(unstructuredPod("api", "uid-1"))
	source.Add(&metav1.Status{Status: metav1.StatusSuccess})
	source.Stop()

	changes := <-done
	if len(changes) != 4 {
		t.Fatalf("changes = %#v", changes)
	}
	if changes[0].Type != string(kwatch.Added) {
		t.Fatalf("first change type = %q", changes[0].Type)
	}
	if _, ok := changes[0].Object.(resources.PodDTO); !ok {
		t.Fatalf("added object = %#v", changes[0].Object)
	}
	if changes[1].Type != string(kwatch.Modified) || changes[1].Object == nil {
		t.Fatalf("modified change = %#v", changes[1])
	}
	if changes[2].Deleted == nil || changes[2].Deleted.Kind != "Pod" || changes[2].Deleted.Name != "api" || changes[2].Deleted.UID != "uid-1" {
		t.Fatalf("deleted ref = %#v", changes[2].Deleted)
	}
	if changes[2].ResourceVersion != "7" {
		t.Fatalf("resource version = %q", changes[2].ResourceVersion)
	}
	if changes[3].Err == nil || resources.ErrorCodeOf(changes[3].Err) != resources.CodeClusterUnavailable {
		t.Fatalf("invalid object change = %#v", changes[3])
	}
}

func TestWatchStreamRunMapsExpiredAndTerminalErrors(t *testing.T) {
	t.Parallel()
	backend := &ResourceBackend{authorizer: &allowResourceAuthorization{}, now: time.Now}
	port := &resourceWatchPort{backend: backend}
	key := resources.WatchKey{Generation: "gen", Context: "ctx", Scope: "scope", Topic: resources.TopicPods, GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespace: "default"}

	source := kwatch.NewFake()
	stream := &resourceWatchStream{ctx: context.Background(), source: source, cancel: func() {}, results: make(chan resources.WatchChange, 16)}
	done := make(chan []resources.WatchChange, 1)
	go func() {
		stream.run(key, port)
		done <- collectWatchChanges(t, stream)
	}()
	source.Error(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonExpired})
	source.Stop()
	changes := <-done
	if len(changes) != 1 || !errors.Is(changes[0].Err, resources.ErrResourceExpired) {
		t.Fatalf("expired changes = %#v", changes)
	}

	notFoundSource := kwatch.NewFake()
	notFoundStream := &resourceWatchStream{ctx: context.Background(), source: notFoundSource, cancel: func() {}, results: make(chan resources.WatchChange, 16)}
	done = make(chan []resources.WatchChange, 1)
	go func() {
		notFoundStream.run(key, port)
		done <- collectWatchChanges(t, notFoundStream)
	}()
	notFoundSource.Error(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonNotFound})
	notFoundSource.Stop()
	changes = <-done
	if len(changes) != 1 || resources.ErrorCodeOf(changes[0].Err) != resources.CodeNotFound {
		t.Fatalf("not found changes = %#v", changes)
	}
}

func TestWatchStreamRunRejectsInvalidConvertsAndObjects(t *testing.T) {
	t.Parallel()
	backend := &ResourceBackend{authorizer: &allowResourceAuthorization{}, now: time.Now}
	port := &resourceWatchPort{backend: backend}
	bogusKey := resources.WatchKey{Generation: "gen", Context: "ctx", Scope: "scope", Topic: resources.TopicPods, GVR: schema.GroupVersionResource{Version: "v1", Resource: "bogus"}, Namespace: "default"}

	source := kwatch.NewFake()
	stream := &resourceWatchStream{ctx: context.Background(), source: source, cancel: func() {}, results: make(chan resources.WatchChange, 16)}
	done := make(chan []resources.WatchChange, 1)
	go func() {
		stream.run(bogusKey, port)
		done <- collectWatchChanges(t, stream)
	}()
	source.Add(unstructuredPod("api", "uid-1"))
	source.Stop()
	changes := <-done
	if len(changes) != 1 || resources.ErrorCodeOf(changes[0].Err) != resources.CodeValidationFailed {
		t.Fatalf("invalid watch resource changes = %#v", changes)
	}

	nonMetaSource := kwatch.NewFake()
	nonMetaStream := &resourceWatchStream{ctx: context.Background(), source: nonMetaSource, cancel: func() {}, results: make(chan resources.WatchChange, 16)}
	done = make(chan []resources.WatchChange, 1)
	go func() {
		nonMetaStream.run(bogusKey, port)
		done <- collectWatchChanges(t, nonMetaStream)
	}()
	nonMetaSource.Add(&metav1.Status{Status: metav1.StatusSuccess})
	nonMetaSource.Stop()
	changes = <-done
	if len(changes) != 1 || resources.ErrorCodeOf(changes[0].Err) != resources.CodeClusterUnavailable {
		t.Fatalf("invalid metadata object changes = %#v", changes)
	}
}
