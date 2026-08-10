package namespaces

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestValidateKeepsManualNamesWhenNamespaceListIsForbidden(t *testing.T) {
	repository := newFakeRepository()
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", Cluster: "https://cluster.example", Generation: "gen_41"}}
	service := NewService(repository, coordinator, catalogFunc(func(_ context.Context, binding SelectionBinding) ([]string, error) {
		if binding.Cluster == "" || binding.Generation != "gen_41" {
			t.Fatalf("catalog received incomplete binding: %#v", binding)
		}
		return nil, ErrNamespaceListForbidden
	}))
	report, err := service.Validate(context.Background(), ScopeWriteRequest{
		ClusterProfileID: 1, Context: "development", Mode: ScopeModeList,
		NamespacesPresent: true, Namespaces: []string{"payments", "billing"}, ExpectedGeneration: "gen_41",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, report.Valid, []string{"payments", "billing"})
	if report.Existence.Checked || report.Existence.ReasonCode != "NAMESPACE_LIST_FORBIDDEN" {
		t.Fatalf("existence = %#v", report.Existence)
	}
}

func TestValidateExistenceMovesOnlyMissingNamesToInvalid(t *testing.T) {
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", Generation: "gen_41"}}
	service := NewService(newFakeRepository(), coordinator, catalogFunc(func(context.Context, SelectionBinding) ([]string, error) {
		return []string{"payments"}, nil
	}))
	report, err := service.Validate(context.Background(), ScopeWriteRequest{
		ClusterProfileID: 1, Context: "development", Mode: ScopeModeList,
		NamespacesPresent: true, Namespaces: []string{"payments", "billing"}, ExpectedGeneration: "gen_41",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, report.Valid, []string{"payments"})
	if !report.Existence.Checked || report.InvalidCount != 1 || report.Invalid[0].Code != NamespaceNotFoundCode {
		t.Fatalf("report = %#v", report)
	}
}

func TestValidateRejectsModeAndDefaultInconsistencies(t *testing.T) {
	service := NewService(newFakeRepository(), &fakeCoordinator{binding: SelectionBinding{
		ClusterProfileID: 1, Context: "development", Generation: "gen_41",
	}}, nil)
	other := "two"
	tests := []ScopeWriteRequest{
		{ClusterProfileID: 1, Context: "development", Mode: ScopeModeSingle, NamespacesPresent: true, Namespaces: []string{"one", "two"}, ExpectedGeneration: "gen_41"},
		{ClusterProfileID: 1, Context: "development", Mode: ScopeModeList, NamespacesPresent: true, Namespaces: []string{}, ExpectedGeneration: "gen_41"},
		{ClusterProfileID: 1, Context: "development", Mode: ScopeModeAll, NamespacesPresent: true, Namespaces: []string{"one"}, ExpectedGeneration: "gen_41"},
		{ClusterProfileID: 1, Context: "development", Mode: ScopeModeList, NamespacesPresent: true, Namespaces: []string{"one"}, DefaultNamespace: &other, ExpectedGeneration: "gen_41"},
	}
	for position, request := range tests {
		if _, err := service.Validate(context.Background(), request, false); !errors.Is(err, ErrValidation) {
			t.Fatalf("case %d: expected validation error, got %v", position, err)
		}
	}
}

func TestCreateAllRequiresAuthorizedListAndPersistsNoItems(t *testing.T) {
	repository := newFakeRepository()
	listed := 0
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", Cluster: "https://cluster.example", Generation: "gen_41"}}
	service := NewService(repository, coordinator, catalogFunc(func(_ context.Context, binding SelectionBinding) ([]string, error) {
		listed++
		if binding.Cluster == "" || binding.Generation != "gen_41" {
			t.Fatalf("catalog received incomplete binding: %#v", binding)
		}
		return []string{"kube-system", "payments"}, nil
	}))
	created, err := service.Create(context.Background(), ScopeWriteRequest{
		ClusterProfileID: 1, Context: "development", Name: "Everything", Mode: ScopeModeAll,
		ExpectedGeneration: "gen_41",
	})
	if err != nil {
		t.Fatal(err)
	}
	if listed != 1 || created.Mode != ScopeModeAll || len(repository.lastDraft.Namespaces) != 0 {
		t.Fatalf("listed=%d created=%#v draft=%#v", listed, created, repository.lastDraft)
	}
	for _, item := range repository.lastDraft.Namespaces {
		if item == "*" {
			t.Fatal("wildcard was persisted")
		}
	}
}

func TestCreateAllFailsClosedWhenNamespaceListIsDenied(t *testing.T) {
	repository := newFakeRepository()
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", Generation: "gen_41"}}
	service := NewService(repository, coordinator, catalogFunc(func(context.Context, SelectionBinding) ([]string, error) {
		return nil, ErrNamespaceListForbidden
	}))
	_, err := service.Create(context.Background(), ScopeWriteRequest{
		ClusterProfileID: 1, Context: "development", Name: "Everything", Mode: ScopeModeAll,
		ExpectedGeneration: "gen_41",
	})
	if !errors.Is(err, ErrNamespaceListForbidden) || repository.createCalls != 0 {
		t.Fatalf("error=%v createCalls=%d", err, repository.createCalls)
	}
}

func TestSelectAllUsesExactlyTheCatalogOrderAndPrefersGlobal(t *testing.T) {
	repository := newFakeRepository()
	repository.scopes[7] = Scope{ID: 7, ClusterProfileID: 1, Context: "development", Name: "Everything", Mode: ScopeModeAll, Version: 1}
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", ActiveScopeID: 3, Generation: "gen_41"}}
	service := NewService(repository, coordinator, catalogFunc(func(context.Context, SelectionBinding) ([]string, error) {
		return []string{"zeta", "alpha", "payments"}, nil
	}))

	resolution, result, err := service.Select(context.Background(), 7, ScopeSelectRequest{ExpectedGeneration: "gen_41"})
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, resolution.Namespaces, []string{"zeta", "alpha", "payments"})
	if !resolution.PreferGlobal || result.Generation != "gen_42" || coordinator.lastMutation.Activation.ScopeID != 7 {
		t.Fatalf("resolution=%#v result=%#v mutation=%#v", resolution, result, coordinator.lastMutation)
	}
}

func TestSelectRejectsScopeFromAnotherProfileContext(t *testing.T) {
	repository := newFakeRepository()
	repository.scopes[7] = Scope{ID: 7, ClusterProfileID: 2, Context: "development", Name: "Other", Mode: ScopeModeSingle, Namespaces: []string{"payments"}, Version: 1}
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", Generation: "gen_41"}}
	service := NewService(repository, coordinator, nil)
	_, _, err := service.Select(context.Background(), 7, ScopeSelectRequest{ExpectedGeneration: "gen_41"})
	if !errors.Is(err, ErrSelectionMismatch) || coordinator.lastMutation.PublishGeneration {
		t.Fatalf("error=%v mutation=%#v", err, coordinator.lastMutation)
	}
}

func TestUpdateActiveScopeChecksVersionAndPublishesGeneration(t *testing.T) {
	repository := newFakeRepository()
	repository.scopes[7] = Scope{ID: 7, ClusterProfileID: 1, Context: "development", Name: "Finance", Mode: ScopeModeSingle, Namespaces: []string{"payments"}, Version: 3, CreatedAt: time.Now()}
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", ActiveScopeID: 7, Generation: "gen_41"}}
	service := NewService(repository, coordinator, nil)
	updated, result, err := service.Update(context.Background(), 7, ScopeWriteRequest{
		ClusterProfileID: 1, Context: "development", Name: "Finance apps", Mode: ScopeModeList,
		NamespacesPresent: true, Namespaces: []string{"payments", "billing"},
		Version: 3, ExpectedGeneration: "gen_41",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 4 || result.Generation != "gen_42" || !coordinator.lastMutation.PublishGeneration {
		t.Fatalf("updated=%#v result=%#v mutation=%#v", updated, result, coordinator.lastMutation)
	}
	assertStrings(t, coordinator.lastMutation.Activation.Namespaces, []string{"payments", "billing"})
}

func TestUpdateRechecksScopeThatBecomesActiveBeforeLocalCommit(t *testing.T) {
	repository := newFakeRepository()
	repository.scopes[7] = Scope{ID: 7, ClusterProfileID: 1, Context: "development", Name: "Finance", Mode: ScopeModeSingle, Namespaces: []string{"payments"}, Version: 3}
	coordinator := &fakeCoordinator{
		binding: SelectionBinding{ClusterProfileID: 1, Context: "development", ActiveScopeID: 3, Generation: "gen_41"},
		beforeCommit: func(coordinator *fakeCoordinator) {
			coordinator.binding.ActiveScopeID = 7
		},
	}
	service := NewService(repository, coordinator, nil)
	updated, result, err := service.Update(context.Background(), 7, ScopeWriteRequest{
		ClusterProfileID: 1, Context: "development", Name: "Finance apps", Mode: ScopeModeList,
		NamespacesPresent: true, Namespaces: []string{"payments", "billing"},
		Version: 3, ExpectedGeneration: "gen_41",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 4 || !result.Changed || result.Binding.ActiveScopeID != 7 || result.Resolution.ScopeID != 7 {
		t.Fatalf("updated=%#v result=%#v", updated, result)
	}
}

func TestClusterConsultsRequireCurrentGenerationAndOrigin(t *testing.T) {
	repository := newFakeRepository()
	catalogCalls := 0
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", Cluster: "cluster-a", Generation: "gen_41"}}
	service := NewService(repository, coordinator, catalogFunc(func(context.Context, SelectionBinding) ([]string, error) {
		catalogCalls++
		return []string{"payments"}, nil
	}))

	_, err := service.Validate(context.Background(), ScopeWriteRequest{
		ClusterProfileID: 2, Context: "development", Mode: ScopeModeList,
		NamespacesPresent: true, Namespaces: []string{"payments"}, ExpectedGeneration: "gen_41",
	}, true)
	if !errors.Is(err, ErrSelectionMismatch) || catalogCalls != 0 {
		t.Fatalf("mismatched validate error=%v catalogCalls=%d", err, catalogCalls)
	}
	_, err = service.Create(context.Background(), ScopeWriteRequest{
		ClusterProfileID: 1, Context: "development", Name: "Everything", Mode: ScopeModeAll,
	})
	if err == nil || catalogCalls != 0 || repository.createCalls != 0 {
		t.Fatalf("unfenced create error=%v catalogCalls=%d createCalls=%d", err, catalogCalls, repository.createCalls)
	}
}

func TestGenerationMismatchRunsNoRepositoryMutation(t *testing.T) {
	repository := newFakeRepository()
	repository.scopes[7] = Scope{ID: 7, ClusterProfileID: 1, Context: "development", Name: "Finance", Mode: ScopeModeSingle, Namespaces: []string{"payments"}, Version: 1}
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", ActiveScopeID: 7, Generation: "gen_current"}}
	service := NewService(repository, coordinator, nil)
	_, _, err := service.Update(context.Background(), 7, ScopeWriteRequest{
		ClusterProfileID: 1, Context: "development", Name: "Changed", Mode: ScopeModeSingle,
		NamespacesPresent: true, Namespaces: []string{"payments"}, Version: 1, ExpectedGeneration: "gen_stale",
	})
	if !errors.Is(err, ErrGenerationChanged) || repository.updateCalls != 0 {
		t.Fatalf("error=%v updateCalls=%d", err, repository.updateCalls)
	}
}

func TestDeleteActiveScopeRequiresSameOriginReplacement(t *testing.T) {
	repository := newFakeRepository()
	repository.scopes[7] = Scope{ID: 7, ClusterProfileID: 1, Context: "development", Name: "Old", Mode: ScopeModeSingle, Namespaces: []string{"payments"}, Version: 2}
	repository.scopes[8] = Scope{ID: 8, ClusterProfileID: 2, Context: "development", Name: "Other", Mode: ScopeModeSingle, Namespaces: []string{"billing"}, Version: 1}
	coordinator := &fakeCoordinator{binding: SelectionBinding{ClusterProfileID: 1, Context: "development", ActiveScopeID: 7, Generation: "gen_41"}}
	service := NewService(repository, coordinator, nil)
	_, err := service.Delete(context.Background(), 7, ScopeDeleteRequest{
		Confirmed: true, Version: 2, ReplacementScopeID: 8, ExpectedGeneration: "gen_41",
	})
	if !errors.Is(err, ErrSelectionMismatch) || repository.deleteCalls != 0 {
		t.Fatalf("error=%v deleteCalls=%d", err, repository.deleteCalls)
	}
}

type catalogFunc func(context.Context, SelectionBinding) ([]string, error)

func (function catalogFunc) List(ctx context.Context, binding SelectionBinding) ([]string, error) {
	return function(ctx, binding)
}

type fakeCoordinator struct {
	binding      SelectionBinding
	resolution   ScopeResolution
	lastMutation SelectionMutation
	beforeCommit func(*fakeCoordinator)
}

func (coordinator *fakeCoordinator) Mutate(ctx context.Context, expected string, prepare SelectionPreparation) (SelectionResult, error) {
	if expected != coordinator.binding.Generation {
		return SelectionResult{}, ErrGenerationChanged
	}
	commit, err := prepare(ctx, coordinator.binding)
	if err != nil {
		return SelectionResult{}, err
	}
	if coordinator.beforeCommit != nil {
		coordinator.beforeCommit(coordinator)
	}
	if expected != coordinator.binding.Generation {
		return SelectionResult{}, ErrGenerationChanged
	}
	mutation, err := commit(ctx, coordinator.binding)
	if err != nil {
		return SelectionResult{}, err
	}
	coordinator.lastMutation = mutation
	if mutation.PublishGeneration {
		coordinator.binding.Generation = "gen_42"
		if mutation.Activation != nil {
			coordinator.binding.ActiveScopeID = mutation.Activation.ScopeID
			coordinator.resolution = cloneResolution(*mutation.Activation)
		}
	}
	return SelectionResult{
		Generation: coordinator.binding.Generation,
		Binding:    coordinator.binding,
		Resolution: cloneResolution(coordinator.resolution),
		Changed:    mutation.PublishGeneration,
	}, nil
}

type fakeRepository struct {
	mutex       sync.Mutex
	scopes      map[int64]Scope
	nextID      int64
	lastDraft   ScopeDraft
	createCalls int
	updateCalls int
	deleteCalls int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{scopes: make(map[int64]Scope), nextID: 1}
}

func (repository *fakeRepository) List(_ context.Context, profileID int64, contextName string) ([]Scope, error) {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()
	var result []Scope
	for _, scope := range repository.scopes {
		if scope.ClusterProfileID == profileID && scope.Context == contextName {
			result = append(result, scope)
		}
	}
	return result, nil
}

func (repository *fakeRepository) Get(_ context.Context, id int64) (Scope, error) {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()
	scope, exists := repository.scopes[id]
	if !exists {
		return Scope{}, ErrNotFound
	}
	return scope, nil
}

func (repository *fakeRepository) Create(_ context.Context, draft ScopeDraft) (Scope, error) {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()
	repository.createCalls++
	repository.lastDraft = draft
	id := repository.nextID
	repository.nextID++
	scope := Scope{ID: id, ClusterProfileID: draft.ClusterProfileID, Context: draft.Context, Name: draft.Name, Mode: draft.Mode, Namespaces: append([]string(nil), draft.Namespaces...), DefaultNamespace: copyStringPointer(draft.DefaultNamespace), Version: 1}
	repository.scopes[id] = scope
	return scope, nil
}

func (repository *fakeRepository) Update(_ context.Context, id, expectedVersion int64, draft ScopeDraft) (Scope, error) {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()
	repository.updateCalls++
	existing, exists := repository.scopes[id]
	if !exists {
		return Scope{}, ErrNotFound
	}
	if existing.Version != expectedVersion {
		return Scope{}, ErrConflict
	}
	existing.Name = draft.Name
	existing.Mode = draft.Mode
	existing.Namespaces = append([]string(nil), draft.Namespaces...)
	existing.DefaultNamespace = copyStringPointer(draft.DefaultNamespace)
	existing.Version++
	repository.scopes[id] = existing
	return existing, nil
}

func (repository *fakeRepository) Delete(_ context.Context, id, expectedVersion int64) error {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()
	repository.deleteCalls++
	existing, exists := repository.scopes[id]
	if !exists {
		return ErrNotFound
	}
	if existing.Version != expectedVersion {
		return ErrConflict
	}
	delete(repository.scopes, id)
	return nil
}
