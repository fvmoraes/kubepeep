package actions

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

const (
	ExecWebSocketProtocol = "kubepeep.exec.v1"
	ExecTicketPrefix      = "kp-ticket."
	maximumPendingTickets = 128
)

type execTicket struct {
	tokenHash [sha256.Size]byte
	binding   namespaces.SelectionBinding
	target    MutationTarget
	init      ExecInit
	expiresAt time.Time
}

type execSessionState struct {
	id                string
	leaseID           string
	binding           namespaces.SelectionBinding
	target            MutationTarget
	init              ExecInit
	terminal          chan ExecTerminal
	touch             chan struct{}
	reservationTimer  *time.Timer
	cancel            context.CancelFunc
	remote            RemoteExec
	active            bool
	startedAt         time.Time
	requestedTerminal *ExecTerminal
	finishOnce        sync.Once
}

type ExecManager struct {
	authorizer  AuthorizationService
	generations GenerationReader
	inspector   ExecTargetInspector
	adapter     ExecAdapter
	audit       AuditSink
	clock       Clock
	identifiers IdentifierSource
	ticketTTL   time.Duration
	duration    time.Duration
	idle        time.Duration
	setupLimit  time.Duration

	mu       sync.Mutex
	tickets  map[string]*execTicket
	sessions map[string]*execSessionState
	shutdown bool
}

func NewExecService(authorizer AuthorizationService, generations GenerationReader, inspector ExecTargetInspector, adapter ExecAdapter, audit AuditSink) (*ExecManager, error) {
	return newExecService(authorizer, generations, inspector, adapter, audit, systemClock{}, cryptoIdentifiers{}, DefaultExecTicketTTL, DefaultExecDuration, DefaultExecIdleTimeout, DefaultExecSetupTimeout)
}

func newExecService(authorizer AuthorizationService, generations GenerationReader, inspector ExecTargetInspector, adapter ExecAdapter, audit AuditSink, clock Clock, identifiers IdentifierSource, ticketTTL, duration, idle, setupLimit time.Duration) (*ExecManager, error) {
	if authorizer == nil || generations == nil || inspector == nil || adapter == nil || audit == nil || clock == nil || identifiers == nil {
		return nil, errors.New("actions: exec dependencies are required")
	}
	if ticketTTL <= 0 || duration <= 0 || idle <= 0 || setupLimit <= 0 {
		return nil, errors.New("actions: exec durations must be positive")
	}
	return &ExecManager{
		authorizer:  authorizer,
		generations: generations,
		inspector:   inspector,
		adapter:     adapter,
		audit:       audit,
		clock:       clock,
		identifiers: identifiers,
		ticketTTL:   ticketTTL,
		duration:    duration,
		idle:        idle,
		setupLimit:  setupLimit,
		tickets:     make(map[string]*execTicket),
		sessions:    make(map[string]*execSessionState),
	}, nil
}

