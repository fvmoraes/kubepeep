package kubernetesruntime

import (
	"context"
	"errors"
	"io"
	"time"

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
	case "replicasets":
		list, listErr := clients.kubernetes.AppsV1().ReplicaSets(page.Origin.Namespace).List(requestContext, options)
		if listErr != nil {
			return result, mapResourceError(listErr)
		}
		for index := range list.Items {
			result.Items = append(result.Items, resources.ConvertReplicaSet(&list.Items[index], now))
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
func (backend *ResourceBackend) listNodePage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.NodeDTO], error) {
	result := resources.OriginPage[resources.NodeDTO]{Origin: page.Origin, Items: []resources.NodeDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoreV1().Nodes().List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertNode(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listLeasePage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.LeaseDTO], error) {
	result := resources.OriginPage[resources.LeaseDTO]{Origin: page.Origin, Items: []resources.LeaseDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoordinationV1().Leases(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertLease(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listServiceAccountPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.ServiceAccountDTO], error) {
	result := resources.OriginPage[resources.ServiceAccountDTO]{Origin: page.Origin, Items: []resources.ServiceAccountDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}
	list, err := clients.metadata.Resource(gvr).Namespace(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapMetadataError(err, "ServiceAccount metadata is unavailable.")
	}
	now := backend.now().UTC()
	for index := range list.Items {
		item := &list.Items[index]
		result.Items = append(result.Items, resources.ServiceAccountDTO{Namespace: item.Namespace, Name: item.Name, UID: string(item.UID), AgeSeconds: int64(now.Sub(item.CreationTimestamp.Time) / time.Second)})
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listResourceQuotaPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.ResourceQuotaDTO], error) {
	result := resources.OriginPage[resources.ResourceQuotaDTO]{Origin: page.Origin, Items: []resources.ResourceQuotaDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoreV1().ResourceQuotas(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertResourceQuota(&list.Items[index]))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listLimitRangePage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.LimitRangeDTO], error) {
	result := resources.OriginPage[resources.LimitRangeDTO]{Origin: page.Origin, Items: []resources.LimitRangeDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoreV1().LimitRanges(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertLimitRange(&list.Items[index]))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listHPAPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.HorizontalPodAutoscalerDTO], error) {
	result := resources.OriginPage[resources.HorizontalPodAutoscalerDTO]{Origin: page.Origin, Items: []resources.HorizontalPodAutoscalerDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.AutoscalingV2().HorizontalPodAutoscalers(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapHPAError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertHorizontalPodAutoscaler(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listPDBPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.PodDisruptionBudgetDTO], error) {
	result := resources.OriginPage[resources.PodDisruptionBudgetDTO]{Origin: page.Origin, Items: []resources.PodDisruptionBudgetDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.PolicyV1().PodDisruptionBudgets(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertPodDisruptionBudget(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

// mapHPAError marks a missing autoscaling/v2 API as feature unavailability
// instead of a generic failure (V4-08 shares this distinction).
func mapHPAError(err error) error {
	if apierrors.IsNotFound(err) {
		return resourceDomain(resources.CodeFeatureUnavailable, "The autoscaling/v2 API is unavailable on this cluster.", err)
	}
	return mapResourceError(err)
}

func (backend *ResourceBackend) listPersistentVolumeClaimPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.PersistentVolumeClaimDTO], error) {
	result := resources.OriginPage[resources.PersistentVolumeClaimDTO]{Origin: page.Origin, Items: []resources.PersistentVolumeClaimDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoreV1().PersistentVolumeClaims(page.Origin.Namespace).List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertPersistentVolumeClaim(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listPersistentVolumePage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.PersistentVolumeDTO], error) {
	result := resources.OriginPage[resources.PersistentVolumeDTO]{Origin: page.Origin, Items: []resources.PersistentVolumeDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.CoreV1().PersistentVolumes().List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertPersistentVolume(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listStorageClassPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.StorageClassDTO], error) {
	result := resources.OriginPage[resources.StorageClassDTO]{Origin: page.Origin, Items: []resources.StorageClassDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.StorageV1().StorageClasses().List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertStorageClass(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listCSIDriverPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.CSIDriverDTO], error) {
	result := resources.OriginPage[resources.CSIDriverDTO]{Origin: page.Origin, Items: []resources.CSIDriverDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.StorageV1().CSIDrivers().List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertCSIDriver(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listCSINodePage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.CSINodeDTO], error) {
	result := resources.OriginPage[resources.CSINodeDTO]{Origin: page.Origin, Items: []resources.CSINodeDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.StorageV1().CSINodes().List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertCSINode(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) listVolumeAttachmentPage(ctx context.Context, binding namespaces.SelectionBinding, page resources.PageRequest) (resources.OriginPage[resources.VolumeAttachmentDTO], error) {
	result := resources.OriginPage[resources.VolumeAttachmentDTO]{Origin: page.Origin, Items: []resources.VolumeAttachmentDTO{}}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return result, err
	}
	defer cancel()
	list, err := clients.kubernetes.StorageV1().VolumeAttachments().List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if err != nil {
		return result, mapResourceError(err)
	}
	now := backend.now().UTC()
	for index := range list.Items {
		result.Items = append(result.Items, resources.ConvertVolumeAttachment(&list.Items[index], now))
	}
	result.Continue, result.ResourceVersion = list.Continue, list.ResourceVersion
	return result, nil
}

func (backend *ResourceBackend) GetNode(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, name string) (resources.NodeDetailDTO, error) {
	if name == "" {
		return resources.NodeDetailDTO{}, resourceDomain(resources.CodeValidationFailed, "The resource target is incomplete.", nil)
	}
	capability := backend.authorizer.Check(ctx, authorization.Key{Generation: binding.Generation, APIGroup: "", Resource: "nodes", Verb: "get", ResourceName: name})
	switch capability.Decision {
	case authorization.DecisionDenied:
		return resources.NodeDetailDTO{}, resourceDomain(resources.CodeForbidden, "Access to this resource was denied.", nil)
	case authorization.DecisionUnknown:
		return resources.NodeDetailDTO{}, resourceDomain(resources.CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
	}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return resources.NodeDetailDTO{}, err
	}
	defer cancel()
	value, err := clients.kubernetes.CoreV1().Nodes().Get(requestContext, name, metav1.GetOptions{})
	if err != nil {
		return resources.NodeDetailDTO{}, mapResourceError(err)
	}
	return resources.ConvertNodeDetail(value, backend.now().UTC()), nil
}

// clusterGet is the shared cluster-scoped detail path: exact-name
// authorization, then one typed GET. Origin.Namespace is always empty here.
func clusterGet[T resources.DetailItem](ctx context.Context, backend *ResourceBackend, binding namespaces.SelectionBinding, origin resources.Origin, name string, get func(context.Context, resourceClientSet) (T, error)) (T, error) {
	var zero T
	if name == "" {
		return zero, resourceDomain(resources.CodeValidationFailed, "The resource target is incomplete.", nil)
	}
	capability := backend.authorizer.Check(ctx, authorization.Key{Generation: binding.Generation, Namespace: origin.Namespace, APIGroup: origin.APIGroup, Resource: origin.Resource, Verb: "get", ResourceName: name})
	switch capability.Decision {
	case authorization.DecisionDenied:
		return zero, resourceDomain(resources.CodeForbidden, "Access to this resource was denied.", nil)
	case authorization.DecisionUnknown:
		return zero, resourceDomain(resources.CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
	}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return zero, err
	}
	defer cancel()
	return get(requestContext, clients)
}

func (backend *ResourceBackend) GetLease(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.LeaseDetailDTO, error) {
	origin := resources.Origin{Namespace: namespace, APIGroup: "coordination.k8s.io", Version: "v1", Resource: "leases"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.LeaseDetailDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.LeaseDetailDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoordinationV1().Leases(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.LeaseDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertLeaseDetail(value), nil
	})
}

func (backend *ResourceBackend) GetPersistentVolumeClaim(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.PersistentVolumeClaimDetailDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "persistentvolumeclaims"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.PersistentVolumeClaimDetailDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.PersistentVolumeClaimDetailDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoreV1().PersistentVolumeClaims(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.PersistentVolumeClaimDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertPersistentVolumeClaimDetail(value), nil
	})
}

func (backend *ResourceBackend) GetPersistentVolume(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, name string) (resources.PersistentVolumeDetailDTO, error) {
	return clusterGet(ctx, backend, binding, resources.Origin{Version: "v1", Resource: "persistentvolumes"}, name, func(ctx context.Context, clients resourceClientSet) (resources.PersistentVolumeDetailDTO, error) {
		value, err := clients.kubernetes.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return resources.PersistentVolumeDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertPersistentVolumeDetail(value), nil
	})
}

func (backend *ResourceBackend) GetStorageClass(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, name string) (resources.StorageClassDetailDTO, error) {
	return clusterGet(ctx, backend, binding, resources.Origin{APIGroup: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}, name, func(ctx context.Context, clients resourceClientSet) (resources.StorageClassDetailDTO, error) {
		value, err := clients.kubernetes.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return resources.StorageClassDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertStorageClassDetail(value), nil
	})
}

func (backend *ResourceBackend) GetCSIDriver(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, name string) (resources.CSIDriverDetailDTO, error) {
	return clusterGet(ctx, backend, binding, resources.Origin{APIGroup: "storage.k8s.io", Version: "v1", Resource: "csidrivers"}, name, func(ctx context.Context, clients resourceClientSet) (resources.CSIDriverDetailDTO, error) {
		value, err := clients.kubernetes.StorageV1().CSIDrivers().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return resources.CSIDriverDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertCSIDriverDetail(value), nil
	})
}

func (backend *ResourceBackend) GetCSINode(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, name string) (resources.CSINodeDetailDTO, error) {
	return clusterGet(ctx, backend, binding, resources.Origin{APIGroup: "storage.k8s.io", Version: "v1", Resource: "csinodes"}, name, func(ctx context.Context, clients resourceClientSet) (resources.CSINodeDetailDTO, error) {
		value, err := clients.kubernetes.StorageV1().CSINodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return resources.CSINodeDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertCSINodeDetail(value), nil
	})
}

func (backend *ResourceBackend) GetVolumeAttachment(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, name string) (resources.VolumeAttachmentDetailDTO, error) {
	return clusterGet(ctx, backend, binding, resources.Origin{APIGroup: "storage.k8s.io", Version: "v1", Resource: "volumeattachments"}, name, func(ctx context.Context, clients resourceClientSet) (resources.VolumeAttachmentDetailDTO, error) {
		value, err := clients.kubernetes.StorageV1().VolumeAttachments().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return resources.VolumeAttachmentDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertVolumeAttachmentDetail(value), nil
	})
}

// GetNamespace serves the V2-01 Namespace object inspection. It is fully
// separate from the local scope management: selecting scopes never calls it.
func (backend *ResourceBackend) GetNamespace(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, name string) (resources.NamespaceObjectDetailDTO, error) {
	return clusterGet(ctx, backend, binding, resources.Origin{Version: "v1", Resource: "namespaces"}, name, func(ctx context.Context, clients resourceClientSet) (resources.NamespaceObjectDetailDTO, error) {
		value, err := clients.kubernetes.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return resources.NamespaceObjectDetailDTO{}, mapResourceError(err)
		}
		return resources.ConvertNamespaceObjectDetail(value), nil
	})
}

func (backend *ResourceBackend) GetServiceAccount(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.ServiceAccountDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "serviceaccounts"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.ServiceAccountDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.ServiceAccountDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoreV1().ServiceAccounts(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.ServiceAccountDTO{}, mapResourceError(err)
		}
		return resources.ConvertServiceAccount(value, backend.now().UTC()), nil
	})
}

func (backend *ResourceBackend) GetResourceQuota(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.ResourceQuotaDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "resourcequotas"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.ResourceQuotaDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.ResourceQuotaDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoreV1().ResourceQuotas(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.ResourceQuotaDTO{}, mapResourceError(err)
		}
		return resources.ConvertResourceQuota(value), nil
	})
}

func (backend *ResourceBackend) GetLimitRange(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.LimitRangeDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "limitranges"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.LimitRangeDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.LimitRangeDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoreV1().LimitRanges(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.LimitRangeDTO{}, mapResourceError(err)
		}
		return resources.ConvertLimitRange(value), nil
	})
}

func (backend *ResourceBackend) GetHPA(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.HorizontalPodAutoscalerDTO, error) {
	origin := resources.Origin{Namespace: namespace, APIGroup: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.HorizontalPodAutoscalerDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.HorizontalPodAutoscalerDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.HorizontalPodAutoscalerDTO{}, mapHPAError(err)
		}
		return resources.ConvertHorizontalPodAutoscaler(value, backend.now().UTC()), nil
	})
}

func (backend *ResourceBackend) GetPDB(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.PodDisruptionBudgetDTO, error) {
	origin := resources.Origin{Namespace: namespace, APIGroup: "policy", Version: "v1", Resource: "poddisruptionbudgets"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.PodDisruptionBudgetDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.PodDisruptionBudgetDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.PolicyV1().PodDisruptionBudgets(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.PodDisruptionBudgetDTO{}, mapResourceError(err)
		}
		return resources.ConvertPodDisruptionBudget(value, backend.now().UTC()), nil
	})
}

// PersistentVolumeYAMLDocument serves a curated PV document: identity,
// capacity, access modes, reclaim policy, class, claim reference and phase.
// Source-specific blocks (csi/nfs/hostPath/...), secret references and
// volume attributes are labeled as omitted (V2-08).
func (backend *ResourceBackend) PersistentVolumeYAMLDocument(ctx context.Context, binding namespaces.SelectionBinding, name string) ([]byte, error) {
	return clusterYAMLDocument(ctx, backend, binding, resources.Origin{Version: "v1", Resource: "persistentvolumes"}, name, func(ctx context.Context, clients resourceClientSet) (any, error) {
		value, err := clients.kubernetes.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, mapResourceError(err)
		}
		detail := resources.ConvertPersistentVolumeDetail(value)
		return persistentVolumeSafeYAML{
			APIVersion: "v1",
			Kind:       "PersistentVolume",
			Metadata: nodeSafeMetadata{
				Name:              detail.Metadata.Name,
				UID:               detail.Metadata.UID,
				CreationTimestamp: detail.Metadata.CreationTimestamp,
			},
			Spec: persistentVolumeSafeSpec{
				Capacity:      detail.Capacity,
				AccessModes:   detail.AccessModes,
				ReclaimPolicy: detail.ReclaimPolicy,
				StorageClass:  detail.StorageClass,
				VolumeMode:    detail.VolumeMode,
				Claim:         detail.Claim,
			},
			Status:  persistentVolumeSafeStatus{Phase: detail.Status, Reason: detail.Reason, Message: detail.Message},
			Omitted: detail.Omitted,
		}, nil
	})
}

