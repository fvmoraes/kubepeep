package authorization

import (
	"context"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	maxQueryNamespaces    = 20
	maxQueryCapabilities  = 100
	maxQueryResourceNames = 20
	maxExpandedDecisions  = 100
)

// PermissionsRequest is the already-decoded, credential-free input for
// GET /api/v1/permissions. ActiveNamespaces is supplied by the current scope.
type PermissionsRequest struct {
	Generation       string
	ActiveNamespaces []string
	Namespaces       []string
	CapabilityIDs    []string
	ResourceNames    []string
	Refresh          bool
}

// PartialError is an allowlisted matrix error; it never carries a raw
// Kubernetes response or evaluationError.
type PartialError struct {
	Namespace string    `json:"namespace,omitempty"`
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
}

// CapabilityMatrix is the data payload described by docs/api.md.
type CapabilityMatrix struct {
	Generation string         `json:"generation"`
	Decisions  []Capability   `json:"decisions"`
	Complete   bool           `json:"complete"`
	Truncated  bool           `json:"truncated"`
	Errors     []PartialError `json:"errors"`
}

// ExpandedCapability is one validated allowlist entry ready for SSAR.
type ExpandedCapability struct {
	CapabilityID string
	Key          Key
}

// ExpandPermissions validates allowlisted IDs and builds at most 100 complete
// SSAR keys. It never accepts raw group/resource/verb attributes from the UI.
func ExpandPermissions(request PermissionsRequest) ([]ExpandedCapability, bool, error) {
	if !safeOpaque(request.Generation, maxGenerationLength) ||
		len(request.Namespaces) > maxQueryNamespaces ||
		len(request.CapabilityIDs) > maxQueryCapabilities ||
		len(request.ResourceNames) > maxQueryResourceNames {
		return nil, false, validationError()
	}
	if err := validateUniqueNamespaces(request.ActiveNamespaces, nil); err != nil {
		return nil, false, err
	}
	active := make(map[string]struct{}, len(request.ActiveNamespaces))
	for _, namespace := range request.ActiveNamespaces {
		active[namespace] = struct{}{}
	}

	namespaces := request.Namespaces
	truncated := false
	if len(namespaces) == 0 {
		namespaces = request.ActiveNamespaces
		if len(namespaces) > maxQueryNamespaces {
			namespaces = namespaces[:maxQueryNamespaces]
			truncated = true
		}
	}
	if err := validateUniqueNamespaces(namespaces, active); err != nil {
		return nil, false, err
	}
	if err := validateResourceNames(request.ResourceNames); err != nil {
		return nil, false, err
	}

	specifications, err := selectedSpecifications(request.CapabilityIDs)
	if err != nil {
		return nil, false, err
	}
	hasTarget := false
	for _, specification := range specifications {
		if specification.ResourceNamePolicy == ResourceNameTarget {
			hasTarget = true
			break
		}
	}
	if len(request.ResourceNames) != 0 && !hasTarget {
		return nil, false, validationError()
	}

	expanded := make([]ExpandedCapability, 0, min(maxExpandedDecisions, len(specifications)*max(1, len(namespaces))))
	appendDecision := func(id, namespace, resourceName string) error {
		if len(expanded) == maxExpandedDecisions {
			return validationError()
		}
		key, keyError := KeyForCapability(request.Generation, namespace, id, resourceName)
		if keyError != nil {
			return keyError
		}
		expanded = append(expanded, ExpandedCapability{CapabilityID: id, Key: key})
		return nil
	}

	for _, specification := range specifications {
		if specification.Scope == ScopeCluster {
			if err := appendDecision(specification.ID, "", ""); err != nil {
				return nil, false, err
			}
			continue
		}
		if len(namespaces) == 0 {
			return nil, false, validationError()
		}
		for _, namespace := range namespaces {
			if specification.ResourceNamePolicy == ResourceNameTarget && len(request.ResourceNames) != 0 {
				for _, resourceName := range request.ResourceNames {
					if err := appendDecision(specification.ID, namespace, resourceName); err != nil {
						return nil, false, err
					}
				}
				continue
			}
			if err := appendDecision(specification.ID, namespace, ""); err != nil {
				return nil, false, err
			}
		}
	}
	if len(expanded) == 0 {
		return nil, false, validationError()
	}
	return expanded, truncated, nil
}

func selectedSpecifications(ids []string) ([]CapabilitySpec, error) {
	if len(ids) == 0 {
		return Allowlist(), nil
	}
	seen := make(map[string]struct{}, len(ids))
	result := make([]CapabilitySpec, 0, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return nil, validationError()
		}
		specification, ok := LookupCapability(id)
		if !ok {
			return nil, validationError()
		}
		seen[id] = struct{}{}
		result = append(result, specification)
	}
	return result, nil
}

func validateUniqueNamespaces(namespaces []string, active map[string]struct{}) error {
	seen := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		if len(validation.IsDNS1123Label(namespace)) != 0 {
			return validationError()
		}
		if _, duplicate := seen[namespace]; duplicate {
			return validationError()
		}
		if active != nil {
			if _, selected := active[namespace]; !selected {
				return validationError()
			}
		}
		seen[namespace] = struct{}{}
	}
	return nil
}

func validateResourceNames(resourceNames []string) error {
	seen := make(map[string]struct{}, len(resourceNames))
	for _, resourceName := range resourceNames {
		if len(validation.IsDNS1123Subdomain(resourceName)) != 0 {
			return validationError()
		}
		if _, duplicate := seen[resourceName]; duplicate {
			return validationError()
		}
		seen[resourceName] = struct{}{}
	}
	return nil
}

// Matrix evaluates the expanded allowlist. Partial unknown decisions remain
// HTTP-200-compatible; when every review is unknown it also returns
// AUTHORIZATION_UNAVAILABLE so the handler can emit the documented 503.
func (s *Service) Matrix(ctx context.Context, request PermissionsRequest) (CapabilityMatrix, error) {
	expanded, truncated, err := ExpandPermissions(request)
	matrix := CapabilityMatrix{
		Generation: request.Generation,
		Decisions:  []Capability{},
		Errors:     []PartialError{},
		Truncated:  truncated,
	}
	if err != nil {
		return matrix, err
	}

	known := 0
	for _, item := range expanded {
		var capability Capability
		if request.Refresh {
			capability = s.Refresh(ctx, item.Key)
		} else {
			capability = s.Check(ctx, item.Key)
		}
		capability.CapabilityID = item.CapabilityID
		matrix.Decisions = append(matrix.Decisions, capability)
		if capability.Decision == DecisionUnknown {
			matrix.Errors = append(matrix.Errors, PartialError{
				Namespace: capability.Namespace,
				Code:      CodeAuthorizationUnavailable,
				Message:   publicMessages[CodeAuthorizationUnavailable],
			})
			continue
		}
		known++
	}
	matrix.Complete = len(matrix.Errors) == 0
	if known == 0 {
		return matrix, authorizationUnavailableError(nil)
	}
	return matrix, nil
}
