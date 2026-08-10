package contexts

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type fakeCandidate struct {
	paths    []string
	contexts []ContextDescriptor
	selected *ContextDescriptor
}

func (c *fakeCandidate) Paths() []string { return append([]string(nil), c.paths...) }
func (c *fakeCandidate) Contexts() []ContextDescriptor {
	return append([]ContextDescriptor(nil), c.contexts...)
}
func (c *fakeCandidate) Selected() (ContextDescriptor, bool) {
	if c.selected == nil {
		return ContextDescriptor{}, false
	}
	return *c.selected, true
}

type fakeRuntime struct {
	candidates  []Candidate
	errors      []error
	requests    []SourceRequest
	activation  api.ComponentState
	activateErr error
	activated   namespaces.SelectionBinding
	generations []string
	onActivate  func(namespaces.SelectionBinding)
}

func (r *fakeRuntime) Resolve(_ context.Context, request SourceRequest) (Candidate, error) {
	r.requests = append(r.requests, request)
	position := len(r.requests) - 1
	if position < len(r.errors) && r.errors[position] != nil {
		return nil, r.errors[position]
	}
	if position >= len(r.candidates) {
		return nil, errors.New("unexpected resolve")
	}
	return r.candidates[position], nil
}
func (r *fakeRuntime) Activate(_ context.Context, _ Candidate, binding namespaces.SelectionBinding) (api.ComponentState, error) {
	r.activated = binding
	if r.onActivate != nil {
		r.onActivate(binding)
	}
	return r.activation, r.activateErr
}
func (r *fakeRuntime) OnGeneration(value string) { r.generations = append(r.generations, value) }

type fakeProfiles struct {
	profiles   []clusterprofiles.Profile
	setCalls   int
	reconciled bool
}

func (r *fakeProfiles) List(context.Context) ([]clusterprofiles.Profile, error) {
	return append([]clusterprofiles.Profile(nil), r.profiles...), nil
}
func (r *fakeProfiles) Default(context.Context) (clusterprofiles.Profile, error) {
	for _, profile := range r.profiles {
		if profile.IsDefault {
			return profile, nil
		}
	}
	return clusterprofiles.Profile{}, clusterprofiles.ErrNotFound
}
func (r *fakeProfiles) Get(_ context.Context, id int64) (clusterprofiles.Profile, error) {
	for _, profile := range r.profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return clusterprofiles.Profile{}, clusterprofiles.ErrNotFound
}
func (r *fakeProfiles) Reconcile(_ context.Context, name string, paths []string, makeDefault bool) (clusterprofiles.Profile, bool, error) {
	r.reconciled = true
	for _, profile := range r.profiles {
		if slices.Equal(profile.Paths, paths) {
			return profile, false, nil
		}
	}
	profile := clusterprofiles.Profile{ID: 1, Name: name, Paths: append([]string(nil), paths...), IsDefault: makeDefault}
	r.profiles = append(r.profiles, profile)
	return profile, true, nil
}
func (r *fakeProfiles) SetContext(_ context.Context, id int64, value *string, makeDefault bool) (clusterprofiles.Profile, error) {
	r.setCalls++
	for position := range r.profiles {
		if r.profiles[position].ID != id {
			continue
		}
		copied := *value
		r.profiles[position].Context = &copied
		if makeDefault {
			for other := range r.profiles {
				r.profiles[other].IsDefault = other == position
			}
		}
		return r.profiles[position], nil
	}
	return clusterprofiles.Profile{}, clusterprofiles.ErrNotFound
}

type fakeSelection struct {
	binding         namespaces.SelectionBinding
	resolution      namespaces.ScopeResolution
	initializeCalls int
	replaceCalls    int
}

