package kubernetesruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	kubeadapter "github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

func (backend *ResourceBackend) helmMetadata(ctx context.Context, binding namespaces.SelectionBinding, namespace, storage, name string, labelFilters ...string) ([]resources.HelmReleaseDTO, error) {
	if _, ok := resources.HelmCollection(storage); !ok {
		return nil, resourceDomain(resources.CodeValidationFailed, "Unsupported Helm storage driver.", nil)
	}
	capability := backend.authorizer.Check(ctx, authorization.Key{Generation: binding.Generation, Namespace: namespace, Resource: storage, Verb: "list"})
	if capability.Decision == authorization.DecisionDenied {
		return nil, resourceDomain(resources.CodeForbidden, "Kubernetes denied listing Helm release metadata in this namespace.", nil)
	}
	requestContext, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return nil, err
	}
	defer cancel()
	selector := labels.Set{"owner": "helm"}
	if name != "" {
		selector["name"] = name
	}
	options := metav1.ListOptions{LabelSelector: selector.AsSelector().String(), Limit: 500}
	for _, filter := range labelFilters {
		if filter != "" {
			options.LabelSelector += "," + filter
		}
	}
	result := []resources.HelmReleaseDTO{}
	seenRecords := 0
	seenTokens := map[string]bool{}
	for {
		if err := requestContext.Err(); err != nil {
			return nil, mapResourceError(err)
		}
		list, err := clients.metadata.Resource(schema.GroupVersionResource{Version: "v1", Resource: storage}).Namespace(namespace).List(requestContext, options)
		if err != nil {
			return nil, mapResourceError(err)
		}
		seenRecords += len(list.Items)
		if seenRecords > resources.MaximumHelmMetadataItems {
			return nil, resourceDomain(resources.CodeLimitExceeded, "Helm metadata exceeds 10,000 records in this scope. Narrow the namespace or release filter.", nil)
		}
		for _, item := range list.Items {
			name := item.Labels["name"]
			revision, err := strconv.Atoi(item.Labels["version"])
			if err != nil || revision < 1 || chartutil.ValidateReleaseName(name) != nil || item.Name != fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, revision) {
				continue
			}
			result = append(result, resources.HelmReleaseDTO{Name: name, Namespace: item.Namespace, Driver: storage, Status: item.Labels["status"], Revision: revision, StorageName: item.Name, UID: string(item.UID), ResourceVersion: item.ResourceVersion, AgeSeconds: resources.HelmAge(item.CreationTimestamp.Time, backend.now()), UpdatedAt: item.CreationTimestamp.UTC().Format(time.RFC3339), Related: []resources.ResourceRef{}})
		}
		if list.Continue == "" {
			break
		}
		if options.Continue == list.Continue || seenTokens[list.Continue] || len(seenTokens) >= resources.MaximumHelmMetadataItems {
			return nil, resourceDomain(resources.CodeClusterUnavailable, "Helm metadata pagination made no progress.", nil)
		}
		seenTokens[list.Continue] = true
		options.Continue = list.Continue
	}
	return result, nil
}

func latestHelmReleases(items []resources.HelmReleaseDTO) []resources.HelmReleaseDTO {
	latest := map[string]resources.HelmReleaseDTO{}
	for _, item := range items {
		key := item.Namespace + "/" + item.Name
		if current, ok := latest[key]; !ok || current.Revision < item.Revision {
			latest[key] = item
		}
	}
	result := make([]resources.HelmReleaseDTO, 0, len(latest))
	for _, item := range latest {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Namespace+"/"+result[i].Name < result[j].Namespace+"/"+result[j].Name
	})
	return result
}