type persistentVolumeSafeYAML struct {
	APIVersion string                     `json:"apiVersion"`
	Kind       string                     `json:"kind"`
	Metadata   nodeSafeMetadata           `json:"metadata"`
	Spec       persistentVolumeSafeSpec   `json:"spec"`
	Status     persistentVolumeSafeStatus `json:"status"`
	Omitted    []string                   `json:"x-kubepeep-omitted"`
}

type persistentVolumeSafeSpec struct {
	Capacity      map[string]string            `json:"capacity,omitempty"`
	AccessModes   []string                     `json:"accessModes,omitempty"`
	ReclaimPolicy string                       `json:"persistentVolumeReclaimPolicy,omitempty"`
	StorageClass  string                       `json:"storageClassName,omitempty"`
	VolumeMode    string                       `json:"volumeMode,omitempty"`
	Claim         *resources.VolumeClaimRefDTO `json:"claimRef,omitempty"`
}

type persistentVolumeSafeStatus struct {
	Phase   string  `json:"phase,omitempty"`
	Reason  *string `json:"reason,omitempty"`
	Message *string `json:"message,omitempty"`
}

// StorageClassYAMLDocument serves the curated StorageClass document with
// parameters and topologies omitted (they may carry provider credentials).
func (backend *ResourceBackend) StorageClassYAMLDocument(ctx context.Context, binding namespaces.SelectionBinding, name string) ([]byte, error) {
	return clusterYAMLDocument(ctx, backend, binding, resources.Origin{APIGroup: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}, name, func(ctx context.Context, clients resourceClientSet) (any, error) {
		value, err := clients.kubernetes.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, mapResourceError(err)
		}
		detail := resources.ConvertStorageClassDetail(value)
		return map[string]any{
			"apiVersion": "storage.k8s.io/v1",
			"kind":       "StorageClass",
			"metadata": map[string]any{
				"name":              detail.Metadata.Name,
				"uid":               detail.Metadata.UID,
				"creationTimestamp": detail.Metadata.CreationTimestamp,
			},
			"provisioner":          detail.Provisioner,
			"reclaimPolicy":        detail.ReclaimPolicy,
			"volumeBindingMode":    detail.VolumeBindingMode,
			"allowVolumeExpansion": detail.AllowVolumeExpansion,
			"default":              detail.Default,
			"x-kubepeep-omitted":   detail.Omitted,
		}, nil
	})
}

