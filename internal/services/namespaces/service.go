package namespaces

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

type Service struct {
	repository  Repository
	coordinator SelectionCoordinator
	catalog     NamespaceCatalog
}

func NewService(repository Repository, coordinator SelectionCoordinator, catalog NamespaceCatalog) *Service {
	return &Service{repository: repository, coordinator: coordinator, catalog: catalog}
}

func (service *Service) List(ctx context.Context, profileID int64, contextName string) ([]Scope, error) {
	return service.repository.List(ctx, profileID, contextName)
}

func (service *Service) Get(ctx context.Context, id int64) (Scope, error) {
	if id <= 0 {
		return Scope{}, ErrNotFound
	}
	return service.repository.Get(ctx, id)
}

// Validate parses every entry and optionally verifies existence. Explicit RBAC
// denial leaves a syntactically valid manual list valid, as required by the
// public contract.
func (service *Service) Validate(ctx context.Context, request ScopeWriteRequest, checkExistence bool) (ValidationReport, error) {
	if request.ClusterProfileID <= 0 {
		return ValidationReport{}, fieldError("clusterProfileId", "must identify an existing profile")
	}
	if !utf8.ValidString(request.Context) || strings.TrimSpace(request.Context) != request.Context || len(request.Context) == 0 || len(request.Context) > MaxContextBytes {
		return ValidationReport{}, fieldError("context", "must contain between 1 and 1024 trimmed UTF-8 bytes")
	}
	if request.Name != "" && (!utf8.ValidString(request.Name) || strings.TrimSpace(request.Name) != request.Name || utf8.RuneCountInString(request.Name) > MaxScopeNameRunes) {
		return ValidationReport{}, fieldError("name", "must contain at most 120 trimmed characters")
	}
	if request.Mode != ScopeModeSingle && request.Mode != ScopeModeList && request.Mode != ScopeModeAll {
		return ValidationReport{}, fieldError("mode", "must be single, list, or all")
	}
	if request.DefaultNamespace != nil && !ValidNamespaceName(*request.DefaultNamespace) {
		return ValidationReport{}, fieldError("defaultNamespace", "must be a Kubernetes namespace name")
	}
	if request.Mode == ScopeModeAll && request.DefaultNamespace != nil {
		return ValidationReport{}, fieldError("defaultNamespace", "must be null in all mode")
	}
	report, err := parseRequestInput(request)
	if err != nil {
		return ValidationReport{}, err
	}
	if err := validateRequestShape(request, report); err != nil {
		return ValidationReport{}, err
	}
	if !checkExistence || service.catalog == nil || len(report.Valid) == 0 {
		return report, nil
	}
	if request.ExpectedGeneration == "" {
		return ValidationReport{}, fieldError("expectedGeneration", "is required when checking namespace existence")
	}
	if service.coordinator == nil {
		return ValidationReport{}, ErrGenerationChanged
	}

	var existing []string
	var catalogErr error
	_, err = service.coordinator.Mutate(ctx, request.ExpectedGeneration, func(intentContext context.Context, binding SelectionBinding) (SelectionCommit, error) {
		if err := validateActiveOrigin(request.ClusterProfileID, request.Context, request.ExpectedGeneration, binding); err != nil {
			return nil, err
		}
		existing, catalogErr = service.catalog.List(intentContext, binding)
		return func(commitContext context.Context, current SelectionBinding) (SelectionMutation, error) {
			if err := commitContext.Err(); err != nil {
				return SelectionMutation{}, err
			}
			if err := validateActiveOrigin(request.ClusterProfileID, request.Context, request.ExpectedGeneration, current); err != nil {
				return SelectionMutation{}, err
			}
			return SelectionMutation{}, nil
		}, nil
	})
	if err != nil {
		return ValidationReport{}, err
	}
	switch {
	case catalogErr == nil:
		return applyExistence(report, existing), nil
	case errors.Is(catalogErr, ErrNamespaceListForbidden):
		report.Existence = ExistenceReport{Checked: false, ReasonCode: "NAMESPACE_LIST_FORBIDDEN"}
		return report, nil
	default:
		report.Existence = ExistenceReport{Checked: false, ReasonCode: "NAMESPACE_LIST_UNAVAILABLE"}
		return report, nil
	}
}

func validateRequestShape(request ScopeWriteRequest, report ValidationReport) error {
	inputCount := report.ValidCount + report.InvalidCount
	switch request.Mode {
	case ScopeModeSingle:
		if inputCount != 1 {
			return fieldError("namespaces", "single mode requires exactly one namespace")
		}
	case ScopeModeList:
		if inputCount == 0 {
			return fieldError("namespaces", "list mode requires at least one namespace")
		}
	case ScopeModeAll:
		if inputCount != 0 {
			return fieldError("namespaces", "all mode stores no namespace items")
		}
	}
	if request.DefaultNamespace != nil && !slices.Contains(report.Valid, *request.DefaultNamespace) {
		return fieldError("defaultNamespace", "must belong to the namespace input")
	}
	return nil
}

