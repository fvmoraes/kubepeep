package kubernetesruntime

import (
	"context"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetSecretData is an explicit, uncached read. Listing, watching, indexing and
// the ordinary Secret detail remain metadata-only.
func (backend *ResourceBackend) GetSecretData(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, namespace, name string) (resources.ConfigMapDetailDTO, error) {
	origin := resources.Origin{Namespace: namespace, Version: "v1", Resource: "secrets"}
	return getAuthorized(ctx, backend, binding, resolution, origin, name, func(ctx context.Context, _ resources.Origin, name string) (resources.ConfigMapDetailDTO, error) {
		requestContext, cancel, clients, err := backend.unary(ctx, binding)
		if err != nil {
			return resources.ConfigMapDetailDTO{}, err
		}
		defer cancel()
		value, err := clients.kubernetes.CoreV1().Secrets(namespace).Get(requestContext, name, metav1.GetOptions{})
		if err != nil {
			return resources.ConfigMapDetailDTO{}, mapMetadataError(err, "Secret data is unavailable.")
		}
		// Secret.data is Base64 on the Kubernetes wire, even for readable UTF-8
		// bytes. Preserve that representation instead of silently decoding it.
		detail := resources.ConvertConfigMapDetail(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: value.Namespace, Name: value.Name, UID: value.UID, ResourceVersion: value.ResourceVersion}, BinaryData: value.Data})
		for index := range detail.Entries {
			detail.Entries[index].Field = "data"
		}
		return detail, nil
	})
}
