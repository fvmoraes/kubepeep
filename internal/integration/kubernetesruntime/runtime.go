// Package kubernetesruntime adapts credential-bearing Kubernetes clients to
// the narrow, credential-free ports consumed by application services.
package kubernetesruntime

import (
	"context"
	"errors"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/contexts"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type Runtime struct {
	loader *kubernetes.Loader
	cache  *kubernetes.ClientCache
	now    func() time.Time

	mu        sync.RWMutex
	candidate *candidate
	lease     *kubernetes.Lease
	binding   namespaces.SelectionBinding
}

func New(parent context.Context, loader *kubernetes.Loader, factory *kubernetes.ClientFactory) (*Runtime, error) {
	if parent == nil || loader == nil || factory == nil {
		return nil, errors.New("kubernetes runtime: loader, factory and parent context are required")
	}
	cache, err := kubernetes.NewClientCache(parent, factory, kubernetes.DefaultUnaryTimeout)
	if err != nil {
		return nil, err
	}
	return &Runtime{loader: loader, cache: cache, now: time.Now}, nil
}

type candidate struct {
	resolution *kubernetes.Resolution
}

func (c *candidate) Paths() []string {
	if c == nil || c.resolution == nil {
		return []string{}
	}
	return c.resolution.Descriptor().Paths
}

func (c *candidate) Contexts() []contexts.ContextDescriptor {
	if c == nil || c.resolution == nil {
		return []contexts.ContextDescriptor{}
	}
	items := c.resolution.Contexts()
	result := make([]contexts.ContextDescriptor, 0, len(items))
	for _, item := range items {
		result = append(result, contexts.ContextDescriptor{Name: item.Name, Cluster: item.Cluster})
	}
	return result
}

func (c *candidate) Selected() (contexts.ContextDescriptor, bool) {
	if c == nil || c.resolution == nil {
		return contexts.ContextDescriptor{}, false
	}
	selected, ok := c.resolution.SelectedContext()
	return contexts.ContextDescriptor{Name: selected.Name, Cluster: selected.Cluster}, ok
}

func (r *Runtime) Resolve(ctx context.Context, request contexts.SourceRequest) (contexts.Candidate, error) {
	var persisted *kubernetes.ProfileReference
	if request.Persisted != nil {
		persisted = &kubernetes.ProfileReference{
			Paths: append([]string(nil), request.Persisted.Paths...), Context: request.Persisted.Context,
		}
	}
	resolution, err := r.loader.Resolve(ctx, kubernetes.ResolveRequest{
		ExplicitPath: request.ExplicitPath, ExplicitContext: request.ExplicitContext,
		Persisted: persisted, FirstReconcile: request.FirstReconcile, ProfileOnly: request.ProfileOnly,
	})
	if err != nil {
		return nil, externalError(err)
	}
	return &candidate{resolution: resolution}, nil
}

func (r *Runtime) Activate(ctx context.Context, value contexts.Candidate, binding namespaces.SelectionBinding) (api.ComponentState, error) {
	resolved, ok := value.(*candidate)
	if !ok || resolved == nil || resolved.resolution == nil || binding.ClusterProfileID <= 0 || binding.Context == "" || binding.Generation == "" {
		return api.ComponentState{}, errors.New("kubernetes runtime: invalid activation candidate")
	}
	// Publish the committed candidate before attempting network/client work so
	// an offline or temporarily invalid credential never leaves the previous
	// context available under the newly published generation.
	if !r.beginActivation(resolved, binding) {
		return api.ComponentState{}, contexts.ErrGenerationChange
	}
	lease, err := r.cache.Activate(ctx, resolved.resolution)
	if err != nil {
		if !r.matchesBinding(binding) {
			return api.ComponentState{}, contexts.ErrGenerationChange
		}
		return api.ComponentState{}, externalError(err)
	}
	if !r.publishLease(binding, lease) {
		return api.ComponentState{}, contexts.ErrGenerationChange
	}

	requestContext, cancel, err := lease.Generation.Unary(ctx)
	if err != nil {
		return api.ComponentState{}, externalError(err)
	}
	defer cancel()
	result := kubernetes.CheckConnectivity(requestContext, lease.Clients)
	if !r.matchesBinding(binding) {
		return api.ComponentState{}, contexts.ErrGenerationChange
	}
	checked := r.now().UTC()
	status := api.StatusUnknown
	switch result.Status {
	case kubernetes.ConnectivityHealthy:
		status = api.StatusHealthy
	case kubernetes.ConnectivityDegraded:
		status = api.StatusDegraded
	}
	return api.ComponentState{Status: status, Code: string(result.Code), Message: result.Message, CheckedAt: &checked}, nil
}