func (service *Service) Create(ctx context.Context, request ScopeWriteRequest) (Scope, error) {
	draft, _, err := draftFromRequest(request)
	if err != nil {
		return Scope{}, err
	}
	if request.ExpectedGeneration == "" {
		return Scope{}, fieldError("expectedGeneration", "is required")
	}
	if service.coordinator == nil {
		return Scope{}, ErrGenerationChanged
	}

	var created Scope
	_, err = service.coordinator.Mutate(ctx, request.ExpectedGeneration, func(intentContext context.Context, binding SelectionBinding) (SelectionCommit, error) {
		if err := validateActiveOrigin(draft.ClusterProfileID, draft.Context, request.ExpectedGeneration, binding); err != nil {
			return nil, err
		}
		if draft.Mode == ScopeModeAll {
			if _, err := service.listAll(intentContext, binding); err != nil {
				return nil, err
			}
		}
		return func(commitContext context.Context, current SelectionBinding) (SelectionMutation, error) {
			if err := validateActiveOrigin(draft.ClusterProfileID, draft.Context, request.ExpectedGeneration, current); err != nil {
				return SelectionMutation{}, err
			}
			var err error
			created, err = service.repository.Create(commitContext, draft)
			return SelectionMutation{}, err
		}, nil
	})
	if err != nil {
		return Scope{}, err
	}
	return created, nil
}

func (service *Service) Update(ctx context.Context, id int64, request ScopeWriteRequest) (Scope, SelectionResult, error) {
	if id <= 0 {
		return Scope{}, SelectionResult{}, ErrNotFound
	}
	if request.Version <= 0 {
		return Scope{}, SelectionResult{}, fieldError("version", "must be a positive precondition")
	}
	if request.ExpectedGeneration == "" {
		return Scope{}, SelectionResult{}, fieldError("expectedGeneration", "is required")
	}
	draft, _, err := draftFromRequest(request)
	if err != nil {
		return Scope{}, SelectionResult{}, err
	}
	if service.coordinator == nil {
		return Scope{}, SelectionResult{}, ErrGenerationChanged
	}

	var updated Scope
	result, err := service.coordinator.Mutate(ctx, request.ExpectedGeneration, func(intentContext context.Context, binding SelectionBinding) (SelectionCommit, error) {
		if err := validateActiveOrigin(draft.ClusterProfileID, draft.Context, request.ExpectedGeneration, binding); err != nil {
			return nil, err
		}
		existing, err := service.repository.Get(intentContext, id)
		if err != nil {
			return nil, err
		}
		if !scopeMatchesBinding(existing, binding) || existing.ClusterProfileID != draft.ClusterProfileID || existing.Context != draft.Context {
			return nil, ErrSelectionMismatch
		}

		var allNamespaces []string
		if draft.Mode == ScopeModeAll {
			allNamespaces, err = service.listAll(intentContext, binding)
			if err != nil {
				return nil, err
			}
		}
		return func(commitContext context.Context, current SelectionBinding) (SelectionMutation, error) {
			if err := validateActiveOrigin(draft.ClusterProfileID, draft.Context, request.ExpectedGeneration, current); err != nil {
				return SelectionMutation{}, err
			}
			currentScope, err := service.repository.Get(commitContext, id)
			if err != nil {
				return SelectionMutation{}, err
			}
			if !scopeMatchesBinding(currentScope, current) || currentScope.ClusterProfileID != draft.ClusterProfileID || currentScope.Context != draft.Context {
				return SelectionMutation{}, ErrSelectionMismatch
			}
			updated, err = service.repository.Update(commitContext, id, request.Version, draft)
			if err != nil {
				return SelectionMutation{}, err
			}
			if current.ActiveScopeID != id {
				return SelectionMutation{}, nil
			}
			resolution := resolutionFor(updated, allNamespaces)
			return SelectionMutation{PublishGeneration: true, Activation: &resolution}, nil
		}, nil
	})
	if err != nil {
		return Scope{}, SelectionResult{}, err
	}
	return updated, result, nil
}