func (s *fakeSelection) Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution) {
	return s.binding, s.resolution
}
func (s *fakeSelection) Initialize(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) error {
	s.initializeCalls++
	binding.Generation = "gen_initial"
	s.binding, s.resolution = binding, resolution
	return nil
}
func (s *fakeSelection) ReplaceContextPrepared(ctx context.Context, expected string, prepare func(context.Context) (namespaces.SelectionBinding, namespaces.ScopeResolution, func(context.Context) error, error)) (namespaces.SelectionResult, error) {
	s.replaceCalls++
	if expected != s.binding.Generation {
		return namespaces.SelectionResult{}, namespaces.ErrGenerationChanged
	}
	binding, resolution, transaction, err := prepare(ctx)
	if err != nil {
		return namespaces.SelectionResult{}, err
	}
	if err := transaction(ctx); err != nil {
		return namespaces.SelectionResult{}, err
	}
	binding.Generation = "gen_next"
	binding.ActiveScopeID = resolution.ScopeID
	s.binding, s.resolution = binding, resolution
	return namespaces.SelectionResult{Generation: binding.Generation, Binding: binding, Resolution: resolution, Changed: true}, nil
}
func (s *fakeSelection) IfCurrent(binding namespaces.SelectionBinding, fn func()) bool {
	if s.binding != binding {
		return false
	}
	fn()
	return true
}

type fakeSnapshots struct {
	states    map[api.Component]api.ComponentState
	selection *api.SelectionSummary
}

func (s *fakeSnapshots) SetState(component api.Component, state api.ComponentState) error {
	if s.states == nil {
		s.states = make(map[api.Component]api.ComponentState)
	}
	s.states[component] = state
	return nil
}
func (s *fakeSnapshots) SetSelection(value *api.SelectionSummary) { s.selection = value }

func TestListUsesOnlyRequestedProfileAndMarksPersistedSelection(t *testing.T) {
	selected := "dev"
	repository := &fakeProfiles{profiles: []clusterprofiles.Profile{{ID: 7, Paths: []string{"/safe/config"}, Context: &selected}}}
	runtime := &fakeRuntime{candidates: []Candidate{&fakeCandidate{contexts: []ContextDescriptor{{Name: "dev", Cluster: "cluster-a"}, {Name: "prod", Cluster: "cluster-b"}}}}}
	service, err := NewService(repository, runtime, &fakeSelection{}, &fakeSnapshots{})
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || !items[0].Selected || items[1].Selected || !runtime.requests[0].ProfileOnly || runtime.requests[0].Persisted.Context != "" {
		t.Fatalf("unexpected contexts: %#v request=%#v", items, runtime.requests[0])
	}
}

func TestBootstrapResolvesSourceBeforeApplyingPreviousDefaultContext(t *testing.T) {
	oldContext := "removed-from-new-source"
	repository := &fakeProfiles{profiles: []clusterprofiles.Profile{{ID: 7, Name: "Old", Paths: []string{"/old/config"}, Context: &oldContext, IsDefault: true}}}
	selected := ContextDescriptor{Name: "new", Cluster: "new-cluster"}
	runtime := &fakeRuntime{candidates: []Candidate{
		&fakeCandidate{paths: []string{"/new/config"}, contexts: []ContextDescriptor{selected}},
		&fakeCandidate{paths: []string{"/new/config"}, selected: &selected},
	}}
	service, err := NewService(repository, runtime, &fakeSelection{}, &fakeSnapshots{})
	if err != nil {
		t.Fatal(err)
	}
	explicit := "/new/config"
	if err := service.Bootstrap(context.Background(), BootstrapRequest{ExplicitPath: &explicit}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.requests) != 2 || runtime.requests[0].ExplicitContext != nil || runtime.requests[0].Persisted.Context != "" {
		t.Fatalf("source resolution inherited stale context: %#v", runtime.requests)
	}
	if !repository.reconciled || !slices.Equal(runtime.requests[1].Persisted.Paths, []string{"/new/config"}) {
		t.Fatalf("new source was not reconciled: repo=%#v requests=%#v", repository, runtime.requests)
	}
}