// clusterYAMLDocument authorizes a cluster-scoped YAML action, fetches the
// object, delegates assembly to build and encodes through the shared ceiling.
func clusterYAMLDocument(ctx context.Context, backend *ResourceBackend, binding namespaces.SelectionBinding, origin resources.Origin, name string, build func(context.Context, resourceClientSet) (any, error)) ([]byte, error) {
	capability := backend.authorizer.Check(ctx, authorization.Key{Generation: binding.Generation, Namespace: origin.Namespace, APIGroup: origin.APIGroup, Resource: origin.Resource, Verb: "get", ResourceName: name})
	switch capability.Decision {
	case authorization.DecisionDenied:
		return nil, resourceDomain(resources.CodeForbidden, "Access to this resource was denied.", nil)
	case authorization.DecisionUnknown:
		return nil, resourceDomain(resources.CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
	}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return nil, err
	}
	defer cancel()
	document, err := build(requestContext, clients)
	if err != nil {
		return nil, err
	}
	return resources.MarshalYAMLDocument(document)
}

type nodeSafeYAML struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   nodeSafeMetadata `json:"metadata"`
	Status     nodeSafeStatus   `json:"status"`
	Omitted    []string         `json:"x-kubepeep-omitted"`
}

type nodeSafeMetadata struct {
	Name              string   `json:"name"`
	UID               string   `json:"uid"`
	CreationTimestamp string   `json:"creationTimestamp"`
	Roles             []string `json:"roles,omitempty"`
}

