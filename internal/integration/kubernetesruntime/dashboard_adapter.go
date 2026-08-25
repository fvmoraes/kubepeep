package kubernetesruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	kubeadapter "github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

const maximumInternalContinueBytes = 16 << 10

// dashboardClientSet deliberately exposes only typed clients. REST configs,
// transports and credentials remain private to the Phase 4 runtime.
type dashboardClientSet struct {
	kubernetes kubeclient.Interface
	streaming  kubeclient.Interface
	metrics    metricsclient.Interface
}

type dashboardClientProvider interface {
	Unary(context.Context, namespaces.SelectionBinding) (context.Context, context.CancelFunc, dashboardClientSet, error)
}

type runtimeDashboardClientProvider struct{ runtime *Runtime }

func (provider runtimeDashboardClientProvider) Unary(ctx context.Context, binding namespaces.SelectionBinding) (context.Context, context.CancelFunc, dashboardClientSet, error) {
	lease, err := provider.runtime.leaseFor(ctx, binding)
	if err != nil {
		return nil, nil, dashboardClientSet{}, err
	}
	requestContext, cancel, err := lease.Generation.Unary(ctx)
	if err != nil {
		return nil, nil, dashboardClientSet{}, err
	}
	clients := dashboardClientSet{
		kubernetes: lease.Clients.UnaryKubernetes(),
		streaming:  lease.Clients.StreamingKubernetes(),
		metrics:    lease.Clients.Metrics(),
	}
	if clients.kubernetes == nil || clients.streaming == nil {
		cancel()
		return nil, nil, dashboardClientSet{}, errors.New("dashboard adapter: Kubernetes clients are unavailable")
	}
	return requestContext, cancel, clients, nil
}

// dashboardAdapter is immutable and bound to exactly one selection
// generation. A new instance is created for every HTTP request.
type dashboardAdapter struct {
	clients       dashboardClientProvider
	authorization authorization.AuthorizationService
	binding       namespaces.SelectionBinding
}

func (adapter *dashboardAdapter) ListPods(ctx context.Context, namespace string, page dashboard.PageRequest) (dashboard.PodPage, error) {
	requestContext, cancel, clients, err := adapter.unary(ctx)
	if err != nil {
		return dashboard.PodPage{}, err
	}
	defer cancel()
	var result *corev1.PodList
	err = adapter.guard(requestContext, authorization.Key{
		Generation: adapter.binding.Generation, Namespace: namespace, Resource: "pods", Verb: "list",
	}, func(operationContext context.Context) error {
		var listErr error
		result, listErr = clients.kubernetes.CoreV1().Pods(namespace).List(operationContext, metav1.ListOptions{
			Limit: int64(normalizedPageLimit(page.Limit)), Continue: page.Continue,
		})
		return listErr
	})
	if err != nil {
		return dashboard.PodPage{}, err
	}
	return dashboard.PodPage{Items: result.Items, Continue: result.Continue}, nil
}

func (adapter *dashboardAdapter) ListEvents(ctx context.Context, namespace string, page dashboard.PageRequest) (dashboard.EventPage, error) {
	requestContext, cancel, clients, err := adapter.unary(ctx)
	if err != nil {
		return dashboard.EventPage{}, err
	}
	defer cancel()
	var result *corev1.EventList
	err = adapter.guard(requestContext, authorization.Key{
		Generation: adapter.binding.Generation, Namespace: namespace, Resource: "events", Verb: "list",
	}, func(operationContext context.Context) error {
		var listErr error
		result, listErr = clients.kubernetes.CoreV1().Events(namespace).List(operationContext, metav1.ListOptions{
			Limit: int64(normalizedPageLimit(page.Limit)), Continue: page.Continue,
		})
		return listErr
	})
	if err != nil {
		return dashboard.EventPage{}, err
	}
	items := make([]dashboard.NormalizedEvent, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, dashboard.NormalizeCoreEvent(item))
	}
	return dashboard.EventPage{Items: items, Continue: result.Continue}, nil
}

type workloadContinue struct {
	Kind  int    `json:"kind"`
	Token string `json:"token,omitempty"`
}

