package kubernetesruntime

import (
	"context"
	"errors"
	"io"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"

	kubeadapter "github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

type resourceClientSet struct {
	kubernetes       kubeclient.Interface
	streaming        kubeclient.Interface
	dynamic          dynamic.Interface
	streamingDynamic dynamic.Interface
	metadata         metadata.Interface
	streamMetadata   metadata.Interface
}

type resourceClientProvider interface {
	Unary(context.Context, namespaces.SelectionBinding) (context.Context, context.CancelFunc, resourceClientSet, error)
}
type runtimeResourceClientProvider struct{ runtime *Runtime }

func (backend *ResourceBackend) unary(ctx context.Context, binding namespaces.SelectionBinding) (context.Context, context.CancelFunc, resourceClientSet, error) {
	if backend == nil || backend.clients == nil {
		return nil, nil, resourceClientSet{}, resourceDomain(resources.CodeFeatureUnavailable, "The Kubernetes resource reader is unavailable.", nil)
	}
	return backend.clients.Unary(ctx, binding)
}

func (provider runtimeResourceClientProvider) Unary(ctx context.Context, binding namespaces.SelectionBinding) (context.Context, context.CancelFunc, resourceClientSet, error) {
	if provider.runtime == nil || binding.ClusterProfileID <= 0 || binding.Context == "" || binding.Generation == "" {
		return nil, nil, resourceClientSet{}, resourceDomain(resources.CodeFeatureUnavailable, "The Kubernetes resource reader is unavailable.", nil)
	}
	lease, err := provider.runtime.leaseFor(ctx, binding)
	if err != nil {
		return nil, nil, resourceClientSet{}, mapResourceError(err)
	}
	requestContext, cancel, err := lease.Generation.Unary(ctx)
	if err != nil {
		return nil, nil, resourceClientSet{}, mapResourceError(err)
	}
	clients := resourceClientSet{
		kubernetes: lease.Clients.UnaryKubernetes(), streaming: lease.Clients.StreamingKubernetes(),
		dynamic: lease.Clients.UnaryDynamic(), streamingDynamic: lease.Clients.StreamingDynamic(),
		metadata: lease.Clients.UnaryMetadata(), streamMetadata: lease.Clients.StreamingMetadata(),
	}
	if clients.kubernetes == nil || clients.metadata == nil {
		cancel()
		return nil, nil, resourceClientSet{}, resourceDomain(resources.CodeFeatureUnavailable, "The Kubernetes resource reader is unavailable.", nil)
	}
	return requestContext, cancel, clients, nil
}

func (backend *ResourceBackend) listWorkloadPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.WorkloadDTO], error) {
	result := resources.OriginPage[resources.WorkloadDTO]{Origin: page.Origin, Items: []resources.WorkloadDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	options := metav1.ListOptions{Limit: page.Limit, Continue: page.Continue}
	now := backend.now().UTC()
	switch page.Origin.Resource {
	case "deployments":
		list, listErr := clients.kubernetes.AppsV1().Deployments(page.Origin.Namespace).List(requestContext, options)
		if listErr != nil {
			return result, mapResourceError(listErr)
		}
		for index := range list.Items {
			result.Items = append(result.Items, resources.ConvertDeployment(&list.Items[index], now))
		}
		result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	case "statefulsets":
		list, listErr := clients.kubernetes.AppsV1().StatefulSets(page.Origin.Namespace).List(requestContext, options)
		if listErr != nil {
			return result, mapResourceError(listErr)
		}
		for index := range list.Items {
			result.Items = append(result.Items, resources.ConvertStatefulSet(&list.Items[index], now))
		}
		result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	case "daemonsets":
		list, listErr := clients.kubernetes.AppsV1().DaemonSets(page.Origin.Namespace).List(requestContext, options)
		if listErr != nil {
			return result, mapResourceError(listErr)
		}
		for index := range list.Items {
			result.Items = append(result.Items, resources.ConvertDaemonSet(&list.Items[index], now))
		}
		result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	case "jobs":
		list, listErr := clients.kubernetes.BatchV1().Jobs(page.Origin.Namespace).List(requestContext, options)
		if listErr != nil {
			return result, mapResourceError(listErr)
		}
		for index := range list.Items {
			result.Items = append(result.Items, resources.ConvertJob(&list.Items[index], now))
		}
		result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	case "cronjobs":
		list, listErr := clients.kubernetes.BatchV1().CronJobs(page.Origin.Namespace).List(requestContext, options)
		if listErr != nil {
			return result, mapResourceError(listErr)
		}
		jobs, historyComplete := backend.cronJobHistory(requestContext, binding, clients.kubernetes, page.Origin.Namespace)
		for index := range list.Items {
			completeForObject := historyComplete && list.Items[index].UID != ""
			result.Items = append(result.Items, resources.ConvertCronJobWithHistory(&list.Items[index], jobs, completeForObject, now))
		}
		result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	default:
		return result, resourceDomain(resources.CodeValidationFailed, "The workload kind is invalid.", nil)
	}
	return result, nil
}

func (backend *ResourceBackend) listPodPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.PodDTO], error) {
	result := resources.OriginPage[resources.PodDTO]{Origin: page.Origin, Items: []resources.PodDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoreV1().Pods(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertPod(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listEventPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.EventDTO], error) {
	result := resources.OriginPage[resources.EventDTO]{Origin: page.Origin, Items: []resources.EventDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoreV1().Events(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertEvent(&list.Items[index], backend.redactor))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listServicePage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.ServiceDTO], error) {
	result := resources.OriginPage[resources.ServiceDTO]{Origin: page.Origin, Items: []resources.ServiceDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoreV1().Services(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertService(&list.Items[index]))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}
func (backend *ResourceBackend) listIngressPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.IngressDTO], error) {
	result := resources.OriginPage[resources.IngressDTO]{Origin: page.Origin, Items: []resources.IngressDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.NetworkingV1().Ingresses(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	for index := range list.Items {
		dto, convertErr := resources.ConvertIngress(&list.Items[index])
		if convertErr != nil {
			return result, convertErr
		}
		result.Items = append(result.Items, dto)
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}
func (backend *ResourceBackend) listEndpointSlicePage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.EndpointSliceDTO], error) {
	result := resources.OriginPage[resources.EndpointSliceDTO]{Origin: page.Origin, Items: []resources.EndpointSliceDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.DiscoveryV1().EndpointSlices(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapOptionalResourceError(err)
	}
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertEndpointSlice(&list.Items[index]))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listConfigMapPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.ConfigMapListDTO], error) {
	result := resources.OriginPage[resources.ConfigMapListDTO]{Origin: page.Origin, Items: []resources.ConfigMapListDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	list, err := clients.metadata.Resource(gvr).Namespace(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapMetadataError(err, "ConfigMap metadata is unavailable.")
	}
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertConfigMapMetadata(&list.Items[index]))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}
func (backend *ResourceBackend) listSecretPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.SecretMetadataDTO], error) {
	result := resources.OriginPage[resources.SecretMetadataDTO]{Origin: page.Origin, Items: []resources.SecretMetadataDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	list, err := clients.metadata.Resource(gvr).Namespace(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapMetadataError(err, "Secret metadata is unavailable.")
	}
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertSecretMetadata(list.Items[index]))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

type resourceGetterFunc[T resources.DetailItem] func(context.Context, resources.Origin, string) (T, error)

func (function resourceGetterFunc[T]) Get(ctx context.Context, origin resources.Origin, name string) (T, error) {
	return function(ctx, origin, name)
}

func getAuthorized[T resources.DetailItem](ctx context.Context, backend *ResourceBackend, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, origin resources.Origin, name string, get resourceGetterFunc[T]) (T, error) {
	return resources.GetAuthorized(ctx, resources.GetRequest[T]{Selection: resourceSelection(binding, resolution), Origin: origin, Name: name, Getter: get, Authorizer: backend.authorizer})
}

func (backend *ResourceBackend) GetWorkload(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, kind, namespace, name string) (resources.WorkloadDetailDTO, error) {
	origin, err := workloadOrigin(kind, namespace)
	if err != nil {
		return resources.WorkloadDetailDTO{}, err
	}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.WorkloadDetailDTO, error) {
		value, err := backend.getWorkloadObject(ctx, binding, kind, namespace, name)
		if err != nil {
			return resources.WorkloadDetailDTO{}, err
		}
		related, jobs, historyComplete := backend.relatedWorkload(ctx, binding, value)
		now := backend.now().UTC()
		switch object := value.(type) {
		case *appsv1.Deployment:
			return resources.DeploymentDetail(object, related, now), nil
		case *appsv1.StatefulSet:
			return resources.StatefulSetDetail(object, related, now), nil
		case *appsv1.DaemonSet:
			return resources.DaemonSetDetail(object, related, now), nil
		case *batchv1.Job:
			return resources.JobDetail(object, related, now), nil
		case *batchv1.CronJob:
			return resources.CronJobDetailWithHistory(object, jobs, historyComplete, related, now), nil
		default:
			return resources.WorkloadDetailDTO{}, resourceDomain(resources.CodeFeatureUnavailable, "The workload reader is unavailable.", nil)
		}
	})
}