func (r *Runtime) beginActivation(resolved *candidate, binding namespaces.SelectionBinding) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	// OnGeneration publishes the coordinator's newest generation before an
	// asynchronous context activation reaches this boundary. Never allow an
	// older activation to overwrite that fence.
	if r.binding.Generation != "" && r.binding.Generation != binding.Generation {
		return false
	}
	r.candidate = resolved
	r.lease = nil
	r.binding = binding
	return true
}

func (r *Runtime) publishLease(binding namespaces.SelectionBinding, lease *kubernetes.Lease) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !sameBinding(r.binding, binding) {
		return false
	}
	r.lease = lease
	return true
}

func (r *Runtime) matchesBinding(binding namespaces.SelectionBinding) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sameBinding(r.binding, binding)
}

func sameBinding(current, expected namespaces.SelectionBinding) bool {
	return current.ClusterProfileID == expected.ClusterProfileID &&
		current.Context == expected.Context &&
		current.Cluster == expected.Cluster &&
		current.ActiveScopeID == expected.ActiveScopeID &&
		current.Generation == expected.Generation
}

// OnGeneration cancels old Kubernetes work and makes the retained parsed
// resolution available for a lazy rebuild under the new selection generation.
func (r *Runtime) OnGeneration(generation string) {
	r.mu.Lock()
	var descriptor kubernetes.Descriptor
	if r.candidate != nil && r.candidate.resolution != nil {
		descriptor = r.candidate.resolution.Descriptor()
	}
	r.lease = nil
	r.binding.Generation = generation
	r.mu.Unlock()
	if len(descriptor.Paths) != 0 {
		r.cache.Invalidate(descriptor)
	}
}

func (r *Runtime) leaseFor(ctx context.Context, binding namespaces.SelectionBinding) (*kubernetes.Lease, error) {
	r.mu.RLock()
	activeCandidate := r.candidate
	current := r.binding
	r.mu.RUnlock()
	if activeCandidate == nil || current.ClusterProfileID != binding.ClusterProfileID || current.Context != binding.Context || current.Generation != binding.Generation {
		return nil, errors.New("kubernetes runtime: active selection does not match")
	}
	rebuilt, err := r.cache.Activate(ctx, activeCandidate.resolution)
	if kubernetes.IsGenerationChanged(err) {
		descriptor := activeCandidate.resolution.Descriptor()
		fresh, resolveErr := r.loader.Resolve(ctx, kubernetes.ResolveRequest{
			Persisted:   &kubernetes.ProfileReference{Paths: descriptor.Paths, Context: descriptor.Context},
			ProfileOnly: true,
		})
		if resolveErr != nil {
			return nil, externalError(resolveErr)
		}
		activeCandidate = &candidate{resolution: fresh}
		rebuilt, err = r.cache.Activate(ctx, fresh)
		if err == nil {
			r.mu.Lock()
			if r.binding == current {
				r.candidate = activeCandidate
			}
			r.mu.Unlock()
		}
	}
	if err != nil {
		return nil, externalError(err)
	}
	r.mu.Lock()
	if r.binding == current {
		r.lease = rebuilt
	}
	r.mu.Unlock()
	return rebuilt, nil
}

// List implements namespaces.NamespaceCatalog. The real Kubernetes list is
// the authority: explicit 403 is distinct from operational unavailability.
func (r *Runtime) List(ctx context.Context, binding namespaces.SelectionBinding) ([]string, error) {
	page, err := r.ListPage(ctx, binding, namespaces.NamespacePageRequest{})
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		result = append(result, item.Name)
	}
	return result, nil
}