func (adapter *dashboardAdapter) ListWorkloads(ctx context.Context, namespace string, page dashboard.PageRequest) (dashboard.WorkloadPage, error) {
	state, err := decodeWorkloadContinue(page.Continue)
	if err != nil {
		return dashboard.WorkloadPage{}, err
	}
	requestContext, cancel, clients, err := adapter.unary(ctx)
	if err != nil {
		return dashboard.WorkloadPage{}, err
	}
	defer cancel()
	limit := normalizedPageLimit(page.Limit)
	result := dashboard.WorkloadPage{}
	for state.Kind < workloadKindCount && workloadPageLength(result) < limit {
		remaining := limit - workloadPageLength(result)
		next, listErr := adapter.listWorkloadKind(requestContext, clients.kubernetes, namespace, state, remaining, &result)
		if listErr != nil {
			result.Issues = append(result.Issues, dashboard.WorkloadIssue{Kind: workloadKindName(state.Kind), Err: listErr})
			// A canceled/deadline-bound request cannot safely fan out further, but
			// objects already collected in this page remain useful partial data.
			if requestContext.Err() != nil {
				return result, nil
			}
			state = workloadContinue{Kind: state.Kind + 1}
			continue
		}
		if next.Token != "" {
			result.Continue, err = encodeWorkloadContinue(next)
			return result, err
		}
		state = workloadContinue{Kind: state.Kind + 1}
		if workloadPageLength(result) == limit && state.Kind < workloadKindCount {
			result.Continue, err = encodeWorkloadContinue(state)
			return result, err
		}
	}
	return result, nil
}

const (
	workloadDeployments = iota
	workloadStatefulSets
	workloadDaemonSets
	workloadJobs
	workloadCronJobs
	workloadKindCount
)

func (adapter *dashboardAdapter) listWorkloadKind(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	state workloadContinue,
	limit int,
	result *dashboard.WorkloadPage,
) (workloadContinue, error) {
	options := metav1.ListOptions{Limit: int64(limit), Continue: state.Token}
	switch state.Kind {
	case workloadDeployments:
		var list *appsv1.DeploymentList
		err := adapter.guard(ctx, workloadKey(adapter.binding.Generation, namespace, "apps", "deployments"), func(operationContext context.Context) error {
			var listErr error
			list, listErr = client.AppsV1().Deployments(namespace).List(operationContext, options)
			return listErr
		})
		if err != nil {
			return workloadContinue{}, err
		}
		result.Deployments = append(result.Deployments, list.Items...)
		return workloadContinue{Kind: state.Kind, Token: list.Continue}, nil
	case workloadStatefulSets:
		var list *appsv1.StatefulSetList
		err := adapter.guard(ctx, workloadKey(adapter.binding.Generation, namespace, "apps", "statefulsets"), func(operationContext context.Context) error {
			var listErr error
			list, listErr = client.AppsV1().StatefulSets(namespace).List(operationContext, options)
			return listErr
		})
		if err != nil {
			return workloadContinue{}, err
		}
		result.StatefulSets = append(result.StatefulSets, list.Items...)
		return workloadContinue{Kind: state.Kind, Token: list.Continue}, nil
	case workloadDaemonSets:
		var list *appsv1.DaemonSetList
		err := adapter.guard(ctx, workloadKey(adapter.binding.Generation, namespace, "apps", "daemonsets"), func(operationContext context.Context) error {
			var listErr error
			list, listErr = client.AppsV1().DaemonSets(namespace).List(operationContext, options)
			return listErr
		})
		if err != nil {
			return workloadContinue{}, err
		}
		result.DaemonSets = append(result.DaemonSets, list.Items...)
		return workloadContinue{Kind: state.Kind, Token: list.Continue}, nil
	case workloadJobs:
		var list *batchv1.JobList
		err := adapter.guard(ctx, workloadKey(adapter.binding.Generation, namespace, "batch", "jobs"), func(operationContext context.Context) error {
			var listErr error
			list, listErr = client.BatchV1().Jobs(namespace).List(operationContext, options)
			return listErr
		})
		if err != nil {
			return workloadContinue{}, err
		}
		result.Jobs = append(result.Jobs, list.Items...)
		return workloadContinue{Kind: state.Kind, Token: list.Continue}, nil
	case workloadCronJobs:
		var list *batchv1.CronJobList
		err := adapter.guard(ctx, workloadKey(adapter.binding.Generation, namespace, "batch", "cronjobs"), func(operationContext context.Context) error {
			var listErr error
			list, listErr = client.BatchV1().CronJobs(namespace).List(operationContext, options)
			return listErr
		})
		if err != nil {
			return workloadContinue{}, err
		}
		result.CronJobs = append(result.CronJobs, list.Items...)
		return workloadContinue{Kind: state.Kind, Token: list.Continue}, nil
	default:
		return workloadContinue{}, errors.New("dashboard adapter: invalid workload continuation")
	}
}