// The sealed DetailItem contract intentionally prevents local wrapper types.
// YAML authorization therefore uses the same DTO getter once and fetches the
// raw object only after the authorization decision. The helpers below avoid a
// second SAR while keeping raw objects confined to this package.
func (backend *ResourceBackend) authorizeGet(ctx context.Context, binding namespaces.SelectionBinding, origin resources.Origin, name string) error {
	capability := backend.authorizer.Check(ctx, authorization.Key{Generation: binding.Generation, Namespace: origin.Namespace, APIGroup: origin.APIGroup, Resource: origin.Resource, Verb: "get", ResourceName: name})
	switch capability.Decision {
	case authorization.DecisionAllowed:
		return nil
	case authorization.DecisionDenied:
		return resourceDomain(resources.CodeForbidden, "Access to this resource was denied.", nil)
	default:
		return resourceDomain(resources.CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
	}
}

func (backend *ResourceBackend) WorkloadYAMLDocument(ctx context.Context, binding namespaces.SelectionBinding, kind, namespace, name string) ([]byte, error) {
	origin, err := workloadOrigin(kind, namespace)
	if err != nil {
		return nil, err
	}
	if err = backend.authorizeGet(ctx, binding, origin, name); err != nil {
		return nil, err
	}
	value, err := backend.getWorkloadObject(ctx, binding, kind, namespace, name)
	if err != nil {
		return nil, err
	}
	return resources.MarshalReadOnlyYAML(value)
}

func (backend *ResourceBackend) getWorkloadObject(ctx context.Context, binding namespaces.SelectionBinding, kind, namespace, name string) (any, error) {
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return nil, err
	}
	defer cancel()
	switch kind {
	case "deployments":
		value, getErr := clients.kubernetes.AppsV1().Deployments(namespace).Get(requestContext, name, metav1.GetOptions{})
		return value, mapResourceError(getErr)
	case "statefulsets":
		value, getErr := clients.kubernetes.AppsV1().StatefulSets(namespace).Get(requestContext, name, metav1.GetOptions{})
		return value, mapResourceError(getErr)
	case "daemonsets":
		value, getErr := clients.kubernetes.AppsV1().DaemonSets(namespace).Get(requestContext, name, metav1.GetOptions{})
		return value, mapResourceError(getErr)
	case "jobs":
		value, getErr := clients.kubernetes.BatchV1().Jobs(namespace).Get(requestContext, name, metav1.GetOptions{})
		return value, mapResourceError(getErr)
	case "cronjobs":
		value, getErr := clients.kubernetes.BatchV1().CronJobs(namespace).Get(requestContext, name, metav1.GetOptions{})
		return value, mapResourceError(getErr)
	default:
		return nil, resourceDomain(resources.CodeValidationFailed, "The workload kind is invalid.", nil)
	}
}