func TestSelectValidatesBeforeCommitAndPreservesGenerationOnFailure(t *testing.T) {
	contextName := "old"
	repository := &fakeProfiles{profiles: []clusterprofiles.Profile{{ID: 4, Paths: []string{"/safe/config"}, Context: &contextName}}}
	runtime := &fakeRuntime{errors: []error{&ExternalError{Code: api.CodeContextNotFound, Message: "missing"}}}
	state := &fakeSelection{binding: namespaces.SelectionBinding{ClusterProfileID: 4, Context: "old", Generation: "gen_old"}}
	service, _ := NewService(repository, runtime, state, &fakeSnapshots{})
	_, err := service.Select(context.Background(), SelectRequest{ClusterProfileID: 4, Context: "missing", ExpectedGeneration: "gen_old"})
	var external *ExternalError
	if !errors.As(err, &external) || external.Code != api.CodeContextNotFound {
		t.Fatalf("expected safe context error, got %v", err)
	}
	if repository.setCalls != 0 || state.replaceCalls != 1 || state.binding.Generation != "gen_old" {
		t.Fatalf("precommit failure mutated state: repo=%d replace=%d binding=%#v", repository.setCalls, state.replaceCalls, state.binding)
	}
}

func TestSelectDoesNotPublishAfterNewerSelectionWinsDuringActivation(t *testing.T) {
	old := "old"
	selected := ContextDescriptor{Name: "new", Cluster: "cluster-new"}
	repository := &fakeProfiles{profiles: []clusterprofiles.Profile{{ID: 4, Paths: []string{"/safe/config"}, Context: &old}}}
	state := &fakeSelection{binding: namespaces.SelectionBinding{ClusterProfileID: 4, Context: old, Generation: "gen_old"}}
	runtime := &fakeRuntime{candidates: []Candidate{&fakeCandidate{selected: &selected}}}
	runtime.onActivate = func(namespaces.SelectionBinding) {
		state.binding.Generation = "gen_newer"
		state.binding.Context = "winner"
	}
	snapshots := &fakeSnapshots{}
	service, _ := NewService(repository, runtime, state, snapshots)
	_, err := service.Select(context.Background(), SelectRequest{ClusterProfileID: 4, Context: selected.Name, ExpectedGeneration: "gen_old"})
	if !errors.Is(err, ErrGenerationChange) {
		t.Fatalf("expected post-activation generation fence, got %v", err)
	}
	if snapshots.selection != nil {
		t.Fatalf("stale selection was published: %#v", snapshots.selection)
	}
}

func TestSelectCommitsBeforeOfflineActivationAndReturnsDegradedSelection(t *testing.T) {
	old := "old"
	selected := ContextDescriptor{Name: "new", Cluster: "cluster-new"}
	repository := &fakeProfiles{profiles: []clusterprofiles.Profile{{ID: 4, Paths: []string{"/safe/config"}, Context: &old, IsDefault: true}}}
	checked := time.Now().UTC()
	runtime := &fakeRuntime{
		candidates:  []Candidate{&fakeCandidate{selected: &selected}},
		activateErr: &ExternalError{Code: api.CodeClusterUnavailable, Message: "offline", Retryable: true},
		activation:  api.ComponentState{Status: api.StatusHealthy, Code: "OK", Message: "ok", CheckedAt: &checked},
	}
	state := &fakeSelection{binding: namespaces.SelectionBinding{ClusterProfileID: 4, Context: "old", Generation: "gen_old"}}
	snapshots := &fakeSnapshots{}
	service, _ := NewService(repository, runtime, state, snapshots)
	service.now = func() time.Time { return checked }
	dto, err := service.Select(context.Background(), SelectRequest{ClusterProfileID: 4, Context: "new", ExpectedGeneration: "gen_old"})
	if err != nil {
		t.Fatal(err)
	}
	if repository.setCalls != 1 || state.binding.Generation != "gen_next" || dto.Generation != "gen_next" {
		t.Fatalf("selection was not committed: repo=%d state=%#v dto=%#v", repository.setCalls, state.binding, dto)
	}
	if dto.Components.Cluster.Status != api.StatusDegraded || dto.Components.Cluster.Code != api.CodeClusterUnavailable {
		t.Fatalf("offline cluster should be degraded after commit: %#v", dto.Components.Cluster)
	}
	if snapshots.selection == nil || snapshots.selection.Cluster != "cluster-new" || snapshots.selection.ScopeSource != "none" {
		t.Fatalf("sanitized selection snapshot missing: %#v", snapshots.selection)
	}
}

