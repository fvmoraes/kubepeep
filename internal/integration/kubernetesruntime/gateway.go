package kubernetesruntime

import (
	"context"
	"sort"
	"strings"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func gatewayEndpoint(clients resourceClientSet, resource resourcecatalog.Resource, version, namespace string) dynamic.ResourceInterface {
	endpoint := clients.dynamic.Resource(schema.GroupVersionResource{Group: resource.Group, Version: version, Resource: resource.Resource})
	if resource.Namespaced {
		return endpoint.Namespace(namespace)
	}
	return endpoint
}

func (backend *ResourceBackend) ListGatewayResources(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, collection string, options resources.ListOptions, cursor *resources.CompositeCursor[resources.GatewayDTO]) (resources.ListResult[resources.GatewayDTO], error) {
	resource, ok := resourcecatalog.Lookup(collection)
	if !ok || resource.Group != resourcecatalog.GatewayGroup {
		return resources.ListResult[resources.GatewayDTO]{}, resourceDomain(resources.CodeValidationFailed, "Unknown Gateway API resource.", nil)
	}
	list := func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.GatewayDTO], error) {
		result := resources.OriginPage[resources.GatewayDTO]{Origin: page.Origin, Items: []resources.GatewayDTO{}}
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return result, err
		}
		defer cancel()
		for _, version := range resourcecatalog.Versions(resource) {
			values, err := gatewayEndpoint(clients, resource, version, page.Origin.Namespace).List(requestContext, listOptionsForPage(page))
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return result, mapResourceError(err)
			}
			for i := range values.Items {
				values.Items[i].SetKind(resource.Kind)
				values.Items[i].SetAPIVersion(resource.Group + "/" + version)
				result.Items = append(result.Items, resources.ConvertGateway(&values.Items[i], backend.now()))
			}
			result.Continue, result.ResourceVersion = values.GetContinue(), values.GetResourceVersion()
			return result, nil
		}
		return result, resourceDomain(resources.CodeFeatureUnavailable, "This Gateway API resource is not served by the selected cluster. Install the corresponding Gateway API CRD and controller, or select a supported resource.", nil)
	}
	less := func(a, b resources.GatewayDTO) bool { return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name }
	filter := func(items []resources.GatewayDTO, options resources.ListOptions) []resources.GatewayDTO {
		out := make([]resources.GatewayDTO, 0, len(items))
		for _, item := range items {
			if matchesSearch(options, item.Name, item.Namespace, item.Kind, item.Status, item.ClassName, item.ControllerName, strings.Join(item.Hosts, " "), strings.Join(item.Addresses, " ")) {
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
		return clusterCollect(ctx, backend, binding, resolution, resources.Collection(collection), options, cursor, less, list, filter)
	}
	return collectFilteredResource(ctx, backend, binding, resolution, resources.Collection(collection), options, cursor, less, list, filter)
}

func (backend *ResourceBackend) GetGatewayResource(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, collection, namespace, name string) (resources.GatewayDTO, error) {
	resource, ok := resourcecatalog.Lookup(collection)
	if !ok || resource.Group != resourcecatalog.GatewayGroup {
		return resources.GatewayDTO{}, resourceDomain(resources.CodeValidationFailed, "Unknown Gateway API resource.", nil)
	}
	origin := resources.Origin{APIGroup: resource.Group, Version: resource.Version, Resource: resource.Resource, Namespace: namespace}
	get := func(ctx context.Context, clients resourceClientSet) (resources.GatewayDTO, error) {
		for _, version := range resourcecatalog.Versions(resource) {
			value, err := gatewayEndpoint(clients, resource, version, namespace).Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return resources.GatewayDTO{}, mapResourceError(err)
			}
			value.SetKind(resource.Kind)
			value.SetAPIVersion(resource.Group + "/" + version)
			return resources.ConvertGateway(value, backend.now()), nil
		}
		return resources.GatewayDTO{}, resourceDomain(resources.CodeNotFound, "This Gateway API object or its CRD was not found in the selected cluster.", nil)
	}
	if resource.Namespaced {
		return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, _ string) (resources.GatewayDTO, error) {
			requestContext, cancel, clients, err := backend.unary(ctx, binding)
			if err != nil {
				return resources.GatewayDTO{}, err
			}
			defer cancel()
			return get(requestContext, clients)
		})
	}
	if namespace != "" {
		return resources.GatewayDTO{}, resourceDomain(resources.CodeValidationFailed, "This Gateway API resource is cluster-scoped.", nil)
	}
	return clusterGet(ctx, backend, binding, origin, name, get)
}
