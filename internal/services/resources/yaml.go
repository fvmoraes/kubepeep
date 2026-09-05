package resources

import (
	"encoding/json"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const MaximumYAMLBytes = 10 << 20

// MarshalYAMLDocument encodes an already-sanitized application document and
// enforces the same response ceiling as MarshalReadOnlyYAML. It is for curated
// documents (ADR 0006) whose assembly already applied the per-family policy;
// raw Kubernetes objects still have to pass MarshalReadOnlyYAML.
func MarshalYAMLDocument(value any) ([]byte, error) {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("resources: marshal YAML source: %w", err)
	}
	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return nil, fmt.Errorf("resources: encode YAML: %w", err)
	}
	if len(yamlBytes) > MaximumYAMLBytes {
		return nil, domainError(CodeLimitExceeded, "The YAML document exceeds the response limit.", nil)
	}
	return yamlBytes, nil
}

// MarshalReadOnlyYAML accepts only the concrete MVP kinds that have a YAML
// route. Secret, unstructured and generic runtime.Object inputs are rejected
// before serialization. The input is deep-copied before managedFields removal.
func MarshalReadOnlyYAML(value any) ([]byte, error) {
	var sanitized any
	switch object := value.(type) {
	case *corev1.Pod:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *corev1.Service:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *corev1.ConfigMap:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *appsv1.Deployment:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *appsv1.StatefulSet:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *appsv1.DaemonSet:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *batchv1.Job:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *batchv1.CronJob:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *networkingv1.Ingress:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *discoveryv1.EndpointSlice:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *coordinationv1.Lease:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *corev1.PersistentVolumeClaim:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *corev1.Namespace:
		copy := object.DeepCopy()
		copy.ManagedFields = nil
		sanitized = copy
	case *corev1.Secret:
		return nil, domainError(CodeForbidden, "Secret YAML is not available.", ErrSecretYAML)
	case *metav1.PartialObjectMetadata:
		if object.Kind == "Secret" {
			return nil, domainError(CodeForbidden, "Secret YAML is not available.", ErrSecretYAML)
		}
		return nil, domainError(CodeFeatureUnavailable, "YAML is unavailable for this resource type.", nil)
	default:
		return nil, domainError(CodeFeatureUnavailable, "YAML is unavailable for this resource type.", nil)
	}
	jsonBytes, err := json.Marshal(sanitized)
	if err != nil {
		return nil, fmt.Errorf("resources: marshal YAML source: %w", err)
	}
	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return nil, fmt.Errorf("resources: encode YAML: %w", err)
	}
	if len(yamlBytes) > MaximumYAMLBytes {
		return nil, domainError(CodeLimitExceeded, "The YAML document exceeds the response limit.", nil)
	}
	return yamlBytes, nil
}

func IsSecretYAMLError(err error) bool { return errors.Is(err, ErrSecretYAML) }
