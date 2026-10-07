package kubernetesruntime

import (
	"context"
	"github.com/fvmoraes/kubepeep/internal/services/actions"
)

func (backend *MutationBackend) ReadResourceYAML(ctx context.Context, target actions.MutationTarget, collection string) (string, error) {
	requestContext, cancel, client, err := backend.unary(ctx, target)
	if err != nil {
		return "", err
	}
	defer cancel()
	return client.ReadResourceYAML(requestContext, target, collection)
}

func (backend *MutationBackend) UpdateResourceYAML(ctx context.Context, command actions.ResourceYAMLCommand) (actions.MutationResult, error) {
	requestContext, cancel, client, err := backend.unary(ctx, command.Target)
	if err != nil {
		return actions.MutationResult{}, err
	}
	defer cancel()
	return client.UpdateResourceYAML(requestContext, command)
}
