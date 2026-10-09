package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	sigsyaml "sigs.k8s.io/yaml"
)

func (client *ActionClient) yamlClient(target actions.MutationTarget, collection string, versions ...string) (dynamic.ResourceInterface, resourcecatalog.Resource, error) {
	resource, ok := resourcecatalog.Lookup(collection)
	if len(versions) > 0 {
		if !slices.Contains(resourcecatalog.Versions(resource), versions[0]) {
			return nil, resource, apierrors.NewBadRequest("unsupported resource version")
		}
		resource.Version = versions[0]
	}
	if client == nil || client.dynamic == nil {
		return nil, resource, errActionsClientUnavailable
	}
	if !ok || target.Kind != resource.Kind || !resourcecatalog.ValidName(target.Name) || resource.Namespaced != (target.Namespace != "") {
		return nil, resource, apierrors.NewBadRequest("invalid resource target")
	}
	endpoint := client.dynamic.Resource(schema.GroupVersionResource{Group: resource.Group, Version: resource.Version, Resource: resource.Resource})
	if resource.Namespaced {
		return endpoint.Namespace(target.Namespace), resource, nil
	}
	return endpoint, resource, nil
}

// ReadResourceYAML is an explicit, uncached, exact-object read. In particular,
// Secrets never pass through list projections, indexes or a shared YAML cache.
func (client *ActionClient) ReadResourceYAML(ctx context.Context, target actions.MutationTarget, collection string) (string, error) {
	endpoint, resource, err := client.yamlClient(target, collection)
	if err != nil {
		return "", err
	}
	value, err := endpoint.Get(ctx, target.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) && resource.Group == resourcecatalog.GatewayGroup {
		for _, version := range resourcecatalog.Versions(resource)[1:] {
			endpoint, resource, err = client.yamlClient(target, collection, version)
			if err != nil {
				break
			}
			value, err = endpoint.Get(ctx, target.Name, metav1.GetOptions{})
			if !apierrors.IsNotFound(err) {
				break
			}
		}
	}
	if err != nil {
		return "", fmt.Errorf("read resource YAML: %w", err)
	}
	value.SetAPIVersion(resource.APIVersion())
	value.SetKind(resource.Kind)
	value.SetManagedFields(nil)
	document, err := sigsyaml.Marshal(value.Object)
	if err != nil {
		return "", errors.New("could not encode resource YAML")
	}
	if len(document) > actions.MaximumResourceYAMLBytes {
		return "", apierrors.NewRequestEntityTooLargeError("resource YAML exceeds the editor limit")
	}
	return string(document), nil
}

func resourceYAML(command actions.ResourceYAMLCommand, resource resourcecatalog.Resource) (*unstructured.Unstructured, error) {
	invalid := func() (*unstructured.Unstructured, error) {
		return nil, apierrors.NewBadRequest("invalid YAML or resource identity")
	}
	if len(command.YAML) == 0 || len(command.YAML) > actions.MaximumResourceYAMLBytes || command.ExpectedUID == "" || command.ExpectedResourceVersion == "" {
		return invalid()
	}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(command.YAML), 4096)
	var document, extra json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return invalid()
	}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return invalid()
	}
	// Decode the original text strictly so duplicate keys cannot silently win.
	encoded, err := sigsyaml.YAMLToJSONStrict([]byte(command.YAML))
	if err != nil {
		return invalid()
	}
	var value unstructured.Unstructured
	if err := value.UnmarshalJSON(encoded); err != nil {
		return invalid()
	}
	if value.GetAPIVersion() != resource.APIVersion() || value.GetKind() != resource.Kind || value.GetName() != command.Target.Name || value.GetNamespace() != command.Target.Namespace || string(value.GetUID()) != command.ExpectedUID || value.GetResourceVersion() != command.ExpectedResourceVersion {
		return invalid()
	}
	return &value, nil
}

func (client *ActionClient) UpdateResourceYAML(ctx context.Context, command actions.ResourceYAMLCommand) (actions.MutationResult, error) {
	if len(command.YAML) == 0 || len(command.YAML) > actions.MaximumResourceYAMLBytes {
		return actions.MutationResult{}, apierrors.NewBadRequest("invalid YAML or resource identity")
	}
	endpoint, resource, err := client.yamlClient(command.Target, command.Collection)
	if err != nil {
		return actions.MutationResult{}, err
	}
	if resource.Group == resourcecatalog.GatewayGroup {
		var header struct {
			APIVersion string `json:"apiVersion"`
		}
		// Read only the routing version here. resourceYAML below validates the
		// complete document strictly, including duplicate keys and identity.
		if err := sigsyaml.Unmarshal([]byte(command.YAML), &header); err != nil {
			return actions.MutationResult{}, apierrors.NewBadRequest("invalid YAML")
		}
		groupVersion, parseErr := schema.ParseGroupVersion(header.APIVersion)
		if parseErr != nil || groupVersion.Group != resource.Group {
			return actions.MutationResult{}, apierrors.NewBadRequest("invalid API group")
		}
		endpoint, resource, err = client.yamlClient(command.Target, command.Collection, groupVersion.Version)
		if err != nil {
			return actions.MutationResult{}, err
		}
	}
	value, err := resourceYAML(command, resource)
	if err != nil {
		return actions.MutationResult{}, err
	}
	updated, err := endpoint.Update(ctx, value, metav1.UpdateOptions{FieldValidation: "Strict"})
	if err != nil {
		// Never return parser/admission messages that can echo Secret or env values.
		if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
			return actions.MutationResult{}, apierrors.NewBadRequest("Kubernetes rejected the document; check its schema and immutable fields")
		}
		if apierrors.IsConflict(err) {
			return actions.MutationResult{}, apierrors.NewConflict(schema.GroupResource{Group: resource.Group, Resource: resource.Resource}, command.Target.Name, errors.New("resource changed; reload before saving"))
		}
		return actions.MutationResult{}, fmt.Errorf("update resource YAML: %w", err)
	}
	return actions.MutationResult{ResourceVersion: updated.GetResourceVersion()}, nil
}
