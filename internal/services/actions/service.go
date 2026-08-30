package actions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type Service struct {
	authorizer  AuthorizationService
	generations GenerationReader
	adapter     KubernetesActions
	audit       AuditSink
	clock       Clock
	restarts    *idempotencyRegistry[ActionAcceptedDTO]

	mu             sync.Mutex
	nextOperation  uint64
	operations     map[uint64]generationOperation
	shutdown       bool
	operationLimit time.Duration
}

type generationOperation struct {
	generation string
	cancel     context.CancelFunc
}

func NewActionService(authorizer AuthorizationService, generations GenerationReader, adapter KubernetesActions, audit AuditSink) (*Service, error) {
	return newActionService(authorizer, generations, adapter, audit, systemClock{}, DefaultActionTimeout, DefaultIdempotencyTTL)
}

func newActionService(authorizer AuthorizationService, generations GenerationReader, adapter KubernetesActions, audit AuditSink, clock Clock, operationLimit, idempotencyTTL time.Duration) (*Service, error) {
	if authorizer == nil || generations == nil || adapter == nil || audit == nil || clock == nil {
		return nil, errors.New("actions: authorizer, generation reader, adapter, audit sink, and clock are required")
	}
	if operationLimit <= 0 || idempotencyTTL <= 0 {
		return nil, errors.New("actions: operation and idempotency durations must be positive")
	}
	return &Service{
		authorizer:     authorizer,
		generations:    generations,
		adapter:        adapter,
		audit:          audit,
		clock:          clock,
		restarts:       newIdempotencyRegistry[ActionAcceptedDTO](clock, idempotencyTTL),
		operations:     make(map[uint64]generationOperation),
		operationLimit: operationLimit,
	}, nil
}

func (s *Service) Restart(ctx context.Context, binding namespaces.SelectionBinding, route RouteTarget, idempotencyKey string, request RestartRequest) (ActionAcceptedDTO, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateRestart(binding, route, request); err != nil {
		return ActionAcceptedDTO{}, false, err
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return ActionAcceptedDTO{}, false, err
	}
	if err := s.requireCurrent(binding.Generation); err != nil {
		return ActionAcceptedDTO{}, false, err
	}
	bodyHash, err := canonicalBodyHash(request)
	if err != nil {
		return ActionAcceptedDTO{}, false, err
	}
	identity := idempotencyIdentity{
		Method:           http.MethodPost,
		Path:             canonicalWorkloadPath(route, "restart"),
		ClusterProfileID: binding.ClusterProfileID,
		Generation:       binding.Generation,
		BodyHash:         bodyHash,
	}
	return s.restarts.Do(ctx, idempotencyKey, identity, func() (ActionAcceptedDTO, error) {
		return s.restartOnce(ctx, binding, request)
	})
}

