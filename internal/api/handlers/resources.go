package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
	"k8s.io/apimachinery/pkg/util/validation"
)

const maximumPreferencesBodyBytes = 1 << 20

// ResourceService is the complete read-side Phase 6 contract consumed by the
// HTTP layer. Kubernetes types and credentials cannot cross this boundary.
type ResourceService interface {
	ListWorkloads(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.WorkloadDTO]) (resourcecore.ListResult[resourcecore.WorkloadDTO], error)
	ListPods(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.PodDTO]) (resourcecore.ListResult[resourcecore.PodDTO], error)
	ListEvents(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.EventDTO]) (resourcecore.ListResult[resourcecore.EventDTO], error)
	ListServices(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.ServiceDTO]) (resourcecore.ListResult[resourcecore.ServiceDTO], error)
	ListIngresses(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.IngressDTO]) (resourcecore.ListResult[resourcecore.IngressDTO], error)
	ListEndpointSlices(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.EndpointSliceDTO]) (resourcecore.ListResult[resourcecore.EndpointSliceDTO], error)
	ListConfigMaps(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.ConfigMapListDTO]) (resourcecore.ListResult[resourcecore.ConfigMapListDTO], error)
	ListSecrets(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.SecretMetadataDTO]) (resourcecore.ListResult[resourcecore.SecretMetadataDTO], error)
	ListNodes(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.NodeDTO]) (resourcecore.ListResult[resourcecore.NodeDTO], error)
	ListLeases(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.LeaseDTO]) (resourcecore.ListResult[resourcecore.LeaseDTO], error)
	ListPersistentVolumeClaims(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.PersistentVolumeClaimDTO]) (resourcecore.ListResult[resourcecore.PersistentVolumeClaimDTO], error)
	ListPersistentVolumes(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.PersistentVolumeDTO]) (resourcecore.ListResult[resourcecore.PersistentVolumeDTO], error)
	ListStorageClasses(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.StorageClassDTO]) (resourcecore.ListResult[resourcecore.StorageClassDTO], error)
	ListCSIDrivers(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.CSIDriverDTO]) (resourcecore.ListResult[resourcecore.CSIDriverDTO], error)
	ListCSINodes(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.CSINodeDTO]) (resourcecore.ListResult[resourcecore.CSINodeDTO], error)
	ListVolumeAttachments(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.VolumeAttachmentDTO]) (resourcecore.ListResult[resourcecore.VolumeAttachmentDTO], error)
	GetWorkload(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string, string) (resourcecore.WorkloadDetailDTO, error)
	WorkloadYAMLDocument(context.Context, namespaces.SelectionBinding, string, string, string) ([]byte, error)
	GetPod(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.PodDetailDTO, error)
	PodYAML(context.Context, namespaces.SelectionBinding, string, string) ([]byte, error)
	GetService(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.ServiceDetailDTO, error)
	GetIngress(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.IngressDetailDTO, error)
	GetEndpointSlice(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.EndpointSliceDetailDTO, error)
	GetConfigMap(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.ConfigMapDetailDTO, error)
	GetSecret(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.SecretMetadataDTO, error)
	GetNode(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string) (resourcecore.NodeDetailDTO, error)
	NodeYAMLDocument(context.Context, namespaces.SelectionBinding, string) ([]byte, error)
	GetLease(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.LeaseDetailDTO, error)
	GetPersistentVolumeClaim(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.PersistentVolumeClaimDetailDTO, error)
	GetPersistentVolume(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string) (resourcecore.PersistentVolumeDetailDTO, error)
	PersistentVolumeYAMLDocument(context.Context, namespaces.SelectionBinding, string) ([]byte, error)
	GetStorageClass(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string) (resourcecore.StorageClassDetailDTO, error)
	StorageClassYAMLDocument(context.Context, namespaces.SelectionBinding, string) ([]byte, error)
	GetCSIDriver(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string) (resourcecore.CSIDriverDetailDTO, error)
	GetCSINode(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string) (resourcecore.CSINodeDetailDTO, error)
	GetVolumeAttachment(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string) (resourcecore.VolumeAttachmentDetailDTO, error)
	GetNamespace(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string) (resourcecore.NamespaceObjectDetailDTO, error)
	GetServiceAccount(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.ServiceAccountDTO, error)
	GetResourceQuota(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.ResourceQuotaDTO, error)
	GetLimitRange(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.LimitRangeDTO, error)
	GetHPA(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.HorizontalPodAutoscalerDTO, error)
	GetPDB(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resourcecore.PodDisruptionBudgetDTO, error)
	ListServiceAccounts(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.ServiceAccountDTO]) (resourcecore.ListResult[resourcecore.ServiceAccountDTO], error)
	ListResourceQuotas(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.ResourceQuotaDTO]) (resourcecore.ListResult[resourcecore.ResourceQuotaDTO], error)
	ListLimitRanges(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.LimitRangeDTO]) (resourcecore.ListResult[resourcecore.LimitRangeDTO], error)
	ListHPAs(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.HorizontalPodAutoscalerDTO]) (resourcecore.ListResult[resourcecore.HorizontalPodAutoscalerDTO], error)
	ListPDBs(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.PodDisruptionBudgetDTO]) (resourcecore.ListResult[resourcecore.PodDisruptionBudgetDTO], error)
	ResourceYAML(context.Context, namespaces.SelectionBinding, string, string, string) ([]byte, error)
	ResourceLastAppliedDiff(context.Context, namespaces.SelectionBinding, string, string, string) (resourcecore.LastAppliedDiffDTO, error)
	ReadLogs(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string, resourcecore.LogQuery) (resourcecore.LogReadDTO, error)
}