func (m *ExecManager) CreateTicket(ctx context.Context, binding namespaces.SelectionBinding, route RouteTarget, request ExecInit) (dto ExecTicketDTO, returnedErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateExec(binding, route, request); err != nil {
		return dto, err
	}
	if err := m.requireCurrent(binding.Generation); err != nil {
		return dto, err
	}
	target := mutationTarget(binding, request.Target)
	started := m.clock.Now().UTC()
	defer func() { recordAudit(m.audit, m.clock, started, "exec_ticket_create", target, returnedErr) }()
	if err := m.inspect(ctx, target, request.Container, false); err != nil {
		return dto, err
	}
	key, err := authorization.KeyForCapability(binding.Generation, target.Namespace, "pods.exec.create", target.Name)
	if err != nil {
		return dto, translateError(err)
	}
	if _, err := m.authorizer.Revalidate(ctx, key, authorization.OperationUpgrade); err != nil {
		return dto, translateError(err)
	}
	if err := m.requireCurrent(binding.Generation); err != nil {
		return dto, err
	}
	sessionID, err := m.identifiers.NewID("exec_")
	if err != nil {
		return dto, publicError(CodeInternal, http.StatusInternalServerError, false, err)
	}
	token, err := m.identifiers.NewToken()
	if err != nil {
		return dto, publicError(CodeInternal, http.StatusInternalServerError, false, err)
	}
	if token == "" || strings.ContainsAny(token, ", \t\r\n") {
		return dto, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	now := m.clock.Now().UTC()
	expiresAt := now.Add(m.ticketTTL)
	request.Command = copyStrings(request.Command)
	ticket := &execTicket{
		tokenHash: sha256.Sum256([]byte(token)),
		binding:   binding,
		target:    target,
		init:      request,
		expiresAt: expiresAt,
	}
	m.mu.Lock()
	m.pruneTicketsLocked(now)
	if m.shutdown {
		m.mu.Unlock()
		return dto, publicError(CodeServerShutdown, http.StatusServiceUnavailable, true, nil)
	}
	if m.generations.CurrentGeneration() != binding.Generation {
		m.mu.Unlock()
		return dto, publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	if len(m.tickets) >= maximumPendingTickets {
		m.mu.Unlock()
		return dto, publicError(CodeLimitExceeded, http.StatusTooManyRequests, false, nil)
	}
	if _, collision := m.tickets[sessionID]; collision {
		m.mu.Unlock()
		return dto, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	m.tickets[sessionID] = ticket
	m.mu.Unlock()
	return ExecTicketDTO{
		SessionID:    sessionID,
		WebsocketURL: "/api/v1/exec/" + sessionID + "/stream",
		Protocols:    []string{ExecWebSocketProtocol, ExecTicketPrefix + token},
		ExpiresAt:    expiresAt,
	}, nil
}

func (m *ExecManager) AuthorizeUpgrade(ctx context.Context, binding namespaces.SelectionBinding, sessionID string, offeredProtocols []string) (grant ExecGrant, returnedErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateBinding(binding); err != nil {
		return grant, err
	}
	if err := validateSessionID(sessionID, "exec_"); err != nil {
		return grant, err
	}
	token, err := parseExecProtocols(offeredProtocols)
	if err != nil {
		return grant, err
	}
	now := m.clock.Now().UTC()
	m.mu.Lock()
	m.pruneTicketsLocked(now)
	ticket, ok := m.tickets[sessionID]
	if !ok {
		m.mu.Unlock()
		return grant, publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	tokenHash := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(tokenHash[:], ticket.tokenHash[:]) != 1 {
		m.mu.Unlock()
		return grant, publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	delete(m.tickets, sessionID)
	m.mu.Unlock()

	started := m.clock.Now().UTC()
	defer func() { recordAudit(m.audit, m.clock, started, "exec_upgrade_authorize", ticket.target, returnedErr) }()
	if ticket.binding.ClusterProfileID != binding.ClusterProfileID || ticket.binding.Context != binding.Context || ticket.binding.Generation != binding.Generation {
		return grant, publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	if err := m.requireCurrent(binding.Generation); err != nil {
		return grant, err
	}
	if err := m.inspect(ctx, ticket.target, ticket.init.Container, true); err != nil {
		return grant, err
	}
	key, err := authorization.KeyForCapability(binding.Generation, ticket.target.Namespace, "pods.exec.create", ticket.target.Name)
	if err != nil {
		return grant, translateError(err)
	}
	if _, err := m.authorizer.Revalidate(ctx, key, authorization.OperationUpgrade); err != nil {
		return grant, translateError(err)
	}
	if err := m.requireCurrent(binding.Generation); err != nil {
		return grant, err
	}
	leaseID, err := m.identifiers.NewID("lease_")
	if err != nil {
		return grant, publicError(CodeInternal, http.StatusInternalServerError, false, err)
	}
	state := &execSessionState{
		id:       sessionID,
		leaseID:  leaseID,
		binding:  binding,
		target:   ticket.target,
		init:     cloneExecInit(ticket.init),
		terminal: make(chan ExecTerminal, 1),
		touch:    make(chan struct{}, 1),
	}
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		return grant, publicError(CodeServerShutdown, http.StatusServiceUnavailable, true, nil)
	}
	if m.generations.CurrentGeneration() != binding.Generation {
		m.mu.Unlock()
		return grant, publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	if len(m.sessions) >= MaximumExecSessions {
		m.mu.Unlock()
		return grant, publicError(CodeLimitExceeded, http.StatusTooManyRequests, false, nil)
	}
	if _, collision := m.sessions[sessionID]; collision {
		m.mu.Unlock()
		return grant, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	m.sessions[sessionID] = state
	state.reservationTimer = time.AfterFunc(m.setupLimit, func() { m.expireReservation(sessionID, leaseID) })
	m.mu.Unlock()
	return ExecGrant{SessionID: sessionID, Generation: binding.Generation, leaseID: leaseID}, nil
}

func (m *ExecManager) Start(ctx context.Context, grant ExecGrant) (active ActiveExec, returnedErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return active, translateError(err)
	}
	m.mu.Lock()
	state, ok := m.sessions[grant.SessionID]
	if !ok || state.leaseID == "" || subtle.ConstantTimeCompare([]byte(state.leaseID), []byte(grant.leaseID)) != 1 {
		m.mu.Unlock()
		return active, publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	if state.active {
		m.mu.Unlock()
		return active, publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	if state.binding.Generation != grant.Generation || m.generations.CurrentGeneration() != grant.Generation {
		delete(m.sessions, grant.SessionID)
		state.reservationTimer.Stop()
		m.mu.Unlock()
		return active, publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	state.reservationTimer.Stop()
	lifetimeContext, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	m.mu.Unlock()

	setupContext, setupCancel := context.WithTimeout(context.Background(), m.setupLimit)
	defer setupCancel()
	remote, err := m.adapter.Start(setupContext, lifetimeContext, ExecCommand{
		Target:    state.target,
		Container: state.init.Container,
		Command:   copyStrings(state.init.Command),
		TTY:       state.init.TTY,
		Stdin:     state.init.Stdin,
	})
	if err != nil || remote == nil {
		cancel()
		m.removeReservation(state)
		if currentErr := m.requireCurrent(grant.Generation); currentErr != nil {
			if remote != nil {
				_ = remote.Close()
			}
			return active, currentErr
		}
		m.mu.Lock()
		shutdown := m.shutdown
		m.mu.Unlock()
		if shutdown {
			if remote != nil {
				_ = remote.Close()
			}
			return active, publicError(CodeServerShutdown, http.StatusServiceUnavailable, true, nil)
		}
		if err == nil {
			err = publicError(CodeInternal, http.StatusInternalServerError, false, nil)
		}
		return active, translateExecSetupError(err)
	}
	if err := m.requireCurrent(grant.Generation); err != nil {
		cancel()
		_ = remote.Close()
		m.removeReservation(state)
		return active, err
	}
	m.mu.Lock()
	current, exists := m.sessions[state.id]
	if !exists || current != state || m.shutdown {
		m.mu.Unlock()
		cancel()
		_ = remote.Close()
		return active, publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	state.remote = remote
	state.active = true
	state.startedAt = m.clock.Now().UTC()
	m.mu.Unlock()
	go m.monitor(state, lifetimeContext)
	recordAudit(m.audit, m.clock, state.startedAt, "exec_start", state.target, nil)
	return ActiveExec{
		SessionID:  state.id,
		Generation: state.binding.Generation,
		TTY:        state.init.TTY,
		Stdin:      state.init.Stdin,
		Remote:     remote,
		Terminal:   state.terminal,
	}, nil
}

func (m *ExecManager) monitor(state *execSessionState, lifetime context.Context) {
	wait := make(chan RemoteExecExit, 1)
	go func() { wait <- state.remote.Wait() }()
	idleTimer := time.NewTimer(m.idle)
	durationTimer := time.NewTimer(m.duration)
	defer idleTimer.Stop()
	defer durationTimer.Stop()
	for {
		select {
		case result := <-wait:
			m.mu.Lock()
			requested := state.requestedTerminal
			m.mu.Unlock()
			if requested != nil {
				m.finish(state, *requested)
			} else {
				m.finish(state, terminalForRemoteExit(result))
			}
			return
		case <-state.touch:
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(m.idle)
		case <-idleTimer.C:
			m.requestTerminal(state, terminalError(CodeExecIdleTimeout, false, 1001))
		case <-durationTimer.C:
			m.requestTerminal(state, terminalError(CodeExecDurationLimit, false, 1001))
		case <-lifetime.Done():
			m.mu.Lock()
			requested := state.requestedTerminal
			m.mu.Unlock()
			if requested == nil {
				terminal := terminalError(CodeInternal, false, 1011)
				requested = &terminal
			}
			m.finish(state, *requested)
			return
		}
	}
}

func (m *ExecManager) Touch(sessionID string) error {
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok || !state.active {
		m.mu.Unlock()
		return publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	m.mu.Unlock()
	select {
	case state.touch <- struct{}{}:
	default:
	}
	return nil
}

func (m *ExecManager) Cancel(sessionID string) error {
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok || !state.active {
		m.mu.Unlock()
		return publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	m.mu.Unlock()
	m.requestTerminal(state, ExecTerminal{Type: ExecTerminalExit, Reason: ExecExitCanceled, CloseCode: 1000})
	return nil
}

func (m *ExecManager) requestTerminal(state *execSessionState, terminal ExecTerminal) {
	m.mu.Lock()
	current, ok := m.sessions[state.id]
	if !ok || current != state || state.requestedTerminal != nil {
		m.mu.Unlock()
		return
	}
	copy := terminal
	state.requestedTerminal = &copy
	cancel := state.cancel
	remote := state.remote
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if remote != nil {
		_ = remote.Close()
	}
}

func (m *ExecManager) finish(state *execSessionState, terminal ExecTerminal) {
	state.finishOnce.Do(func() {
		m.mu.Lock()
		if current, ok := m.sessions[state.id]; ok && current == state {
			delete(m.sessions, state.id)
		}
		cancel := state.cancel
		remote := state.remote
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if remote != nil {
			_ = remote.Close()
		}
		state.terminal <- terminal
		close(state.terminal)
		var terminalErr error
		if terminal.Type == ExecTerminalError {
			terminalErr = publicError(terminal.Code, statusForTerminal(terminal.Code), terminal.Retryable, nil)
		}
		recordAudit(m.audit, m.clock, state.startedAt, "exec_end", state.target, terminalErr)
	})
}

func (m *ExecManager) removeReservation(state *execSessionState) {
	m.mu.Lock()
	if current, ok := m.sessions[state.id]; ok && current == state {
		delete(m.sessions, state.id)
	}
	m.mu.Unlock()
}

func (m *ExecManager) expireReservation(sessionID, leaseID string) {
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok || state.active || state.leaseID != leaseID {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	close(state.terminal)
}

func (m *ExecManager) OnGeneration(current string) {
	m.mu.Lock()
	for identifier, ticket := range m.tickets {
		if ticket.binding.Generation != current {
			delete(m.tickets, identifier)
		}
	}
	states := make([]*execSessionState, 0)
	reservations := make([]*execSessionState, 0)
	for identifier, state := range m.sessions {
		if state.binding.Generation == current {
			continue
		}
		if !state.active {
			delete(m.sessions, identifier)
			state.reservationTimer.Stop()
			reservations = append(reservations, state)
			continue
		}
		states = append(states, state)
	}
	m.mu.Unlock()
	for _, state := range reservations {
		if state.cancel != nil {
			state.cancel()
		}
		close(state.terminal)
	}
	for _, state := range states {
		m.requestTerminal(state, terminalError(CodeGenerationChanged, false, 1001))
	}
}

func (m *ExecManager) Shutdown() {
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		return
	}
	m.shutdown = true
	m.tickets = make(map[string]*execTicket)
	states := make([]*execSessionState, 0)
	reservations := make([]*execSessionState, 0)
	for identifier, state := range m.sessions {
		if !state.active {
			delete(m.sessions, identifier)
			state.reservationTimer.Stop()
			reservations = append(reservations, state)
			continue
		}
		states = append(states, state)
	}
	m.mu.Unlock()
	for _, state := range reservations {
		if state.cancel != nil {
			state.cancel()
		}
		close(state.terminal)
	}
	for _, state := range states {
		m.requestTerminal(state, terminalError(CodeServerShutdown, true, 1001))
	}
}

func (m *ExecManager) inspect(ctx context.Context, target MutationTarget, container string, upgrade bool) error {
	state, err := m.inspector.InspectExecTarget(ctx, target, container)
	if err != nil {
		if upgrade && (errors.Is(err, ErrExecTargetGone) || ErrorCodeOf(translateError(err)) == CodeNotFound) {
			return publicError(CodeExecTargetGone, http.StatusNotFound, false, err)
		}
		return translateError(err)
	}
	if !state.PodExists {
		if upgrade {
			return publicError(CodeExecTargetGone, http.StatusNotFound, false, nil)
		}
		return publicError(CodeNotFound, http.StatusNotFound, false, nil)
	}
	if !state.ContainerExists || !state.ContainerRunning {
		if upgrade && !state.ContainerExists {
			return publicError(CodeExecTargetGone, http.StatusNotFound, false, nil)
		}
		return validationError(FieldViolation{Field: "container", Rule: "existing_running_container"})
	}
	return nil
}

func (m *ExecManager) requireCurrent(generation string) error {
	if generation == "" || m.generations.CurrentGeneration() != generation {
		return publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	return nil
}

func (m *ExecManager) pruneTicketsLocked(now time.Time) {
	for identifier, ticket := range m.tickets {
		if !now.Before(ticket.expiresAt) {
			delete(m.tickets, identifier)
		}
	}
}

func parseExecProtocols(protocols []string) (string, error) {
	if len(protocols) != 2 {
		return "", validationError(FieldViolation{Field: "Sec-WebSocket-Protocol", Rule: "exec_protocol_and_one_ticket"})
	}
	seenProtocol := false
	ticket := ""
	for _, protocol := range protocols {
		switch {
		case protocol == ExecWebSocketProtocol && !seenProtocol:
			seenProtocol = true
		case strings.HasPrefix(protocol, ExecTicketPrefix) && ticket == "":
			ticket = strings.TrimPrefix(protocol, ExecTicketPrefix)
		default:
			return "", validationError(FieldViolation{Field: "Sec-WebSocket-Protocol", Rule: "exec_protocol_and_one_ticket"})
		}
	}
	if !seenProtocol || ticket == "" || len(ticket) > 256 || strings.ContainsAny(ticket, ", \t\r\n") {
		return "", validationError(FieldViolation{Field: "Sec-WebSocket-Protocol", Rule: "exec_protocol_and_one_ticket"})
	}
	return ticket, nil
}

func cloneExecInit(value ExecInit) ExecInit {
	value.Command = copyStrings(value.Command)
	return value
}

func translateExecSetupError(err error) *Error {
	if errors.Is(err, ErrExecTargetGone) {
		return publicError(CodeExecTargetGone, http.StatusNotFound, false, err)
	}
	return translateError(err)
}

func terminalForRemoteExit(result RemoteExecExit) ExecTerminal {
	if result.Err != nil {
		if errors.Is(result.Err, ErrExecTargetGone) {
			return terminalError(CodeExecTargetGone, false, 1008)
		}
		translated := translateError(result.Err)
		switch translated.Code {
		case CodeForbidden:
			return terminalError(CodeForbidden, false, 1008)
		case CodeAuthorizationUnavailable:
			return terminalError(CodeAuthorizationUnavailable, true, 1008)
		case CodeAuthenticationUnavailable:
			return terminalError(CodeAuthenticationUnavailable, true, 1011)
		case CodeClusterUnavailable, CodeUpstreamTimeout:
			return terminalError(CodeClusterUnavailable, true, 1011)
		default:
			return terminalError(CodeExecUpstreamError, false, 1011)
		}
	}
	if result.Signaled {
		return ExecTerminal{Type: ExecTerminalExit, Reason: ExecExitSignal, CloseCode: 1000}
	}
	if result.ExitCode != nil {
		code := *result.ExitCode
		if code < 0 || code > 255 {
			return terminalError(CodeExecUpstreamError, false, 1011)
		}
		copied := code
		reason := ExecExitCompleted
		if code != 0 {
			reason = ExecExitRemoteError
		}
		return ExecTerminal{Type: ExecTerminalExit, ExitCode: &copied, Reason: reason, CloseCode: 1000}
	}
	return ExecTerminal{Type: ExecTerminalExit, Reason: ExecExitCompleted, CloseCode: 1000}
}

func terminalError(code ErrorCode, retryable bool, closeCode int) ExecTerminal {
	return ExecTerminal{Type: ExecTerminalError, Code: code, Message: messages[code], Retryable: retryable, CloseCode: closeCode}
}

func statusForTerminal(code ErrorCode) int {
	switch code {
	case CodeForbidden:
		return http.StatusForbidden
	case CodeGenerationChanged:
		return http.StatusConflict
	case CodeLimitExceeded:
		return http.StatusTooManyRequests
	case CodeAuthorizationUnavailable, CodeAuthenticationUnavailable, CodeClusterUnavailable, CodeServerShutdown:
		return http.StatusServiceUnavailable
	case CodeUpstreamTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

var _ ExecService = (*ExecManager)(nil)