func (adapter *dashboardAdapter) Available(ctx context.Context) (bool, error) {
	requestContext, cancel, clients, err := adapter.unary(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	if err := requestContext.Err(); err != nil {
		return false, toDashboardError(err)
	}
	type discoveryResult struct {
		resources *metav1.APIResourceList
		err       error
	}
	discovered := make(chan discoveryResult, 1)
	go func() {
		resources, discoveryErr := clients.kubernetes.Discovery().ServerResourcesForGroupVersion("metrics.k8s.io/v1beta1")
		discovered <- discoveryResult{resources: resources, err: discoveryErr}
	}()
	var resources *metav1.APIResourceList
	select {
	case <-requestContext.Done():
		return false, toDashboardError(requestContext.Err())
	case result := <-discovered:
		resources, err = result.resources, result.err
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, toDashboardError(err)
	}
	if resources == nil {
		return false, nil
	}
	for _, resource := range resources.APIResources {
		if resource.Name == "pods" {
			return clients.metrics != nil, nil
		}
	}
	return false, nil
}

func (adapter *dashboardAdapter) ListPodMetrics(ctx context.Context, namespace string, page dashboard.PageRequest) (dashboard.MetricsPage, error) {
	requestContext, cancel, clients, err := adapter.unary(ctx)
	if err != nil {
		return dashboard.MetricsPage{}, err
	}
	defer cancel()
	if clients.metrics == nil {
		return dashboard.MetricsPage{}, dashboard.NewFeatureUnavailableError()
	}
	startedAt := time.Now()
	var result *metricsv1beta1.PodMetricsList
	err = adapter.guard(requestContext, workloadKey(adapter.binding.Generation, namespace, "metrics.k8s.io", "pods"), func(operationContext context.Context) error {
		var listErr error
		result, listErr = clients.metrics.MetricsV1beta1().PodMetricses(namespace).List(operationContext, metav1.ListOptions{
			Limit: int64(normalizedPageLimit(page.Limit)), Continue: page.Continue,
		})
		return listErr
	})
	if err != nil {
		return dashboard.MetricsPage{}, err
	}
	window := time.Duration(0)
	if len(result.Items) > 0 {
		window = result.Items[0].Window.Duration
	} else {
		// PodMetricsList has no list-level sampling window. For an
		// authoritative empty response, expose only the positive elapsed
		// observation interval (rounded up to seconds). This never creates a
		// CPU or memory measurement that the Metrics API did not return.
		window = emptyMetricsObservationWindow(time.Since(startedAt))
	}
	return dashboard.MetricsPage{Items: result.Items, Continue: result.Continue, Window: window}, nil
}

func (adapter *dashboardAdapter) CanListPodMetrics(ctx context.Context, namespace string) (dashboard.PermissionDecision, error) {
	return adapter.permission(ctx, workloadKey(adapter.binding.Generation, namespace, "metrics.k8s.io", "pods"))
}

func (adapter *dashboardAdapter) CanReadPodLogs(ctx context.Context, namespace, pod string) (dashboard.PermissionDecision, error) {
	return adapter.permission(ctx, authorization.Key{
		Generation: adapter.binding.Generation, Namespace: namespace, Resource: "pods", Subresource: "log", Verb: "get", ResourceName: pod,
	})
}

func (adapter *dashboardAdapter) ReadLogs(ctx context.Context, request dashboard.LogReadRequest) (io.ReadCloser, error) {
	requestContext, cancel, clients, err := adapter.unary(ctx)
	if err != nil {
		return nil, err
	}
	if clients.streaming == nil {
		cancel()
		return nil, dashboard.NewFeatureUnavailableError()
	}
	tailLines := int64(request.TailLines)
	sinceTime := metav1.NewTime(request.SinceTime.UTC())
	var reader io.ReadCloser
	err = adapter.guard(requestContext, authorization.Key{
		Generation: adapter.binding.Generation, Namespace: request.Namespace, Resource: "pods", Subresource: "log", Verb: "get", ResourceName: request.Pod,
	}, func(operationContext context.Context) error {
		var streamErr error
		reader, streamErr = clients.streaming.CoreV1().Pods(request.Namespace).GetLogs(request.Pod, &corev1.PodLogOptions{
			Container: request.Container, Previous: request.Previous, SinceTime: &sinceTime,
			TailLines: &tailLines, Timestamps: request.Timestamps,
		}).Stream(operationContext)
		return streamErr
	})
	if err != nil {
		cancel()
		return nil, err
	}
	if reader == nil {
		cancel()
		return nil, errors.New("dashboard adapter: Kubernetes returned an empty log reader")
	}
	return &cancelReadCloser{ReadCloser: reader, cancel: cancel}, nil
}

func (adapter *dashboardAdapter) ResolvePodOwner(ctx context.Context, pod *corev1.Pod) (*dashboard.ResourceRef, error) {
	direct := dashboard.DirectPodOwner(pod)
	if direct == nil || pod == nil {
		return direct, nil
	}
	if direct.Kind != "ReplicaSet" && direct.Kind != "Job" {
		return direct, nil
	}
	requestContext, cancel, clients, err := adapter.unary(ctx)
	if err != nil {
		return direct, err
	}
	defer cancel()
	switch direct.Kind {
	case "ReplicaSet":
		var replicaSet *appsv1.ReplicaSet
		err = adapter.guard(requestContext, authorization.Key{
			Generation: adapter.binding.Generation, Namespace: pod.Namespace, APIGroup: "apps", Resource: "replicasets", Verb: "get", ResourceName: direct.Name,
		}, func(operationContext context.Context) error {
			var getErr error
			replicaSet, getErr = clients.kubernetes.AppsV1().ReplicaSets(pod.Namespace).Get(operationContext, direct.Name, metav1.GetOptions{})
			return getErr
		})
		if err != nil {
			return direct, err
		}
		if owner := controllingOwner(replicaSet.OwnerReferences); owner != nil {
			return ownerResourceRef(owner, pod.Namespace), nil
		}
	case "Job":
		var job *batchv1.Job
		err = adapter.guard(requestContext, authorization.Key{
			Generation: adapter.binding.Generation, Namespace: pod.Namespace, APIGroup: "batch", Resource: "jobs", Verb: "get", ResourceName: direct.Name,
		}, func(operationContext context.Context) error {
			var getErr error
			job, getErr = clients.kubernetes.BatchV1().Jobs(pod.Namespace).Get(operationContext, direct.Name, metav1.GetOptions{})
			return getErr
		})
		if err != nil {
			return direct, err
		}
		if owner := controllingOwner(job.OwnerReferences); owner != nil {
			return ownerResourceRef(owner, pod.Namespace), nil
		}
	}
	return direct, nil
}

func (adapter *dashboardAdapter) unary(ctx context.Context) (context.Context, context.CancelFunc, dashboardClientSet, error) {
	if adapter == nil || adapter.clients == nil || adapter.binding.ClusterProfileID <= 0 || adapter.binding.Context == "" || adapter.binding.Generation == "" {
		return nil, nil, dashboardClientSet{}, dashboard.NewFeatureUnavailableError()
	}
	requestContext, cancel, clients, err := adapter.clients.Unary(ctx, adapter.binding)
	if err != nil {
		return nil, nil, dashboardClientSet{}, toDashboardError(err)
	}
	return requestContext, cancel, clients, nil
}

func (adapter *dashboardAdapter) guard(ctx context.Context, key authorization.Key, operation authorization.Operation) error {
	if adapter.authorization == nil {
		return dashboard.NewAuthorizationUnavailableError()
	}
	_, err := adapter.authorization.Guard(ctx, key, authorization.OperationRead, operation)
	return toDashboardError(err)
}

func (adapter *dashboardAdapter) permission(ctx context.Context, key authorization.Key) (dashboard.PermissionDecision, error) {
	if adapter.authorization == nil {
		return dashboard.PermissionUnknown, dashboard.NewAuthorizationUnavailableError()
	}
	capability := adapter.authorization.Check(ctx, key)
	if err := ctx.Err(); err != nil {
		return dashboard.PermissionUnknown, err
	}
	switch capability.Decision {
	case authorization.DecisionAllowed:
		return dashboard.PermissionAllowed, nil
	case authorization.DecisionDenied:
		return dashboard.PermissionDenied, nil
	default:
		return dashboard.PermissionUnknown, nil
	}
}

func toDashboardError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var clientError *kubeadapter.SafeError
	if errors.As(err, &clientError) {
		switch clientError.Code {
		case kubeadapter.CodeAuthenticationUnavailable:
			return dashboard.NewAuthenticationUnavailableError()
		case kubeadapter.CodeRequestTimeout:
			return context.DeadlineExceeded
		case kubeadapter.CodeRequestCanceled, kubeadapter.CodeGenerationChanged:
			return context.Canceled
		default:
			return err
		}
	}
	switch authorization.ErrorCodeOf(err) {
	case authorization.CodeForbidden:
		return dashboard.NewDeniedError()
	case authorization.CodeAuthorizationUnavailable:
		return dashboard.NewAuthorizationUnavailableError()
	case authorization.CodeAuthenticationUnavailable:
		return dashboard.NewAuthenticationUnavailableError()
	case authorization.CodeUpstreamTimeout:
		return context.DeadlineExceeded
	case authorization.CodeClientCanceled:
		return context.Canceled
	}
	return err
}