type PreferenceService interface {
	Get(context.Context) (resourcecore.PreferencesDTO, error)
	Put(context.Context, resourcecore.PreferencesDTO) (resourcecore.PreferencesDTO, error)
}

type Resources struct {
	service     ResourceService
	preferences PreferenceService
	selection   SelectionReader
	cursors     *api.CursorCodec
	now         func() time.Time
}

func NewResources(service ResourceService, preferences PreferenceService, selection SelectionReader, cursors *api.CursorCodec) *Resources {
	return &Resources{service: service, preferences: preferences, selection: selection, cursors: cursors, now: time.Now}
}

func (handler *Resources) Workloads(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionWorkloads, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.WorkloadDTO]) (resourcecore.ListResult[resourcecore.WorkloadDTO], error) {
		return handler.service.ListWorkloads(ctx, b, s, o, c)
	})
}
func (handler *Resources) Pods(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionPods, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.PodDTO]) (resourcecore.ListResult[resourcecore.PodDTO], error) {
		return handler.service.ListPods(ctx, b, s, o, c)
	})
}
func (handler *Resources) Events(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionEvents, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.EventDTO]) (resourcecore.ListResult[resourcecore.EventDTO], error) {
		return handler.service.ListEvents(ctx, b, s, o, c)
	})
}
func (handler *Resources) Services(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionServices, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.ServiceDTO]) (resourcecore.ListResult[resourcecore.ServiceDTO], error) {
		return handler.service.ListServices(ctx, b, s, o, c)
	})
}
func (handler *Resources) Ingresses(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionIngresses, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.IngressDTO]) (resourcecore.ListResult[resourcecore.IngressDTO], error) {
		return handler.service.ListIngresses(ctx, b, s, o, c)
	})
}
func (handler *Resources) EndpointSlices(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionEndpointSlices, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.EndpointSliceDTO]) (resourcecore.ListResult[resourcecore.EndpointSliceDTO], error) {
		return handler.service.ListEndpointSlices(ctx, b, s, o, c)
	})
}
func (handler *Resources) ConfigMaps(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionConfigMaps, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.ConfigMapListDTO]) (resourcecore.ListResult[resourcecore.ConfigMapListDTO], error) {
		return handler.service.ListConfigMaps(ctx, b, s, o, c)
	})
}
func (handler *Resources) Secrets(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionSecrets, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.SecretMetadataDTO]) (resourcecore.ListResult[resourcecore.SecretMetadataDTO], error) {
		return handler.service.ListSecrets(ctx, b, s, o, c)
	})
}

func (handler *Resources) Nodes(w http.ResponseWriter, r *http.Request) {
	handleClusterList(handler, w, r, resourcecore.CollectionNodes, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.NodeDTO]) (resourcecore.ListResult[resourcecore.NodeDTO], error) {
		return handler.service.ListNodes(ctx, b, s, o, c)
	})
}

