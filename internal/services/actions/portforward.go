package actions

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type netLoopbackBinder struct{}

func (netLoopbackBinder) Listen(ctx context.Context, port int) (net.Listener, error) {
	var config net.ListenConfig
	return config.Listen(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

type portForwardSession struct {
	dto      PortForwardDTO
	target   MutationTarget
	cancel   context.CancelFunc
	listener net.Listener
	handle   PortForwardHandle
	timer    *time.Timer
	stopOnce sync.Once
}

type portForwardStart struct {
	generation string
	cancel     context.CancelFunc
	stale      bool
}

func (s *portForwardSession) stop() {
	s.stopOnce.Do(func() {
		if s.timer != nil {
			s.timer.Stop()
		}
		s.cancel()
		_ = s.listener.Close()
		_ = s.handle.Close()
	})
}

type PortForwardManager struct {
	authorizer  AuthorizationService
	generations GenerationReader
	adapter     PortForwardAdapter
	binder      LoopbackBinder
	audit       AuditSink
	clock       Clock
	identifiers IdentifierSource
	idempotency *idempotencyRegistry[PortForwardDTO]
	duration    time.Duration
	retention   time.Duration
	setupLimit  time.Duration

	mu        sync.Mutex
	sessions  map[string]*portForwardSession
	starts    map[uint64]portForwardStart
	nextStart uint64
	shutdown  bool
}

func NewPortForwardService(authorizer AuthorizationService, generations GenerationReader, adapter PortForwardAdapter, audit AuditSink) (*PortForwardManager, error) {
	return newPortForwardService(authorizer, generations, adapter, netLoopbackBinder{}, audit, systemClock{}, cryptoIdentifiers{}, DefaultPortForwardDuration, DefaultPortForwardRetention, DefaultPortForwardSetup, DefaultIdempotencyTTL)
}

func newPortForwardService(authorizer AuthorizationService, generations GenerationReader, adapter PortForwardAdapter, binder LoopbackBinder, audit AuditSink, clock Clock, identifiers IdentifierSource, duration, retention, setupLimit, idempotencyTTL time.Duration) (*PortForwardManager, error) {
	if authorizer == nil || generations == nil || adapter == nil || binder == nil || audit == nil || clock == nil || identifiers == nil {
		return nil, errors.New("actions: port-forward dependencies are required")
	}
	if duration <= 0 || retention <= 0 || setupLimit <= 0 || idempotencyTTL <= 0 {
		return nil, errors.New("actions: port-forward durations must be positive")
	}
	return &PortForwardManager{
		authorizer:  authorizer,
		generations: generations,
		adapter:     adapter,
		binder:      binder,
		audit:       audit,
		clock:       clock,
		identifiers: identifiers,
		idempotency: newIdempotencyRegistry[PortForwardDTO](clock, idempotencyTTL),
		duration:    duration,
		retention:   retention,
		setupLimit:  setupLimit,
		sessions:    make(map[string]*portForwardSession),
		starts:      make(map[uint64]portForwardStart),
	}, nil
}

func (m *PortForwardManager) Create(ctx context.Context, binding namespaces.SelectionBinding, route RouteTarget, idempotencyKey string, request PortForwardCreateRequest) (PortForwardDTO, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validatePortForward(binding, route, request); err != nil {
		return PortForwardDTO{}, false, err
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return PortForwardDTO{}, false, err
	}
	if err := m.requireCurrent(binding.Generation); err != nil {
		return PortForwardDTO{}, false, err
	}
	bodyHash, err := canonicalBodyHash(request)
	if err != nil {
		return PortForwardDTO{}, false, err
	}
	identity := idempotencyIdentity{
		Method:           http.MethodPost,
		Path:             canonicalPodPath(route, "port-forward"),
		ClusterProfileID: binding.ClusterProfileID,
		Generation:       binding.Generation,
		BodyHash:         bodyHash,
	}
	dto, replayed, err := m.idempotency.Do(ctx, idempotencyKey, identity, func() (PortForwardDTO, error) {
		return m.createOnce(ctx, binding, request)
	})
	return copyPortForwardDTO(dto), replayed, err
}

func (m *PortForwardManager) createOnce(ctx context.Context, binding namespaces.SelectionBinding, request PortForwardCreateRequest) (dto PortForwardDTO, returnedErr error) {
	target := mutationTarget(binding, request.Target)
	started := m.clock.Now().UTC()
	defer func() { recordAudit(m.audit, m.clock, started, "port_forward_start", target, returnedErr) }()
	key, err := authorization.KeyForCapability(binding.Generation, target.Namespace, "pods.portforward.create", target.Name)
	if err != nil {
		return dto, translateError(err)
	}

	var (
		operationErr error
		listener     net.Listener
		handle       PortForwardHandle
		lifetimeCtx  context.Context
		cancel       context.CancelFunc
		identifier   string
		reserved     bool
		startID      uint64
	)
	guardResult, guardErr := m.authorizer.Guard(ctx, key, authorization.OperationUpgrade, func(context.Context) error {
		if err := ctx.Err(); err != nil {
			operationErr = err
			return err
		}
		startID, operationErr = m.reserve(binding.Generation)
		if operationErr != nil {
			return operationErr
		}
		reserved = true
		lifetimeCtx, cancel = context.WithCancel(context.Background())
		if err := m.attachReservation(startID, cancel); err != nil {
			operationErr = err
			return err
		}
		if err := m.requireCurrent(binding.Generation); err != nil {
			operationErr = err
			return err
		}
		identifier, operationErr = m.identifiers.NewID("pf_")
		if operationErr != nil {
			operationErr = publicError(CodeInternal, http.StatusInternalServerError, false, operationErr)
			return operationErr
		}
		localPort := 0
		if request.LocalPort != nil {
			localPort = *request.LocalPort
		}
		setupContext, setupCancel := context.WithTimeout(lifetimeCtx, m.setupLimit)
		defer setupCancel()
		listener, operationErr = m.binder.Listen(setupContext, localPort)
		if operationErr != nil {
			operationErr = translateBindError(setupContext, operationErr)
			return operationErr
		}
		tcpAddress, ok := listener.Addr().(*net.TCPAddr)
		if !ok || tcpAddress.IP == nil || !tcpAddress.IP.Equal(net.ParseIP("127.0.0.1")) || tcpAddress.Port < 1024 || tcpAddress.Port > 65535 {
			operationErr = publicError(CodeInternal, http.StatusInternalServerError, false, nil)
			_ = listener.Close()
			return operationErr
		}
		handle, operationErr = m.adapter.Start(setupContext, lifetimeCtx, PortForwardCommand{Target: target, RemotePort: request.RemotePort}, listener)
		if operationErr != nil {
			cancel()
			_ = listener.Close()
			return operationErr
		}
		if handle == nil {
			cancel()
			_ = listener.Close()
			operationErr = publicError(CodeInternal, http.StatusInternalServerError, false, nil)
			return operationErr
		}
		if err := m.requireCurrent(binding.Generation); err != nil {
			operationErr = err
			cancel()
			_ = listener.Close()
			_ = handle.Close()
			return operationErr
		}
		return nil
	})
	if guardErr != nil {
		if listener != nil && handle == nil {
			_ = listener.Close()
		}
		if reserved {
			m.releaseReservation(startID)
		}
		if currentErr := m.requireCurrent(binding.Generation); currentErr != nil {
			return dto, currentErr
		}
		m.mu.Lock()
		shutdown := m.shutdown
		m.mu.Unlock()
		if shutdown {
			return dto, publicError(CodeServerShutdown, http.StatusServiceUnavailable, true, nil)
		}
		if guardResult.Executed && operationErr != nil {
			return dto, translateError(operationErr)
		}
		return dto, translateError(guardErr)
	}
	if !guardResult.Executed || !reserved || listener == nil || handle == nil || cancel == nil {
		if reserved {
			m.releaseReservation(startID)
		}
		return dto, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}

	address := listener.Addr().(*net.TCPAddr)
	now := m.clock.Now().UTC()
	dto = PortForwardDTO{
		ID:               identifier,
		ClusterProfileID: binding.ClusterProfileID,
		Context:          binding.Context,
		Generation:       binding.Generation,
		Namespace:        target.Namespace,
		Pod:              target.Name,
		RemotePort:       request.RemotePort,
		LocalAddress:     "127.0.0.1",
		LocalPort:        address.Port,
		Status:           PortForwardActive,
		CreatedAt:        now,
		ExpiresAt:        now.Add(m.duration),
	}
	session := &portForwardSession{dto: dto, target: target, cancel: cancel, listener: listener, handle: handle}
	m.mu.Lock()
	start := m.starts[startID]
	delete(m.starts, startID)
	if m.shutdown || start.stale || m.generations.CurrentGeneration() != binding.Generation {
		m.mu.Unlock()
		session.stop()
		return PortForwardDTO{}, publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	if _, collision := m.sessions[identifier]; collision {
		m.mu.Unlock()
		session.stop()
		return PortForwardDTO{}, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	m.sessions[identifier] = session
	session.timer = time.AfterFunc(m.duration, func() { m.finish(identifier, PortForwardExpired) })
	m.mu.Unlock()
	go m.monitor(session)
	return copyPortForwardDTO(dto), nil
}

func (m *PortForwardManager) reserve(generation string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(m.clock.Now())
	if m.shutdown {
		return 0, publicError(CodeServerShutdown, http.StatusServiceUnavailable, true, nil)
	}
	active := len(m.starts)
	for _, session := range m.sessions {
		if session.dto.Status == PortForwardActive {
			active++
		}
	}
	if active >= MaximumPortForwardSessions {
		return 0, publicError(CodeLimitExceeded, http.StatusTooManyRequests, false, nil)
	}
	m.nextStart++
	identifier := m.nextStart
	m.starts[identifier] = portForwardStart{generation: generation}
	return identifier, nil
}

func (m *PortForwardManager) attachReservation(identifier uint64, cancel context.CancelFunc) error {
	m.mu.Lock()
	start, ok := m.starts[identifier]
	if !ok {
		m.mu.Unlock()
		cancel()
		return publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	start.cancel = cancel
	m.starts[identifier] = start
	shutdown := m.shutdown
	stale := start.stale
	m.mu.Unlock()
	if shutdown || stale {
		cancel()
		if shutdown {
			return publicError(CodeServerShutdown, http.StatusServiceUnavailable, true, nil)
		}
		return publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	return nil
}

func (m *PortForwardManager) releaseReservation(identifier uint64) {
	m.mu.Lock()
	start := m.starts[identifier]
	delete(m.starts, identifier)
	m.mu.Unlock()
	if start.cancel != nil {
		start.cancel()
	}
}

func (m *PortForwardManager) monitor(session *portForwardSession) {
	err := session.handle.Wait()
	status := PortForwardFailed
	if errors.Is(err, ErrPortForwardPodGone) {
		status = PortForwardPodGone
	}
	m.finish(session.dto.ID, status)
}

func (m *PortForwardManager) finish(identifier string, status PortForwardStatus) {
	m.mu.Lock()
	session, ok := m.sessions[identifier]
	if !ok || session.dto.Status != PortForwardActive {
		m.mu.Unlock()
		return
	}
	now := m.clock.Now().UTC()
	reason := string(status)
	session.dto.Status = status
	session.dto.EndedAt = &now
	session.dto.EndReason = &reason
	target := session.target
	m.mu.Unlock()
	session.stop()
	recordAudit(m.audit, m.clock, session.dto.CreatedAt, "port_forward_end", target, terminalStatusError(status))
}

func terminalStatusError(status PortForwardStatus) error {
	if status == PortForwardClosed || status == PortForwardExpired || status == PortForwardPodGone {
		return nil
	}
	return publicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, nil)
}

func (m *PortForwardManager) List(binding namespaces.SelectionBinding) ([]PortForwardDTO, error) {
	if err := validateBinding(binding); err != nil {
		return nil, err
	}
	if err := m.requireCurrent(binding.Generation); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(m.clock.Now())
	rows := make([]PortForwardDTO, 0, len(m.sessions))
	for _, session := range m.sessions {
		if session.dto.Generation == binding.Generation {
			rows = append(rows, copyPortForwardDTO(session.dto))
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
	return rows, nil
}

func (m *PortForwardManager) Close(ctx context.Context, binding namespaces.SelectionBinding, identifier string, request PortForwardDeleteRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateBinding(binding); err != nil {
		return err
	}
	if err := validateSessionID(identifier, "pf_"); err != nil {
		return err
	}
	if !request.Confirmed {
		return validationError(FieldViolation{Field: "confirmed", Rule: "must_be_true"})
	}
	if request.ExpectedGeneration != binding.Generation {
		return publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	if err := m.requireCurrent(binding.Generation); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return translateError(err)
	}
	m.mu.Lock()
	m.pruneLocked(m.clock.Now())
	session, ok := m.sessions[identifier]
	if !ok {
		m.mu.Unlock()
		return publicError(CodeNotFound, http.StatusNotFound, false, nil)
	}
	if session.dto.Generation != binding.Generation {
		m.mu.Unlock()
		return publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	if session.dto.Status != PortForwardActive {
		m.mu.Unlock()
		return publicError(CodeSessionGone, http.StatusGone, false, nil)
	}
	now := m.clock.Now().UTC()
	reason := string(PortForwardClosed)
	session.dto.Status = PortForwardClosed
	session.dto.EndedAt = &now
	session.dto.EndReason = &reason
	target := session.target
	m.mu.Unlock()
	session.stop()
	recordAudit(m.audit, m.clock, now, "port_forward_end", target, nil)
	return nil
}

func (m *PortForwardManager) OnGeneration(current string) {
	m.mu.Lock()
	stale := make([]*portForwardSession, 0)
	startCancels := make([]context.CancelFunc, 0)
	for identifier, start := range m.starts {
		if start.generation == current {
			continue
		}
		start.stale = true
		m.starts[identifier] = start
		if start.cancel != nil {
			startCancels = append(startCancels, start.cancel)
		}
	}
	for identifier, session := range m.sessions {
		if session.dto.Generation != current {
			delete(m.sessions, identifier)
			stale = append(stale, session)
		}
	}
	m.mu.Unlock()
	for _, cancel := range startCancels {
		cancel()
	}
	for _, session := range stale {
		session.stop()
		recordAudit(m.audit, m.clock, session.dto.CreatedAt, "port_forward_end", session.target, publicError(CodeGenerationChanged, http.StatusConflict, false, nil))
	}
}

func (m *PortForwardManager) Shutdown() {
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		return
	}
	m.shutdown = true
	startCancels := make([]context.CancelFunc, 0, len(m.starts))
	for identifier, start := range m.starts {
		start.stale = true
		m.starts[identifier] = start
		if start.cancel != nil {
			startCancels = append(startCancels, start.cancel)
		}
	}
	sessions := make([]*portForwardSession, 0, len(m.sessions))
	for identifier, session := range m.sessions {
		delete(m.sessions, identifier)
		sessions = append(sessions, session)
	}
	m.mu.Unlock()
	for _, cancel := range startCancels {
		cancel()
	}
	for _, session := range sessions {
		session.stop()
		recordAudit(m.audit, m.clock, session.dto.CreatedAt, "port_forward_end", session.target, publicError(CodeServerShutdown, http.StatusServiceUnavailable, true, nil))
	}
}

func (m *PortForwardManager) requireCurrent(generation string) error {
	if generation == "" || m.generations.CurrentGeneration() != generation {
		return publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	return nil
}

func (m *PortForwardManager) pruneLocked(now time.Time) {
	for identifier, session := range m.sessions {
		if session.dto.EndedAt != nil && !now.Before(session.dto.EndedAt.Add(m.retention)) {
			delete(m.sessions, identifier)
		}
	}
}

func copyPortForwardDTO(value PortForwardDTO) PortForwardDTO {
	if value.EndedAt != nil {
		endedAt := *value.EndedAt
		value.EndedAt = &endedAt
	}
	if value.EndReason != nil {
		reason := *value.EndReason
		value.EndReason = &reason
	}
	return value
}

func translateBindError(ctx context.Context, err error) error {
	if contextError := ctx.Err(); contextError != nil {
		return translateError(contextError)
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return publicError(CodeConflict, http.StatusConflict, false, errors.Join(errLocalPortUnavailable, err))
	}
	return publicError(CodeInternal, http.StatusInternalServerError, false, err)
}

var _ PortForwardService = (*PortForwardManager)(nil)
