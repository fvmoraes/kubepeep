package actions

import (
	"context"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestYAMLLimitIsNotReportedAsTransientClusterFailure(t *testing.T) {
	err := translateError(apierrors.NewRequestEntityTooLargeError("private-value"))
	if err.HTTPStatus != 413 || err.Code != CodeLimitExceeded || err.Retryable || err.Message == "private-value" {
		t.Fatalf("wrong size error classification: %#v", err)
	}
}

type resourceYAMLStub struct {
	*actionAdapterStub
	reads, updates int
}

func (s *resourceYAMLStub) ReadResourceYAML(context.Context, MutationTarget, string) (string, error) {
	s.reads++
	return "private-document", nil
}
func (s *resourceYAMLStub) UpdateResourceYAML(context.Context, ResourceYAMLCommand) (MutationResult, error) {
	s.updates++
	return MutationResult{ResourceVersion: "18"}, nil
}

func TestResourceYAMLExactAuthorizationAndScope(t *testing.T) {
	for _, collection := range []string{"pods", "secrets", "configmaps", "cluster-roles", "storage-classes"} {
		for _, scenario := range []string{"allowed", "denied", "unconfirmed", "old generation", "outside scope", "wrong kind", "wrong name", "canceled"} {
			t.Run(collection+"/"+scenario, func(t *testing.T) {
				resource, _ := resourcecatalog.Lookup(collection)
				ns := ""
				if resource.Namespaced {
					ns = "payments"
				}
				name := "sample"
				if collection == "cluster-roles" {
					name = "system:discovery"
				}
				auth := &authorizerStub{}
				adapter := &resourceYAMLStub{actionAdapterStub: &actionAdapterStub{}}
				service, err := NewActionService(t.Context(), auth, &generationStub{generation: "gen_1"}, adapter, NoopAuditSink{})
				if err != nil {
					t.Fatal(err)
				}
				binding := testBinding("gen_1")
				resolution := namespaces.ScopeResolution{Namespaces: []string{"payments"}}
				route := RouteTarget{Kind: collection, Namespace: ns, Name: name}
				confirmation := testRestart("gen_1", ns, name).Confirmation
				confirmation.Action, confirmation.ConsequenceCode, confirmation.Target.Kind = ActionUpdateResource, ConsequenceUpdateResource, resource.Kind
				request := ResourceYAMLRequest{Confirmation: confirmation, ExpectedUID: "uid", ExpectedResourceVersion: "17", YAML: "document"}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				switch scenario {
				case "denied":
					auth.err = apierrors.NewForbidden(schema.GroupResource{Resource: resource.Resource}, name, nil)
				case "unconfirmed":
					request.Confirmed = false
				case "old generation":
					request.ExpectedGeneration = "old"
				case "outside scope":
					route.Namespace, request.Target.Namespace = "outside", "outside"
				case "wrong kind":
					request.Target.Kind = "WrongKind"
				case "wrong name":
					request.Target.Name = "another"
				case "canceled":
					cancel()
				}
				result, err := service.UpdateResourceYAML(ctx, binding, resolution, route, request)
				if scenario == "allowed" {
					if err != nil || !result.Accepted || adapter.updates != 1 {
						t.Fatalf("save failed: %v", err)
					}
					key := auth.snapshot()[0].key
					if key.Verb != "update" || key.Resource != resource.Resource || key.APIGroup != resource.Group || key.Namespace != ns || key.ResourceName != name {
						t.Fatalf("inexact authorization: %#v", key)
					}
					document, err := service.ReadResourceYAML(ctx, binding, resolution, route)
					if err != nil || document.Generation != "gen_1" || document.UpdateCapability != resource.UpdateCapability() || adapter.reads != 1 {
						t.Fatalf("read failed: %v", err)
					}
					if auth.snapshot()[1].key.Verb != "get" {
						t.Fatal("read reused update permission")
					}
				} else if err == nil || adapter.updates != 0 {
					t.Fatalf("unsafe save: %v", err)
				}
				if scenario == "outside scope" || scenario == "denied" || scenario == "canceled" {
					_, err := service.ReadResourceYAML(ctx, binding, resolution, route)
					if err == nil || adapter.reads != 0 {
						t.Fatal("unauthorized document read")
					}
				}
			})
		}
	}
}