func (backend *ResourceBackend) ListHelmReleases(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, storage string, options resources.ListOptions, cursor *resources.CompositeCursor[resources.HelmReleaseDTO]) (resources.ListResult[resources.HelmReleaseDTO], error) {
	collection, ok := resources.HelmCollection(storage)
	if !ok {
		return resources.ListResult[resources.HelmReleaseDTO]{}, resourceDomain(resources.CodeValidationFailed, "Unsupported Helm storage driver.", nil)
	}
	// Reuse metadata within one fan-out window; neither chart values nor
	// manifests enter this request-local cache or the paginated DTO buffers.
	var mu sync.Mutex
	snapshots := map[string][]resources.HelmReleaseDTO{}
	list := func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.HelmReleaseDTO], error) {
		result := resources.OriginPage[resources.HelmReleaseDTO]{Origin: page.Origin, Items: []resources.HelmReleaseDTO{}}
		mu.Lock()
		items, loaded := snapshots[page.Origin.Namespace]
		mu.Unlock()
		if !loaded {
			name := ""
			if page.FieldSelector != "" {
				selector, err := fields.ParseSelector(page.FieldSelector)
				if err != nil {
					return result, resourceDomain(resources.CodeValidationFailed, "Invalid Helm field selector.", nil)
				}
				name, _ = selector.RequiresExactMatch("metadata.name")
			}
			values, err := backend.helmMetadata(ctx, binding, page.Origin.Namespace, storage, name, page.LabelSelector)
			if err != nil {
				return result, err
			}
			items = latestHelmReleases(values)
			mu.Lock()
			snapshots[page.Origin.Namespace] = items
			mu.Unlock()
		}
		start := 0
		if page.Continue != "" {
			if !strings.HasPrefix(page.Continue, "after:") {
				return result, resourceDomain(resources.CodeValidationFailed, "Invalid Helm pagination cursor.", nil)
			}
			after := strings.TrimPrefix(page.Continue, "after:")
			start = sort.Search(len(items), func(i int) bool { return items[i].Namespace+"/"+items[i].Name > after })
		}
		filtered := items[start:]
		if page.FieldSelector != "" {
			// The common query validator permits only metadata.name/namespace.
			// Helm names are logical release names, not backing Secret names.
			selector, err := resources.HelmFieldSelector(page.FieldSelector)
			if err != nil {
				return result, err
			}
			filtered = []resources.HelmReleaseDTO{}
			for _, item := range items[start:] {
				if selector(item.Name, item.Namespace) {
					filtered = append(filtered, item)
				}
			}
		}
		end := min(len(filtered), int(page.Limit))
		result.Items = append(result.Items, filtered[:end]...)
		if end < len(filtered) && end > 0 {
			result.Continue = "after:" + filtered[end-1].Namespace + "/" + filtered[end-1].Name
		}
		return result, nil
	}
	less := func(a, b resources.HelmReleaseDTO) bool { return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name }
	filter := func(items []resources.HelmReleaseDTO, options resources.ListOptions) []resources.HelmReleaseDTO {
		result := make([]resources.HelmReleaseDTO, 0, len(items))
		for _, item := range items {
			if matchesSearch(options, item.Name, item.Namespace, item.Status) {
				result = append(result, item)
			}
		}
		sort.SliceStable(result, func(i, j int) bool {
			if options.Order == resources.OrderDescending {
				return less(result[j], result[i])
			}
			return less(result[i], result[j])
		})
		return result
	}
	return collectFilteredResource(ctx, backend, binding, resolution, collection, options, cursor, less, list, filter)
}

func (backend *ResourceBackend) helmReleaseMetadata(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, storage, namespace, name string) (resources.HelmReleaseDTO, error) {
	if _, err := resources.ResolveNamespaces(resolution.Namespaces, []string{namespace}); err != nil {
		return resources.HelmReleaseDTO{}, err
	}
	records, err := backend.helmMetadata(ctx, binding, namespace, storage, name)
	if err != nil {
		return resources.HelmReleaseDTO{}, err
	}
	if len(records) == 0 {
		return resources.HelmReleaseDTO{}, resourceDomain(resources.CodeNotFound, "The Helm release was not found in this namespace and storage driver.", nil)
	}
	latest := latestHelmReleases(records)[0]
	sort.Slice(records, func(i, j int) bool { return records[i].Revision > records[j].Revision })
	for _, item := range records {
		latest.History = append(latest.History, resources.HelmRevisionDTO{Revision: item.Revision, Status: item.Status, UpdatedAt: item.UpdatedAt})
	}
	return latest, nil
}

func (backend *ResourceBackend) helmClient(ctx context.Context, binding namespaces.SelectionBinding, namespace, storage string) (*kubeadapter.HelmClient, context.CancelFunc, error) {
	if backend.runtime == nil {
		return nil, nil, resourceDomain(resources.CodeFeatureUnavailable, "The Helm runtime is unavailable.", nil)
	}
	lease, err := backend.runtime.leaseFor(ctx, binding)
	if err != nil {
		return nil, nil, mapResourceError(err)
	}
	lifetime, release := generationLifetime(ctx, lease.Generation.Context())
	bounded, cancel := context.WithTimeout(lifetime, 2*time.Minute)
	cleanup := func() { cancel(); release() }
	client, err := lease.Clients.HelmClient(bounded, namespace, storage)
	if err != nil {
		cleanup()
		return nil, nil, mapResourceError(err)
	}
	return client, cleanup, nil
}

