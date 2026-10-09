package kubernetesruntime

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type resourceDiscoveryCache struct {
	mu      sync.Mutex
	key     discoveryCacheKey
	value   resources.ResourceDiscoveryDTO
	expires time.Time
}

func (backend *ResourceBackend) DiscoverResources(ctx context.Context, binding namespaces.SelectionBinding, refresh bool) (resources.ResourceDiscoveryDTO, error) {
	cache := &backend.viewDiscovery
	key := discoveryKey(binding)
	cache.mu.Lock()
	if !refresh && cache.key == key && time.Now().Before(cache.expires) {
		value := cache.value
		cache.mu.Unlock()
		return value, nil
	}
	cache.mu.Unlock()
	value, err := backend.requestCoalescer().Do(ctx, key.requestID()+":resource-views", func(ctx context.Context) (any, error) {
		result := resources.ResourceDiscoveryDTO{Resources: []resources.DynamicResource{}, Failures: []resources.DiscoveryFailure{}}
		request, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return result, err
		}
		defer cancel()
		if clients.views == nil {
			return result, resourceDomain(resources.CodeFeatureUnavailable, "Resource discovery is unavailable.", nil)
		}
		var core metav1.APIVersions
		var groups metav1.APIGroupList
		versions := []string{}
		var firstFailure error
		failure := func(group string, err error) {
			mapped := mapResourceError(err)
			if firstFailure == nil {
				firstFailure = mapped
			}
			backend.recoverReadAuthentication(binding, mapped)
			result.Failures = append(result.Failures, resources.DiscoveryFailure{GroupVersion: group, Code: string(resources.ErrorCodeOf(mapped)), Message: resources.PublicMessage(mapped)})
		}
		if err := clients.views.JSONLimited(request, "/api", url.Values{}, "application/json", &core, 2<<20); err != nil {
			failure("core", err)
		} else {
			versions = append(versions, core.Versions...)
		}
		if err := clients.views.JSONLimited(request, "/apis", url.Values{}, "application/json", &groups, 2<<20); err != nil {
			failure("apis", err)
		} else {
			for _, group := range groups.Groups {
				if group.PreferredVersion.GroupVersion != "" {
					versions = append(versions, group.PreferredVersion.GroupVersion)
				}
				for _, version := range group.Versions {
					if version.GroupVersion != group.PreferredVersion.GroupVersion {
						versions = append(versions, version.GroupVersion)
					}
				}
			}
		}
		if len(versions) > 512 {
			versions = versions[:512]
			result.Truncated = true
		}
		type discovered struct {
			items []resources.DynamicResource
			err   error
		}
		responses := make([]discovered, len(versions))
		jobs := make(chan int)
		var wg sync.WaitGroup
		var retained atomic.Int32
		for worker := 0; worker < 4; worker++ {
			wg.Go(func() {
				for index := range jobs {
					gv, err := schema.ParseGroupVersion(versions[index])
					if err != nil {
						responses[index].err = err
						continue
					}
					check := resources.DynamicResource{Group: gv.Group, Version: gv.Version, Resource: "resources"}
					if !check.Valid() {
						responses[index].err = fmt.Errorf("invalid discovery group version")
						continue
					}
					path := "/apis/" + versions[index]
					if gv.Group == "" {
						path = "/api/" + gv.Version
					}
					var list metav1.APIResourceList
					responses[index].err = clients.views.JSONLimited(request, path, url.Values{}, "application/json", &list, 2<<20)
					for _, item := range list.APIResources {
						resource, ok := discoveredViewResource(gv, item)
						if !ok {
							continue
						}
						if retained.Add(1) > 4096 {
							break
						}
						responses[index].items = append(responses[index].items, resource)
					}
				}
			})
		}
		for index := range versions {
			select {
			case jobs <- index:
			case <-request.Done():
				close(jobs)
				wg.Wait()
				return result, mapResourceError(request.Err())
			}
		}
		close(jobs)
		wg.Wait()
		result.Truncated = result.Truncated || retained.Load() > 4096
		seen := map[string]bool{}
		for index, response := range responses {
			if response.err != nil {
				if len(result.Failures) < 64 {
					failure(versions[index], response.err)
				}
				continue
			}
			for _, resource := range response.items {
				identity := string(resource.Collection())
				if seen[identity] {
					continue
				}
				seen[identity] = true
				if len(result.Resources) >= 4096 {
					result.Truncated = true
					continue
				}
				result.Resources = append(result.Resources, resource)
			}
		}
		if len(result.Resources) == 0 && len(result.Failures) > 0 {
			return result, firstFailure
		}
		sort.Slice(result.Resources, func(i, j int) bool {
			a, b := result.Resources[i], result.Resources[j]
			return a.Resource+"/"+a.Group+"/"+a.Version < b.Resource+"/"+b.Group+"/"+b.Version
		})
		ttl := 5 * time.Minute
		if len(result.Failures) > 0 {
			ttl = 10 * time.Second
		}
		cache.mu.Lock()
		cache.key = key
		cache.value = result
		cache.expires = time.Now().Add(ttl)
		cache.mu.Unlock()
		return result, nil
	})
	if err != nil {
		backend.recoverReadAuthentication(binding, err)
		return resources.ResourceDiscoveryDTO{}, err
	}
	result, ok := value.(resources.ResourceDiscoveryDTO)
	if !ok {
		return resources.ResourceDiscoveryDTO{}, resourceDomain(resources.CodeClusterUnavailable, "Resource discovery returned an invalid result.", nil)
	}
	return result, nil
}