func (handler *Resources) Leases(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionLeases, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.LeaseDTO]) (resourcecore.ListResult[resourcecore.LeaseDTO], error) {
		return handler.service.ListLeases(ctx, b, s, o, c)
	})
}
func (handler *Resources) PersistentVolumeClaims(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionPersistentVolumeClaims, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.PersistentVolumeClaimDTO]) (resourcecore.ListResult[resourcecore.PersistentVolumeClaimDTO], error) {
		return handler.service.ListPersistentVolumeClaims(ctx, b, s, o, c)
	})
}
func (handler *Resources) PersistentVolumes(w http.ResponseWriter, r *http.Request) {
	handleClusterList(handler, w, r, resourcecore.CollectionPersistentVolumes, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.PersistentVolumeDTO]) (resourcecore.ListResult[resourcecore.PersistentVolumeDTO], error) {
		return handler.service.ListPersistentVolumes(ctx, b, s, o, c)
	})
}
func (handler *Resources) StorageClasses(w http.ResponseWriter, r *http.Request) {
	handleClusterList(handler, w, r, resourcecore.CollectionStorageClasses, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.StorageClassDTO]) (resourcecore.ListResult[resourcecore.StorageClassDTO], error) {
		return handler.service.ListStorageClasses(ctx, b, s, o, c)
	})
}
func (handler *Resources) CSIDrivers(w http.ResponseWriter, r *http.Request) {
	handleClusterList(handler, w, r, resourcecore.CollectionCSIDrivers, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.CSIDriverDTO]) (resourcecore.ListResult[resourcecore.CSIDriverDTO], error) {
		return handler.service.ListCSIDrivers(ctx, b, s, o, c)
	})
}
func (handler *Resources) CSINodes(w http.ResponseWriter, r *http.Request) {
	handleClusterList(handler, w, r, resourcecore.CollectionCSINodes, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.CSINodeDTO]) (resourcecore.ListResult[resourcecore.CSINodeDTO], error) {
		return handler.service.ListCSINodes(ctx, b, s, o, c)
	})
}
func (handler *Resources) VolumeAttachments(w http.ResponseWriter, r *http.Request) {
	handleClusterList(handler, w, r, resourcecore.CollectionVolumeAttachments, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.VolumeAttachmentDTO]) (resourcecore.ListResult[resourcecore.VolumeAttachmentDTO], error) {
		return handler.service.ListVolumeAttachments(ctx, b, s, o, c)
	})
}

type listCall[T resourcecore.ListItem] func(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.ListOptions, *resourcecore.CompositeCursor[T]) (resourcecore.ListResult[T], error)
type resourceListEnvelope[T resourcecore.ListItem] struct {
	Data []T              `json:"data"`
	Meta resourceListMeta `json:"meta"`
}
type resourceListMeta struct {
	RequestID   string                   `json:"requestId"`
	Generation  string                   `json:"generation"`
	CollectedAt string                   `json:"collectedAt"`
	Page        resourcecore.PageDTO     `json:"page"`
	Coverage    resourcecore.CoverageDTO `json:"coverage"`
}

func handleList[T resourcecore.ListItem](handler *Resources, w http.ResponseWriter, r *http.Request, collection resourcecore.Collection, call listCall[T]) {
	if handler == nil || handler.service == nil || handler.selection == nil || handler.cursors == nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The resource API is unavailable.", nil, nil))
		return
	}
	options, err := decodeResourceListQuery(r, collection)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	writeListResult(handler, w, r, collection, options, binding, resolution, call)
}

// handleClusterList serves cluster-scoped collections (ADR 0006): a valid
// context is required, but no namespace scope. The namespace filter is
// rejected by the collection normalization.
func handleClusterList[T resourcecore.ListItem](handler *Resources, w http.ResponseWriter, r *http.Request, collection resourcecore.Collection, call listCall[T]) {
	if handler == nil || handler.service == nil || handler.selection == nil || handler.cursors == nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The resource API is unavailable.", nil, nil))
		return
	}
	options, err := decodeResourceListQuery(r, collection)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.clusterSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	writeListResult(handler, w, r, collection, options, binding, resolution, call)
}

func writeListResult[T resourcecore.ListItem](handler *Resources, w http.ResponseWriter, r *http.Request, collection resourcecore.Collection, options resourcecore.ListOptions, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, call listCall[T]) {
	queryOptions := options
	queryOptions.Continue = ""
	queryJSON, _ := json.Marshal(queryOptions)
	cursorBinding := api.CursorBinding{QueryHash: api.HashCursorQuery(string(queryJSON)), Context: binding.Context, Scope: resourceScope(resolution), Generation: binding.Generation}
	var cursor *resourcecore.CompositeCursor[T]
	if options.Continue != "" {
		decoded := new(resourcecore.CompositeCursor[T])
		if err := handler.cursors.Decode(options.Continue, cursorBinding, decoded); err != nil {
			api.WriteError(w, r, err)
			return
		}
		cursor = decoded
	}
	result, err := call(r.Context(), binding, resolution, options, cursor)
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	result.Page.Next = ""
	if result.Cursor != nil && !result.Cursor.Complete() {
		token, encodeErr := handler.cursors.Encode(cursorBinding, result.Cursor)
		if encodeErr != nil {
			api.WriteError(w, r, api.NewHTTPError(http.StatusTooManyRequests, api.CodeLimitExceeded, "The resource cursor exceeded its safe limit.", nil, encodeErr))
			return
		}
		result.Page.Next = token
	}
	envelope := resourceListEnvelope[T]{Data: result.Items, Meta: resourceListMeta{RequestID: api.RequestIDFromContext(r.Context()), Generation: binding.Generation, CollectedAt: result.CollectedAt.UTC().Format(time.RFC3339Nano), Page: result.Page, Coverage: result.Coverage}}
	handler.writeJSONIfCurrent(w, r, binding, envelope)
}

