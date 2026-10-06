package kubernetesruntime

import (
	"context"
	"github.com/fvmoraes/kubepeep/internal/services/actions"
)

func (backend *MutationBackend) UpdateDeploymentYAML(ctx context.Context, command actions.DeploymentYAMLCommand) (actions.MutationResult, error) {
	requestContext, cancel, client, err := backend.unary(ctx, command.Target)
	if err != nil {
		return actions.MutationResult{}, err
	}
	defer cancel()
	return client.UpdateDeploymentYAML(requestContext, command)
}
