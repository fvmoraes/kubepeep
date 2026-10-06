package actions

import (
	"context"
	"net/http"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

const MaximumDeploymentYAMLBytes = 2 << 20
const ActionUpdateDeployment Action = "updateDeployment"
const ConsequenceUpdateDeployment ConsequenceCode = "UPDATE_DEPLOYMENT"

type DeploymentYAMLRequest struct {
	Confirmation
	ExpectedUID             string `json:"expectedUid"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
	YAML                    string `json:"yaml"`
}

type DeploymentYAMLCommand struct {
	Target                  MutationTarget
	ExpectedUID             string
	ExpectedResourceVersion string
	YAML                    string
}

type DeploymentYAMLUpdater interface {
	UpdateDeploymentYAML(context.Context, DeploymentYAMLCommand) (MutationResult, error)
}

func (s *Service) UpdateDeploymentYAML(ctx context.Context, binding namespaces.SelectionBinding, route RouteTarget, request DeploymentYAMLRequest) (result ActionAcceptedDTO, returnedErr error) {
	if err := validateContext(ctx); err != nil {
		return result, err
	}
	if err := validateConfirmation(binding, route, request.Confirmation, ActionUpdateDeployment, ConsequenceUpdateDeployment, "deployments", "Deployment"); err != nil {
		return result, err
	}
	if err := validateOpaquePrecondition("expectedUid", request.ExpectedUID); err != nil {
		return result, err
	}
	if err := validateOpaquePrecondition("expectedResourceVersion", request.ExpectedResourceVersion); err != nil {
		return result, err
	}
	if len(request.YAML) == 0 || len(request.YAML) > MaximumDeploymentYAMLBytes {
		return result, validationError(FieldViolation{Field: "yaml", Rule: "bounded_deployment_document"})
	}
	if err := s.requireCurrent(binding.Generation); err != nil {
		return result, err
	}
	adapter, ok := s.adapter.(DeploymentYAMLUpdater)
	if !ok {
		return result, publicError(CodeInternal, http.StatusNotImplemented, false, nil)
	}
	target := mutationTarget(binding, request.Target)
	started := s.clock.Now().UTC()
	defer func() { recordAudit(ctx, s.audit, s.clock, started, "update_deployment", target, returnedErr) }()
	key, err := authorization.KeyForCapability(binding.Generation, target.Namespace, "deployments.update", target.Name)
	if err != nil {
		return result, translateError(err)
	}
	var mutation MutationResult
	err = s.guarded(ctx, binding.Generation, key, authorization.OperationMutation, func(operationContext context.Context) error {
		var operationErr error
		mutation, operationErr = adapter.UpdateDeploymentYAML(operationContext, DeploymentYAMLCommand{Target: target, ExpectedUID: request.ExpectedUID, ExpectedResourceVersion: request.ExpectedResourceVersion, YAML: request.YAML})
		return operationErr
	})
	if err != nil {
		return result, err
	}
	if mutation.ResourceVersion == "" {
		return result, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	return ActionAcceptedDTO{Accepted: true, Action: ActionUpdateDeployment, Target: request.Target, Generation: binding.Generation, ResourceVersion: &mutation.ResourceVersion}, nil
}