func (handler *Resources) WorkloadDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetWorkload(ctx, b, s, r.PathValue("kind"), r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) PodDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetPod(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) ServiceDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetService(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) IngressDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetIngress(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) EndpointSliceDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetEndpointSlice(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) ConfigMapDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetConfigMap(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) SecretDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetSecret(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}

func (handler *Resources) NodeDetail(w http.ResponseWriter, r *http.Request) {
	handler.clusterDetail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetNode(ctx, b, s, r.PathValue("name"))
	})
}

func (handler *Resources) LeaseDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetLease(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) PersistentVolumeClaimDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetPersistentVolumeClaim(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) PersistentVolumeDetail(w http.ResponseWriter, r *http.Request) {
	handler.clusterDetail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetPersistentVolume(ctx, b, s, r.PathValue("name"))
	})
}
func (handler *Resources) StorageClassDetail(w http.ResponseWriter, r *http.Request) {
	handler.clusterDetail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetStorageClass(ctx, b, s, r.PathValue("name"))
	})
}
func (handler *Resources) CSIDriverDetail(w http.ResponseWriter, r *http.Request) {
	handler.clusterDetail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetCSIDriver(ctx, b, s, r.PathValue("name"))
	})
}
func (handler *Resources) CSINodeDetail(w http.ResponseWriter, r *http.Request) {
	handler.clusterDetail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetCSINode(ctx, b, s, r.PathValue("name"))
	})
}
func (handler *Resources) VolumeAttachmentDetail(w http.ResponseWriter, r *http.Request) {
	handler.clusterDetail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetVolumeAttachment(ctx, b, s, r.PathValue("name"))
	})
}
func (handler *Resources) ServiceAccounts(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionServiceAccounts, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.ServiceAccountDTO]) (resourcecore.ListResult[resourcecore.ServiceAccountDTO], error) {
		return handler.service.ListServiceAccounts(ctx, b, s, o, c)
	})
}
func (handler *Resources) ResourceQuotas(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionResourceQuotas, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.ResourceQuotaDTO]) (resourcecore.ListResult[resourcecore.ResourceQuotaDTO], error) {
		return handler.service.ListResourceQuotas(ctx, b, s, o, c)
	})
}
func (handler *Resources) LimitRanges(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionLimitRanges, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.LimitRangeDTO]) (resourcecore.ListResult[resourcecore.LimitRangeDTO], error) {
		return handler.service.ListLimitRanges(ctx, b, s, o, c)
	})
}
func (handler *Resources) HPAs(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionHPAs, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.HorizontalPodAutoscalerDTO]) (resourcecore.ListResult[resourcecore.HorizontalPodAutoscalerDTO], error) {
		return handler.service.ListHPAs(ctx, b, s, o, c)
	})
}
func (handler *Resources) PDBs(w http.ResponseWriter, r *http.Request) {
	handleList(handler, w, r, resourcecore.CollectionPDBs, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.PodDisruptionBudgetDTO]) (resourcecore.ListResult[resourcecore.PodDisruptionBudgetDTO], error) {
		return handler.service.ListPDBs(ctx, b, s, o, c)
	})
}
func (handler *Resources) ServiceAccountDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetServiceAccount(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) ResourceQuotaDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetResourceQuota(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) LimitRangeDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetLimitRange(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) HPADetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetHPA(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) PDBDetail(w http.ResponseWriter, r *http.Request) {
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetPDB(ctx, b, s, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) NamespaceDetail(w http.ResponseWriter, r *http.Request) {
	handler.clusterDetail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return handler.service.GetNamespace(ctx, b, s, r.PathValue("name"))
	})
}

