package dashboard

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

var errInvalidMetric = errors.New("invalid metric value")

func BuildMetrics(values []metricsv1beta1.PodMetrics, capturedAt time.Time) (MetricsDTO, error) {
	return buildMetrics(values, capturedAt, 0)
}

func buildMetrics(values []metricsv1beta1.PodMetrics, capturedAt time.Time, emptyWindow time.Duration) (MetricsDTO, error) {
	result := MetricsDTO{
		CollectedAt: capturedAt.UTC().Format(time.RFC3339Nano),
		Pods:        make([]PodMetricDTO, 0, len(values)),
		TopCPU:      make([]MetricRankDTO, 0),
		TopMemory:   make([]MetricRankDTO, 0),
	}
	var window time.Duration
	for index := range values {
		metric := &values[index]
		if metric.Window.Duration <= 0 {
			return MetricsDTO{}, errInvalidMetric
		}
		if window == 0 {
			window = metric.Window.Duration
		} else if window != metric.Window.Duration {
			return MetricsDTO{}, errInvalidMetric
		}
		pod := PodMetricDTO{
			Namespace:  metric.Namespace,
			Pod:        metric.Name,
			Containers: make([]ContainerMetricDTO, 0, len(metric.Containers)),
		}
		for _, container := range metric.Containers {
			cpu := container.Usage.Cpu().MilliValue()
			memory := container.Usage.Memory().Value()
			if cpu < 0 || memory < 0 || pod.CPUMillicores > math.MaxInt64-cpu || pod.MemoryBytes > math.MaxInt64-memory {
				return MetricsDTO{}, errInvalidMetric
			}
			pod.CPUMillicores += cpu
			pod.MemoryBytes += memory
			pod.Containers = append(pod.Containers, ContainerMetricDTO{
				Name:          container.Name,
				CPUMillicores: cpu,
				MemoryBytes:   memory,
			})
		}
		sort.Slice(pod.Containers, func(left, right int) bool {
			return pod.Containers[left].Name < pod.Containers[right].Name
		})
		result.Pods = append(result.Pods, pod)
	}
	if window == 0 {
		window = emptyWindow
	}
	if window == 0 {
		return MetricsDTO{}, errInvalidMetric
	}
	result.WindowSeconds = int64(window / time.Second)
	if result.WindowSeconds <= 0 {
		return MetricsDTO{}, errInvalidMetric
	}
	sort.Slice(result.Pods, func(left, right int) bool {
		if result.Pods[left].Namespace != result.Pods[right].Namespace {
			return result.Pods[left].Namespace < result.Pods[right].Namespace
		}
		return result.Pods[left].Pod < result.Pods[right].Pod
	})
	ranks := make([]MetricRankDTO, 0, len(result.Pods))
	for _, pod := range result.Pods {
		ranks = append(ranks, MetricRankDTO{
			Namespace:     pod.Namespace,
			Pod:           pod.Pod,
			CPUMillicores: pod.CPUMillicores,
			MemoryBytes:   pod.MemoryBytes,
		})
	}
	result.TopCPU = append(result.TopCPU, ranks...)
	sort.Slice(result.TopCPU, func(left, right int) bool {
		if result.TopCPU[left].CPUMillicores != result.TopCPU[right].CPUMillicores {
			return result.TopCPU[left].CPUMillicores > result.TopCPU[right].CPUMillicores
		}
		return lessMetricIdentity(result.TopCPU[left], result.TopCPU[right])
	})
	result.TopMemory = append(result.TopMemory, ranks...)
	sort.Slice(result.TopMemory, func(left, right int) bool {
		if result.TopMemory[left].MemoryBytes != result.TopMemory[right].MemoryBytes {
			return result.TopMemory[left].MemoryBytes > result.TopMemory[right].MemoryBytes
		}
		return lessMetricIdentity(result.TopMemory[left], result.TopMemory[right])
	})
	if len(result.TopCPU) > 10 {
		result.TopCPU = result.TopCPU[:10]
	}
	if len(result.TopMemory) > 10 {
		result.TopMemory = result.TopMemory[:10]
	}
	return result, nil
}

func lessMetricIdentity(left, right MetricRankDTO) bool {
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	return left.Pod < right.Pod
}

type MetricsService struct {
	port       MetricsPort
	authorizer MetricsAuthorizer
	clock      Clock
	budget     QueryBudget
}

func NewMetricsService(port MetricsPort, authorizer MetricsAuthorizer, clock Clock, budget QueryBudget) *MetricsService {
	if clock == nil {
		clock = realClock{}
	}
	return &MetricsService{port: port, authorizer: authorizer, clock: clock, budget: budget.Normalized()}
}

