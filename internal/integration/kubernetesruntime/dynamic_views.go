package kubernetesruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	kubeadapter "github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func dynamicGVR(resource resources.DynamicResource) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: resource.Group, Version: resource.Version, Resource: resource.Resource}
}

func (backend *ResourceBackend) ListDynamicResources(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, target resources.DynamicResource, options resources.ListOptions, cursor *resources.CompositeCursor[resources.DynamicRow]) (resources.ListResult[resources.DynamicRow], error) {
	resource, err := backend.resolveDynamicResource(ctx, binding, target)
	if err != nil {
		return resources.ListResult[resources.DynamicRow]{}, err
	}
	list := func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.DynamicRow], error) {
		result := resources.OriginPage[resources.DynamicRow]{Origin: page.Origin, Items: []resources.DynamicRow{}}
		request, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return result, err
		}
		defer cancel()
		// Core Secrets use metadata only even if an aggregated server overrides its printer.
		if !(resource.Group == "" && resource.Resource == "secrets") && clients.views != nil {
			table, err := clients.views.Table(request, dynamicGVR(resource), page.Origin.Namespace, "", listOptionsForPage(page))
			if err == nil {
				for _, row := range table.Rows {
					converted, err := backend.dynamicTableRow(resource, table.ColumnDefinitions, row)
					if err != nil {
						return result, err
					}
					result.Items = append(result.Items, converted)
				}
				result.Continue, result.ResourceVersion = table.Continue, table.ResourceVersion
				return result, nil
			}
			if !apierrors.IsNotAcceptable(err) && !errors.Is(err, kubeadapter.ErrTableUnsupported) {
				return result, mapResourceError(err)
			}
		}
		endpoint := clients.metadata.Resource(dynamicGVR(resource))
		var values *metav1.PartialObjectMetadataList
		if resource.Namespaced {
			values, err = endpoint.Namespace(page.Origin.Namespace).List(request, listOptionsForPage(page))
		} else {
			values, err = endpoint.List(request, listOptionsForPage(page))
		}
		if err != nil {
			return result, mapResourceError(err)
		}
		for i := range values.Items {
			result.Items = append(result.Items, backend.dynamicMetadata(resource, &values.Items[i]))
		}
		result.Continue, result.ResourceVersion = values.Continue, values.ResourceVersion
		return result, nil
	}
	less := func(a, b resources.DynamicRow) bool { return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name }
	filter := func(items []resources.DynamicRow, options resources.ListOptions) []resources.DynamicRow {
		out := make([]resources.DynamicRow, 0, len(items))
		for _, item := range items {
			if matchesSearch(options, item.Name, item.Namespace, item.Kind, strings.Join(item.Cells, " ")) {
				out = append(out, item)
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			if options.Order == resources.OrderDescending {
				return less(out[j], out[i])
			}
			return less(out[i], out[j])
		})
		return out
	}
	if !resource.Namespaced {
		return clusterCollect(ctx, backend, binding, resolution, resource.Collection(), options, cursor, less, list, filter)
	}
	return collectFilteredResource(ctx, backend, binding, resolution, resource.Collection(), options, cursor, less, list, filter)
}

func (backend *ResourceBackend) dynamicMetadata(resource resources.DynamicResource, value metav1.Object) resources.DynamicRow {
	created := value.GetCreationTimestamp()
	row := resources.DynamicRow{Name: value.GetName(), Namespace: value.GetNamespace(), Kind: resource.Kind, UID: string(value.GetUID()), ResourceVersion: value.GetResourceVersion(), Columns: []resources.DynamicColumn{}, Cells: []string{}}
	if !created.IsZero() {
		row.CreatedAt = created.UTC().Format("2006-01-02T15:04:05Z")
		row.AgeSeconds = max(0, int64(backend.now().Sub(created.Time).Seconds()))
	}
	return row
}

func (backend *ResourceBackend) dynamicTableRow(resource resources.DynamicResource, columns []metav1.TableColumnDefinition, row metav1.TableRow) (resources.DynamicRow, error) {
	var object metav1.PartialObjectMetadata
	raw := row.Object.Raw
	if len(raw) == 0 && row.Object.Object != nil {
		raw, _ = json.Marshal(row.Object.Object)
	}
	if err := json.Unmarshal(raw, &object); err != nil || object.Name == "" {
		return resources.DynamicRow{}, resourceDomain(resources.CodeClusterUnavailable, "The resource printer omitted object metadata.", err)
	}
	result := backend.dynamicMetadata(resource, &object)
	// Bound both printer definitions and scalar cells before they enter cursor storage.
	result.Truncated = len(columns) > 24
	for index, column := range columns[:min(24, len(columns))] {
		result.Columns = append(result.Columns, resources.DynamicColumn{Name: boundedViewText(column.Name, 128), Type: boundedViewText(column.Type, 32), Format: boundedViewText(column.Format, 32), Priority: column.Priority})
		cell := "—"
		if index < len(row.Cells) {
			switch value := row.Cells[index].(type) {
			case string:
				cell = value
			case float64, bool, int, int64, json.Number:
				cell = fmt.Sprint(value)
			}
		}
		if len(cell) > 2048 {
			result.Truncated = true
		}
		result.Cells = append(result.Cells, boundedViewText(cell, 2048))
	}
	return result, nil
}
func boundedViewText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return strings.ToValidUTF8(value[:limit], "") + "…"
}

