package api

import (
	"context"
	"fmt"
	"time"
)

// ComponentStatus is the public, allowlisted state of one operational
// component. It deliberately does not contain the underlying checker error.
type ComponentStatus string

const (
	StatusHealthy   ComponentStatus = "healthy"
	StatusDegraded  ComponentStatus = "degraded"
	StatusUnhealthy ComponentStatus = "unhealthy"
	StatusUnknown   ComponentStatus = "unknown"
)

// Component identifies the fixed operational components exposed by the API.
type Component string

const (
	ComponentApplication Component = "application"
	ComponentSQLite      Component = "sqlite"
	ComponentKubeconfig  Component = "kubeconfig"
	ComponentContext     Component = "context"
	ComponentCluster     Component = "cluster"
	ComponentMetrics     Component = "metrics"
)

// ComponentState is the canonical public state. All four fields are always
// serialized; CheckedAt remains nil until a check has actually run.
type ComponentState struct {
	Status    ComponentStatus `json:"status"`
	Code      string          `json:"code"`
	Message   string          `json:"message"`
	CheckedAt *time.Time      `json:"checkedAt"`
}

// StatusComponents is intentionally a struct rather than a map so the six
// required keys cannot silently disappear from a response.
type StatusComponents struct {
	Application ComponentState `json:"application"`
	SQLite      ComponentState `json:"sqlite"`
	Kubeconfig  ComponentState `json:"kubeconfig"`
	Context     ComponentState `json:"context"`
	Cluster     ComponentState `json:"cluster"`
	Metrics     ComponentState `json:"metrics"`
}

// HealthComponents omits Metrics by contract while preserving all other fixed
// component keys.
type HealthComponents struct {
	Application ComponentState `json:"application"`
	SQLite      ComponentState `json:"sqlite"`
	Kubeconfig  ComponentState `json:"kubeconfig"`
	Context     ComponentState `json:"context"`
	Cluster     ComponentState `json:"cluster"`
}

// SelectionSummary is the sanitized active selection included in status.
// Nullable scope fields distinguish "selected context without a scope" from
// an absent selection.
type SelectionSummary struct {
	ClusterProfileID int64   `json:"clusterProfileId"`
	Context          string  `json:"context"`
	Cluster          string  `json:"cluster"`
	ScopeID          *int64  `json:"scopeId"`
	ScopeName        *string `json:"scopeName"`
	ScopeMode        *string `json:"scopeMode"`
	ScopeSource      string  `json:"scopeSource"`
	DefaultNamespace *string `json:"defaultNamespace"`
	NamespaceCount   int     `json:"namespaceCount"`
	Generation       string  `json:"generation"`
}

// Snapshot is the single source shared by health and status handlers.
type Snapshot struct {
	Components StatusComponents
	Selection  *SelectionSummary
}

// SnapshotProvider is implemented by the operational service composed by the
// lifecycle. Implementations must return only sanitized ComponentState values.
type SnapshotProvider interface {
	Snapshot(context.Context) (Snapshot, error)
}

type HealthData struct {
	Status     ComponentStatus  `json:"status"`
	Components HealthComponents `json:"components"`
}

type StatusData struct {
	Version    string            `json:"version"`
	Commit     string            `json:"commit"`
	BuildDate  string            `json:"buildDate"`
	Port       int               `json:"port"`
	Components StatusComponents  `json:"components"`
	Selection  *SelectionSummary `json:"selection"`
}

type SessionData struct {
	CSRFToken  string    `json:"csrfToken"`
	Origin     string    `json:"origin"`
	Generation string    `json:"generation"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

// HealthDataFromSnapshot applies the degraded-state policy from ADR 0002.
// Unknown external dependencies do not make the local process unavailable.
func HealthDataFromSnapshot(snapshot Snapshot) HealthData {
	components := snapshot.Components
	status := StatusHealthy
	if components.Application.Status != StatusHealthy || components.SQLite.Status != StatusHealthy {
		status = StatusUnhealthy
	} else if isExternalFailure(components.Kubeconfig.Status) ||
		isExternalFailure(components.Context.Status) ||
		isExternalFailure(components.Cluster.Status) {
		status = StatusDegraded
	}

	return HealthData{
		Status: status,
		Components: HealthComponents{
			Application: components.Application,
			SQLite:      components.SQLite,
			Kubeconfig:  components.Kubeconfig,
			Context:     components.Context,
			Cluster:     components.Cluster,
		},
	}
}

// ValidateSnapshot rejects accidental zero-value or non-allowlisted states
// before they reach either public endpoint.
func ValidateSnapshot(snapshot Snapshot) error {
	states := []struct {
		component Component
		state     ComponentState
	}{
		{ComponentApplication, snapshot.Components.Application},
		{ComponentSQLite, snapshot.Components.SQLite},
		{ComponentKubeconfig, snapshot.Components.Kubeconfig},
		{ComponentContext, snapshot.Components.Context},
		{ComponentCluster, snapshot.Components.Cluster},
		{ComponentMetrics, snapshot.Components.Metrics},
	}
	for _, entry := range states {
		if err := validateState(entry.state); err != nil {
			return fmt.Errorf("api: invalid %s state: %w", entry.component, err)
		}
	}
	if selection := snapshot.Selection; selection != nil {
		if selection.ClusterProfileID <= 0 || selection.Context == "" || selection.Cluster == "" || selection.NamespaceCount < 0 {
			return fmt.Errorf("api: active selection is incomplete")
		}
	}
	return nil
}

func isExternalFailure(status ComponentStatus) bool {
	return status == StatusDegraded || status == StatusUnhealthy
}

func (s *StatusComponents) set(component Component, state ComponentState) {
	switch component {
	case ComponentApplication:
		s.Application = state
	case ComponentSQLite:
		s.SQLite = state
	case ComponentKubeconfig:
		s.Kubeconfig = state
	case ComponentContext:
		s.Context = state
	case ComponentCluster:
		s.Cluster = state
	case ComponentMetrics:
		s.Metrics = state
	}
}
