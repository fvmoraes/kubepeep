// Package contexts coordinates kubeconfig profiles, context selection, client
// activation, generation publication, and the sanitized operational snapshot.
package contexts

import (
	"context"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type ContextDescriptor struct {
	Name    string
	Cluster string
}

type ContextDTO struct {
	ClusterProfileID int64  `json:"clusterProfileId"`
	Name             string `json:"name"`
	Cluster          string `json:"cluster"`
	Selected         bool   `json:"selected"`
}

type ProfileReference struct {
	Paths   []string
	Context string
}

type SourceRequest struct {
	ExplicitPath    *string
	ExplicitContext *string
	Persisted       *ProfileReference
	FirstReconcile  bool
	ProfileOnly     bool
}

// Candidate is an opaque, already parsed kubeconfig resolution. The service
// can inspect only credential-free metadata; adapters retain rest.Config and
// credentials behind Runtime.Activate.
type Candidate interface {
	Paths() []string
	Contexts() []ContextDescriptor
	Selected() (ContextDescriptor, bool)
}

type Runtime interface {
	Resolve(context.Context, SourceRequest) (Candidate, error)
	Activate(context.Context, Candidate, namespaces.SelectionBinding) (api.ComponentState, error)
	OnGeneration(string)
}

type ProfileRepository interface {
	List(context.Context) ([]clusterprofiles.Profile, error)
	Default(context.Context) (clusterprofiles.Profile, error)
	Get(context.Context, int64) (clusterprofiles.Profile, error)
	Reconcile(context.Context, string, []string, bool) (clusterprofiles.Profile, bool, error)
	SetContext(context.Context, int64, *string, bool) (clusterprofiles.Profile, error)
}

type SelectionState interface {
	Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution)
	Initialize(namespaces.SelectionBinding, namespaces.ScopeResolution) error
	ReplaceContextPrepared(context.Context, string, func(context.Context) (namespaces.SelectionBinding, namespaces.ScopeResolution, func(context.Context) error, error)) (namespaces.SelectionResult, error)
	IfCurrent(namespaces.SelectionBinding, func()) bool
}

type SnapshotWriter interface {
	SetState(api.Component, api.ComponentState) error
	SetSelection(*api.SelectionSummary)
}

type BootstrapRequest struct {
	ExplicitPath    *string
	ExplicitContext *string
	EphemeralNS     string
}

type SelectRequest struct {
	ClusterProfileID   int64  `json:"clusterProfileId"`
	Context            string `json:"context"`
	SetDefault         bool   `json:"setDefault"`
	ExpectedGeneration string `json:"expectedGeneration"`
}

type SelectionComponents struct {
	Cluster api.ComponentState `json:"cluster"`
}

type SelectionDTO struct {
	ClusterProfileID int64               `json:"clusterProfileId"`
	Context          string              `json:"context"`
	Cluster          string              `json:"cluster"`
	ScopeID          *int64              `json:"scopeId"`
	ScopeName        *string             `json:"scopeName"`
	ScopeMode        *string             `json:"scopeMode"`
	ScopeSource      string              `json:"scopeSource"`
	DefaultNamespace *string             `json:"defaultNamespace"`
	NamespaceCount   int                 `json:"namespaceCount"`
	Generation       string              `json:"generation"`
	Components       SelectionComponents `json:"components"`
}

// ExternalError contains only allowlisted information suitable for API and
// status surfaces. Adapter causes, paths, plugin output and credentials are
// intentionally not retained.
type ExternalError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *ExternalError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

func healthyState(message string, now time.Time) api.ComponentState {
	checked := now.UTC()
	return api.ComponentState{Status: api.StatusHealthy, Code: "OK", Message: message, CheckedAt: &checked}
}