func (backend *ResourceBackend) readHelmRelease(ctx context.Context, binding namespaces.SelectionBinding, metadata resources.HelmReleaseDTO) (*release.Release, error) {
	key := authorization.Key{Generation: binding.Generation, Namespace: metadata.Namespace, Resource: metadata.Driver, Verb: "get", ResourceName: metadata.StorageName}
	if backend.authorizer.Check(ctx, key).Decision == authorization.DecisionDenied {
		return nil, resourceDomain(resources.CodeForbidden, "Kubernetes denied reading this Helm release record.", nil)
	}
	client, cleanup, err := backend.helmClient(ctx, binding, metadata.Namespace, metadata.Driver)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	result, err := client.Get(metadata.Name, metadata.Revision)
	if err != nil {
		return nil, mapHelmError(err)
	}
	if result.Name != metadata.Name || result.Namespace != metadata.Namespace || result.Version != metadata.Revision {
		return nil, resourceDomain(resources.CodeValidationFailed, "The stored Helm release identity does not match its metadata.", nil)
	}
	return result, nil
}

func (backend *ResourceBackend) GetHelmRelease(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, storage, namespace, name string) (resources.HelmReleaseDTO, error) {
	metadata, err := backend.helmReleaseMetadata(ctx, binding, resolution, storage, namespace, name)
	if err != nil {
		return metadata, err
	}
	value, err := backend.readHelmRelease(ctx, binding, metadata)
	if err != nil {
		return metadata, err
	}
	if value.Chart != nil && value.Chart.Metadata != nil {
		metadata.Chart = value.Chart.Metadata.Name + "-" + value.Chart.Metadata.Version
		metadata.AppVersion = value.Chart.Metadata.AppVersion
	}
	metadata.Related = helmRelated(value.Manifest, namespace)
	return metadata, nil
}

func helmRelated(manifest, namespace string) []resources.ResourceRef {
	refs := []resources.ResourceRef{}
	seen := map[string]bool{}
	if len(manifest) > resources.MaximumHelmDocumentBytes {
		return refs
	}
	decoder := kyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for len(refs) < 500 {
		var object unstructured.Unstructured
		if err := decoder.Decode(&object); err != nil {
			break
		}
		if object.GetName() == "" {
			continue
		}
		ns := object.GetNamespace()
		groupVersion, err := schema.ParseGroupVersion(object.GetAPIVersion())
		if err != nil {
			continue
		}
		if ns == "" {
			ns = namespace
		}
		key := object.GetKind() + "/" + ns + "/" + object.GetName()
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, resources.ResourceRef{APIGroup: groupVersion.Group, Kind: object.GetKind(), Namespace: ns, Name: object.GetName()})
	}
	return refs
}

func (backend *ResourceBackend) HelmDocument(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, storage, namespace, name, format string) (resources.HelmDocumentDTO, error) {
	if format != "values" && format != "manifest" {
		return resources.HelmDocumentDTO{}, resourceDomain(resources.CodeValidationFailed, "Choose values or manifest.", nil)
	}
	metadata, err := backend.helmReleaseMetadata(ctx, binding, resolution, storage, namespace, name)
	if err != nil {
		return resources.HelmDocumentDTO{}, err
	}
	value, err := backend.readHelmRelease(ctx, binding, metadata)
	if err != nil {
		return resources.HelmDocumentDTO{}, err
	}
	document := ""
	switch format {
	case "values":
		encoded, err := yaml.Marshal(value.Config)
		if err != nil {
			return resources.HelmDocumentDTO{}, resourceDomain(resources.CodeClusterUnavailable, "Helm values could not be encoded.", err)
		}
		document = string(encoded)
	case "manifest":
		document = value.Manifest
	default:
		return resources.HelmDocumentDTO{}, resourceDomain(resources.CodeValidationFailed, "Choose values or manifest.", nil)
	}
	if len(document) > resources.MaximumHelmDocumentBytes {
		return resources.HelmDocumentDTO{}, resourceDomain(resources.CodeLimitExceeded, "The Helm document exceeds the 2 MiB viewer limit.", nil)
	}
	return resources.HelmDocumentDTO{Document: document, Format: format, Revision: metadata.Revision, UID: metadata.UID, ResourceVersion: metadata.ResourceVersion}, nil
}