func (service *Service) Delete(ctx context.Context, id int64, request ScopeDeleteRequest) (SelectionResult, error) {
	if id <= 0 {
		return SelectionResult{}, ErrNotFound
	}
	if !request.Confirmed {
		return SelectionResult{}, fieldError("confirmed", "must be true")
	}
	if request.Version <= 0 {
		return SelectionResult{}, fieldError("version", "must be a positive precondition")
	}
	if request.ExpectedGeneration == "" {
		return SelectionResult{}, fieldError("expectedGeneration", "is required")
	}
	if service.coordinator == nil {
		return SelectionResult{}, ErrGenerationChanged
	}

	return service.coordinator.Mutate(ctx, request.ExpectedGeneration, func(intentContext context.Context, binding SelectionBinding) (SelectionCommit, error) {
		existing, err := service.repository.Get(intentContext, id)
		if err != nil {
			return nil, err
		}
		if !scopeMatchesBinding(existing, binding) {
			return nil, ErrSelectionMismatch
		}

		var preparedReplacement Scope
		var replacementErr error
		var allNamespaces []string
		if request.ReplacementScopeID > 0 && request.ReplacementScopeID != id {
			preparedReplacement, replacementErr = service.repository.Get(intentContext, request.ReplacementScopeID)
			if replacementErr == nil && !scopeMatchesBinding(preparedReplacement, binding) {
				replacementErr = ErrSelectionMismatch
			}
			if replacementErr == nil && preparedReplacement.Mode == ScopeModeAll {
				allNamespaces, replacementErr = service.listAll(intentContext, binding)
			}
		}

		return func(commitContext context.Context, current SelectionBinding) (SelectionMutation, error) {
			currentScope, err := service.repository.Get(commitContext, id)
			if err != nil {
				return SelectionMutation{}, err
			}
			if !scopeMatchesBinding(currentScope, current) {
				return SelectionMutation{}, ErrSelectionMismatch
			}
			if current.ActiveScopeID != id {
				if err := service.repository.Delete(commitContext, id, request.Version); err != nil {
					return SelectionMutation{}, err
				}
				return SelectionMutation{}, nil
			}
			if request.ReplacementScopeID <= 0 || request.ReplacementScopeID == id {
				return SelectionMutation{}, fieldError("replacementScopeId", "must identify another scope in the active origin")
			}
			if replacementErr != nil {
				return SelectionMutation{}, replacementErr
			}
			replacement, err := service.repository.Get(commitContext, request.ReplacementScopeID)
			if err != nil {
				return SelectionMutation{}, err
			}
			if !scopeMatchesBinding(replacement, current) {
				return SelectionMutation{}, ErrSelectionMismatch
			}
			if replacement.Version != preparedReplacement.Version {
				return SelectionMutation{}, ErrConflict
			}
			if err := service.repository.Delete(commitContext, id, request.Version); err != nil {
				return SelectionMutation{}, err
			}
			resolution := resolutionFor(replacement, allNamespaces)
			return SelectionMutation{PublishGeneration: true, Activation: &resolution}, nil
		}, nil
	})
}

func (service *Service) Select(ctx context.Context, id int64, request ScopeSelectRequest) (ScopeResolution, SelectionResult, error) {
	if id <= 0 {
		return ScopeResolution{}, SelectionResult{}, ErrNotFound
	}
	if request.ExpectedGeneration == "" {
		return ScopeResolution{}, SelectionResult{}, fieldError("expectedGeneration", "is required")
	}
	if service.coordinator == nil {
		return ScopeResolution{}, SelectionResult{}, ErrGenerationChanged
	}

	result, err := service.coordinator.Mutate(ctx, request.ExpectedGeneration, func(intentContext context.Context, binding SelectionBinding) (SelectionCommit, error) {
		preparedScope, err := service.repository.Get(intentContext, id)
		if err != nil {
			return nil, err
		}
		if !scopeMatchesBinding(preparedScope, binding) {
			return nil, ErrSelectionMismatch
		}
		var allNamespaces []string
		if preparedScope.Mode == ScopeModeAll {
			allNamespaces, err = service.listAll(intentContext, binding)
			if err != nil {
				return nil, err
			}
		}
		return func(commitContext context.Context, current SelectionBinding) (SelectionMutation, error) {
			scope, err := service.repository.Get(commitContext, id)
			if err != nil {
				return SelectionMutation{}, err
			}
			if !scopeMatchesBinding(scope, current) {
				return SelectionMutation{}, ErrSelectionMismatch
			}
			if scope.Version != preparedScope.Version {
				return SelectionMutation{}, ErrConflict
			}
			resolution := resolutionFor(scope, allNamespaces)
			return SelectionMutation{PublishGeneration: true, Activation: &resolution}, nil
		}, nil
	})
	if err != nil {
		return ScopeResolution{}, SelectionResult{}, err
	}
	return cloneResolution(result.Resolution), result, nil
}

