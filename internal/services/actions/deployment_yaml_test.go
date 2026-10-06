package actions

import (
	"context"
	"net/http"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type yamlAdapterStub struct {
	*actionAdapterStub
	calls []DeploymentYAMLCommand
	err   error
}

func (a *yamlAdapterStub) UpdateDeploymentYAML(_ context.Context, command DeploymentYAMLCommand) (MutationResult, error) {
	a.calls = append(a.calls, command)
	return MutationResult{ResourceVersion: "18"}, a.err
}

func TestDeploymentYAMLRequiresConfirmationGenerationAndExactUpdate(t *testing.T) {
	for _, scenario := range []string{"allowed", "unconfirmed", "generation changed", "denied", "invalid document"} {
		t.Run(scenario, func(t *testing.T) {
			auth := &authorizerStub{}
			adapter := &yamlAdapterStub{actionAdapterStub: &actionAdapterStub{}}
			generation := &generationStub{generation: "gen_1"}
			service, err := NewActionService(t.Context(), auth, generation, adapter, NoopAuditSink{})
			if err != nil {
				t.Fatal(err)
			}
			confirmation := testRestart("gen_1", "payments", "api").Confirmation
			confirmation.Action, confirmation.ConsequenceCode = ActionUpdateDeployment, ConsequenceUpdateDeployment
			request := DeploymentYAMLRequest{Confirmation: confirmation, ExpectedUID: "uid-api", ExpectedResourceVersion: "17", YAML: "document"}
			switch scenario {
			case "unconfirmed":
				request.Confirmed = false
			case "generation changed":
				request.ExpectedGeneration = "old"
			case "denied":
				auth.err = apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "api", nil)
			case "invalid document":
				adapter.err = apierrors.NewBadRequest("DO_NOT_ECHO")
			}
			result, err := service.UpdateDeploymentYAML(t.Context(), testBinding("gen_1"), RouteTarget{Kind: "deployments", Namespace: "payments", Name: "api"}, request)
			if scenario == "allowed" {
				if err != nil || !result.Accepted || *result.ResourceVersion != "18" {
					t.Fatalf("save failed: %v", err)
				}
				key := auth.snapshot()[0].key
				if key.Verb != "update" || key.APIGroup != "apps" || key.Resource != "deployments" || key.ResourceName != "api" || key.Namespace != "payments" || key.Generation != "gen_1" {
					t.Fatalf("inexact permission: %#v", key)
				}
			} else {
				if err == nil {
					t.Fatal("unsafe request accepted")
				}
				if scenario != "invalid document" && len(adapter.calls) != 0 {
					t.Fatal("rejected request reached adapter")
				}
				if scenario == "invalid document" {
					failure := translateError(err)
					if failure.Code != CodeValidationFailed || failure.HTTPStatus != http.StatusBadRequest || failure.Retryable || failure.Message == "DO_NOT_ECHO" {
						t.Fatalf("bad validation response: %#v", failure)
					}
				}
			}
		})
	}
}
