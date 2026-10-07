package kubernetesruntime

import (
	"context"
	"errors"
	"fmt"

	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
)

// Share only overlapping reads of exactly the same page and selection. No
// completed data or failures are cached, and authorization still runs inside
// the shared operation. One block timing out must not cancel another waiter.
func coalesceDashboardPage[T any](ctx context.Context, adapter *dashboardAdapter, kind, namespace string, page dashboard.PageRequest, load func(context.Context, string, dashboard.PageRequest) (T, error)) (T, error) {
	if adapter == nil || adapter.requests == nil {
		return load(ctx, namespace, page)
	}
	key := fmt.Sprintf("%d:%q:%q:%q:%q:%d:%q", adapter.binding.ClusterProfileID, adapter.binding.Context, adapter.binding.Generation, kind, namespace, page.Limit, page.Continue)
	value, err := adapter.requests.Do(ctx, key, func(shared context.Context) (any, error) {
		return load(shared, namespace, page)
	})
	result, ok := value.(T)
	if !ok {
		var empty T
		if err != nil {
			return empty, err
		}
		return empty, errors.New("dashboard collection returned an invalid page type")
	}
	return result, err
}
