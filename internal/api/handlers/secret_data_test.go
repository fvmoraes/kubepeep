package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

type secretDataStub struct {
	ResourceService
	calls int
}

func (s *secretDataStub) GetSecretData(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string) (resources.ConfigMapDetailDTO, error) {
	s.calls++
	return resources.ConfigMapDetailDTO{}, nil
}

func TestSecretDataScopesAccessAndPreventsCaching(t *testing.T) {
	for _, namespace := range []string{"payments", "outside"} {
		t.Run(namespace, func(t *testing.T) {
			service := &secretDataStub{}
			selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{Namespaces: []string{"payments"}}}
			handler := NewResources(service, nil, selection, nil)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+namespace+"/credentials/data", nil)
			request.SetPathValue("namespace", namespace)
			request.SetPathValue("name", "credentials")
			response := httptest.NewRecorder()
			handler.SecretData(response, request)
			if namespace == "payments" {
				if response.Code != http.StatusOK || service.calls != 1 {
					t.Fatalf("allowed read failed: %d", response.Code)
				}
			} else if response.Code != http.StatusBadRequest || service.calls != 0 {
				t.Fatalf("out-of-scope read reached service: %d", response.Code)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("Secret response can be cached")
			}
		})
	}
}