func (backend *ResourceBackend) GetPod(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.PodDetailDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "pods"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.PodDetailDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.PodDetailDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoreV1().Pods(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.PodDetailDTO{}, mapResourceError(err)
		}
		relatedEvents := backend.relatedPodEvents(requestContext, binding, clients.kubernetes, value)
		return resources.PodDetail(value, relatedEvents, backend.now().UTC()), nil
	})
}
func (backend *ResourceBackend) PodYAML(ctx context.Context, binding namespaces.SelectionBinding, namespace, name string) ([]byte, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "pods"}
	if err := backend.authorizeGet(ctx, binding, origin, name); err != nil {
		return nil, err
	}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return nil, err
	}
	defer cancel()
	value, err := clients.kubernetes.CoreV1().Pods(namespace).Get(requestContext, name, metav1.GetOptions{})
	if err != nil {
		return nil, mapResourceError(err)
	}
	return resources.MarshalReadOnlyYAML(value)
}

func (backend *ResourceBackend) GetService(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.ServiceDetailDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "services"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.ServiceDetailDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.ServiceDetailDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoreV1().Services(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.ServiceDetailDTO{}, mapResourceError(err)
		}
		return resources.ServiceDetail(value), nil
	})
}
func (backend *ResourceBackend) GetIngress(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.IngressDetailDTO, error) {
	origin := resources.Origin{Namespace: namespace, APIGroup: "networking.k8s.io", Version: "v1", Resource: "ingresses"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.IngressDetailDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.IngressDetailDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.NetworkingV1().Ingresses(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.IngressDetailDTO{}, mapResourceError(err)
		}
		return resources.IngressDetail(value)
	})
}
func (backend *ResourceBackend) GetEndpointSlice(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.EndpointSliceDetailDTO, error) {
	origin := resources.Origin{Namespace: namespace, APIGroup: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.EndpointSliceDetailDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.EndpointSliceDetailDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.DiscoveryV1().EndpointSlices(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.EndpointSliceDetailDTO{}, mapResourceError(err)
		}
		return resources.EndpointSliceDetail(value), nil
	})
}
func (backend *ResourceBackend) GetConfigMap(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.ConfigMapDetailDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "configmaps"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.ConfigMapDetailDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.ConfigMapDetailDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoreV1().ConfigMaps(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.ConfigMapDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertConfigMapDetail(value), nil
	})
}
func (backend *ResourceBackend) GetSecret(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.SecretMetadataDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "secrets"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.SecretMetadataDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.SecretMetadataDTO{}, err
		}
		defer cancel()
		value, err := clients.metadata.Resource(schema.GroupVersionResource{Version: "v1", Resource: "secrets"}).Namespace(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.SecretMetadataDTO{}, mapMetadataError(err, "Secret metadata is unavailable.")
		}
		return resources.ConvertSecretMetadata(*value), nil
	})
}

