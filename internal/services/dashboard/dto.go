// Package dashboard contains the pure, bounded classifiers used by the
// dashboard and by the resource services that are introduced in phase 6.
// Kubernetes objects are accepted only as input; public values are copied to
// the closed DTOs in this package.
package dashboard

import "time"

// ResourceRef is the allowlisted identity of a Kubernetes object.
type ResourceRef struct {
	APIGroup  string `json:"apiGroup"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}

// PartialError is safe to return to a browser. Upstream error strings are
// deliberately never copied into this value.
type PartialError struct {
	Namespace string `json:"namespace,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// CoverageDTO describes namespace fan-out without implying that missing
// namespaces were successfully queried.
type CoverageDTO struct {
	RequestedNamespaces int            `json:"requestedNamespaces"`
	CompletedNamespaces int            `json:"completedNamespaces"`
	DeniedNamespaces    []string       `json:"deniedNamespaces"`
	Failed              []PartialError `json:"failed"`
}

// DashboardBlockDTO is the closed payload shared by independent dashboard
// blocks. Errors is always non-nil; Coverage is nil only for non-fan-out work.
type DashboardBlockDTO[T any] struct {
	Value     T              `json:"value"`
	Complete  bool           `json:"complete"`
	Truncated bool           `json:"truncated"`
	Coverage  *CoverageDTO   `json:"coverage"`
	Errors    []PartialError `json:"errors"`
}

// Selection identifies the immutable generation against which a service
// request is evaluated. It is intentionally separate from SummaryDTO, whose
// HTTP schema contains exactly the eight documented counters.
type Selection struct {
	Generation string
	Context    string
	Cluster    string
	Scope      string
	Namespaces []string
}

// SelectionDTO is suitable for API metadata or a dashboard header.
type SelectionDTO struct {
	Generation  string   `json:"generation"`
	Context     string   `json:"context"`
	Cluster     string   `json:"cluster"`
	Scope       string   `json:"scope"`
	Namespaces  []string `json:"namespaces"`
	CollectedAt string   `json:"collectedAt"`
}

func (s Selection) DTO(now time.Time) SelectionDTO {
	return SelectionDTO{
		Generation:  s.Generation,
		Context:     s.Context,
		Cluster:     s.Cluster,
		Scope:       s.Scope,
		Namespaces:  append([]string(nil), canonicalNamespaces(s.Namespaces)...),
		CollectedAt: now.UTC().Format(time.RFC3339Nano),
	}
}

type CounterState string

const (
	CounterAvailable    CounterState = "available"
	CounterDenied       CounterState = "denied"
	CounterUnavailable  CounterState = "unavailable"
	CounterNotCollected CounterState = "notCollected"
	CounterCollecting   CounterState = "collecting"
	CounterTruncated    CounterState = "truncated"
)

// CounterDTO uses a pointer so JSON distinguishes an authoritative zero from
// a value which was not collected.
type CounterDTO struct {
	State CounterState `json:"state"`
	Value *int64       `json:"value"`
}

func AvailableCounter(value int64) CounterDTO {
	if value < 0 {
		value = 0
	}
	return CounterDTO{State: CounterAvailable, Value: int64Pointer(value)}
}

func TruncatedCounter(value int64) CounterDTO {
	if value < 0 {
		value = 0
	}
	return CounterDTO{State: CounterTruncated, Value: int64Pointer(value)}
}

func EmptyCounter(state CounterState) CounterDTO {
	switch state {
	case CounterDenied, CounterUnavailable, CounterNotCollected, CounterCollecting:
		return CounterDTO{State: state}
	default:
		return CounterDTO{State: CounterUnavailable}
	}
}

type SummaryDTO struct {
	Namespaces         CounterDTO `json:"namespaces"`
	PodsTotal          CounterDTO `json:"podsTotal"`
	PodsHealthy        CounterDTO `json:"podsHealthy"`
	PodsProblematic    CounterDTO `json:"podsProblematic"`
	WorkloadsDegraded  CounterDTO `json:"workloadsDegraded"`
	Restarts           CounterDTO `json:"restarts"`
	WarningEvents      CounterDTO `json:"warningEvents"`
	PossibleLogMatches CounterDTO `json:"possibleLogMatches"`
}

type NamespaceHealthDTO struct {
	Namespace         string `json:"namespace"`
	ProblematicPods   int64  `json:"problematicPods"`
	ContainerRestarts int64  `json:"containerRestarts"`
	DegradedWorkloads int64  `json:"degradedWorkloads"`
}

type ContainerType string

const (
	ContainerRegular   ContainerType = "regular"
	ContainerInit      ContainerType = "init"
	ContainerEphemeral ContainerType = "ephemeral"
)

type RestartSeverity string

const (
	RestartHealthy   RestartSeverity = "healthy"
	RestartAttention RestartSeverity = "attention"
	RestartWarning   RestartSeverity = "warning"
	RestartCritical  RestartSeverity = "critical"
)

type RestartDTO struct {
	Namespace     string          `json:"namespace"`
	Pod           string          `json:"pod"`
	Owner         *ResourceRef    `json:"owner"`
	Container     string          `json:"container"`
	ContainerType ContainerType   `json:"containerType"`
	Restarts      int64           `json:"restarts"`
	Severity      RestartSeverity `json:"severity"`
	Status        string          `json:"status"`
	LastReason    string          `json:"lastReason"`
	AgeSeconds    int64           `json:"ageSeconds"`
}

type ProblemSource string

const (
	ProblemPodStatus           ProblemSource = "podStatus"
	ProblemContainerWaiting    ProblemSource = "containerWaiting"
	ProblemContainerTerminated ProblemSource = "containerTerminated"
	ProblemContainerStatus     ProblemSource = "containerStatus"
	ProblemCondition           ProblemSource = "condition"
	ProblemEvent               ProblemSource = "event"
)

type ProblemSeverity string

const (
	ProblemWarning  ProblemSeverity = "warning"
	ProblemCritical ProblemSeverity = "critical"
)

type ProblemPodDTO struct {
	Namespace     string          `json:"namespace"`
	Pod           string          `json:"pod"`
	Owner         *ResourceRef    `json:"owner"`
	Container     *string         `json:"container"`
	ContainerType *ContainerType  `json:"containerType"`
	Status        string          `json:"status"`
	Reason        *string         `json:"reason"`
	Message       *string         `json:"message"`
	Source        ProblemSource   `json:"source"`
	Severity      ProblemSeverity `json:"severity"`
	AgeSeconds    int64           `json:"ageSeconds"`
}

type WorkloadStatus string

const (
	WorkloadHealthy     WorkloadStatus = "Healthy"
	WorkloadProgressing WorkloadStatus = "Progressing"
	WorkloadDegraded    WorkloadStatus = "Degraded"
	WorkloadSuspended   WorkloadStatus = "Suspended"
	WorkloadCompleted   WorkloadStatus = "Completed"
	WorkloadFailed      WorkloadStatus = "Failed"
	WorkloadUnknown     WorkloadStatus = "Unknown"
)

type WorkloadDTO struct {
	Namespace  string         `json:"namespace"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Ready      *int64         `json:"ready"`
	Desired    *int64         `json:"desired"`
	Available  *int64         `json:"available"`
	Updated    *int64         `json:"updated"`
	Status     WorkloadStatus `json:"status"`
	AgeSeconds int64          `json:"ageSeconds"`
}

type EventDTO struct {
	Timestamp      *string `json:"timestamp"`
	Namespace      string  `json:"namespace"`
	ObjectKind     string  `json:"objectKind"`
	ObjectName     string  `json:"objectName"`
	Reason         string  `json:"reason"`
	Message        string  `json:"message"`
	Count          int64   `json:"count"`
	Source         *string `json:"source"`
	Type           string  `json:"type"`
	cursorIdentity string
}

type LogReasonCode string

const (
	LogErrorKeyword   LogReasonCode = "ERROR_KEYWORD"
	LogJSONErrorLevel LogReasonCode = "JSON_ERROR_LEVEL"
	LogJSONErrorField LogReasonCode = "JSON_ERROR_FIELD"
	LogStackTrace     LogReasonCode = "STACK_TRACE"
	LogOOM            LogReasonCode = "OOM"
	LogPanic          LogReasonCode = "PANIC"
)

type LogMatchDTO struct {
	Namespace  string        `json:"namespace"`
	Pod        string        `json:"pod"`
	Container  string        `json:"container"`
	Workload   *ResourceRef  `json:"workload"`
	Timestamp  *string       `json:"timestamp"`
	Excerpt    string        `json:"excerpt"`
	ReasonCode LogReasonCode `json:"reasonCode"`
	Redacted   bool          `json:"redacted"`
	Truncated  bool          `json:"truncated"`
}

type ContainerMetricDTO struct {
	Name          string `json:"name"`
	CPUMillicores int64  `json:"cpuMillicores"`
	MemoryBytes   int64  `json:"memoryBytes"`
}

type PodMetricDTO struct {
	Namespace     string               `json:"namespace"`
	Pod           string               `json:"pod"`
	CPUMillicores int64                `json:"cpuMillicores"`
	MemoryBytes   int64                `json:"memoryBytes"`
	Containers    []ContainerMetricDTO `json:"containers"`
}

type MetricRankDTO struct {
	Namespace     string `json:"namespace"`
	Pod           string `json:"pod"`
	CPUMillicores int64  `json:"cpuMillicores"`
	MemoryBytes   int64  `json:"memoryBytes"`
}

type MetricsDTO struct {
	CollectedAt   string          `json:"collectedAt"`
	WindowSeconds int64           `json:"windowSeconds"`
	Pods          []PodMetricDTO  `json:"pods"`
	TopCPU        []MetricRankDTO `json:"topCPU"`
	TopMemory     []MetricRankDTO `json:"topMemory"`
}

func int64Pointer(value int64) *int64 { return &value }