func validateHelmMutation(binding namespaces.SelectionBinding, metadata resources.HelmReleaseDTO, request resources.HelmMutationRequest) error {
	if request.ExpectedGeneration != binding.Generation {
		return resourceDomain(resources.CodeGenerationChanged, "The active Kubernetes context changed. Reload the release before applying.", nil)
	}
	if !request.Confirmed || request.ExpectedGeneration != binding.Generation || request.ExpectedRevision < 1 || request.ExpectedUID == "" || request.ExpectedResourceVersion == "" || (request.Action != "values" && request.Action != "rollback") {
		return resourceDomain(resources.CodeValidationFailed, "Confirm the release, action and current generation before applying.", nil)
	}
	if request.ExpectedRevision != metadata.Revision || request.ExpectedUID != metadata.UID || request.ExpectedResourceVersion != metadata.ResourceVersion {
		return apierrors.NewConflict(schema.GroupResource{Resource: "helmreleases"}, metadata.Name, errors.New("the release changed; reload before applying"))
	}
	if request.Action == "rollback" && (request.Revision < 1 || request.Revision >= metadata.Revision) {
		return resourceDomain(resources.CodeValidationFailed, "Choose an earlier Helm revision.", nil)
	}
	if strings.HasPrefix(metadata.Status, "pending-") {
		return apierrors.NewConflict(schema.GroupResource{Resource: "helmreleases"}, metadata.Name, errors.New("another Helm operation is still pending"))
	}
	if request.Action == "rollback" && request.Values != "" || request.Action == "values" && request.Revision != 0 {
		return resourceDomain(resources.CodeValidationFailed, "Values updates and rollbacks are separate actions.", nil)
	}
	if request.Action == "values" && (len(request.Values) == 0 || len(request.Values) > resources.MaximumHelmDocumentBytes) {
		return resourceDomain(resources.CodeValidationFailed, "Helm values must contain a YAML mapping up to 2 MiB.", nil)
	}
	return nil
}

func (backend *ResourceBackend) MutateHelmRelease(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, storage, namespace, name string, request resources.HelmMutationRequest) (resources.HelmMutationResult, error) {
	// Reject malformed commands before reading any release values.
	if request.ExpectedGeneration != binding.Generation {
		return resources.HelmMutationResult{}, resourceDomain(resources.CodeGenerationChanged, "The active Kubernetes context changed. Reload the release before applying.", nil)
	}
	if !request.Confirmed || request.ExpectedRevision < 1 || request.ExpectedUID == "" || request.ExpectedResourceVersion == "" || (request.Action != "values" && request.Action != "rollback") {
		return resources.HelmMutationResult{}, resourceDomain(resources.CodeValidationFailed, "Confirm the release, action and current generation before applying.", nil)
	}
	var values map[string]interface{}
	if request.Action == "values" {
		if len(request.Values) == 0 || len(request.Values) > resources.MaximumHelmDocumentBytes || yaml.UnmarshalStrict([]byte(request.Values), &values) != nil || values == nil {
			return resources.HelmMutationResult{}, resourceDomain(resources.CodeValidationFailed, "Invalid values YAML. Use a mapping up to 2 MiB and check duplicate keys and indentation.", nil)
		}
	}
	metadata, err := backend.helmReleaseMetadata(ctx, binding, resolution, storage, namespace, name)
	if err != nil {
		return resources.HelmMutationResult{}, err
	}
	if err = validateHelmMutation(binding, metadata, request); err != nil {
		return resources.HelmMutationResult{}, err
	}
	if request.Action == "rollback" {
		found := false
		for _, revision := range metadata.History {
			found = found || revision.Revision == request.Revision
		}
		if !found {
			return resources.HelmMutationResult{}, resourceDomain(resources.CodeNotFound, "The selected Helm rollback revision is no longer available.", nil)
		}
	}
	refresher, ok := backend.authorizer.(resources.AuthorizationRefresher)
	if !ok {
		return resources.HelmMutationResult{}, resourceDomain(resources.CodeAuthorizationUnavailable, "Live authorization checks are required to update a Helm release.", nil)
	}
	for _, verb := range []string{"get", "create", "update"} {
		resourceName := metadata.StorageName
		if verb == "create" {
			resourceName = ""
		}
		capability := refresher.Refresh(ctx, authorization.Key{Generation: binding.Generation, Namespace: namespace, Resource: storage, Verb: verb, ResourceName: resourceName})
		if capability.Decision == authorization.DecisionDenied {
			return resources.HelmMutationResult{}, resourceDomain(resources.CodeForbidden, "Kubernetes denied "+verb+" permission for Helm release storage in this namespace.", nil)
		}
		if capability.Decision != authorization.DecisionAllowed {
			failure := authorization.ReviewFailure(capability)
			return resources.HelmMutationResult{}, resourceDomain(resources.ErrorCode(failure.Code), "Could not verify "+verb+" permission for Helm release storage. "+failure.Message, nil)
		}
	}
	value, err := backend.readHelmRelease(ctx, binding, metadata)
	if err != nil {
		return resources.HelmMutationResult{}, err
	}
	client, cleanup, err := backend.helmClient(ctx, binding, namespace, storage)
	if err != nil {
		return resources.HelmMutationResult{}, err
	}
	defer cleanup()
	var updated *release.Release
	if err := client.RequireRevision(kubeadapter.HelmRevisionGuard{Name: name, Revision: metadata.Revision, UID: metadata.UID, ResourceVersion: metadata.ResourceVersion}); err != nil {
		return resources.HelmMutationResult{}, mapHelmError(err)
	}
	if request.Action == "values" {
		if value.Chart == nil || value.Chart.Metadata == nil {
			return resources.HelmMutationResult{}, resourceDomain(resources.CodeValidationFailed, "The stored release is missing its chart. Values cannot be applied from this record.", nil)
		}
		updated, err = client.Upgrade(ctx, name, value.Chart, values)
	} else {
		updated, err = client.Rollback(name, request.Revision)
	}
	if err != nil {
		return resources.HelmMutationResult{}, mapHelmError(err)
	}
	status := "unknown"
	if updated == nil {
		return resources.HelmMutationResult{}, resourceDomain(resources.CodeClusterUnavailable, "Helm did not return the updated release. Reload its history to verify the result before retrying.", nil)
	}
	if updated.Info != nil {
		status = updated.Info.Status.String()
	}
	return resources.HelmMutationResult{Accepted: true, Revision: updated.Version, Status: status}, nil
}