// clusterDetail serves one cluster-scoped detail: no namespace in the path,
// name-only validation, no scope membership check.
func (handler *Resources) clusterDetail(w http.ResponseWriter, r *http.Request, call detailCall) {
	if err := validateDetailRequest(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.clusterSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	name := r.PathValue("name")
	if name == "" || len(validation.IsDNS1123Subdomain(name)) > 0 {
		api.WriteError(w, r, validationHTTPError("The resource path is invalid.", nil))
		return
	}
	value, err := call(r.Context(), binding, resolution)
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	handler.writeJSONIfCurrent(w, r, binding, map[string]any{"data": value, "meta": map[string]any{"requestId": api.RequestIDFromContext(r.Context()), "generation": binding.Generation, "collectedAt": handler.now().UTC().Format(time.RFC3339Nano)}})
}

type detailCall func(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) (any, error)

func (handler *Resources) detail(w http.ResponseWriter, r *http.Request, call detailCall) {
	if err := validateDetailRequest(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err = validateResourcePath(r, resolution); err != nil {
		api.WriteError(w, r, err)
		return
	}
	value, err := call(r.Context(), binding, resolution)
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	handler.writeJSONIfCurrent(w, r, binding, map[string]any{"data": value, "meta": map[string]any{"requestId": api.RequestIDFromContext(r.Context()), "generation": binding.Generation, "collectedAt": handler.now().UTC().Format(time.RFC3339Nano)}})
}

func (handler *Resources) WorkloadYAML(w http.ResponseWriter, r *http.Request) {
	handler.yaml(w, r, func(ctx context.Context, b namespaces.SelectionBinding) ([]byte, error) {
		return handler.service.WorkloadYAMLDocument(ctx, b, r.PathValue("kind"), r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) PodYAML(w http.ResponseWriter, r *http.Request) {
	handler.yaml(w, r, func(ctx context.Context, b namespaces.SelectionBinding) ([]byte, error) {
		return handler.service.PodYAML(ctx, b, r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) ServiceYAML(w http.ResponseWriter, r *http.Request) {
	handler.collectionYAML(w, r, "services")
}
func (handler *Resources) IngressYAML(w http.ResponseWriter, r *http.Request) {
	handler.collectionYAML(w, r, "ingresses")
}
func (handler *Resources) EndpointSliceYAML(w http.ResponseWriter, r *http.Request) {
	handler.collectionYAML(w, r, "endpointslices")
}
func (handler *Resources) ConfigMapYAML(w http.ResponseWriter, r *http.Request) {
	handler.collectionYAML(w, r, "configmaps")
}
func (handler *Resources) NodeYAML(w http.ResponseWriter, r *http.Request) {
	handler.clusterYAML(w, r, func(ctx context.Context, b namespaces.SelectionBinding) ([]byte, error) {
		return handler.service.NodeYAMLDocument(ctx, b, r.PathValue("name"))
	})
}
func (handler *Resources) PersistentVolumeYAML(w http.ResponseWriter, r *http.Request) {
	handler.clusterYAML(w, r, func(ctx context.Context, b namespaces.SelectionBinding) ([]byte, error) {
		return handler.service.PersistentVolumeYAMLDocument(ctx, b, r.PathValue("name"))
	})
}
func (handler *Resources) StorageClassYAML(w http.ResponseWriter, r *http.Request) {
	handler.clusterYAML(w, r, func(ctx context.Context, b namespaces.SelectionBinding) ([]byte, error) {
		return handler.service.StorageClassYAMLDocument(ctx, b, r.PathValue("name"))
	})
}
func (handler *Resources) LeaseYAML(w http.ResponseWriter, r *http.Request) {
	handler.collectionYAML(w, r, "leases")
}
func (handler *Resources) PersistentVolumeClaimYAML(w http.ResponseWriter, r *http.Request) {
	handler.collectionYAML(w, r, "persistent-volume-claims")
}

// clusterYAML serves one cluster-scoped YAML action with the same fencing and
// no-store rules as the namespaced YAML routes.
func (handler *Resources) collectionYAML(w http.ResponseWriter, r *http.Request, collection string) {
	handler.yaml(w, r, func(ctx context.Context, b namespaces.SelectionBinding) ([]byte, error) {
		return handler.service.ResourceYAML(ctx, b, collection, r.PathValue("namespace"), r.PathValue("name"))
	})
}

// clusterYAML serves one cluster-scoped YAML action with the same fencing and
// no-store rules as the namespaced YAML routes.
func (handler *Resources) clusterYAML(w http.ResponseWriter, r *http.Request, call func(context.Context, namespaces.SelectionBinding) ([]byte, error)) {
	if err := validateDetailRequest(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, _, err := handler.clusterSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	name := r.PathValue("name")
	if name == "" || len(validation.IsDNS1123Subdomain(name)) > 0 {
		api.WriteError(w, r, validationHTTPError("The resource path is invalid.", nil))
		return
	}
	document, err := call(r.Context(), binding)
	if err != nil {
		httpErr := resourceHTTPError(err)
		if resourcecore.ErrorCodeOf(err) == resourcecore.CodeLimitExceeded {
			httpErr = api.NewHTTPError(http.StatusRequestEntityTooLarge, api.CodeBodyTooLarge, "The YAML document exceeds the response limit.", nil, err)
		}
		api.WriteError(w, r, httpErr)
		return
	}
	handler.writeIfCurrent(w, r, binding, func() {
		noStore(w)
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(document)
	})
}

// yamlDiffCollections are the metadata-bearing collections eligible for the
// last-applied diff. Secrets are deliberately absent.
var yamlDiffCollections = map[string]string{
	"pods":            "pods",
	"deployments":     "deployments",
	"replicasets":     "replicasets",
	"statefulsets":    "statefulsets",
	"daemonsets":      "daemonsets",
	"jobs":            "jobs",
	"cronjobs":        "cronjobs",
	"services":        "services",
	"ingresses":       "ingresses",
	"endpoint-slices": "endpointslices",
	"configmaps":      "configmaps",
}

func (handler *Resources) ResourceYAMLDiff(w http.ResponseWriter, r *http.Request) {
	collection, ok := yamlDiffCollections[r.PathValue("collection")]
	if !ok {
		api.WriteError(w, r, api.NewHTTPError(http.StatusNotFound, api.CodeNotFound, "The YAML diff resource type is invalid.", nil, nil))
		return
	}
	if err := validateDetailRequest(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err = validateResourcePath(r, resolution); err != nil {
		api.WriteError(w, r, err)
		return
	}
	diff, err := handler.service.ResourceLastAppliedDiff(r.Context(), binding, collection, r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		httpErr := resourceHTTPError(err)
		if resourcecore.ErrorCodeOf(err) == resourcecore.CodeLimitExceeded {
			httpErr = api.NewHTTPError(http.StatusRequestEntityTooLarge, api.CodeBodyTooLarge, "The YAML diff exceeds the response limit.", nil, err)
		}
		api.WriteError(w, r, httpErr)
		return
	}
	handler.writeJSONIfCurrent(w, r, binding, map[string]any{"data": diff})
}

type yamlCall func(context.Context, namespaces.SelectionBinding) ([]byte, error)

func (handler *Resources) yaml(w http.ResponseWriter, r *http.Request, call yamlCall) {
	if err := validateDetailRequest(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err = validateResourcePath(r, resolution); err != nil {
		api.WriteError(w, r, err)
		return
	}
	document, err := call(r.Context(), binding)
	if err != nil {
		httpErr := resourceHTTPError(err)
		if resourcecore.ErrorCodeOf(err) == resourcecore.CodeLimitExceeded {
			httpErr = api.NewHTTPError(http.StatusRequestEntityTooLarge, api.CodeBodyTooLarge, "The YAML document exceeds the response limit.", nil, err)
		}
		api.WriteError(w, r, httpErr)
		return
	}
	handler.writeIfCurrent(w, r, binding, func() {
		noStore(w)
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(document)
	})
}

func (handler *Resources) PodLogs(w http.ResponseWriter, r *http.Request) {
	query, err := decodeLogQuery(r, false)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err = validateResourcePath(r, resolution); err != nil {
		api.WriteError(w, r, err)
		return
	}
	value, err := handler.service.ReadLogs(r.Context(), binding, resolution, r.PathValue("namespace"), r.PathValue("name"), query)
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	handler.writeJSONIfCurrent(w, r, binding, map[string]any{"data": value})
}

func (handler *Resources) PreferencesGet(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		api.WriteError(w, r, validationHTTPError("Preferences do not accept query parameters.", nil))
		return
	}
	if handler == nil || handler.preferences == nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "Preferences are unavailable.", nil, nil))
		return
	}
	value, err := handler.preferences.Get(r.Context())
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	noStore(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = writeEnvelope(w, map[string]any{"data": value})
}
func (handler *Resources) PreferencesPut(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		api.WriteError(w, r, validationHTTPError("Preferences do not accept query parameters.", nil))
		return
	}
	if handler == nil || handler.preferences == nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "Preferences are unavailable.", nil, nil))
		return
	}
	var value resourcecore.PreferencesDTO
	if err := api.DecodeStrict(w, r, &value, maximumPreferencesBodyBytes); err != nil {
		api.WriteError(w, r, err)
		return
	}
	saved, err := handler.preferences.Put(r.Context(), value)
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	noStore(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = writeEnvelope(w, map[string]any{"data": saved})
}

func (handler *Resources) activeSelection() (namespaces.SelectionBinding, namespaces.ScopeResolution, error) {
	if handler == nil || handler.service == nil || handler.selection == nil {
		return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The resource API is unavailable.", nil, nil)
	}
	binding, resolution := handler.selection.Snapshot()
	if binding.ClusterProfileID <= 0 || binding.Context == "" || binding.Generation == "" || len(resolution.Namespaces) == 0 {
		return binding, resolution, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "No active Kubernetes resource scope is available.", nil, nil)
	}
	return binding, resolution, nil
}

// clusterSelection is the cluster-scoped reader binding (ADR 0006): a valid
// context and generation are required; namespaces in the scope are not.
func (handler *Resources) clusterSelection() (namespaces.SelectionBinding, namespaces.ScopeResolution, error) {
	if handler == nil || handler.service == nil || handler.selection == nil {
		return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The resource API is unavailable.", nil, nil)
	}
	binding, resolution := handler.selection.Snapshot()
	if binding.ClusterProfileID <= 0 || binding.Context == "" || binding.Generation == "" {
		return binding, resolution, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "No active Kubernetes context is available.", nil, nil)
	}
	return binding, resolution, nil
}
func (handler *Resources) writeJSONIfCurrent(w http.ResponseWriter, r *http.Request, binding namespaces.SelectionBinding, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "The resource response could not be encoded.", nil, err))
		return
	}
	handler.writeIfCurrent(w, r, binding, func() {
		noStore(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	})
}
func (handler *Resources) writeIfCurrent(w http.ResponseWriter, r *http.Request, binding namespaces.SelectionBinding, write func()) {
	if fenced, ok := handler.selection.(interface {
		IfCurrent(namespaces.SelectionBinding, func()) bool
	}); ok {
		if !fenced.IfCurrent(binding, write) {
			api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed before the resource response was published.", nil, nil))
		}
		return
	}
	current, _ := handler.selection.Snapshot()
	if !sameSelectionBinding(current, binding) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed before the resource response was published.", nil, nil))
		return
	}
	write()
}