func TestBootstrapReconcilesFirstProfileAndAppliesEphemeralNamespaceOnce(t *testing.T) {
	selected := ContextDescriptor{Name: "dev", Cluster: "cluster-dev"}
	first := &fakeCandidate{paths: []string{"/safe/config"}}
	second := &fakeCandidate{paths: []string{"/safe/config"}, selected: &selected}
	runtime := &fakeRuntime{candidates: []Candidate{first, second}, activation: api.ComponentState{Status: api.StatusHealthy, Code: "OK", Message: "reachable"}}
	repository := &fakeProfiles{}
	state := &fakeSelection{}
	snapshots := &fakeSnapshots{}
	service, _ := NewService(repository, runtime, state, snapshots)
	if err := service.Bootstrap(context.Background(), BootstrapRequest{EphemeralNS: "payments"}); err != nil {
		t.Fatal(err)
	}
	if !repository.reconciled || repository.setCalls != 1 || state.initializeCalls != 1 {
		t.Fatalf("bootstrap did not reconcile selection: repo=%#v state=%#v", repository, state)
	}
	if state.resolution.ScopeSource != "cli" || !slices.Equal(state.resolution.Namespaces, []string{"payments"}) {
		t.Fatalf("ephemeral namespace was not applied: %#v", state.resolution)
	}
	if snapshots.selection == nil || snapshots.selection.ScopeMode == nil || *snapshots.selection.ScopeMode != "single" {
		t.Fatalf("CLI scope missing from snapshot: %#v", snapshots.selection)
	}
}

func TestBootstrapPublishesSafeKubeconfigFailureWithoutFailingLocalStartup(t *testing.T) {
	runtime := &fakeRuntime{errors: []error{&ExternalError{Code: api.CodeKubeconfigNotFound, Message: "No kubeconfig is available."}}}
	snapshots := &fakeSnapshots{}
	service, _ := NewService(&fakeProfiles{}, runtime, &fakeSelection{}, snapshots)
	if err := service.Bootstrap(context.Background(), BootstrapRequest{}); err != nil {
		t.Fatalf("external absence must not fail local startup: %v", err)
	}
	state := snapshots.states[api.ComponentKubeconfig]
	if state.Code != api.CodeKubeconfigNotFound || state.Status != api.StatusUnknown || snapshots.selection != nil {
		t.Fatalf("unexpected degraded bootstrap snapshot: %#v selection=%#v", state, snapshots.selection)
	}
}

func TestBootstrapKeepsEphemeralNamespaceUntilFirstExplicitSelection(t *testing.T) {
	selected := ContextDescriptor{Name: "dev", Cluster: "cluster-dev"}
	runtime := &fakeRuntime{candidates: []Candidate{
		&fakeCandidate{paths: []string{"/safe/config"}},
		&fakeCandidate{paths: []string{"/safe/config"}},
		&fakeCandidate{paths: []string{"/safe/config"}, selected: &selected},
	}}
	repository := &fakeProfiles{}
	state := &fakeSelection{binding: namespaces.SelectionBinding{Generation: "gen_initial"}}
	service, _ := NewService(repository, runtime, state, &fakeSnapshots{})
	if err := service.Bootstrap(context.Background(), BootstrapRequest{EphemeralNS: "payments"}); err != nil {
		t.Fatal(err)
	}
	if service.ephemeralNamespace() != "payments" || state.initializeCalls != 0 {
		t.Fatalf("pending namespace was lost: pending=%q state=%#v", service.ephemeralNamespace(), state)
	}
	dto, err := service.Select(context.Background(), SelectRequest{ClusterProfileID: 1, Context: "dev", ExpectedGeneration: "gen_initial"})
	if err != nil {
		t.Fatal(err)
	}
	if dto.ScopeSource != "cli" || dto.DefaultNamespace == nil || *dto.DefaultNamespace != "payments" || service.ephemeralNamespace() != "" {
		t.Fatalf("ephemeral namespace was not consumed exactly once: dto=%#v pending=%q", dto, service.ephemeralNamespace())
	}
}