func hasVerb(verbs metav1.Verbs, verb string) bool {
	for _, item := range verbs {
		if item == verb {
			return true
		}
	}
	return false
}

func discoveredViewResource(gv schema.GroupVersion, item metav1.APIResource) (resources.DynamicResource, bool) {
	resource := resources.DynamicResource{Group: gv.Group, Version: gv.Version, Resource: item.Name, Kind: item.Kind, Namespaced: item.Namespaced}
	if !resource.Valid() || len(item.Kind) == 0 || len(item.Kind) > 128 || strings.ContainsAny(item.Kind, " /\\:.") || !hasVerb(item.Verbs, "list") || !hasVerb(item.Verbs, "get") {
		return resource, false
	}
	for _, short := range item.ShortNames {
		if len(resource.ShortNames) >= 16 {
			break
		}
		if (resources.DynamicResource{Version: gv.Version, Resource: short}).Valid() {
			resource.ShortNames = append(resource.ShortNames, short)
		}
	}
	return resource, true
}

func (backend *ResourceBackend) resolveDynamicResource(ctx context.Context, binding namespaces.SelectionBinding, target resources.DynamicResource) (resources.DynamicResource, error) {
	if !target.Valid() {
		return target, resourceDomain(resources.CodeValidationFailed, "Invalid resource identity.", nil)
	}
	catalog, err := backend.DiscoverResources(ctx, binding, false)
	if err != nil && (ctx.Err() != nil || resources.ErrorCodeOf(err) == resources.CodeAuthenticationUnavailable) {
		return target, err
	}
	for _, candidate := range catalog.Resources {
		if candidate.Collection() == target.Collection() {
			return candidate, nil
		}
	}
	// A partial or stale discovery catalog is not proof that this API is absent.
	request, cancel, clients, err := backend.unary(ctx, binding)
	if err != nil {
		return target, err
	}
	defer cancel()
	if clients.views == nil {
		return target, resourceDomain(resources.CodeFeatureUnavailable, "Resource discovery is unavailable.", nil)
	}
	path := "/apis/" + target.Group + "/" + target.Version
	if target.Group == "" {
		path = "/api/" + target.Version
	}
	var list metav1.APIResourceList
	if err := clients.views.JSONLimited(request, path, url.Values{}, "application/json", &list, 2<<20); err != nil {
		mapped := mapResourceError(err)
		backend.recoverReadAuthentication(binding, mapped)
		return target, mapped
	}
	for _, item := range list.APIResources {
		candidate, ok := discoveredViewResource(schema.GroupVersion{Group: target.Group, Version: target.Version}, item)
		if ok && candidate.Collection() == target.Collection() {
			return candidate, nil
		}
	}
	return target, resourceDomain(resources.CodeFeatureUnavailable, "This resource or API version is not available in the active cluster. Refresh the resource catalog and add the served version.", nil)
}
