package actions

import (
	"context"
	"net/http"
	"slices"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
)

const MaximumResourceYAMLBytes = 2 << 20
const ActionUpdateResource Action = "updateResource"
const ConsequenceUpdateResource ConsequenceCode = "UPDATE_RESOURCE"

type ResourceYAMLRequest = DeploymentYAMLRequest

type ResourceYAMLCommand struct {
	DeploymentYAMLCommand
	Collection string
}

type ResourceYAMLDocument struct {
	YAML             string `json:"yaml"`
	Kind             string `json:"kind"`
	UpdateCapability string `json:"updateCapability"`
	Generation       string `json:"generation"`
}

type ResourceYAMLAdapter interface {
	ReadResourceYAML(context.Context, MutationTarget, string) (string, error)
	UpdateResourceYAML(context.Context, ResourceYAMLCommand) (MutationResult, error)
}

func yamlResource(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, route RouteTarget) (resourcecatalog.Resource, error) {
	if err := validateBinding(binding); err != nil {
		return resourcecatalog.Resource{}, err
	}
	resource, ok := resourcecatalog.Lookup(route.Kind)
	if !ok || !resourcecatalog.ValidName(route.Name) || (resource.Namespaced && len(kvalidation.IsDNS1123Label(route.Namespace)) != 0) || (!resource.Namespaced && route.Namespace != "") {
		return resourcecatalog.Resource{}, validationError(FieldViolation{Field: "path", Rule: "registered_resource_target"})
	}
	if resource.Namespaced && !slices.Contains(resolution.Namespaces, route.Namespace) {
		return resourcecatalog.Resource{}, publicError(CodeForbidden, http.StatusForbidden, false, nil)
	}
	return resource, nil
}

func yamlKey(binding namespaces.SelectionBinding, route RouteTarget, resource resourcecatalog.Resource, verb string) authorization.Key {
	return authorization.Key{Generation: binding.Generation, Namespace: route.Namespace, APIGroup: resource.Group, Resource: resource.Resource, Verb: verb, ResourceName: route.Name}
}

func (s *Service) ReadResourceYAML(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, route RouteTarget) (document ResourceYAMLDocument, err error) {
	if err = validateContext(ctx); err != nil {
		return
	}
	resource, err := yamlResource(binding, resolution, route)
	if err != nil {
		return document, err
	}
	adapter, ok := s.adapter.(ResourceYAMLAdapter)
	if !ok {
		return document, publicError(CodeInternal, http.StatusNotImplemented, false, nil)
	}
	target := mutationTarget(binding, ActionTargetDTO{Namespace: route.Namespace, Kind: resource.Kind, Name: route.Name})
	err = s.guarded(ctx, binding.Generation, yamlKey(binding, route, resource, "get"), authorization.OperationRead, func(operationContext context.Context) error {
		var readErr error
		document.YAML, readErr = adapter.ReadResourceYAML(operationContext, target, route.Kind)
		return readErr
	})
	if err != nil {
		return ResourceYAMLDocument{}, err
	}
	document.Kind, document.UpdateCapability, document.Generation = resource.Kind, resource.UpdateCapability(), binding.Generation
	return document, nil
}

func (s *Service) UpdateResourceYAML(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, route RouteTarget, request ResourceYAMLRequest) (result ActionAcceptedDTO, returnedErr error) {
	if err := validateContext(ctx); err != nil {
		return result, err
	}
	resource, err := yamlResource(binding, resolution, route)
	if err != nil {
		return result, err
	}
	if !request.Confirmed || request.Action != ActionUpdateResource || request.ConsequenceCode != ConsequenceUpdateResource || request.ExpectedGeneration != binding.Generation || request.Target.ClusterProfileID != binding.ClusterProfileID || request.Target.Context != binding.Context || request.Target.Namespace != route.Namespace || request.Target.Name != route.Name || request.Target.Kind != resource.Kind {
		return result, validationError(FieldViolation{Field: "confirmation", Rule: "matches_active_target_and_generation"})
	}
	if err := validateOpaquePrecondition("expectedUid", request.ExpectedUID); err != nil {
		return result, err
	}
	if err := validateOpaquePrecondition("expectedResourceVersion", request.ExpectedResourceVersion); err != nil {
		return result, err
	}
	if len(request.YAML) == 0 || len(request.YAML) > MaximumResourceYAMLBytes {
		return result, validationError(FieldViolation{Field: "yaml", Rule: "bounded_resource_document"})
	}
	adapter, ok := s.adapter.(ResourceYAMLAdapter)
	if !ok {
		return result, publicError(CodeInternal, http.StatusNotImplemented, false, nil)
	}
	target := mutationTarget(binding, request.Target)
	started := s.clock.Now().UTC()
	defer func() { recordAudit(ctx, s.audit, s.clock, started, "update_resource_yaml", target, returnedErr) }()
	var mutation MutationResult
	err = s.guarded(ctx, binding.Generation, yamlKey(binding, route, resource, "update"), authorization.OperationMutation, func(operationContext context.Context) error {
		var updateErr error
		mutation, updateErr = adapter.UpdateResourceYAML(operationContext, ResourceYAMLCommand{Collection: route.Kind, DeploymentYAMLCommand: DeploymentYAMLCommand{Target: target, ExpectedUID: request.ExpectedUID, ExpectedResourceVersion: request.ExpectedResourceVersion, YAML: request.YAML}})
		return updateErr
	})
	if err != nil {
		return result, err
	}
	if mutation.ResourceVersion == "" {
		return result, publicError(CodeInternal, http.StatusInternalServerError, false, nil)
	}
	return ActionAcceptedDTO{Accepted: true, Action: ActionUpdateResource, Target: request.Target, Generation: binding.Generation, ResourceVersion: &mutation.ResourceVersion}, nil
}
