package kubernetesruntime

import (
	"context"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

const relatedPrefetchTimeout = 3 * time.Second

// scheduleRelatedPrefetch warms only small, authorized pages related to the
// first visible workload. WithoutCancel preserves trace and authorization
// values after the HTTP response while the explicit timeout and Close cancel
// bound the work.
func (backend *ResourceBackend) scheduleRelatedPrefetch(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, item resources.WorkloadDTO) {
	if backend == nil || !backend.intelligentPrefetch || item.Namespace == "" {
		return
	}
	base := context.WithoutCancel(ctx)
	prefetchCtx, cancel := context.WithTimeout(base, relatedPrefetchTimeout)

	backend.prefetchMu.Lock()
	if backend.prefetchClosed {
		backend.prefetchMu.Unlock()
		cancel()
		return
	}
	backend.prefetchNextID++
	id := backend.prefetchNextID
	backend.prefetchCancels[id] = cancel
	backend.prefetchWG.Add(1)
	backend.prefetchMu.Unlock()

	go func() {
		defer backend.finishPrefetch(id, cancel)
		options := resources.ListOptions{
			Limit: 20, Namespaces: []string{item.Namespace}, Priority: resources.PriorityLikelyNext,
		}
		_, _ = backend.ListPods(prefetchCtx, binding, resolution, options, nil)
		if prefetchCtx.Err() != nil {
			return
		}
		options.ObjectKind = item.Kind
		_, _ = backend.ListEvents(prefetchCtx, binding, resolution, options, nil)
	}()
}

func (backend *ResourceBackend) finishPrefetch(id uint64, cancel context.CancelFunc) {
	cancel()
	backend.prefetchMu.Lock()
	delete(backend.prefetchCancels, id)
	backend.prefetchMu.Unlock()
	backend.prefetchWG.Done()
}

func (backend *ResourceBackend) stopPrefetch() {
	if backend == nil {
		return
	}
	backend.prefetchMu.Lock()
	if backend.prefetchClosed {
		backend.prefetchMu.Unlock()
		return
	}
	backend.prefetchClosed = true
	cancels := make([]context.CancelFunc, 0, len(backend.prefetchCancels))
	for _, cancel := range backend.prefetchCancels {
		cancels = append(cancels, cancel)
	}
	backend.prefetchMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	backend.prefetchWG.Wait()
}
