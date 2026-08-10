package clusterprofiles

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/adapters/sqlite"
)

type fixedActiveProfile int64

func (profile fixedActiveProfile) ActiveProfileID() int64 { return int64(profile) }

func testRepository(t *testing.T) (*Repository, *sqlite.Store) {
	t.Helper()
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "kubePeep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository, err := NewRepository(store.SQLDB())
	if err != nil {
		t.Fatal(err)
	}
	repository.now = func() time.Time { return time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC) }
	return repository, store
}

func TestReconcileUsesOrderedPathIdentityAndNeverStoresFingerprint(t *testing.T) {
	repository, store := testRepository(t)
	ctx := context.Background()
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")

	created, wasCreated, err := repository.Reconcile(ctx, "Development", []string{first, second}, true)
	if err != nil || !wasCreated || !created.IsDefault {
		t.Fatalf("created=%#v wasCreated=%v err=%v", created, wasCreated, err)
	}
	reused, wasCreated, err := repository.Reconcile(ctx, "Ignored", []string{first, second}, false)
	if err != nil || wasCreated || reused.ID != created.ID {
		t.Fatalf("reused=%#v wasCreated=%v err=%v", reused, wasCreated, err)
	}
	reordered, wasCreated, err := repository.Reconcile(ctx, "Development", []string{second, first}, false)
	if err != nil || !wasCreated || reordered.ID == created.ID {
		t.Fatalf("reordered=%#v wasCreated=%v err=%v", reordered, wasCreated, err)
	}

	rows, err := store.SQLDB().QueryContext(ctx, `SELECT path FROM cluster_profile_kubeconfig_files ORDER BY cluster_profile_id, position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var stored string
		if err := rows.Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != first && stored != second {
			t.Fatalf("unexpected persisted value %q", stored)
		}
	}
}

func TestReconcileSerializesConcurrentOrderedPathIdentity(t *testing.T) {
	repository, _ := testRepository(t)
	paths := []string{filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")}
	const callers = 12
	type result struct {
		profile Profile
		created bool
		err     error
	}
	results := make(chan result, callers)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			profile, created, err := repository.Reconcile(context.Background(), "Concurrent", paths, false)
			results <- result{profile: profile, created: created, err: err}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var id int64
	createdCount := 0
	for item := range results {
		if item.err != nil {
			t.Fatal(item.err)
		}
		if id == 0 {
			id = item.profile.ID
		}
		if item.profile.ID != id {
			t.Fatalf("profile ids diverged: got %d want %d", item.profile.ID, id)
		}
		if item.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
}

func TestSetContextChangesDefaultAtomically(t *testing.T) {
	repository, _ := testRepository(t)
	ctx := context.Background()
	one, _, err := repository.Reconcile(ctx, "One", []string{filepath.Join(t.TempDir(), "one")}, true)
	if err != nil {
		t.Fatal(err)
	}
	two, _, err := repository.Reconcile(ctx, "Two", []string{filepath.Join(t.TempDir(), "two")}, false)
	if err != nil {
		t.Fatal(err)
	}
	selected := "staging"
	two, err = repository.SetContext(ctx, two.ID, &selected, true)
	if err != nil {
		t.Fatal(err)
	}
	if !two.IsDefault || two.Context == nil || *two.Context != selected {
		t.Fatalf("selected profile = %#v", two)
	}
	one, err = repository.Get(ctx, one.ID)
	if err != nil {
		t.Fatal(err)
	}
	if one.IsDefault {
		t.Fatal("previous profile remained default")
	}
	active, err := repository.Default(ctx)
	if err != nil || active.ID != two.ID {
		t.Fatalf("default=%#v err=%v", active, err)
	}
}

func TestServiceActiveUsesCoordinatorSelectionWithoutChangingDefault(t *testing.T) {
	repository, _ := testRepository(t)
	ctx := context.Background()
	defaultProfile, _, err := repository.Reconcile(ctx, "Default", []string{filepath.Join(t.TempDir(), "default")}, true)
	if err != nil {
		t.Fatal(err)
	}
	selectedProfile, _, err := repository.Reconcile(ctx, "Selected", []string{filepath.Join(t.TempDir(), "selected")}, false)
	if err != nil {
		t.Fatal(err)
	}
	selectedContext := "development"
	if _, err := repository.SetContext(ctx, selectedProfile.ID, &selectedContext, false); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, "", fixedActiveProfile(selectedProfile.ID))
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.Active(ctx)
	if err != nil || active.ID != selectedProfile.ID || active.IsDefault {
		t.Fatalf("active=%#v err=%v", active, err)
	}
	persistedDefault, err := repository.Default(ctx)
	if err != nil || persistedDefault.ID != defaultProfile.ID {
		t.Fatalf("default=%#v err=%v", persistedDefault, err)
	}
}

func TestDTOOnlyExposesSanitizedDisplayPaths(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "users", "someone")
	profile := Profile{ID: 1, Name: "Local", Paths: []string{
		filepath.Join(home, ".kube", "config"),
		filepath.Join(string(filepath.Separator), "private", "clusters", "other.yaml"),
	}}
	dto := ToDTO(profile, home)
	if dto.KubeconfigFiles[0].DisplayPath != "~/.kube/config" {
		t.Fatalf("home display = %q", dto.KubeconfigFiles[0].DisplayPath)
	}
	if dto.KubeconfigFiles[1].DisplayPath != "other.yaml" {
		t.Fatalf("external display = %q", dto.KubeconfigFiles[1].DisplayPath)
	}
}

func TestRepositoryRejectsInvalidReferencesAndMissingProfiles(t *testing.T) {
	repository, _ := testRepository(t)
	ctx := context.Background()
	for _, paths := range [][]string{nil, {"relative"}, {"/same", "/same"}} {
		if _, _, err := repository.Reconcile(ctx, "Invalid", paths, false); err == nil {
			t.Fatalf("accepted invalid paths %#v", paths)
		}
	}
	if _, err := repository.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing profile error = %v", err)
	}
}