type nodeSafeStatus struct {
	Ready       bool                     `json:"ready"`
	Conditions  []resources.ConditionDTO `json:"conditions,omitempty"`
	InternalIP  *string                  `json:"internalIP,omitempty"`
	Capacity    map[string]string        `json:"capacity,omitempty"`
	Allocatable map[string]string        `json:"allocatable,omitempty"`
	Taints      []resources.NodeTaintDTO `json:"taints,omitempty"`
	NodeInfo    nodeSafeNodeInfo         `json:"nodeInfo,omitempty"`
}

type nodeSafeNodeInfo struct {
	KubeletVersion  string `json:"kubeletVersion,omitempty"`
	OSImage         string `json:"osImage,omitempty"`
	Architecture    string `json:"architecture,omitempty"`
	OperatingSystem string `json:"operatingSystem,omitempty"`
}

// NodeYAMLDocument serves the curated Node document after an exact get
// authorization. The raw Kubernetes object is never serialized.
func (backend *ResourceBackend) NodeYAMLDocument(ctx context.Context, binding namespaces.SelectionBinding, name string) ([]byte, error) {
	capability := backend.authorizer.Check(ctx, authorization.Key{Generation: binding.Generation, APIGroup: "", Resource: "nodes", Verb: "get", ResourceName: name})
	switch capability.Decision {
	case authorization.DecisionDenied:
		return nil, resourceDomain(resources.CodeForbidden, "Access to this resource was denied.", nil)
	case authorization.DecisionUnknown:
		return nil, resourceDomain(resources.CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
	}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return nil, err
	}
	defer cancel()
	value, err := clients.kubernetes.CoreV1().Nodes().Get(requestContext, name, metav1.GetOptions{})
	if err != nil {
		return nil, mapResourceError(err)
	}
	detail := resources.ConvertNodeDetail(value, backend.now().UTC())
	document := nodeSafeYAML{
		APIVersion: "v1",
		Kind:       "Node",
		Metadata: nodeSafeMetadata{
			Name:              detail.Metadata.Name,
			UID:               detail.Metadata.UID,
			CreationTimestamp: detail.Metadata.CreationTimestamp,
			Roles:             detail.Roles,
		},
		Status: nodeSafeStatus{
			Ready:       detail.Ready,
			Conditions:  detail.Conditions,
			InternalIP:  detail.InternalIP,
			Capacity:    detail.Capacity,
			Allocatable: detail.Allocatable,
			Taints:      detail.Taints,
			NodeInfo: nodeSafeNodeInfo{
				KubeletVersion:  value.Status.NodeInfo.KubeletVersion,
				OSImage:         value.Status.NodeInfo.OSImage,
				Architecture:    value.Status.NodeInfo.Architecture,
				OperatingSystem: value.Status.NodeInfo.OperatingSystem,
			},
		},
		Omitted: []string{"metadata.annotations", "metadata.labels (non-role)", "metadata.managedFields", "metadata.finalizers", "spec", "status.config", "status.images", "status.volumesInUse", "status.volumesAttached"},
	}
	return resources.MarshalYAMLDocument(document)
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
		case *appsv1.ReplicaSet:
			return resources.ReplicaSetDetail(object, related, now), nil
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
	case "replicasets":
		value, getErr := clients.kubernetes.AppsV1().ReplicaSets(namespace).Get(requestContext, name, metav1.GetOptions{})
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
	value, err := backend.fetchYAMLObject(ctx, binding, collection, namespace, name)
	if err != nil {
		return nil, err
	}
	return resources.MarshalReadOnlyYAML(value)
}

