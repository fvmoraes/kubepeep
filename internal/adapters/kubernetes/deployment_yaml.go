package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fvmoraes/kubepeep/internal/services/actions"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

// Decode exactly one Deployment and preserve Kubernetes optimistic concurrency.
// Parser errors are intentionally fixed messages: upstream errors can echo env.
func deploymentYAML(command actions.DeploymentYAMLCommand) (*appsv1.Deployment, error) {
	invalid := func() (*appsv1.Deployment, error) {
		return nil, apierrors.NewBadRequest("invalid Deployment YAML or target identity")
	}
	if command.Target.Kind != "Deployment" || command.ExpectedUID == "" || command.ExpectedResourceVersion == "" || len(command.YAML) == 0 || len(command.YAML) > actions.MaximumDeploymentYAMLBytes {
		return invalid()
	}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(command.YAML), 4096)
	var document json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return invalid()
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return invalid()
	}
	var value appsv1.Deployment
	// Strict parsing of the original document catches duplicate/unknown fields.
	if err := sigsyaml.UnmarshalStrict([]byte(command.YAML), &value); err != nil {
		return invalid()
	}
	if value.APIVersion != "apps/v1" || value.Kind != "Deployment" || value.Name != command.Target.Name || value.Namespace != command.Target.Namespace || string(value.UID) != command.ExpectedUID || value.ResourceVersion != command.ExpectedResourceVersion {
		return invalid()
	}
	return &value, nil
}

func (client *ActionClient) UpdateDeploymentYAML(ctx context.Context, command actions.DeploymentYAMLCommand) (actions.MutationResult, error) {
	if client == nil || client.unary == nil {
		return actions.MutationResult{}, errActionsClientUnavailable
	}
	value, err := deploymentYAML(command)
	if err != nil {
		return actions.MutationResult{}, fmt.Errorf("decode deployment: %w", err)
	}
	updated, err := client.unary.AppsV1().Deployments(command.Target.Namespace).Update(ctx, value, metav1.UpdateOptions{FieldValidation: "Strict"})
	if err != nil {
		// Preserve conflict/denial classification without echoing user values.
		if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
			return actions.MutationResult{}, apierrors.NewBadRequest("Kubernetes rejected the Deployment document")
		}
		if apierrors.IsConflict(err) {
			return actions.MutationResult{}, apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, command.Target.Name, fmt.Errorf("resource changed; reload before saving"))
		}
		return actions.MutationResult{}, fmt.Errorf("update deployment: %w", err)
	}
	return actions.MutationResult{ResourceVersion: updated.ResourceVersion}, nil
}
