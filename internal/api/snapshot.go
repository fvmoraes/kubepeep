package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	gingerhealth "github.com/fvmoraes/ginger/pkg/health"
)

const defaultCheckTimeout = 2 * time.Second

// CheckDefinition binds a Ginger health.Checker to a public, sanitized result.
// FailureCode and FailureMessage are fixed application strings; checker errors
// are intentionally not copied into the API snapshot.
type CheckDefinition struct {
	Component      Component
	Checker        gingerhealth.Checker
	FailureStatus  ComponentStatus
	FailureCode    string
	FailureMessage string
	SuccessMessage string
}

// CheckerSnapshotProvider is a concurrency-safe source shared by /health and
// /api/v1/status. It preserves Ginger's health.Checker boundary while applying
// per-check deadlines, panic recovery and degraded-vs-critical policy locally.
type CheckerSnapshotProvider struct {
	mu       sync.RWMutex
	base     Snapshot
	checks   map[Component]CheckDefinition
	inFlight map[Component]bool
	timeout  time.Duration
	now      func() time.Time
}

func NewCheckerSnapshotProvider(base Snapshot, timeout time.Duration, definitions ...CheckDefinition) (*CheckerSnapshotProvider, error) {
	if err := ValidateSnapshot(base); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	provider := &CheckerSnapshotProvider{
		base:     base,
		checks:   make(map[Component]CheckDefinition, len(definitions)),
		inFlight: make(map[Component]bool, len(definitions)),
		timeout:  timeout,
		now:      time.Now,
	}
	for _, definition := range definitions {
		if err := validateCheckDefinition(definition); err != nil {
			return nil, err
		}
		provider.checks[definition.Component] = definition
	}
	return provider, nil
}

func (p *CheckerSnapshotProvider) Snapshot(ctx context.Context) (Snapshot, error) {
	p.mu.RLock()
	snapshot := cloneSnapshot(p.base)
	definitions := make([]CheckDefinition, 0, len(p.checks))
	for _, component := range componentOrder {
		if definition, ok := p.checks[component]; ok {
			definitions = append(definitions, definition)
		}
	}
	timeout := p.timeout
	now := p.now
	p.mu.RUnlock()

	type result struct {
		component Component
		state     ComponentState
	}
	checkContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	results := make(chan result, len(definitions))
	pending := make(map[Component]CheckDefinition, len(definitions))
	for _, definition := range definitions {
		definition := definition
		if checkContext.Err() != nil || !p.beginCheck(definition.Component) {
			snapshot.Components.set(definition.Component, checkFailureState(now, definition))
			continue
		}
		pending[definition.Component] = definition
		go func() {
			defer p.finishCheck(definition.Component)
			state := runCheck(checkContext, timeout, now, definition)
			// The channel is sized for every definition, so a checker that returns
			// after the request deadline can never block trying to publish its result.
			results <- result{component: definition.Component, state: state}
		}()
	}
	for len(pending) > 0 {
		select {
		case checked := <-results:
			if _, ok := pending[checked.component]; !ok {
				continue
			}
			snapshot.Components.set(checked.component, checked.state)
			delete(pending, checked.component)
		case <-checkContext.Done():
			for component, definition := range pending {
				snapshot.Components.set(component, checkFailureState(now, definition))
			}
			return snapshot, nil
		}
	}
	return snapshot, nil
}

func (p *CheckerSnapshotProvider) beginCheck(component Component) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inFlight[component] {
		return false
	}
	p.inFlight[component] = true
	return true
}

func (p *CheckerSnapshotProvider) finishCheck(component Component) {
	p.mu.Lock()
	delete(p.inFlight, component)
	p.mu.Unlock()
}

func (p *CheckerSnapshotProvider) SetState(component Component, state ComponentState) error {
	if !validComponent(component) {
		return fmt.Errorf("api: unsupported component %q", component)
	}
	if err := validateState(state); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.base.Components.set(component, state)
	return nil
}

func (p *CheckerSnapshotProvider) SetSelection(selection *SelectionSummary) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if selection == nil {
		p.base.Selection = nil
		return
	}
	copy := *selection
	p.base.Selection = &copy
}