// ResolveForOperation resolves stored modes without ever inventing namespace
// names. PreferGlobal tells callers to use a cluster-scoped Kubernetes list
// when that concrete operation is authorized, avoiding needless fan-out.
func (service *Service) ResolveForOperation(ctx context.Context, binding SelectionBinding, scope Scope) (ScopeResolution, error) {
	if !scopeMatchesBinding(scope, binding) {
		return ScopeResolution{}, ErrSelectionMismatch
	}
	if scope.Mode != ScopeModeAll {
		return resolutionFor(scope, nil), nil
	}
	namespaces, err := service.listAll(ctx, binding)
	if err != nil {
		return ScopeResolution{}, err
	}
	return resolutionFor(scope, namespaces), nil
}

func draftFromRequest(request ScopeWriteRequest) (ScopeDraft, ValidationReport, error) {
	report, err := parseRequestInput(request)
	if err != nil {
		return ScopeDraft{}, ValidationReport{}, err
	}
	if report.InvalidCount > 0 {
		return ScopeDraft{}, report, &ReportError{Report: report}
	}
	draft := ScopeDraft{
		ClusterProfileID: request.ClusterProfileID,
		Context:          request.Context,
		Name:             request.Name,
		Mode:             request.Mode,
		Namespaces:       append([]string(nil), report.Valid...),
		DefaultNamespace: copyStringPointer(request.DefaultNamespace),
	}
	if err := ValidateDraft(draft); err != nil {
		return ScopeDraft{}, report, err
	}
	return draft, report, nil
}

func parseRequestInput(request ScopeWriteRequest) (ValidationReport, error) {
	if request.NamespacesPresent && request.RawInputPresent {
		return ValidationReport{}, fieldError("namespaces", "cannot be combined with rawInput")
	}
	if request.RawInputPresent {
		return ParseRawInput(request.RawInput)
	}
	if request.NamespacesPresent {
		return ParseNamespaceList(request.Namespaces)
	}
	return ParseNamespaceList(nil)
}

func (service *Service) listAll(ctx context.Context, binding SelectionBinding) ([]string, error) {
	if service.catalog == nil {
		return nil, ErrNamespaceListUnavailable
	}
	listed, err := service.catalog.List(ctx, binding)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(listed))
	for _, namespace := range listed {
		if !ValidNamespaceName(namespace) {
			return nil, ErrNamespaceListUnavailable
		}
		if _, duplicate := seen[namespace]; duplicate {
			return nil, ErrNamespaceListUnavailable
		}
		seen[namespace] = struct{}{}
	}
	return append([]string(nil), listed...), nil
}

func applyExistence(report ValidationReport, existing []string) ValidationReport {
	known := make(map[string]struct{}, len(existing))
	for _, namespace := range existing {
		known[namespace] = struct{}{}
	}
	valid := make([]string, 0, len(report.Valid))
	for _, namespace := range report.Valid {
		if _, found := known[namespace]; found {
			valid = append(valid, namespace)
			continue
		}
		report.Invalid = append(report.Invalid, InvalidNamespace{Input: namespace, Code: NamespaceNotFoundCode})
	}
	report.Valid = valid
	report.ValidCount = len(valid)
	report.InvalidCount = len(report.Invalid)
	report.Existence = ExistenceReport{Checked: true, ReasonCode: "NAMESPACE_LIST_ALLOWED"}
	return report
}

func scopeMatchesBinding(scope Scope, binding SelectionBinding) bool {
	return scope.ClusterProfileID == binding.ClusterProfileID && scope.Context == binding.Context
}

func validateActiveOrigin(profileID int64, contextName, expectedGeneration string, binding SelectionBinding) error {
	if binding.Generation != expectedGeneration {
		return ErrGenerationChanged
	}
	if binding.ClusterProfileID != profileID || binding.Context != contextName {
		return ErrSelectionMismatch
	}
	return nil
}

func cloneResolution(value ScopeResolution) ScopeResolution {
	value.Namespaces = append([]string(nil), value.Namespaces...)
	value.DefaultNamespace = copyStringPointer(value.DefaultNamespace)
	return value
}

func resolutionFor(scope Scope, allNamespaces []string) ScopeResolution {
	namespaces := scope.Namespaces
	preferGlobal := false
	if scope.Mode == ScopeModeAll {
		namespaces = allNamespaces
		preferGlobal = true
	}
	return ScopeResolution{
		ScopeID: scope.ID, ScopeName: scope.Name, ScopeMode: scope.Mode, ScopeSource: "saved",
		DefaultNamespace: copyStringPointer(scope.DefaultNamespace),
		Namespaces:       append([]string(nil), namespaces...), PreferGlobal: preferGlobal,
	}
}