func decodeResourceListQuery(r *http.Request, collection resourcecore.Collection) (resourcecore.ListOptions, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return resourcecore.ListOptions{}, validationHTTPError("The resource query is invalid.", nil)
	}
	allowed := map[string]bool{"limit": true, "continue": true, "search": true, "namespace": true, "status": true, "sort": true, "order": true}
	switch collection {
	case resourcecore.CollectionWorkloads:
		allowed["kind"] = true
	case resourcecore.CollectionPods:
		allowed["workload"] = true
		allowed["node"] = true
		allowed["restarts"] = true
		allowed["problematic"] = true
	case resourcecore.CollectionEvents:
		allowed["objectKind"] = true
		allowed["reason"] = true
	case resourcecore.CollectionEndpointSlices:
		allowed["addressType"] = true
	}
	for key, list := range values {
		if !allowed[key] || len(list) == 0 {
			return resourcecore.ListOptions{}, validationHTTPError("The resource query contains an unknown field.", nil)
		}
		repeatable := key == "namespace" || key == "status" || key == "kind"
		if !repeatable && len(list) != 1 {
			return resourcecore.ListOptions{}, validationHTTPError("The resource query contains a repeated field.", nil)
		}
		for _, value := range list {
			if value == "" {
				return resourcecore.ListOptions{}, validationHTTPError("Resource query values must be non-empty.", nil)
			}
		}
	}
	options := resourcecore.ListOptions{Continue: first(values, "continue"), Search: first(values, "search"), Namespaces: values["namespace"], Statuses: values["status"], Sort: first(values, "sort"), Order: resourcecore.SortOrder(first(values, "order")), Workload: first(values, "workload"), Node: first(values, "node"), Restarts: resourcecore.RestartFilter(first(values, "restarts")), ObjectKind: first(values, "objectKind"), Reason: first(values, "reason"), AddressType: first(values, "addressType")}
	for _, kind := range values["kind"] {
		options.Kinds = append(options.Kinds, resourcecore.WorkloadKind(kind))
	}
	if raw := first(values, "limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return resourcecore.ListOptions{}, validationHTTPError("limit must be a decimal integer.", nil)
		}
		options.Limit = parsed
	}
	if raw := first(values, "problematic"); raw != "" {
		parsed, parseErr := strconv.ParseBool(raw)
		if parseErr != nil || raw != "true" && raw != "false" {
			return resourcecore.ListOptions{}, validationHTTPError("problematic must be true or false.", nil)
		}
		options.Problematic = &parsed
	}
	normalized, normalizeErr := resourcecore.NormalizeListOptions(collection, options)
	if normalizeErr != nil {
		return resourcecore.ListOptions{}, resourceHTTPError(normalizeErr)
	}
	return normalized, nil
}
func first(values url.Values, key string) string {
	items := values[key]
	if len(items) == 0 {
		return ""
	}
	return items[0]
}
func resourceScope(resolution namespaces.ScopeResolution) string {
	if resolution.ScopeName != "" {
		return resolution.ScopeName
	}
	return resolution.ScopeSource
}
func validateDetailRequest(r *http.Request) error {
	if r.URL.RawQuery != "" {
		return validationHTTPError("This resource route does not accept query parameters.", nil)
	}
	return nil
}
func validateResourcePath(r *http.Request, resolution namespaces.ScopeResolution) error {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	if len(validation.IsDNS1123Subdomain(namespace)) > 0 || len(validation.IsDNS1123Subdomain(name)) > 0 {
		return validationHTTPError("The resource path is invalid.", nil)
	}
	allowed := false
	for _, candidate := range resolution.Namespaces {
		if candidate == namespace {
			allowed = true
			break
		}
	}
	if !allowed {
		return validationHTTPError("The namespace is outside the active scope.", nil)
	}
	if kind := r.PathValue("kind"); kind != "" && !containsString([]string{"deployments", "statefulsets", "daemonsets", "jobs", "cronjobs", "replicasets"}, kind) {
		return validationHTTPError("The workload kind is invalid.", nil)
	}
	return nil
}
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func decodeLogQuery(r *http.Request, follow bool) (resourcecore.LogQuery, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return resourcecore.LogQuery{}, validationHTTPError("The log query is invalid.", nil)
	}
	allowed := map[string]bool{"container": true, "previous": true, "timestamps": true, "tailLines": true, "since": true}
	for key, list := range values {
		if !allowed[key] || len(list) != 1 || list[0] == "" {
			return resourcecore.LogQuery{}, validationHTTPError("The log query contains an invalid field.", nil)
		}
	}
	query := resourcecore.LogQuery{Container: first(values, "container"), Since: first(values, "since")}
	if raw := first(values, "previous"); raw != "" {
		value, parseErr := strconv.ParseBool(raw)
		if parseErr != nil || raw != "true" && raw != "false" {
			return query, validationHTTPError("previous must be true or false.", nil)
		}
		query.Previous = value
	}
	if raw := first(values, "timestamps"); raw != "" {
		value, parseErr := strconv.ParseBool(raw)
		if parseErr != nil || raw != "true" && raw != "false" {
			return query, validationHTTPError("timestamps must be true or false.", nil)
		}
		query.Timestamps = &value
	}
	if raw := first(values, "tailLines"); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			return query, validationHTTPError("tailLines must be a decimal integer.", nil)
		}
		query.TailLines = value
	}
	normalized, normalizeErr := resourcecore.NormalizeLogQuery(query, follow)
	if normalizeErr != nil {
		return query, resourceHTTPError(normalizeErr)
	}
	return normalized, nil
}