func workloadKey(generation, namespace, apiGroup, resource string) authorization.Key {
	return authorization.Key{Generation: generation, Namespace: namespace, APIGroup: apiGroup, Resource: resource, Verb: "list"}
}

func normalizedPageLimit(value int) int {
	if value < 1 || value > 500 {
		return dashboard.DefaultPageSize
	}
	return value
}

func workloadPageLength(page dashboard.WorkloadPage) int {
	return len(page.Deployments) + len(page.StatefulSets) + len(page.DaemonSets) + len(page.Jobs) + len(page.CronJobs)
}

func workloadKindName(kind int) string {
	switch kind {
	case workloadDeployments:
		return "Deployment"
	case workloadStatefulSets:
		return "StatefulSet"
	case workloadDaemonSets:
		return "DaemonSet"
	case workloadJobs:
		return "Job"
	case workloadCronJobs:
		return "CronJob"
	default:
		return "Unknown"
	}
}

func emptyMetricsObservationWindow(elapsed time.Duration) time.Duration {
	seconds := elapsed / time.Second
	if elapsed%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	return seconds * time.Second
}

func encodeWorkloadContinue(value workloadContinue) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(encoded)
	if len(token) > maximumInternalContinueBytes {
		return "", errors.New("dashboard adapter: workload continuation is too large")
	}
	return token, nil
}

