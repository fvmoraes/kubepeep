package kubernetesruntime

import (
	"context"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const jobRunCacheTTL = 4 * time.Second
const maximumJobRunNamespaces = 64
const maximumCachedJobRuns = 40000

type jobRunCacheKey struct {
	Generation string
	Namespace  string
}

type jobRunCacheEntry struct {
	// Store only UIDs proven to belong to older runs, never Kubernetes objects.
	older   map[string]bool
	expires time.Time
}

// latestJobRunPods filters only the inventory projection. Native cursors and
// focused Pod/Job reads remain intact, including retry and parallel Pods.
func (backend *ResourceBackend) latestJobRunPods(ctx context.Context, binding namespaces.SelectionBinding, pods []resources.PodDTO) ([]resources.PodDTO, bool) {
	namespacesWithJobs := map[string]bool{}
	for _, pod := range pods {
		if batchJobOwner(pod.Owner) {
			namespacesWithJobs[pod.Namespace] = true
		}
	}
	if len(namespacesWithJobs) == 0 {
		return pods, false
	}
	// A slow/denied Job history must not hold the Pod inventory indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	byNamespace := make(map[string]map[string]bool, len(namespacesWithJobs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	queue := make(chan string, len(namespacesWithJobs))
	for namespace := range namespacesWithJobs {
		queue <- namespace
	}
	close(queue)
	for range min(4, len(namespacesWithJobs)) {
		wg.Go(func() {
			for namespace := range queue {
				older, complete := backend.olderJobRuns(ctx, binding, namespace)
				if complete {
					mu.Lock()
					byNamespace[namespace] = older
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	visible := make([]resources.PodDTO, 0, len(pods))
	for _, pod := range pods {
		if !batchJobOwner(pod.Owner) || !byNamespace[pod.Namespace][pod.Owner.UID] {
			visible = append(visible, pod)
		}
	}
	return visible, len(byNamespace) != len(namespacesWithJobs)
}

func batchJobOwner(owner *resources.OwnerDTO) bool {
	return owner != nil && owner.Kind == "Job" && owner.UID != "" && owner.APIVersion == "batch/v1"
}

func (backend *ResourceBackend) olderJobRuns(ctx context.Context, binding namespaces.SelectionBinding, namespace string) (map[string]bool, bool) {
	if ctx.Err() != nil || !backend.canListNamespace(ctx, binding, namespace, "batch", "jobs") {
		return nil, false
	}
	key := jobRunCacheKey{Generation: binding.Generation, Namespace: namespace}
	now := time.Now()
	if backend.now != nil {
		now = backend.now()
	}
	backend.jobRunsMu.Lock()
	if backend.jobRunsClosed || backend.jobRunsGeneration != "" && backend.jobRunsGeneration != binding.Generation {
		backend.jobRunsMu.Unlock()
		return nil, false
	}
	epoch := backend.jobRunsEpoch
	cached, found := backend.jobRuns[key]
	backend.jobRunsMu.Unlock()
	if found && now.Before(cached.expires) {
		return cached.older, true
	}
	value, err := backend.requestCoalescer().Do(ctx, "pod-job-runs/"+binding.Generation+"/"+namespace, func(shared context.Context) (any, error) {
		// Another caller may have filled the cache just before this request
		// joined the coalescer, after its first cache read.
		backend.jobRunsMu.Lock()
		if epoch != backend.jobRunsEpoch || backend.jobRunsClosed {
			backend.jobRunsMu.Unlock()
			return nil, nil
		}
		cached, found := backend.jobRuns[key]
		backend.jobRunsMu.Unlock()
		if found && now.Before(cached.expires) {
			return cached.older, nil
		}
		requestContext, cancel, clients, err := backend.unary(shared, binding)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, err
		}
		jobs, complete := backend.cronJobHistory(requestContext, binding, clients.kubernetes, namespace)
		if !complete {
			return nil, nil
		}
		older := olderCronJobRuns(jobs)
		backend.jobRunsMu.Lock()
		defer backend.jobRunsMu.Unlock()
		// A successful LIST can cross a context switch or Close before cache
		// publication, even if its request context was canceled meanwhile.
		if epoch != backend.jobRunsEpoch || backend.jobRunsClosed || requestContext.Err() != nil {
			return nil, nil
		}
		if backend.jobRuns == nil {
			backend.jobRuns = make(map[jobRunCacheKey]jobRunCacheEntry)
		}
		for other, entry := range backend.jobRuns {
			if other.Generation != binding.Generation || !now.Before(entry.expires) {
				delete(backend.jobRuns, other)
			}
		}
		retained := 0
		for _, entry := range backend.jobRuns {
			retained += len(entry.older)
		}
		for len(backend.jobRuns) >= maximumJobRunNamespaces || retained+len(older) > maximumCachedJobRuns {
			var oldest jobRunCacheKey
			var expires time.Time
			for other, entry := range backend.jobRuns {
				if expires.IsZero() || entry.expires.Before(expires) {
					oldest, expires = other, entry.expires
				}
			}
			retained -= len(backend.jobRuns[oldest].older)
			delete(backend.jobRuns, oldest)
		}
		backend.jobRuns[key] = jobRunCacheEntry{older: older, expires: now.Add(jobRunCacheTTL)}
		return older, nil
	})
	older, complete := value.(map[string]bool)
	return older, err == nil && complete
}

func olderCronJobRuns(jobs []batchv1.Job) map[string]bool {
	// Creation time identifies the run, not Pod age or completion time. A retry
	// of an old run must not replace a more recent successful or failed run.
	latest := map[string]*batchv1.Job{}
	older := map[string]bool{}
	for i := range jobs {
		job := &jobs[i]
		owner := metav1.GetControllerOf(job)
		if owner == nil || owner.Kind != "CronJob" || owner.UID == "" || job.UID == "" || job.CreationTimestamp.IsZero() || (owner.APIVersion != "batch/v1" && owner.APIVersion != "batch/v1beta1") {
			continue
		}
		key := job.Namespace + "/" + string(owner.UID)
		previous := latest[key]
		if previous == nil {
			latest[key] = job
		} else if job.CreationTimestamp.After(previous.CreationTimestamp.Time) || job.CreationTimestamp.Equal(&previous.CreationTimestamp) && job.Name > previous.Name {
			older[string(previous.UID)] = true
			latest[key] = job
		} else if job.Name != previous.Name {
			older[string(job.UID)] = true
		}
	}
	return older
}
