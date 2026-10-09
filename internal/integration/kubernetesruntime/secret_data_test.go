package kubernetesruntime

import (
	"errors"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestSecretDataRequiresExactReadAndDoesNotCacheValues(t *testing.T) {
	for _, test := range []struct {
		name      string
		decision  authorization.Decision
		namespace string
		allowed   bool
	}{
		{"allowed", authorization.DecisionAllowed, "default", true},
		{"denied", authorization.DecisionDenied, "default", false},
		{"unknown uses actual authorized GET", authorization.DecisionUnknown, "default", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "default", Annotations: map[string]string{"private": "hidden"}}, Data: map[string][]byte{"token": []byte("first"), "binary": {0xff}}}
			client := fake.NewSimpleClientset(secret)
			auth := &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}}
			if test.decision != authorization.DecisionAllowed {
				auth.denied["/secrets/get"] = test.decision
			}
			backend := detailTestBackend(client, auth)
			binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
			scope := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
			data, err := backend.GetSecretData(t.Context(), binding, scope, test.namespace, "credentials")
			if !test.allowed {
				if err == nil || len(client.Actions()) > 0 {
					t.Fatalf("unauthorized read reached API: %v", err)
				}
				return
			}
			if err != nil || len(data.Entries) != 2 || data.Entries[0].Encoding != "base64" || data.Entries[1].Value != "Zmlyc3Q=" || data.Entries[1].Encoding != "base64" || data.Entries[1].Field != "data" {
				t.Fatalf("data read failed: %v", err)
			}
			secret.Data["token"] = []byte("second")
			if _, err := client.CoreV1().Secrets("default").Update(t.Context(), secret, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			data, err = backend.GetSecretData(t.Context(), binding, scope, "default", "credentials")
			if err != nil || data.Entries[1].Value != "c2Vjb25k" {
				t.Fatal("stale Secret cached")
			}
			if _, err := backend.ResourceYAML(t.Context(), binding, "secrets", "default", "credentials"); err == nil {
				t.Fatal("generic secret YAML became available")
			}
		})
	}
}

func TestUnknownSecretReviewCannotOverrideActualKubernetesDenial(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "credentials", errors.New("private diagnostic"))
	})
	backend := detailTestBackend(client, &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"/secrets/get": authorization.DecisionUnknown}})
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	data, err := backend.GetSecretData(t.Context(), binding, namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}, "default", "credentials")
	if resources.ErrorCodeOf(err) != resources.CodeForbidden || len(data.Entries) != 0 || len(client.Actions()) != 1 {
		t.Fatalf("real Kubernetes denial bypassed or retried: entries=%d err=%v calls=%d", len(data.Entries), err, len(client.Actions()))
	}
}