func decodeWorkloadContinue(token string) (workloadContinue, error) {
	if token == "" {
		return workloadContinue{}, nil
	}
	if len(token) > maximumInternalContinueBytes {
		return workloadContinue{}, errors.New("dashboard adapter: invalid workload continuation")
	}
	encoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return workloadContinue{}, errors.New("dashboard adapter: invalid workload continuation")
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	var value workloadContinue
	if err := decoder.Decode(&value); err != nil || value.Kind < 0 || value.Kind >= workloadKindCount || len(value.Token) > maximumInternalContinueBytes || value.Kind == workloadDeployments && value.Token == "" {
		return workloadContinue{}, errors.New("dashboard adapter: invalid workload continuation")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return workloadContinue{}, errors.New("dashboard adapter: invalid workload continuation")
	}
	return value, nil
}

func controllingOwner(values []metav1.OwnerReference) *metav1.OwnerReference {
	for index := range values {
		if values[index].Controller != nil && *values[index].Controller {
			copy := values[index]
			return &copy
		}
	}
	return nil
}

func ownerResourceRef(owner *metav1.OwnerReference, namespace string) *dashboard.ResourceRef {
	if owner == nil {
		return nil
	}
	apiGroup := ""
	if separator := strings.IndexByte(owner.APIVersion, '/'); separator >= 0 {
		apiGroup = owner.APIVersion[:separator]
	}
	return &dashboard.ResourceRef{APIGroup: apiGroup, Kind: owner.Kind, Namespace: namespace, Name: owner.Name, UID: string(owner.UID)}
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (reader *cancelReadCloser) Close() error {
	var err error
	reader.once.Do(func() {
		err = reader.ReadCloser.Close()
		reader.cancel()
	})
	return err
}

var (
	_ dashboard.PodPort           = (*dashboardAdapter)(nil)
	_ dashboard.EventPort         = (*dashboardAdapter)(nil)
	_ dashboard.WorkloadPort      = (*dashboardAdapter)(nil)
	_ dashboard.MetricsPort       = (*dashboardAdapter)(nil)
	_ dashboard.MetricsAuthorizer = (*dashboardAdapter)(nil)
	_ dashboard.LogAuthorizer     = (*dashboardAdapter)(nil)
	_ dashboard.LogReader         = (*dashboardAdapter)(nil)
	_ dashboard.OwnerResolver     = (*dashboardAdapter)(nil)
)