func mapHelmError(err error) error {
	if err == nil {
		return nil
	}
	if apierrors.IsConflict(err) || errors.Is(err, driver.ErrReleaseExists) {
		return apierrors.NewConflict(schema.GroupResource{Resource: "helmreleases"}, "", errors.New("the Helm release changed"))
	}
	if errors.Is(err, kubeadapter.ErrHelmReleaseLimit) {
		return resourceDomain(resources.CodeLimitExceeded, "The Helm release or decoded history exceeds the safe read limit. Narrow the release history request.", err)
	}
	if errors.Is(err, kubeadapter.ErrHelmInvalidStorage) {
		return resourceDomain(resources.CodeValidationFailed, "The stored Helm release is incomplete, malformed, or has inconsistent identity metadata.", err)
	}
	if errors.Is(err, kubeadapter.ErrHelmReadOnly) {
		return resourceDomain(resources.CodeForbidden, "This Helm storage operation is unavailable. Reload the current release revision before applying.", err)
	}
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return resourceDomain(resources.CodeNotFound, "The Helm release revision was not found.", err)
	}
	if errors.Is(err, driver.ErrNoDeployedReleases) {
		return resourceDomain(resources.CodeNotFound, "Helm found no deployed revision for this release. Check its revision history before applying values.", err)
	}
	var schemaError chartutil.JSONSchemaValidationError
	if errors.As(err, &schemaError) || strings.Contains(err.Error(), "values don't meet the specifications of the schema(s)") {
		return resourceDomain(resources.CodeValidationFailed, "Helm values do not satisfy the chart's values schema. Check the required keys and value types.", err)
	}
	if strings.Contains(err.Error(), "another operation (install/upgrade/rollback) is in progress") {
		return apierrors.NewConflict(schema.GroupResource{Resource: "helmreleases"}, "", errors.New("another Helm operation is pending"))
	}
	if strings.Contains(err.Error(), "template:") {
		return resourceDomain(resources.CodeValidationFailed, "Helm could not render the chart templates with these values. Check the chart's required values and template expressions.", err)
	}
	if errors.Is(err, io.EOF) {
		return resourceDomain(resources.CodeClusterUnavailable, "The Helm release record is incomplete.", err)
	}
	return mapResourceError(err)
}
