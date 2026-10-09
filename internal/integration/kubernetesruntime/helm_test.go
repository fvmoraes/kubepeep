package kubernetesruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	kubeadapter "github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	"helm.sh/helm/v3/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
)

func helmMetadataFixture(namespace, name string, revision int) metav1.PartialObjectMetadata {
	return metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, revision), Namespace: namespace, UID: types.UID(fmt.Sprintf("%s-%s-%d", namespace, name, revision)), ResourceVersion: "1", Labels: map[string]string{"owner": "helm", "name": name, "version": fmt.Sprint(revision), "status": "deployed"}}}
}
func TestHelmMetadataListPreservesSameNamesAcrossNamespacesAndDoesNotReadValues(t *testing.T) {
	for _, storage := range []string{"secrets", "configmaps"} {
		t.Run(storage, func(t *testing.T) {
			core := kubefake.NewSimpleClientset()
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("limit") != "500" || !strings.Contains(r.URL.Query().Get("labelSelector"), "owner=helm") {
					t.Errorf("unbounded metadata query: %s", r.URL.RawQuery)
				}
				calls++
				items := []metav1.PartialObjectMetadata{helmMetadataFixture("a", "demo", 1), helmMetadataFixture("b", "demo", 3)}
				next := "last"
				if r.URL.Query().Get("continue") != "" {
					items = []metav1.PartialObjectMetadata{helmMetadataFixture("a", "demo", 2)}
					next = ""
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(&metav1.PartialObjectMetadataList{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "PartialObjectMetadataList"}, ListMeta: metav1.ListMeta{Continue: next}, Items: items}); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			metadataClient, err := metadata.NewForConfig(&rest.Config{Host: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			backend := &ResourceBackend{clients: fixedResourceClientProvider{resourceClientSet{kubernetes: core, metadata: metadataClient}}, authorizer: &allowResourceAuthorization{}, now: time.Now}
			records, err := backend.helmMetadata(t.Context(), namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, "", storage, "")
			if err != nil {
				t.Fatal(err)
			}
			latest := latestHelmReleases(records)
			if len(latest) != 2 || latest[0].Namespace != "a" || latest[0].Revision != 2 || latest[1].Namespace != "b" || latest[1].Revision != 3 {
				t.Fatalf("release identity collapsed: %+v", latest)
			}
			if calls != 2 || len(core.Actions()) != 0 {
				t.Fatalf("inventory read values or lost continuation: %d %+v", calls, core.Actions())
			}
		})
	}
}
func TestHelmInvalidAndDeniedRequestsNeverReadPayloads(t *testing.T) {
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}
	scope := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"ns"}}
	backend := &ResourceBackend{authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"/secrets/list": authorization.DecisionDenied}}, now: time.Now}
	if _, err := backend.helmMetadata(t.Context(), binding, "ns", "secrets", ""); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("explicit denial ignored: %v", err)
	}
	if _, err := backend.HelmDocument(t.Context(), binding, scope, "secrets", "ns", "demo", "other"); resources.ErrorCodeOf(err) != resources.CodeValidationFailed {
		t.Fatalf("invalid format reached API: %v", err)
	}
	valid := resources.HelmMutationRequest{ExpectedGeneration: "gen", ExpectedRevision: 2, ExpectedUID: "uid", ExpectedResourceVersion: "1", Confirmed: true, Action: "values", Values: "replicas: 2"}
	for _, change := range []string{"generation", "confirmation", "yaml", "duplicate", "size"} {
		t.Run(change, func(t *testing.T) {
			command := valid
			switch change {
			case "generation":
				command.ExpectedGeneration = "stale"
			case "confirmation":
				command.Confirmed = false
			case "yaml":
				command.Values = "- not-a-map"
			case "duplicate":
				command.Values = "key: one\nkey: two"
			case "size":
				command.Values = strings.Repeat("x", resources.MaximumHelmDocumentBytes+1)
			}
			_, err := backend.MutateHelmRelease(t.Context(), binding, scope, "secrets", "ns", "demo", command)
			if resources.ErrorCodeOf(err) != resources.CodeValidationFailed && resources.ErrorCodeOf(err) != resources.CodeGenerationChanged {
				t.Fatalf("invalid input attempted Kubernetes read: %v", err)
			}
		})
	}
}
func TestHelmMutationRevisionAndIdentityChecks(t *testing.T) {
	binding := namespaces.SelectionBinding{Generation: "gen"}
	metadata := resources.HelmReleaseDTO{Name: "demo", Revision: 3, UID: "uid", ResourceVersion: "27", Status: "deployed"}
	valid := resources.HelmMutationRequest{ExpectedGeneration: "gen", ExpectedRevision: 3, ExpectedUID: "uid", ExpectedResourceVersion: "27", Confirmed: true, Action: "rollback", Revision: 1}
	if err := validateHelmMutation(binding, metadata, valid); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"revision", "uid", "resourceVersion", "pending"} {
		t.Run(change, func(t *testing.T) {
			target := metadata
			switch change {
			case "revision":
				target.Revision++
			case "uid":
				target.UID = "other"
			case "resourceVersion":
				target.ResourceVersion = "28"
			case "pending":
				target.Status = "pending-upgrade"
			}
			if err := validateHelmMutation(binding, target, valid); !apierrors.IsConflict(err) {
				t.Fatalf("stale mutation not blocked: %v", err)
			}
		})
	}
}
func TestHelmErrorsExposeCausesWithoutValues(t *testing.T) {
	for _, test := range []struct {
		err  error
		code resources.ErrorCode
	}{{kubeadapter.ErrHelmReleaseLimit, resources.CodeLimitExceeded}, {kubeadapter.ErrHelmInvalidStorage, resources.CodeValidationFailed}, {driver.ErrReleaseNotFound, resources.CodeNotFound}, {apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "private-name", errors.New("credential-value")), resources.CodeForbidden}, {errors.New("template: chart: credential-value"), resources.CodeValidationFailed}, {errors.New("values don't meet the specifications of the schema(s) in the following chart(s):\ncredential-value"), resources.CodeValidationFailed}, {context.DeadlineExceeded, resources.CodeUpstreamTimeout}} {
		err := mapHelmError(test.err)
		if resources.ErrorCodeOf(err) != test.code || strings.Contains(resources.PublicMessage(err), "credential-value") {
			t.Fatalf("unsafe Helm failure: %s %v", test.code, err)
		}
	}
}
func TestHelmRelatedResourcesSkipValuesAndKeepExplicitIdentity(t *testing.T) {
	refs := helmRelated("apiVersion: v1\nkind: Secret\nmetadata:\n  name: credentials\ndata:\n  token: private-value\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  namespace: other\n  name: api\n", "ns")
	if len(refs) != 2 || refs[0].Namespace != "ns" || refs[0].Name != "credentials" || refs[1].Namespace != "other" || refs[1].APIGroup != "apps" {
		t.Fatalf("relationships: %+v", refs)
	}
}