func resourceHTTPError(err error) error {
	var httpError *api.HTTPError
	if errors.As(err, &httpError) {
		return err
	}
	code := resourcecore.ErrorCodeOf(err)
	message := resourcecore.PublicMessage(err)
	switch code {
	case resourcecore.CodeValidationFailed:
		return api.NewHTTPError(http.StatusBadRequest, api.CodeValidationFailed, message, nil, err)
	case resourcecore.CodeForbidden:
		return api.NewHTTPError(http.StatusForbidden, api.CodeForbidden, message, nil, err)
	case resourcecore.CodeNotFound:
		return api.NewHTTPError(http.StatusNotFound, api.CodeNotFound, message, nil, err)
	case resourcecore.CodeCursorExpired:
		return api.NewHTTPError(http.StatusGone, api.CodeCursorExpired, message, nil, err)
	case resourcecore.CodeGenerationChanged:
		return api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, message, nil, err)
	case resourcecore.CodeLimitExceeded:
		return api.NewHTTPError(http.StatusTooManyRequests, api.CodeLimitExceeded, message, nil, err)
	case resourcecore.CodePreferenceSensitive:
		return api.NewHTTPError(http.StatusBadRequest, api.CodePreferenceSensitive, message, nil, err)
	case resourcecore.CodeFeatureUnavailable:
		return api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, message, nil, err)
	case resourcecore.CodeAuthorizationUnavailable:
		return api.NewHTTPError(http.StatusServiceUnavailable, api.CodeAuthorizationUnavailable, message, nil, err)
	case resourcecore.CodeAuthenticationUnavailable:
		return api.NewHTTPError(http.StatusServiceUnavailable, api.CodeAuthenticationUnavailable, message, nil, err)
	case resourcecore.CodeUpstreamTimeout:
		return api.NewHTTPError(http.StatusGatewayTimeout, api.CodeUpstreamTimeout, message, nil, err)
	default:
		return api.NewHTTPError(http.StatusServiceUnavailable, api.CodeClusterUnavailable, message, nil, err)
	}
}
