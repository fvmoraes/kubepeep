// Package namespaces implements kubePeep's namespace-input grammar, scope
// invariants, selection coordination, and repository ports.
package namespaces

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
)

type ScopeMode string

const (
	ScopeModeSingle ScopeMode = "single"
	ScopeModeList   ScopeMode = "list"
	ScopeModeAll    ScopeMode = "all"
)

const (
	MaxNamespaceEntries = 10_000
	MaxScopeBodyBytes   = 1 << 20
	MaxScopeNameRunes   = 120
	MaxContextBytes     = 1024
)

type Scope struct {
	ID               int64
	ClusterProfileID int64
	Context          string
	Name             string
	Mode             ScopeMode
	Namespaces       []string
	DefaultNamespace *string
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ScopeDraft struct {
	ClusterProfileID int64
	Context          string
	Name             string
	Mode             ScopeMode
	Namespaces       []string
	DefaultNamespace *string
}

type Repository interface {
	List(context.Context, int64, string) ([]Scope, error)
	Get(context.Context, int64) (Scope, error)
	Create(context.Context, ScopeDraft) (Scope, error)
	Update(context.Context, int64, int64, ScopeDraft) (Scope, error)
	Delete(context.Context, int64, int64) error
}

// SelectionBinding is a stable snapshot held by the coordinator for the full
// duration of one mutating callback.
type SelectionBinding struct {
	ClusterProfileID int64
	Context          string
	Cluster          string
	ActiveScopeID    int64
	Generation       string
}

// ScopeResolution is transient selection state. In particular, an all scope
// stores no items; its Namespaces are exactly the collection returned by the
// NamespaceCatalog for this operation.
type ScopeResolution struct {
	ScopeID          int64
	ScopeName        string
	ScopeMode        ScopeMode
	ScopeSource      string
	DefaultNamespace *string
	Namespaces       []string
	PreferGlobal     bool
}

type SelectionMutation struct {
	PublishGeneration bool
	Activation        *ScopeResolution
}

type SelectionResult struct {
	Generation string
	Binding    SelectionBinding
	Resolution ScopeResolution
	Changed    bool
}

// SelectionCommit contains local-only work. Implementations invoke it only
// after rechecking the intent epoch, expected generation, and active binding.
// Kubernetes or other remote I/O belongs in SelectionPreparation instead.
type SelectionCommit func(context.Context, SelectionBinding) (SelectionMutation, error)

// SelectionPreparation runs with the cancelable context of the current user
// intent. It may perform remote validation and returns the local commit that
// can safely run in the coordinator's short critical section.
type SelectionPreparation func(context.Context, SelectionBinding) (SelectionCommit, error)

// SelectionCoordinator registers an intent before preparation, then compares
// its epoch and expectedGeneration again before running the local commit. The
// binding passed to the commit is freshly read and remains stable until the
// commit and any required publication have completed.
type SelectionCoordinator interface {
	Mutate(context.Context, string, SelectionPreparation) (SelectionResult, error)
}

// NamespaceCatalog performs the authorization-aware Kubernetes namespace
// list. Explicit denial and operational unavailability remain distinct.
type NamespaceCatalog interface {
	List(context.Context, SelectionBinding) ([]string, error)
}

// NamespaceRecord is the credential-free metadata exposed by the Kubernetes
// namespace list. UID is retained only as a deterministic page tie-breaker.
type NamespaceRecord struct {
	Name  string
	UID   string
	Phase string
}

// NamespacePageRequest carries the native Kubernetes page boundary. Continue
// is never exposed directly; HTTP adapters wrap it in the process-local signed
// cursor before returning it to a client.
type NamespacePageRequest struct {
	Limit    int64
	Continue string
}

type NamespacePage struct {
	Items    []NamespaceRecord
	Continue string
}

// NamespacePageCatalog is implemented by adapters that can preserve native
// Kubernetes pagination and namespace status metadata.
type NamespacePageCatalog interface {
	ListPage(context.Context, SelectionBinding, NamespacePageRequest) (NamespacePage, error)
}

func ValidateDraft(draft ScopeDraft) error {
	if draft.ClusterProfileID <= 0 {
		return fieldError("clusterProfileId", "must identify an existing profile")
	}
	if strings.TrimSpace(draft.Context) != draft.Context || len(draft.Context) == 0 || len(draft.Context) > MaxContextBytes || !utf8.ValidString(draft.Context) {
		return fieldError("context", "must contain between 1 and 1024 UTF-8 bytes")
	}
	if !utf8.ValidString(draft.Name) || strings.TrimSpace(draft.Name) != draft.Name || utf8.RuneCountInString(draft.Name) < 1 || utf8.RuneCountInString(draft.Name) > MaxScopeNameRunes {
		return fieldError("name", "must contain between 1 and 120 trimmed characters")
	}
	if draft.DefaultNamespace != nil && strings.TrimSpace(*draft.DefaultNamespace) != *draft.DefaultNamespace {
		return fieldError("defaultNamespace", "must be a trimmed Kubernetes namespace name")
	}
	seen := make(map[string]struct{}, len(draft.Namespaces))
	for _, namespace := range draft.Namespaces {
		if !ValidNamespaceName(namespace) || namespace == "*" {
			return fieldError("namespaces", "contains an invalid Kubernetes namespace name")
		}
		if _, exists := seen[namespace]; exists {
			return fieldError("namespaces", "contains a duplicate namespace")
		}
		seen[namespace] = struct{}{}
	}
	switch draft.Mode {
	case ScopeModeSingle:
		if len(draft.Namespaces) != 1 {
			return fieldError("namespaces", "single mode requires exactly one namespace")
		}
		if draft.DefaultNamespace != nil && *draft.DefaultNamespace != draft.Namespaces[0] {
			return fieldError("defaultNamespace", "must equal the single namespace")
		}
	case ScopeModeList:
		if len(draft.Namespaces) == 0 {
			return fieldError("namespaces", "list mode requires at least one namespace")
		}
		if draft.DefaultNamespace != nil && !slices.Contains(draft.Namespaces, *draft.DefaultNamespace) {
			return fieldError("defaultNamespace", "must belong to the namespace list")
		}
	case ScopeModeAll:
		if len(draft.Namespaces) != 0 {
			return fieldError("namespaces", "all mode stores no namespace items")
		}
		if draft.DefaultNamespace != nil {
			return fieldError("defaultNamespace", "must be null in all mode")
		}
	default:
		return fieldError("mode", "must be single, list, or all")
	}
	return nil
}

// ValidNamespaceName applies the same DNS-1123 label validation used by the
// Kubernetes namespace strategy. The wildcard is not a Kubernetes name and is
// additionally rejected at every scope boundary.
func ValidNamespaceName(name string) bool {
	return name != "*" && len(k8svalidation.IsDNS1123Label(name)) == 0
}