func (s *MetricsService) Collect(ctx context.Context, selection Selection) DashboardBlockDTO[MetricsDTO] {
	namespaces := canonicalNamespaces(selection.Namespaces)
	empty := MetricsDTO{Pods: []PodMetricDTO{}, TopCPU: []MetricRankDTO{}, TopMemory: []MetricRankDTO{}}
	block := blockWithValue(empty, emptyCoverage(len(namespaces)))
	if s == nil || s.port == nil {
		addBlockError(&block, "", NewFeatureUnavailableError())
		return block
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	available, err := s.port.Available(requestContext)
	if err != nil {
		addBlockError(&block, "", err)
		return block
	}
	if !available {
		addBlockError(&block, "", NewFeatureUnavailableError())
		return block
	}
	all := make([]metricsv1beta1.PodMetrics, 0)
	sampleWindow := time.Duration(0)
	observationWindow := time.Duration(0)
	for _, namespace := range namespaces {
		remaining := s.budget.MaxItems - len(all)
		if remaining <= 0 {
			block.Complete = false
			block.Truncated = true
			break
		}
		if s.authorizer == nil {
			addBlockError(&block, namespace, NewAuthorizationUnavailableError())
			continue
		}
		decision, permissionErr := s.authorizer.CanListPodMetrics(requestContext, namespace)
		if permissionErr != nil {
			addBlockError(&block, namespace, permissionErr)
			continue
		}
		switch decision {
		case PermissionDenied:
			addBlockError(&block, namespace, NewDeniedError())
			continue
		case PermissionUnknown:
			addBlockError(&block, namespace, NewAuthorizationUnavailableError())
			continue
		case PermissionAllowed:
		default:
			addBlockError(&block, namespace, NewAuthorizationUnavailableError())
			continue
		}
		items, window, complete, truncated, listErr := s.loadNamespace(requestContext, namespace, remaining)
		if window > 0 && len(items) > 0 {
			if sampleWindow == 0 {
				sampleWindow = window
			} else if sampleWindow != window {
				addBlockError(&block, namespace, errInvalidMetric)
				continue
			}
		} else if window > observationWindow {
			// Empty successful lists carry an observation interval rather than
			// a per-pod sample. Across namespaces the longest observed interval
			// is authoritative for the all-empty response.
			observationWindow = window
		}
		all = append(all, items...)
		if listErr != nil {
			addBlockError(&block, namespace, listErr)
			continue
		}
		if complete {
			block.Coverage.CompletedNamespaces++
		}
		if truncated {
			block.Complete = false
			block.Truncated = true
		}
	}
	collectionWindow := sampleWindow
	if collectionWindow == 0 {
		collectionWindow = observationWindow
	}
	if collectionWindow == 0 && len(all) == 0 && len(block.Errors) > 0 {
		return block
	}
	value, buildErr := buildMetrics(all, s.clock.Now(), collectionWindow)
	if buildErr != nil {
		addBlockError(&block, "", buildErr)
		return block
	}
	block.Value = value
	return block
}

func (s *MetricsService) loadNamespace(ctx context.Context, namespace string, maximumItems int) ([]metricsv1beta1.PodMetrics, time.Duration, bool, bool, error) {
	result := make([]metricsv1beta1.PodMetrics, 0)
	continuation := ""
	sampleWindow := time.Duration(0)
	observationWindow := time.Duration(0)
	for page := 0; page < s.budget.MaxPages; page++ {
		remaining := maximumItems - len(result)
		if remaining <= 0 {
			return result, metricsCollectionWindow(sampleWindow, observationWindow), false, true, nil
		}
		if err := ctx.Err(); err != nil {
			return result, metricsCollectionWindow(sampleWindow, observationWindow), false, false, err
		}
		response, err := s.port.ListPodMetrics(ctx, namespace, PageRequest{Limit: s.budget.PageSize, Continue: continuation})
		if err != nil {
			return result, metricsCollectionWindow(sampleWindow, observationWindow), false, false, err
		}
		pageWindow := response.Window
		if pageWindow == 0 && len(response.Items) > 0 {
			pageWindow = response.Items[0].Window.Duration
		}
		if len(response.Items) > 0 {
			if pageWindow > 0 {
				if sampleWindow == 0 {
					sampleWindow = pageWindow
				} else if sampleWindow != pageWindow {
					return result, sampleWindow, false, false, errInvalidMetric
				}
			}
		} else if pageWindow > observationWindow {
			observationWindow = pageWindow
		}
		if len(response.Items) > remaining {
			result = append(result, response.Items[:remaining]...)
			return result, metricsCollectionWindow(sampleWindow, observationWindow), false, true, nil
		}
		result = append(result, response.Items...)
		continuation = response.Continue
		if continuation == "" {
			return result, metricsCollectionWindow(sampleWindow, observationWindow), true, false, nil
		}
	}
	return result, metricsCollectionWindow(sampleWindow, observationWindow), false, continuation != "", nil
}

func metricsCollectionWindow(sample, observation time.Duration) time.Duration {
	if sample > 0 {
		return sample
	}
	return observation
}