// ListPage preserves Kubernetes' native continue token and namespace phase.
// The raw continue token remains behind the HTTP cursor codec.
func (r *Runtime) ListPage(ctx context.Context, binding namespaces.SelectionBinding, page namespaces.NamespacePageRequest) (namespaces.NamespacePage, error) {
	lease, err := r.leaseFor(ctx, binding)
	if err != nil {
		return namespaces.NamespacePage{}, namespaces.ErrNamespaceListUnavailable
	}
	requestContext, cancel, err := lease.Generation.Unary(ctx)
	if err != nil {
		return namespaces.NamespacePage{}, namespaces.ErrNamespaceListUnavailable
	}
	defer cancel()
	list, err := lease.Clients.UnaryKubernetes().CoreV1().Namespaces().List(requestContext, metav1.ListOptions{Limit: page.Limit, Continue: page.Continue})
	if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
		return namespaces.NamespacePage{}, namespaces.ErrNamespacePageExpired
	}
	if apierrors.IsForbidden(err) {
		return namespaces.NamespacePage{}, namespaces.ErrNamespaceListForbidden
	}
	if err != nil || list == nil {
		if err != nil {
			r.cache.InvalidateOnError(lease.Descriptor, err)
		}
		return namespaces.NamespacePage{}, namespaces.ErrNamespaceListUnavailable
	}
	result := make([]namespaces.NamespaceRecord, 0, len(list.Items))
	for _, item := range list.Items {
		phase := string(item.Status.Phase)
		if phase != "Active" && phase != "Terminating" {
			phase = "Unknown"
		}
		result = append(result, namespaces.NamespaceRecord{Name: item.Name, UID: string(item.UID), Phase: phase})
	}
	return namespaces.NamespacePage{Items: result, Continue: list.Continue}, nil
}

func (r *Runtime) ReviewAccess(ctx context.Context, key authorization.Key) (authorization.AccessReviewResult, error) {
	reviewer, err := r.reviewer(ctx, key.Generation)
	if err != nil {
		return authorization.AccessReviewResult{}, err
	}
	result, reviewErr := reviewer.ReviewAccess(ctx, key)
	if reviewErr != nil {
		r.invalidateAuthentication(reviewErr)
	}
	return result, reviewErr
}

func (r *Runtime) ReviewRules(ctx context.Context, namespace string) (authorization.RulesReviewResult, error) {
	r.mu.RLock()
	generation := r.binding.Generation
	r.mu.RUnlock()
	reviewer, err := r.reviewer(ctx, generation)
	if err != nil {
		return authorization.RulesReviewResult{}, err
	}
	result, reviewErr := reviewer.ReviewRules(ctx, namespace)
	if reviewErr != nil {
		r.invalidateAuthentication(reviewErr)
	}
	return result, reviewErr
}

func (r *Runtime) reviewer(ctx context.Context, generation string) (*authorization.KubernetesReviewer, error) {
	r.mu.RLock()
	binding := r.binding
	r.mu.RUnlock()
	if generation == "" || binding.Generation != generation {
		return nil, errors.New("authorization reviewer selection changed")
	}
	lease, err := r.leaseFor(ctx, binding)
	if err != nil {
		return nil, err
	}
	return authorization.NewKubernetesReviewer(lease.Clients.UnaryKubernetes().AuthorizationV1())
}

func (r *Runtime) Close() error { return r.cache.Close() }

func (r *Runtime) invalidateAuthentication(err error) {
	r.mu.RLock()
	candidate := r.candidate
	r.mu.RUnlock()
	if candidate != nil && candidate.resolution != nil {
		r.cache.InvalidateOnError(candidate.resolution.Descriptor(), err)
	}
}

func externalError(err error) error {
	code, message, retryable := kubernetes.ErrorDetails(kubernetes.SanitizeError(err))
	return &contexts.ExternalError{Code: string(code), Message: message, Retryable: retryable}
}

var (
	_ contexts.Runtime                = (*Runtime)(nil)
	_ namespaces.NamespaceCatalog     = (*Runtime)(nil)
	_ namespaces.NamespacePageCatalog = (*Runtime)(nil)
	_ authorization.AccessReviewer    = (*Runtime)(nil)
	_ authorization.RulesReviewer     = (*Runtime)(nil)
)