// fetchYAMLObject loads the live typed object behind every YAML-capable
// collection after an exact get authorization.
func (backend *ResourceBackend) fetchYAMLObject(ctx context.Context, binding namespaces.SelectionBinding, collection, namespace, name string) (any, error) {
	switch collection {
	case "pods":
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
		return value, mapResourceError(err)
	case "deployments", "statefulsets", "daemonsets", "jobs", "cronjobs", "replicasets":
		origin, err := workloadOrigin(collection, namespace)
		if err != nil {
			return nil, err
		}
		if err := backend.authorizeGet(ctx, binding, origin, name); err != nil {
			return nil, err
		}
		return backend.getWorkloadObject(ctx, binding, collection, namespace, name)
	case "services", "ingresses", "endpointslices", "configmaps", "leases", "persistent-volume-claims":
		var origin resources.Origin
		switch collection {
		case "ingresses":
			origin = resources.Origin{Namespace: namespace, APIGroup: "networking.k8s.io", Version: "v1", Resource: collection}
		case "endpointslices":
			origin = resources.Origin{Namespace: namespace, APIGroup: "discovery.k8s.io", Version: "v1", Resource: collection}
		case "leases":
			origin = resources.Origin{Namespace: namespace, APIGroup: "coordination.k8s.io", Version: "v1", Resource: "leases"}
		case "persistent-volume-claims":
			origin = resources.Origin{Namespace: namespace, Version: "v1", Resource: "persistentvolumeclaims"}
		default:
			origin = resources.Origin{Namespace: namespace, Version: "v1", Resource: collection}
		}
		if err := backend.authorizeGet(ctx, binding, origin, name); err != nil {
			return nil, err
		}
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return nil, err
		}
		defer cancel()
		switch collection {
		case "services":
			value, getErr := clients.kubernetes.CoreV1().Services(namespace).Get(requestContext, name, metav1.GetOptions{})
			return value, mapResourceError(getErr)
		case "ingresses":
			value, getErr := clients.kubernetes.NetworkingV1().Ingresses(namespace).Get(requestContext, name, metav1.GetOptions{})
			return value, mapResourceError(getErr)
		case "endpointslices":
			value, getErr := clients.kubernetes.DiscoveryV1().EndpointSlices(namespace).Get(requestContext, name, metav1.GetOptions{})
			return value, mapResourceError(getErr)
		case "leases":
			value, getErr := clients.kubernetes.CoordinationV1().Leases(namespace).Get(requestContext, name, metav1.GetOptions{})
			return value, mapResourceError(getErr)
		case "persistent-volume-claims":
			value, getErr := clients.kubernetes.CoreV1().PersistentVolumeClaims(namespace).Get(requestContext, name, metav1.GetOptions{})
			return value, mapResourceError(getErr)
		default:
			value, getErr := clients.kubernetes.CoreV1().ConfigMaps(namespace).Get(requestContext, name, metav1.GetOptions{})
			return value, mapResourceError(getErr)
		}
	default:
		return nil, resourceDomain(resources.CodeValidationFailed, "The YAML resource type is invalid.", nil)
	}
}