func dynamicGet[T resources.DetailItem](ctx context.Context, backend *ResourceBackend, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, resource resources.DynamicResource, namespace, name string, get func(context.Context, resourceClientSet) (T, error)) (T, error) {
	origin := resources.Origin{APIGroup: resource.Group, Version: resource.Version, Resource: resource.Resource, Namespace: namespace}
	if !resource.Namespaced {
		if namespace != "" {
			var zero T
			return zero, resourceDomain(resources.CodeValidationFailed, "This resource is cluster-scoped.", nil)
		}
		return clusterGet(ctx, backend, binding, origin, name, get)
	}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, _ string) (T, error) {
		request, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			var zero T
			return zero, err
		}
		defer cancel()
		return get(request, clients)
	})
}

func (backend *ResourceBackend) GetDynamicResource(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, target resources.DynamicResource, namespace, name string) (resources.DynamicRow, error) {
	resource, err := backend.resolveDynamicResource(ctx, binding, target)
	if err != nil {
		return resources.DynamicRow{}, err
	}
	return dynamicGet(ctx, backend, binding, resolution, resource, namespace, name, func(ctx context.Context, clients resourceClientSet) (resources.DynamicRow, error) {
		if !(resource.Group == "" && resource.Resource == "secrets") && clients.views != nil {
			table, err := clients.views.Table(ctx, dynamicGVR(resource), namespace, name, metav1.ListOptions{})
			if err == nil {
				if len(table.Rows) != 1 {
					return resources.DynamicRow{}, resourceDomain(resources.CodeNotFound, "The selected resource was not found.", nil)
				}
				return backend.dynamicTableRow(resource, table.ColumnDefinitions, table.Rows[0])
			}
			if !apierrors.IsNotAcceptable(err) && !errors.Is(err, kubeadapter.ErrTableUnsupported) {
				return resources.DynamicRow{}, mapResourceError(err)
			}
		}
		endpoint := clients.metadata.Resource(dynamicGVR(resource))
		var value *metav1.PartialObjectMetadata
		var err error
		if resource.Namespaced {
			value, err = endpoint.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		} else {
			value, err = endpoint.Get(ctx, name, metav1.GetOptions{})
		}
		if err != nil {
			return resources.DynamicRow{}, mapResourceError(err)
		}
		return backend.dynamicMetadata(resource, value), nil
	})
}

func (backend *ResourceBackend) GetDynamicYAML(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, target resources.DynamicResource, namespace, name string) (resources.DynamicDocument, error) {
	resource, err := backend.resolveDynamicResource(ctx, binding, target)
	if err != nil {
		return resources.DynamicDocument{}, err
	}
	return dynamicGet(ctx, backend, binding, resolution, resource, namespace, name, func(ctx context.Context, clients resourceClientSet) (resources.DynamicDocument, error) {
		var raw map[string]any
		if clients.views == nil {
			return resources.DynamicDocument{}, resourceDomain(resources.CodeFeatureUnavailable, "The resource document reader is unavailable.", nil)
		}
		if err := clients.views.Document(ctx, dynamicGVR(resource), namespace, name, &raw); err != nil {
			if errors.Is(err, kubeadapter.ErrViewResponseTooLarge) {
				return resources.DynamicDocument{}, resourceDomain(resources.CodeLimitExceeded, "The resource YAML exceeds the 2 MiB viewer limit.", err)
			}
			return resources.DynamicDocument{}, mapResourceError(err)
		}
		if raw == nil {
			return resources.DynamicDocument{}, resourceDomain(resources.CodeClusterUnavailable, "The resource document is invalid.", nil)
		}
		if metadata, ok := raw["metadata"].(map[string]any); ok {
			delete(metadata, "managedFields")
		}
		raw["kind"] = resource.Kind
		version := resource.Version
		if resource.Group != "" {
			version = resource.Group + "/" + version
		}
		raw["apiVersion"] = version
		document, err := yaml.Marshal(raw)
		if err != nil {
			return resources.DynamicDocument{}, resourceDomain(resources.CodeClusterUnavailable, "The resource YAML could not be rendered.", err)
		}
		if len(document) > 2<<20 {
			return resources.DynamicDocument{}, resourceDomain(resources.CodeLimitExceeded, "The resource YAML exceeds the 2 MiB viewer limit.", nil)
		}
		return resources.DynamicDocument{YAML: string(document), Generation: binding.Generation}, nil
	})
}