func (backend *ResourceBackend) ResourceYAML(ctx context.Context, binding namespaces.SelectionBinding, collection, namespace, name string) ([]byte, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: collection}
	switch collection {
	case "ingresses":
		origin.APIGroup = "networking.k8s.io"
	case "endpointslices":
		origin.APIGroup = "discovery.k8s.io"
	}
	if err := backend.authorizeGet(ctx, binding, origin, name); err != nil {
		return nil, err
	}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var value any
	switch collection {
	case "services":
		value, err = clients.kubernetes.CoreV1().Services(namespace).Get(requestContext, name, metav1.GetOptions{})
	case "ingresses":
		value, err = clients.kubernetes.NetworkingV1().Ingresses(namespace).Get(requestContext, name, metav1.GetOptions{})
	case "endpointslices":
		value, err = clients.kubernetes.DiscoveryV1().EndpointSlices(namespace).Get(requestContext, name, metav1.GetOptions{})
	case "configmaps":
		value, err = clients.kubernetes.CoreV1().ConfigMaps(namespace).Get(requestContext, name, metav1.GetOptions{})
	default:
		return nil, resourceDomain(resources.CodeValidationFailed, "The YAML resource type is invalid.", nil)
	}
	if err != nil {
		return nil, mapResourceError(err)
	}
	return resources.MarshalReadOnlyYAML(value)
}