func (p *CheckerSnapshotProvider) SetChecker(definition CheckDefinition) error {
	if err := validateCheckDefinition(definition); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.checks[definition.Component] = definition
	return nil
}

func InitialSnapshot() Snapshot {
	return Snapshot{Components: StatusComponents{
		Application: unknownState("STARTING", "Application startup is not complete."),
		SQLite:      unknownState("NOT_CHECKED", "SQLite has not been checked."),
		Kubeconfig:  unknownState("NOT_CHECKED", "Kubeconfig has not been checked."),
		Context:     unknownState("NOT_SELECTED", "No context is selected."),
		Cluster:     unknownState("NOT_CHECKED", "The cluster has not been checked."),
		Metrics:     unknownState("NOT_CHECKED", "Metrics API has not been checked."),
	}}
}

func HealthyCheck(component Component, checker gingerhealth.Checker, successMessage, failureCode, failureMessage string) CheckDefinition {
	failureStatus := StatusDegraded
	if component == ComponentApplication || component == ComponentSQLite {
		failureStatus = StatusUnhealthy
	}
	return CheckDefinition{
		Component:      component,
		Checker:        checker,
		FailureStatus:  failureStatus,
		FailureCode:    failureCode,
		FailureMessage: failureMessage,
		SuccessMessage: successMessage,
	}
}

var componentOrder = [...]Component{
	ComponentApplication,
	ComponentSQLite,
	ComponentKubeconfig,
	ComponentContext,
	ComponentCluster,
	ComponentMetrics,
}

func runCheck(parent context.Context, timeout time.Duration, now func() time.Time, definition CheckDefinition) (state ComponentState) {
	state = checkFailureState(now, definition)
	defer func() {
		if recover() != nil {
			// Keep the prebuilt, sanitized failure state.
		}
	}()

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if ctx.Err() != nil {
		return state
	}
	if err := definition.Checker.Check(ctx); err != nil {
		return state
	}
	state.Status = StatusHealthy
	state.Code = "OK"
	state.Message = definition.SuccessMessage
	return state
}

func checkFailureState(now func() time.Time, definition CheckDefinition) ComponentState {
	checkedAt := now().UTC()
	return ComponentState{
		Status:    definition.FailureStatus,
		Code:      definition.FailureCode,
		Message:   definition.FailureMessage,
		CheckedAt: &checkedAt,
	}
}

func validateCheckDefinition(definition CheckDefinition) error {
	if !validComponent(definition.Component) {
		return fmt.Errorf("api: unsupported checker component %q", definition.Component)
	}
	if definition.Checker == nil {
		return fmt.Errorf("api: checker for %s is nil", definition.Component)
	}
	if definition.SuccessMessage == "" || definition.FailureCode == "" || definition.FailureMessage == "" {
		return fmt.Errorf("api: checker for %s has incomplete public messages", definition.Component)
	}
	if definition.FailureStatus != StatusDegraded && definition.FailureStatus != StatusUnhealthy {
		return fmt.Errorf("api: checker for %s has invalid failure status", definition.Component)
	}
	if (definition.Component == ComponentApplication || definition.Component == ComponentSQLite) && definition.FailureStatus != StatusUnhealthy {
		return fmt.Errorf("api: local checker %s must fail as unhealthy", definition.Component)
	}
	return nil
}

func validateState(state ComponentState) error {
	switch state.Status {
	case StatusHealthy, StatusDegraded, StatusUnhealthy, StatusUnknown:
	default:
		return fmt.Errorf("api: invalid component status %q", state.Status)
	}
	if state.Code == "" || state.Message == "" {
		return fmt.Errorf("api: component state requires public code and message")
	}
	return nil
}

func validComponent(component Component) bool {
	for _, candidate := range componentOrder {
		if component == candidate {
			return true
		}
	}
	return false
}

func unknownState(code, message string) ComponentState {
	return ComponentState{Status: StatusUnknown, Code: code, Message: message, CheckedAt: nil}
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	if snapshot.Selection == nil {
		return snapshot
	}
	selection := *snapshot.Selection
	snapshot.Selection = &selection
	return snapshot
}
