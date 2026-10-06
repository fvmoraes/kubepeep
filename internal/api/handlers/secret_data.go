package handlers

import (
	"context"
	"net/http"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

type secretDataReader interface {
	GetSecretData(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resources.ConfigMapDetailDTO, error)
}

func (handler *Resources) SecretData(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	handler.detail(w, r, func(ctx context.Context, binding namespaces.SelectionBinding, scope namespaces.ScopeResolution) (any, error) {
		reader, ok := handler.service.(secretDataReader)
		if !ok {
			return nil, &resources.DomainError{Code: resources.CodeFeatureUnavailable, Message: "Secret data is unavailable."}
		}
		return reader.GetSecretData(ctx, binding, scope, r.PathValue("namespace"), r.PathValue("name"))
	})
}