func workloadOrigin(kind, namespace string) (resources.Origin, error) {
	switch kind {
	case "deployments", "statefulsets", "daemonsets":
		return resources.Origin{Namespace: namespace, APIGroup: "apps", Version: "v1", Resource: kind}, nil
	case "jobs", "cronjobs":
		return resources.Origin{Namespace: namespace, APIGroup: "batch", Version: "v1", Resource: kind}, nil
	default:
		return resources.Origin{}, resourceDomain(resources.CodeValidationFailed, "The workload kind is invalid.", nil)
	}
}

type resourceLogPort struct {
	backend *ResourceBackend
	binding namespaces.SelectionBinding
}

func (port resourceLogPort) Open(ctx context.Context, target resources.LogTarget, options resources.LogSourceOptions) (io.ReadCloser, error) {
	lease, err := port.backend.runtime.leaseFor(ctx, port.binding)
	if err != nil {
		return nil, mapResourceError(err)
	}
	if lease.Clients.StreamingKubernetes() == nil {
		return nil, resourceDomain(resources.CodeFeatureUnavailable, "Pod logs are unavailable.", nil)
	}
	streamContext, err := lease.Generation.Stream(ctx, resources.MaximumLogFollowDuration)
	if err != nil {
		return nil, mapResourceError(err)
	}
	tail := options.TailLines
	logOptions := &corev1.PodLogOptions{Container: target.Container, Previous: options.Previous, Timestamps: options.Timestamps, TailLines: &tail, SinceSeconds: options.SinceSeconds, LimitBytes: &options.LimitBytes, Follow: options.Follow}
	reader, err := lease.Clients.StreamingKubernetes().CoreV1().Pods(target.Namespace).GetLogs(target.Pod, logOptions).Stream(streamContext.Context())
	if err != nil {
		streamContext.Close()
		return nil, mapResourceError(err)
	}
	return &resourceStreamReader{ReadCloser: reader, close: streamContext.Close}, nil
}

func (port resourceLogPort) ContainerTerminated(ctx context.Context, target resources.LogTarget) bool {
	if port.backend == nil || port.backend.authorizer == nil {
		return false
	}
	capability := port.backend.authorizer.Check(ctx, authorization.Key{
		Generation: port.binding.Generation, Namespace: target.Namespace,
		Resource: "pods", Verb: "get", ResourceName: target.Pod,
	})
	if capability.Decision != authorization.DecisionAllowed {
		return false
	}
	requestContext, cancel, clients, err := port.backend.unary(ctx, port.binding)
	if err != nil {
		return false
	}
	defer cancel()
	pod, err := clients.kubernetes.CoreV1().Pods(target.Namespace).Get(requestContext, target.Pod, metav1.GetOptions{})
	if err != nil {
		return false
	}
	return terminatedContainerStatus(pod.Status.ContainerStatuses, target.Container) ||
		terminatedContainerStatus(pod.Status.InitContainerStatuses, target.Container) ||
		terminatedContainerStatus(pod.Status.EphemeralContainerStatuses, target.Container)
}

func terminatedContainerStatus(statuses []corev1.ContainerStatus, name string) bool {
	for _, status := range statuses {
		if status.Name == name {
			return status.State.Terminated != nil
		}
	}
	return false
}

type resourceStreamReader struct {
	io.ReadCloser
	close func()
}

func (reader *resourceStreamReader) Close() error {
	err := reader.ReadCloser.Close()
	reader.close()
	return err
}

func (backend *ResourceBackend) ReadLogs(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, pod string, query resources.LogQuery) (resources.LogReadDTO, error) {
	service := resources.LogService{Port: resourceLogPort{backend: backend, binding: binding}, Authorizer: backend.authorizer, Redactor: backend.redactor, Now: backend.now}
	return service.Read(ctx, resourceSelection(binding, resolution), resources.LogTarget{Namespace: namespace, Pod: pod}, query)
}