func (s *Service) restartOnce(ctx context.Context, binding namespaces.SelectionBinding, request RestartRequest) (result ActionAcceptedDTO, returnedErr error) {
	target := mutationTarget(binding, request.Target)
	started := s.clock.Now().UTC()
	defer func() { recordAudit(s.audit, s.clock, started, "restart", target, returnedErr) }()
	key, err := authorization.KeyForCapability(binding.Generation, target.Namespace, "deployments.restart", target.Name)
	if err != nil {
		return result, translateError(err)
	}
	var mutation MutationResult
	err = s.guarded(ctx, binding.Generation, key, authorization.OperationMutation, func(operationContext context.Context) error {
		var operationErr error
		mutation, operationErr = s.adapter.RestartDeployment(operationContext, RestartDeploymentCommand{
			Target:                  target,
			ExpectedResourceVersion: request.ExpectedResourceVersion,
			RestartedAt:             s.clock.Now().UTC(),
		})
		return operationErr
	})
	if err != nil {
		return result, err
	}
	if mutation.ResourceVersion == "" {
		return result, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	resourceVersion := mutation.ResourceVersion
	return ActionAcceptedDTO{
		Accepted:        true,
		Action:          ActionRestart,
		Target:          request.Target,
		Generation:      binding.Generation,
		ResourceVersion: &resourceVersion,
	}, nil
}

func (s *Service) Scale(ctx context.Context, binding namespaces.SelectionBinding, route RouteTarget, request ScaleRequest) (result ScaleResultDTO, returnedErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateScale(binding, route, request); err != nil {
		return result, err
	}
	if err := s.requireCurrent(binding.Generation); err != nil {
		return result, err
	}
	target := mutationTarget(binding, request.Target)
	started := s.clock.Now().UTC()
	defer func() { recordAudit(s.audit, s.clock, started, "scale", target, returnedErr) }()
	capabilityID := "deployments.scale"
	if route.Kind == "statefulsets" {
		capabilityID = "statefulsets.scale"
	}
	key, err := authorization.KeyForCapability(binding.Generation, target.Namespace, capabilityID, target.Name)
	if err != nil {
		return result, translateError(err)
	}
	var mutation MutationResult
	err = s.guarded(ctx, binding.Generation, key, authorization.OperationMutation, func(operationContext context.Context) error {
		var operationErr error
		commandTarget := target
		commandTarget.Kind = route.Kind
		mutation, operationErr = s.adapter.UpdateScale(operationContext, ScaleCommand{
			Target:                  commandTarget,
			Replicas:                int32(request.Replicas),
			ExpectedResourceVersion: request.ExpectedResourceVersion,
		})
		return operationErr
	})
	if err != nil {
		return result, err
	}
	if mutation.ResourceVersion == "" {
		return result, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	resourceVersion := mutation.ResourceVersion
	return ScaleResultDTO{
		Accepted:        true,
		Action:          ActionScale,
		Target:          request.Target,
		Generation:      binding.Generation,
		ResourceVersion: &resourceVersion,
		Replicas:        int32(request.Replicas),
	}, nil
}

func (s *Service) DeletePod(ctx context.Context, binding namespaces.SelectionBinding, route RouteTarget, request PodDeleteRequest) (result ActionAcceptedDTO, returnedErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateDeletePod(binding, route, request); err != nil {
		return result, err
	}
	if err := s.requireCurrent(binding.Generation); err != nil {
		return result, err
	}
	target := mutationTarget(binding, request.Target)
	started := s.clock.Now().UTC()
	defer func() { recordAudit(s.audit, s.clock, started, "delete_pod", target, returnedErr) }()
	key, err := authorization.KeyForCapability(binding.Generation, target.Namespace, "pods.delete", target.Name)
	if err != nil {
		return result, translateError(err)
	}
	var mutation MutationResult
	err = s.guarded(ctx, binding.Generation, key, authorization.OperationMutation, func(operationContext context.Context) error {
		var operationErr error
		mutation, operationErr = s.adapter.DeletePod(operationContext, DeletePodCommand{
			Target:                  target,
			ExpectedUID:             request.ExpectedUID,
			ExpectedResourceVersion: request.ExpectedResourceVersion,
		})
		return operationErr
	})
	if err != nil {
		return result, err
	}
	var resourceVersion *string
	if mutation.ResourceVersion != "" {
		value := mutation.ResourceVersion
		resourceVersion = &value
	}
	return ActionAcceptedDTO{
		Accepted:        true,
		Action:          ActionDeletePod,
		Target:          request.Target,
		Generation:      binding.Generation,
		ResourceVersion: resourceVersion,
	}, nil
}

func (s *Service) guarded(ctx context.Context, generation string, key authorization.Key, kind authorization.OperationKind, operation func(context.Context) error) error {
	var operationErr error
	guardResult, guardErr := s.authorizer.Guard(ctx, key, kind, func(context.Context) error {
		if err := ctx.Err(); err != nil {
			operationErr = err
			return err
		}
		operationContext, release, err := s.beginOperation(generation)
		if err != nil {
			operationErr = err
			return err
		}
		defer release()
		if err := s.requireCurrent(generation); err != nil {
			operationErr = err
			return err
		}
		operationErr = operation(operationContext)
		if currentErr := s.requireCurrent(generation); currentErr != nil {
			operationErr = currentErr
		}
		return operationErr
	})
	if guardErr != nil {
		if guardResult.Executed && operationErr != nil {
			return translateError(operationErr)
		}
		return translateError(guardErr)
	}
	if !guardResult.Executed {
		return publicError(CodeAuthorizationUnavailable, http.StatusServiceUnavailable, true, nil)
	}
	return nil
}

func (s *Service) beginOperation(generation string) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shutdown {
		return nil, nil, publicError(CodeServerShutdown, http.StatusServiceUnavailable, true, nil)
	}
	s.nextOperation++
	id := s.nextOperation
	operationContext, cancel := context.WithTimeout(context.Background(), s.operationLimit)
	s.operations[id] = generationOperation{generation: generation, cancel: cancel}
	return operationContext, func() {
		cancel()
		s.mu.Lock()
		delete(s.operations, id)
		s.mu.Unlock()
	}, nil
}

func (s *Service) requireCurrent(generation string) error {
	if generation == "" || s.generations.CurrentGeneration() != generation {
		return publicError(CodeGenerationChanged, http.StatusConflict, false, nil)
	}
	return nil
}

// OnGeneration is called by the selection coordinator hook. It cancels all
// in-flight operations owned by an older generation.
func (s *Service) OnGeneration(current string) {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0)
	for _, operation := range s.operations {
		if operation.generation != current {
			cancels = append(cancels, operation.cancel)
		}
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Service) Shutdown() {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return
	}
	s.shutdown = true
	cancels := make([]context.CancelFunc, 0, len(s.operations))
	for _, operation := range s.operations {
		cancels = append(cancels, operation.cancel)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Service) String() string {
	return fmt.Sprintf("actions.Service(%d active)", s.activeOperations())
}

func (s *Service) activeOperations() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.operations)
}

var _ ActionService = (*Service)(nil)
var _ = errors.Is
