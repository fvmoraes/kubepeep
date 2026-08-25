package kubernetesruntime

import (
	"context"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kwatch "k8s.io/apimachinery/pkg/watch"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

type resourceWatchPort struct{ backend *ResourceBackend }

const maximumWatchFanout = 100

func (backend *ResourceBackend) AuthorizeTopics(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, topics []resources.Topic) (namespaces.ScopeResolution, error) {
	if resolution.PreferGlobal {
		global := resolution
		global.Namespaces = []string{metav1.NamespaceAll}
		if err := resources.AuthorizeTopics(ctx, backend.authorizer, resourceSelection(binding, global), topics); err == nil {
			return global, nil
		} else if code := resources.ErrorCodeOf(err); code != resources.CodeForbidden && code != resources.CodeAuthorizationUnavailable {
			return namespaces.ScopeResolution{}, err
		}
	}
	effective := resolution
	effective.PreferGlobal = false
	if err := validateWatchFanout(effective, topics); err != nil {
		return namespaces.ScopeResolution{}, err
	}
	if err := resources.AuthorizeTopics(ctx, backend.authorizer, resourceSelection(binding, effective), topics); err != nil {
		return namespaces.ScopeResolution{}, err
	}
	return effective, nil
}

func (backend *ResourceBackend) ReauthorizeTopics(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, topics []resources.Topic) error {
	if resolution.PreferGlobal {
		resolution.Namespaces = []string{metav1.NamespaceAll}
	} else if err := validateWatchFanout(resolution, topics); err != nil {
		return err
	}
	return resources.ReauthorizeTopics(ctx, backend.authorizer, resourceSelection(binding, resolution), topics)
}

func validateWatchFanout(resolution namespaces.ScopeResolution, topics []resources.Topic) error {
	fanout := 0
	for _, topic := range topics {
		fanout += len(resources.TopicGVRs(topic)) * len(resolution.Namespaces)
	}
	if len(resolution.Namespaces) > resources.MaximumNamespaces || fanout > maximumWatchFanout {
		return resourceDomain(resources.CodeLimitExceeded, "The selected scope is too large for namespace watch fan-out; use HTTP refresh.", nil)
	}
	return nil
}

func (backend *ResourceBackend) Subscribe(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, topic resources.Topic, gvr schema.GroupVersionResource, namespace string) (*resources.Subscription, error) {
	backend.watchMu.Lock()
	manager := backend.watchManager
	if manager != nil {
		backend.watchBindings[binding.Generation] = binding
		backend.watchGeneration = binding.Generation
	}
	backend.watchMu.Unlock()
	if manager == nil {
		return nil, resourceDomain(resources.CodeFeatureUnavailable, "Resource watches are unavailable.", nil)
	}
	selection := resourceSelection(binding, resolution)
	return manager.Subscribe(ctx, resources.WatchKey{Generation: binding.Generation, Context: binding.Context, Scope: selection.Scope, Topic: topic, GVR: gvr, Namespace: namespace})
}

func (port *resourceWatchPort) binding(generation string) (namespaces.SelectionBinding, error) {
	if port == nil || port.backend == nil {
		return namespaces.SelectionBinding{}, resourceDomain(resources.CodeFeatureUnavailable, "Resource watches are unavailable.", nil)
	}
	port.backend.watchMu.Lock()
	binding, ok := port.backend.watchBindings[generation]
	port.backend.watchMu.Unlock()
	if !ok || binding.Generation != generation {
		return namespaces.SelectionBinding{}, resourceDomain(resources.CodeGenerationChanged, "The active selection changed.", nil)
	}
	return binding, nil
}

func (port *resourceWatchPort) List(ctx context.Context, key resources.WatchKey) (resources.WatchSnapshot, error) {
	binding, err := port.binding(key.Generation)
	if err != nil {
		return resources.WatchSnapshot{}, err
	}
	requestContext, cancel, clients, err := port.backend.unary(ctx, binding)
	if err != nil {
		return resources.WatchSnapshot{}, err
	}
	defer cancel()
	if key.Topic != resources.TopicConfigMaps && clients.dynamic == nil {
		return resources.WatchSnapshot{}, resourceDomain(resources.CodeFeatureUnavailable, "Resource watches are unavailable.", nil)
	}
	return port.listWithClients(requestContext, binding, clients, key)
}

// Go interfaces cannot express client-go's covariant NamespaceableResource
// return type. Keep the actual list implementations concrete and small.
func (port *resourceWatchPort) listWithClients(ctx context.Context, binding namespaces.SelectionBinding, clients resourceClientSet, key resources.WatchKey) (resources.WatchSnapshot, error) {
	snapshot := resources.WatchSnapshot{Items: []resources.TopicObject{}}
	var cronJobs []batchv1.Job
	cronHistoryComplete := false
	if key.GVR.Resource == "cronjobs" {
		cronJobs, cronHistoryComplete = port.backend.cronJobHistory(ctx, binding, clients.kubernetes, key.Namespace)
	}
	continueToken := ""
	for {
		options := metav1.ListOptions{Limit: 500, Continue: continueToken}
		if key.Topic == resources.TopicConfigMaps {
			list, err := clients.metadata.Resource(key.GVR).Namespace(key.Namespace).List(ctx, options)
			if err != nil {
				return resources.WatchSnapshot{}, mapMetadataError(err, "ConfigMap metadata watches are unavailable.")
			}
			for index := range list.Items {
				snapshot.Items = append(snapshot.Items, resources.ConvertConfigMapMetadata(&list.Items[index]))
			}
			snapshot.ResourceVersion, continueToken = list.ResourceVersion, list.Continue
		} else {
			list, err := clients.dynamic.Resource(key.GVR).Namespace(key.Namespace).List(ctx, options)
			if err != nil {
				return resources.WatchSnapshot{}, mapResourceError(err)
			}
			for index := range list.Items {
				item, convertErr := port.convertWithCronHistory(key, &list.Items[index], cronJobs, cronHistoryComplete)
				if convertErr != nil {
					return resources.WatchSnapshot{}, convertErr
				}
				snapshot.Items = append(snapshot.Items, item)
			}
			snapshot.ResourceVersion, continueToken = list.GetResourceVersion(), list.GetContinue()
		}
		if len(snapshot.Items) > resources.MaximumSnapshotItems {
			return resources.WatchSnapshot{}, resourceDomain(resources.CodeLimitExceeded, "The initial snapshot is too large.", nil)
		}
		if continueToken == "" {
			return snapshot, nil
		}
	}
}

func (port *resourceWatchPort) Watch(ctx context.Context, key resources.WatchKey, resourceVersion string, timeoutSeconds int64, bookmarks bool) (resources.WatchStream, error) {
	binding, err := port.binding(key.Generation)
	if err != nil {
		return nil, err
	}
	lease, err := port.backend.runtime.leaseFor(ctx, binding)
	if err != nil {
		return nil, mapResourceError(err)
	}
	streamContext, err := lease.Generation.Stream(ctx, time.Duration(timeoutSeconds+30)*time.Second)
	if err != nil {
		return nil, mapResourceError(err)
	}
	options := metav1.ListOptions{ResourceVersion: resourceVersion, TimeoutSeconds: &timeoutSeconds, AllowWatchBookmarks: bookmarks}
	var source kwatch.Interface
	if key.Topic == resources.TopicConfigMaps {
		client := lease.Clients.StreamingMetadata()
		if client == nil {
			streamContext.Close()
			return nil, resourceDomain(resources.CodeFeatureUnavailable, "ConfigMap metadata watches are unavailable.", nil)
		}
		source, err = client.Resource(key.GVR).Namespace(key.Namespace).Watch(streamContext.Context(), options)
		if err != nil {
			streamContext.Close()
			return nil, mapMetadataError(err, "ConfigMap metadata watches are unavailable.")
		}
	} else {
		client := lease.Clients.StreamingDynamic()
		if client == nil {
			streamContext.Close()
			return nil, resourceDomain(resources.CodeFeatureUnavailable, "Resource watches are unavailable.", nil)
		}
		source, err = client.Resource(key.GVR).Namespace(key.Namespace).Watch(streamContext.Context(), options)
		if err != nil {
			streamContext.Close()
			return nil, mapResourceError(err)
		}
	}
	result := &resourceWatchStream{ctx: streamContext.Context(), source: source, cancel: streamContext.Close, results: make(chan resources.WatchChange, 64)}
	go result.run(key, port)
	return result, nil
}

type resourceWatchStream struct {
	ctx     context.Context
	source  kwatch.Interface
	cancel  func()
	results chan resources.WatchChange
	once    sync.Once
}

func (stream *resourceWatchStream) ResultChan() <-chan resources.WatchChange { return stream.results }
func (stream *resourceWatchStream) Stop() {
	stream.once.Do(func() { stream.source.Stop(); stream.cancel() })
}

func (stream *resourceWatchStream) run(key resources.WatchKey, port *resourceWatchPort) {
	defer close(stream.results)
	defer stream.Stop()
	for event := range stream.source.ResultChan() {
		change := resources.WatchChange{Type: string(event.Type)}
		if event.Type == kwatch.Bookmark {
			continue
		}
		if event.Type == kwatch.Error {
			if apierrors.IsResourceExpired(apierrors.FromObject(event.Object)) || apierrors.IsGone(apierrors.FromObject(event.Object)) {
				change.Err = resources.ErrResourceExpired
			} else {
				change.Err = mapResourceError(apierrors.FromObject(event.Object))
			}
			stream.results <- change
			return
		}
		accessor, err := meta.Accessor(event.Object)
		if err != nil {
			change.Err = resourceDomain(resources.CodeClusterUnavailable, "The Kubernetes watch returned an invalid object.", err)
			stream.results <- change
			return
		}
		change.ResourceVersion = accessor.GetResourceVersion()
		if event.Type == kwatch.Deleted {
			change.Deleted = &resources.ResourceRef{APIGroup: key.GVR.Group, Kind: kindForGVR(key.GVR), Namespace: accessor.GetNamespace(), Name: accessor.GetName(), UID: string(accessor.GetUID())}
		} else {
			object, convertErr := port.convertRuntime(stream.ctx, key, event.Object)
			if convertErr != nil {
				change.Err = convertErr
				stream.results <- change
				return
			}
			change.Object = object
		}
		stream.results <- change
	}
}

func (port *resourceWatchPort) convertRuntime(ctx context.Context, key resources.WatchKey, object kruntime.Object) (resources.TopicObject, error) {
	if key.Topic == resources.TopicConfigMaps {
		metadataObject, ok := object.(*metav1.PartialObjectMetadata)
		if !ok {
			return nil, resourceDomain(resources.CodeFeatureUnavailable, "ConfigMap metadata watches are unavailable.", nil)
		}
		return resources.ConvertConfigMapMetadata(metadataObject), nil
	}
	unstructuredObject, ok := object.(*unstructured.Unstructured)
	if !ok {
		return nil, resourceDomain(resources.CodeClusterUnavailable, "The Kubernetes watch returned an invalid object.", nil)
	}
	if key.GVR.Resource == "cronjobs" {
		binding, err := port.binding(key.Generation)
		if err != nil {
			return nil, err
		}
		requestContext, cancel, clients, err := port.backend.unary(ctx, binding)
		if err != nil {
			return nil, err
		}
		defer cancel()
		jobs, complete := port.backend.cronJobHistory(requestContext, binding, clients.kubernetes, key.Namespace)
		return port.convertWithCronHistory(key, unstructuredObject, jobs, complete)
	}
	return port.convertWithCronHistory(key, unstructuredObject, nil, false)
}

func (port *resourceWatchPort) convertWithCronHistory(key resources.WatchKey, object *unstructured.Unstructured, cronJobs []batchv1.Job, cronHistoryComplete bool) (resources.TopicObject, error) {
	now := port.backend.now().UTC()
	switch key.GVR.Resource {
	case "pods":
		var value corev1.Pod
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertPod(&value, now), nil
	case "events":
		var value corev1.Event
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertEvent(&value, port.backend.redactor), nil
	case "deployments":
		var value appsv1.Deployment
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertDeployment(&value, now), nil
	case "statefulsets":
		var value appsv1.StatefulSet
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertStatefulSet(&value, now), nil
	case "daemonsets":
		var value appsv1.DaemonSet
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertDaemonSet(&value, now), nil
	case "jobs":
		var value batchv1.Job
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertJob(&value, now), nil
	case "cronjobs":
		var value batchv1.CronJob
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertCronJobWithHistory(&value, cronJobs, cronHistoryComplete && value.UID != "", now), nil
	case "services":
		var value corev1.Service
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertService(&value), nil
	case "ingresses":
		var value networkingv1.Ingress
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertIngress(&value)
	case "endpointslices":
		var value discoveryv1.EndpointSlice
		if err := kruntime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &value); err != nil {
			return nil, mapResourceError(err)
		}
		return resources.ConvertEndpointSlice(&value), nil
	default:
		return nil, resourceDomain(resources.CodeValidationFailed, "The watch resource is invalid.", nil)
	}
}

func kindForGVR(gvr schema.GroupVersionResource) string {
	switch gvr.Resource {
	case "pods":
		return "Pod"
	case "events":
		return "Event"
	case "deployments":
		return "Deployment"
	case "statefulsets":
		return "StatefulSet"
	case "daemonsets":
		return "DaemonSet"
	case "jobs":
		return "Job"
	case "cronjobs":
		return "CronJob"
	case "services":
		return "Service"
	case "ingresses":
		return "Ingress"
	case "endpointslices":
		return "EndpointSlice"
	case "configmaps":
		return "ConfigMap"
	default:
		return "Unknown"
	}
}

var _ resources.WatchPort = (*resourceWatchPort)(nil)
var _ resources.WatchStream = (*resourceWatchStream)(nil)