func (backend *ResourceBackend) AuthorizeLogs(ctx context.Context, binding namespaces.SelectionBinding, namespace, pod string) error {
	return backend.authorizeLogs(ctx, binding, namespace, pod, false)
}

func (backend *ResourceBackend) ReauthorizeLogs(ctx context.Context, binding namespaces.SelectionBinding, namespace, pod string) error {
	return backend.authorizeLogs(ctx, binding, namespace, pod, true)
}

func (backend *ResourceBackend) authorizeLogs(ctx context.Context, binding namespaces.SelectionBinding, namespace, pod string, refresh bool) error {
	key := authorization.Key{Generation: binding.Generation, Namespace: namespace, Resource: "pods", Subresource: "log", Verb: "get", ResourceName: pod}
	capability := backend.authorizer.Check(ctx, key)
	if refresh {
		if refresher, ok := backend.authorizer.(resources.AuthorizationRefresher); ok {
			capability = refresher.Refresh(ctx, key)
		}
	}
	switch capability.Decision {
	case authorization.DecisionAllowed:
		return nil
	case authorization.DecisionDenied:
		return resourceDomain(resources.CodeForbidden, "Access to pod logs was denied.", nil)
	default:
		return resourceDomain(resources.CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
	}
}
func (backend *ResourceBackend) FollowLogs(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, pod string, query resources.LogQuery, emit func(resources.LogLineDTO) error) (resources.FollowTerminal, error) {
	service := resources.LogService{Port: resourceLogPort{backend: backend, binding: binding}, Authorizer: backend.authorizer, Redactor: backend.redactor, Now: backend.now}
	return service.Follow(ctx, resourceSelection(binding, resolution), resources.LogTarget{Namespace: namespace, Pod: pod}, query, emit)
}

func resourceDomain(code resources.ErrorCode, message string, cause error) error {
	return &resources.DomainError{Code: code, Message: message, Cause: cause}
}
func mapMetadataError(err error, message string) error {
	if apierrors.IsNotAcceptable(err) || apierrors.IsUnsupportedMediaType(err) {
		return resourceDomain(resources.CodeFeatureUnavailable, message, nil)
	}
	return mapResourceError(err)
}
func mapOptionalResourceError(err error) error {
	if apierrors.IsNotFound(err) {
		return resourceDomain(resources.CodeFeatureUnavailable, "This optional Kubernetes API is unavailable.", nil)
	}
	return mapResourceError(err)
}
func mapResourceError(err error) error {
	if err == nil {
		return nil
	}
	if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
		return resources.ErrResourceExpired
	}
	if apierrors.IsNotFound(err) {
		return resourceDomain(resources.CodeNotFound, "The Kubernetes resource was not found.", err)
	}
	if apierrors.IsForbidden(err) {
		return resourceDomain(resources.CodeForbidden, "Access to this resource was denied.", err)
	}
	if apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) || errors.Is(err, context.DeadlineExceeded) {
		return resourceDomain(resources.CodeUpstreamTimeout, "The Kubernetes request timed out.", err)
	}
	if errors.Is(err, context.Canceled) {
		return resourceDomain(resources.CodeGenerationChanged, "The active selection changed.", err)
	}
	var safe *kubeadapter.SafeError
	if errors.As(err, &safe) {
		switch safe.Code {
		case kubeadapter.CodeAuthenticationUnavailable:
			return resourceDomain(resources.CodeAuthenticationUnavailable, "Kubernetes authentication is unavailable.", err)
		case kubeadapter.CodeRequestTimeout:
			return resourceDomain(resources.CodeUpstreamTimeout, "The Kubernetes request timed out.", err)
		case kubeadapter.CodeGenerationChanged, kubeadapter.CodeRequestCanceled:
			return resourceDomain(resources.CodeGenerationChanged, "The active selection changed.", err)
		}
	}
	return resourceDomain(resources.CodeClusterUnavailable, "The Kubernetes API could not complete the request.", err)
}

var _ resources.LogPort = resourceLogPort{}
