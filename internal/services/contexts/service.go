package contexts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/selection"
)

var (
	ErrNotFound         = errors.New("context profile not found")
	ErrValidation       = errors.New("context request is invalid")
	ErrGenerationChange = errors.New("context generation changed")
)

type Service struct {
	repository ProfileRepository
	runtime    Runtime
	selection  SelectionState
	snapshots  SnapshotWriter
	now        func() time.Time
	pendingMu  sync.Mutex
	pendingNS  string
}

func NewService(repository ProfileRepository, runtime Runtime, selection SelectionState, snapshots SnapshotWriter) (*Service, error) {
	if repository == nil || runtime == nil || selection == nil || snapshots == nil {
		return nil, errors.New("contexts: repository, runtime, selection and snapshots are required")
	}
	return &Service{repository: repository, runtime: runtime, selection: selection, snapshots: snapshots, now: time.Now}, nil
}

func (s *Service) List(ctx context.Context, profileID int64) ([]ContextDTO, error) {
	if profileID <= 0 {
		return nil, ErrValidation
	}
	profile, err := s.repository.Get(ctx, profileID)
	if errors.Is(err, clusterprofiles.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	reference := profileReference(profile)
	reference.Context = ""
	candidate, err := s.runtime.Resolve(ctx, SourceRequest{Persisted: reference, ProfileOnly: true})
	if err != nil {
		return nil, err
	}
	result := make([]ContextDTO, 0, len(candidate.Contexts()))
	for _, item := range candidate.Contexts() {
		selected := profile.Context != nil && *profile.Context == item.Name
		result = append(result, ContextDTO{ClusterProfileID: profile.ID, Name: item.Name, Cluster: item.Cluster, Selected: selected})
	}
	return result, nil
}

// Bootstrap resolves the canonical source precedence while preserving a
// usable local shell when kubeconfig, context, authentication or cluster state
// is unavailable. Only local repository/state errors abort startup.
func (s *Service) Bootstrap(ctx context.Context, request BootstrapRequest) error {
	s.rememberEphemeralNamespace(request.EphemeralNS)
	profiles, err := s.repository.List(ctx)
	if err != nil {
		return err
	}
	var persisted *ProfileReference
	defaultProfile, defaultErr := s.repository.Default(ctx)
	if defaultErr == nil {
		persisted = profileReference(defaultProfile)
	} else if !errors.Is(defaultErr, clusterprofiles.ErrNotFound) {
		return defaultErr
	}

	// Resolve the winning source before applying any profile context. An
	// explicit path or KUBECONFIG may point at a different source than the old
	// default profile, whose context must not poison reconciliation.
	sourceReference := persisted
	if sourceReference != nil {
		copy := *sourceReference
		copy.Paths = append([]string(nil), sourceReference.Paths...)
		copy.Context = ""
		sourceReference = &copy
	}
	first, err := s.runtime.Resolve(ctx, SourceRequest{ExplicitPath: request.ExplicitPath, Persisted: sourceReference})
	if err != nil {
		return s.publishBootstrapFailure(err)
	}
	suggestedName := "Kubernetes"
	if available := first.Contexts(); len(available) > 0 {
		suggestedName = available[0].Cluster
		if suggestedName == "" {
			suggestedName = available[0].Name
		}
	}
	profile, _, err := s.repository.Reconcile(ctx, suggestedName, first.Paths(), len(profiles) == 0)
	if err != nil {
		return err
	}

	candidate, err := s.runtime.Resolve(ctx, SourceRequest{
		ExplicitPath: request.ExplicitPath, ExplicitContext: request.ExplicitContext,
		Persisted: profileReference(profile), FirstReconcile: profile.Context == nil,
	})
	if err != nil {
		return s.publishBootstrapFailure(err)
	}
	selected, ok := candidate.Selected()
	if !ok {
		if err := s.snapshots.SetState(api.ComponentKubeconfig, healthyState("Kubeconfig is available.", s.now())); err != nil {
			return err
		}
		return nil
	}
	if profile.Context == nil || *profile.Context != selected.Name {
		profile, err = s.repository.SetContext(ctx, profile.ID, &selected.Name, false)
		if err != nil {
			return err
		}
	}
	resolution := namespaces.ScopeResolution{}
	ephemeralNamespace := s.ephemeralNamespace()
	if ephemeralNamespace != "" {
		resolution.ScopeMode = namespaces.ScopeModeSingle
		resolution.ScopeSource = "cli"
		resolution.DefaultNamespace = &ephemeralNamespace
		resolution.Namespaces = []string{ephemeralNamespace}
	}
	binding := namespaces.SelectionBinding{ClusterProfileID: profile.ID, Context: selected.Name, Cluster: selected.Cluster}
	if err := s.selection.Initialize(binding, resolution); err != nil {
		return err
	}
	s.consumeEphemeralNamespace(ephemeralNamespace)
	binding, resolution = s.selection.Snapshot()
	clusterState, activationErr := s.runtime.Activate(ctx, candidate, binding)
	if activationErr != nil {
		clusterState = externalState(activationErr, s.now())
	}
	s.publishSelection(binding, resolution, selected.Cluster, ephemeralNamespace, clusterState)
	return nil
}

func (s *Service) Select(ctx context.Context, request SelectRequest) (SelectionDTO, error) {
	if err := validateSelectRequest(request); err != nil {
		return SelectionDTO{}, err
	}
	var candidate Candidate
	var selected ContextDescriptor
	var ephemeralNamespace string
	result, err := s.selection.ReplaceContextPrepared(ctx, request.ExpectedGeneration, func(intentContext context.Context) (namespaces.SelectionBinding, namespaces.ScopeResolution, func(context.Context) error, error) {
		profile, err := s.repository.Get(intentContext, request.ClusterProfileID)
		if errors.Is(err, clusterprofiles.ErrNotFound) {
			return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, nil, ErrNotFound
		}
		if err != nil {
			return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, nil, err
		}
		candidate, err = s.runtime.Resolve(intentContext, SourceRequest{
			ExplicitContext: &request.Context,
			Persisted:       profileReference(profile),
			ProfileOnly:     true,
		})
		if err != nil {
			return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, nil, err
		}
		var ok bool
		selected, ok = candidate.Selected()
		if !ok {
			return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, nil, &ExternalError{Code: api.CodeContextNotFound, Message: "The selected Kubernetes context does not exist."}
		}
		binding := namespaces.SelectionBinding{ClusterProfileID: profile.ID, Context: selected.Name, Cluster: selected.Cluster}
		resolution := namespaces.ScopeResolution{}
		ephemeralNamespace = s.ephemeralNamespace()
		if ephemeralNamespace != "" {
			resolution.ScopeMode = namespaces.ScopeModeSingle
			resolution.ScopeSource = "cli"
			resolution.DefaultNamespace = &ephemeralNamespace
			resolution.Namespaces = []string{ephemeralNamespace}
		}
		transaction := func(transactionContext context.Context) error {
			_, updateErr := s.repository.SetContext(transactionContext, profile.ID, &selected.Name, request.SetDefault)
			return updateErr
		}
		return binding, resolution, transaction, nil
	})
	if err != nil {
		if errors.Is(err, namespaces.ErrGenerationChanged) || errors.Is(err, selection.ErrGenerationChanged) || errors.Is(err, selection.ErrSuperseded) {
			return SelectionDTO{}, ErrGenerationChange
		}
		return SelectionDTO{}, err
	}
	binding := result.Binding
	resolution := result.Resolution
	s.consumeEphemeralNamespace(ephemeralNamespace)
	clusterState, activationErr := s.runtime.Activate(ctx, candidate, binding)
	if errors.Is(activationErr, ErrGenerationChange) {
		return SelectionDTO{}, ErrGenerationChange
	}
	if activationErr != nil {
		clusterState = externalState(activationErr, s.now())
	}
	dto := selectionDTO(binding, resolution, selected.Cluster, ephemeralNamespace, clusterState)
	if !s.selection.IfCurrent(binding, func() {
		s.publishSelection(binding, resolution, selected.Cluster, ephemeralNamespace, clusterState)
	}) {
		return SelectionDTO{}, ErrGenerationChange
	}
	return dto, nil
}

func (s *Service) rememberEphemeralNamespace(namespace string) {
	if namespace == "" {
		return
	}
	s.pendingMu.Lock()
	s.pendingNS = namespace
	s.pendingMu.Unlock()
}

func (s *Service) ephemeralNamespace() string {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	return s.pendingNS
}

func (s *Service) consumeEphemeralNamespace(namespace string) {
	if namespace == "" {
		return
	}
	s.pendingMu.Lock()
	if s.pendingNS == namespace {
		s.pendingNS = ""
	}
	s.pendingMu.Unlock()
}

func (s *Service) publishBootstrapFailure(err error) error {
	state := externalState(err, s.now())
	component := api.ComponentKubeconfig
	var public *ExternalError
	if errors.As(err, &public) && (public.Code == api.CodeContextNotFound || public.Code == "CONTEXT_REQUIRED") {
		component = api.ComponentContext
	}
	if setErr := s.snapshots.SetState(component, state); setErr != nil {
		return setErr
	}
	return nil
}

func (s *Service) publishSelection(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, cluster, ephemeralNS string, clusterState api.ComponentState) {
	_ = s.snapshots.SetState(api.ComponentKubeconfig, healthyState("Kubeconfig is available.", s.now()))
	_ = s.snapshots.SetState(api.ComponentContext, healthyState("Kubernetes context is selected.", s.now()))
	_ = s.snapshots.SetState(api.ComponentCluster, clusterState)
	summary := summary(binding, resolution, cluster, ephemeralNS)
	s.snapshots.SetSelection(&summary)
}

func summary(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, cluster, ephemeralNS string) api.SelectionSummary {
	value := api.SelectionSummary{
		ClusterProfileID: binding.ClusterProfileID, Context: binding.Context, Cluster: cluster,
		ScopeSource: "none", NamespaceCount: len(resolution.Namespaces), Generation: binding.Generation,
	}
	if resolution.ScopeID > 0 {
		id := resolution.ScopeID
		value.ScopeID = &id
		value.ScopeSource = resolution.ScopeSource
		name := resolution.ScopeName
		value.ScopeName = &name
		mode := string(resolution.ScopeMode)
		value.ScopeMode = &mode
		value.DefaultNamespace = resolution.DefaultNamespace
	} else if resolution.ScopeSource == "cli" || ephemeralNS != "" {
		mode := string(namespaces.ScopeModeSingle)
		value.ScopeMode = &mode
		value.ScopeSource = "cli"
		value.DefaultNamespace = resolution.DefaultNamespace
	}
	return value
}

func selectionDTO(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, cluster, ephemeralNS string, clusterState api.ComponentState) SelectionDTO {
	summary := summary(binding, resolution, cluster, ephemeralNS)
	return SelectionDTO{
		ClusterProfileID: summary.ClusterProfileID, Context: summary.Context, Cluster: summary.Cluster,
		ScopeID: summary.ScopeID, ScopeName: summary.ScopeName, ScopeMode: summary.ScopeMode,
		ScopeSource: summary.ScopeSource, DefaultNamespace: summary.DefaultNamespace,
		NamespaceCount: summary.NamespaceCount, Generation: summary.Generation,
		Components: SelectionComponents{Cluster: clusterState},
	}
}

func profileReference(profile clusterprofiles.Profile) *ProfileReference {
	contextName := ""
	if profile.Context != nil {
		contextName = *profile.Context
	}
	return &ProfileReference{Paths: append([]string(nil), profile.Paths...), Context: contextName}
}

func validateSelectRequest(request SelectRequest) error {
	if request.ClusterProfileID <= 0 || request.ExpectedGeneration == "" {
		return ErrValidation
	}
	if request.Context == "" || strings.TrimSpace(request.Context) != request.Context || len(request.Context) > 1024 || !utf8.ValidString(request.Context) {
		return ErrValidation
	}
	return nil
}

func externalState(err error, now time.Time) api.ComponentState {
	checked := now.UTC()
	state := api.ComponentState{Status: api.StatusUnknown, Code: "KUBERNETES_CLIENT_UNAVAILABLE", Message: "The Kubernetes client is unavailable.", CheckedAt: &checked}
	var external *ExternalError
	if !errors.As(err, &external) {
		return state
	}
	state.Code = external.Code
	state.Message = external.Message
	if external.Code == api.CodeClusterUnavailable {
		state.Status = api.StatusDegraded
	}
	return state
}

func (s *Service) String() string {
	return fmt.Sprintf("contexts.Service(%T)", s.runtime)
}
