package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func TestNamespaceScopeRepositoryCreatesReadsAndListsStableAggregates(t *testing.T) {
	store := openTestStore(t)
	profileID := namespaceScopeTestProfile(t, store)
	repository := NewNamespaceScopeRepository(store)
	ctx := context.Background()
	defaultPayments := "payments"

	finance, err := repository.Create(ctx, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Finance", Mode: namespaces.ScopeModeList,
		Namespaces: []string{"payments", "billing"}, DefaultNamespace: &defaultPayments,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.Create(ctx, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Alpha", Mode: namespaces.ScopeModeSingle,
		Namespaces: []string{"default"},
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := repository.Get(ctx, finance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 1 || loaded.DefaultNamespace == nil || *loaded.DefaultNamespace != "payments" {
		t.Fatalf("loaded = %#v", loaded)
	}
	assertNamespaceItems(t, loaded.Namespaces, []string{"payments", "billing"})

	listed, err := repository.List(ctx, profileID, "development")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Name != "Alpha" || listed[1].Name != "Finance" {
		t.Fatalf("list order = %#v", listed)
	}
	if !listed[0].CreatedAt.Equal(listed[0].UpdatedAt) || listed[0].CreatedAt.IsZero() {
		t.Fatalf("timestamps = %#v", listed[0])
	}
}

func TestNamespaceScopeRepositoryStoresAllWithoutWildcardItems(t *testing.T) {
	store := openTestStore(t)
	profileID := namespaceScopeTestProfile(t, store)
	repository := NewNamespaceScopeRepository(store)
	created, err := repository.Create(context.Background(), namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Everything", Mode: namespaces.ScopeModeAll,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Namespaces) != 0 {
		t.Fatalf("items = %#v", created.Namespaces)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM namespace_scope_items WHERE namespace_scope_id = ? OR namespace = '*'`, created.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("persisted all items = %d", count)
	}
}

func TestNamespaceScopeRepositoryRejectsNameConflictAndProfileContextMismatchWithoutWrites(t *testing.T) {
	store := openTestStore(t)
	profileID := namespaceScopeTestProfile(t, store)
	repository := NewNamespaceScopeRepository(store)
	ctx := context.Background()
	draft := namespaces.ScopeDraft{ClusterProfileID: profileID, Context: "development", Name: "Finance", Mode: namespaces.ScopeModeSingle, Namespaces: []string{"payments"}}
	if _, err := repository.Create(ctx, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Create(ctx, draft); !errors.Is(err, namespaces.ErrConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
	draft.Name = "Wrong context"
	draft.Context = "production"
	if _, err := repository.Create(ctx, draft); !errors.Is(err, namespaces.ErrSelectionMismatch) {
		t.Fatalf("binding error = %v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM namespace_scopes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("scope count = %d", count)
	}
}

func TestNamespaceScopeRepositoryUpdateUsesExpectedVersionAndCanTransitionToAll(t *testing.T) {
	store := openTestStore(t)
	profileID := namespaceScopeTestProfile(t, store)
	repository := NewNamespaceScopeRepository(store)
	ctx := context.Background()
	created, err := repository.Create(ctx, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Finance", Mode: namespaces.ScopeModeList,
		Namespaces: []string{"payments", "billing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repository.Update(ctx, created.ID, 1, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Everything", Mode: namespaces.ScopeModeAll,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Mode != namespaces.ScopeModeAll || len(updated.Namespaces) != 0 {
		t.Fatalf("updated = %#v", updated)
	}
	_, err = repository.Update(ctx, created.ID, 1, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Stale", Mode: namespaces.ScopeModeSingle,
		Namespaces: []string{"default"},
	})
	if !errors.Is(err, namespaces.ErrConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	loaded, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Everything" || loaded.Version != 2 || len(loaded.Namespaces) != 0 {
		t.Fatalf("loaded after conflict = %#v", loaded)
	}
}

func TestNamespaceScopeRepositoryRollsBackAFailedBatchReplacement(t *testing.T) {
	store := openTestStore(t)
	profileID := namespaceScopeTestProfile(t, store)
	repository := NewNamespaceScopeRepository(store)
	ctx := context.Background()
	created, err := repository.Create(ctx, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Original", Mode: namespaces.ScopeModeList,
		Namespaces: []string{"payments", "billing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER namespace_scope_test_failpoint
		BEFORE INSERT ON namespace_scope_items
		WHEN NEW.namespace = 'failpoint'
		BEGIN SELECT RAISE(ABORT, 'synthetic scope item failure'); END`); err != nil {
		t.Fatal(err)
	}

	_, err = repository.Update(ctx, created.ID, 1, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Changed", Mode: namespaces.ScopeModeList,
		Namespaces: []string{"payments", "failpoint", "invoices"},
	})
	if err == nil {
		t.Fatal("expected injected insert failure")
	}
	loaded, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Original" || loaded.Version != 1 {
		t.Fatalf("header was not rolled back: %#v", loaded)
	}
	assertNamespaceItems(t, loaded.Namespaces, []string{"payments", "billing"})
}

func TestNamespaceScopeRepositoryConcurrentUpdatesHaveOneWinner(t *testing.T) {
	store := openTestStore(t)
	profileID := namespaceScopeTestProfile(t, store)
	repository := NewNamespaceScopeRepository(store)
	ctx := context.Background()
	created, err := repository.Create(ctx, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Original", Mode: namespaces.ScopeModeSingle,
		Namespaces: []string{"payments"},
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, name := range []string{"Winner A", "Winner B"} {
		name := name
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := repository.Update(ctx, created.ID, 1, namespaces.ScopeDraft{
				ClusterProfileID: profileID, Context: "development", Name: name,
				Mode: namespaces.ScopeModeSingle, Namespaces: []string{"payments"},
			})
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, namespaces.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	loaded, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 2 {
		t.Fatalf("version = %d", loaded.Version)
	}
}

func TestNamespaceScopeRepositoryDeleteIsVersionedAndCascades(t *testing.T) {
	store := openTestStore(t)
	profileID := namespaceScopeTestProfile(t, store)
	repository := NewNamespaceScopeRepository(store)
	ctx := context.Background()
	created, err := repository.Create(ctx, namespaces.ScopeDraft{
		ClusterProfileID: profileID, Context: "development", Name: "Finance", Mode: namespaces.ScopeModeList,
		Namespaces: []string{"payments", "billing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, created.ID, 2); !errors.Is(err, namespaces.ErrConflict) {
		t.Fatalf("stale delete = %v", err)
	}
	if err := repository.Delete(ctx, created.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, created.ID); !errors.Is(err, namespaces.ErrNotFound) {
		t.Fatalf("get deleted = %v", err)
	}
	var items int
	if err := store.db.QueryRow(`SELECT count(*) FROM namespace_scope_items WHERE namespace_scope_id = ?`, created.ID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if items != 0 {
		t.Fatalf("orphan items = %d", items)
	}
}

func namespaceScopeTestProfile(t *testing.T, store *Store) int64 {
	t.Helper()
	result, err := store.db.Exec(`
		INSERT INTO cluster_profiles(name, context_name, is_default, created_at, updated_at)
		VALUES ('Namespace scopes test', 'development', 1, 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertNamespaceItems(t *testing.T, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("items=%#v want=%#v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("items=%#v want=%#v", actual, expected)
		}
	}
}