// ResourceLastAppliedDiff (F7-02) compares the live document against the
// kubectl last-applied baseline. Only identity-checked metadata-bearing kinds
// are eligible; Secrets never enter this path.
func (backend *ResourceBackend) ResourceLastAppliedDiff(ctx context.Context, binding namespaces.SelectionBinding, collection, namespace, name string) (resources.LastAppliedDiffDTO, error) {
	if collection == "secrets" {
		return resources.LastAppliedDiffDTO{}, resourceDomain(resources.CodeForbidden, "Secret YAML is not available.", resources.ErrSecretYAML)
	}
	value, err := backend.fetchYAMLObject(ctx, binding, collection, namespace, name)
	if err != nil {
		return resources.LastAppliedDiffDTO{}, err
	}
	previous, found, err := resources.ExtractLastApplied(value)
	if err != nil {
		return resources.LastAppliedDiffDTO{}, err
	}
	if !found {
		return resources.LastAppliedDiffDTO{Absent: true, Lines: []resources.DiffLineDTO{}}, nil
	}
	current := resources.StripLastAppliedAnnotation(value)
	currentYAML, err := resources.MarshalReadOnlyYAML(current)
	if err != nil {
		return resources.LastAppliedDiffDTO{}, err
	}
	return resources.DiffYAML(currentYAML, previous), nil
}

func workloadOrigin(kind, namespace string) (resources.Origin, error) {
	switch kind {
	case "deployments", "statefulsets", "daemonsets", "replicasets":
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
