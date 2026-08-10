package dashboard

import (
	"context"
	"io"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type PageRequest struct {
	Limit    int
	Continue string
}

type PodPage struct {
	Items    []corev1.Pod
	Continue string
}

type EventPage struct {
	Items    []NormalizedEvent
	Continue string
}

type WorkloadPage struct {
	Deployments  []appsv1.Deployment
	StatefulSets []appsv1.StatefulSet
	// StatefulSetAvailable is optional field-presence evidence keyed by UID.
	// A nil map means the typed API schema guaranteed the field; a non-nil map
	// must explicitly contain true before a value may be classified healthy.
	StatefulSetAvailable map[types.UID]bool
	DaemonSets           []appsv1.DaemonSet
	Jobs                 []batchv1.Job
	CronJobs             []batchv1.CronJob
	Continue             string
	// Issues retain per-kind failures without discarding objects collected from
	// other kinds. They are an internal port concern and must be passed through
	// addBlockError before anything reaches the public API.
	Issues []WorkloadIssue
}

type WorkloadIssue struct {
	Kind string
	Err  error
}

type MetricsPage struct {
	Items    []metricsv1beta1.PodMetrics
	Continue string
	// Window is required when Items is empty so the response can still expose
	// an honest positive collection window. In that case it describes only the
	// successful list observation interval; it is not a fabricated usage sample.
	Window time.Duration
}

type PodPort interface {
	ListPods(context.Context, string, PageRequest) (PodPage, error)
}

type EventPort interface {
	ListEvents(context.Context, string, PageRequest) (EventPage, error)
}

type WorkloadPort interface {
	ListWorkloads(context.Context, string, PageRequest) (WorkloadPage, error)
}

type MetricsPort interface {
	Available(context.Context) (bool, error)
	ListPodMetrics(context.Context, string, PageRequest) (MetricsPage, error)
}

type MetricsAuthorizer interface {
	CanListPodMetrics(context.Context, string) (PermissionDecision, error)
}

type OwnerResolver interface {
	ResolvePodOwner(context.Context, *corev1.Pod) (*ResourceRef, error)
}

type PermissionDecision string

const (
	PermissionAllowed PermissionDecision = "allowed"
	PermissionDenied  PermissionDecision = "denied"
	PermissionUnknown PermissionDecision = "unknown"
)

type LogAuthorizer interface {
	CanReadPodLogs(context.Context, string, string) (PermissionDecision, error)
}

type LogReadRequest struct {
	Namespace  string
	Pod        string
	Container  string
	Previous   bool
	SinceTime  time.Time
	TailLines  int
	Timestamps bool
}

type LogReader interface {
	ReadLogs(context.Context, LogReadRequest) (io.ReadCloser, error)
}

// No persistence port exists by design: log readers feed bounded, in-memory
// classifiers directly and payloads cannot be written by this package.
