package kubernetes

import (
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/actions"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

const editableDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: payments
  uid: uid-api
  resourceVersion: "17"
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: example/api:2
`

func TestUpdateDeploymentYAMLValidatesIdentityAndDocumentBeforeWrite(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, document string
		valid          bool
	}{
		{"valid", editableDeployment, true},
		{"different namespace", strings.Replace(editableDeployment, "namespace: payments", "namespace: other", 1), false},
		{"different uid", strings.Replace(editableDeployment, "uid-api", "new-uid", 1), false},
		{"different resource version", strings.Replace(editableDeployment, `"17"`, `"18"`, 1), false},
		{"different kind", strings.Replace(editableDeployment, "kind: Deployment", "kind: Secret", 1), false},
		{"multiple documents", editableDeployment + "---\nkind: Secret\n", false},
		{"duplicate field", strings.Replace(editableDeployment, "replicas: 3", "replicas: 3\n  replicas: 4", 1), false},
		{"unknown field", strings.Replace(editableDeployment, "replicas: 3", "replicass: 3", 1), false},
		{"invalid syntax", "spec: [DO_NOT_ECHO", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			clientset.PrependReactor("update", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
				value := action.(clienttesting.UpdateAction).GetObject().(*appsv1.Deployment)
				if value.ResourceVersion != "17" || value.UID != "uid-api" || *value.Spec.Replicas != 3 {
					t.Fatal("optimistic preconditions or edit lost")
				}
				return true, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "18"}}, nil
			})
			client := &ActionClient{unary: clientset}
			result, err := client.UpdateDeploymentYAML(t.Context(), actions.DeploymentYAMLCommand{Target: actions.MutationTarget{Kind: "Deployment", Namespace: "payments", Name: "api"}, ExpectedUID: "uid-api", ExpectedResourceVersion: "17", YAML: test.document})
			if test.valid {
				if err != nil || result.ResourceVersion != "18" {
					t.Fatalf("save failed: %v", err)
				}
			} else {
				if err == nil || len(clientset.Actions()) != 0 {
					t.Fatal("invalid document reached Kubernetes")
				}
				if strings.Contains(err.Error(), "DO_NOT_ECHO") {
					t.Fatal("parser leaked document")
				}
			}
		})
	}
}

func TestUpdateDeploymentYAMLPreservesConflict(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("update", "deployments", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "api", nil)
	})
	_, err := (&ActionClient{unary: clientset}).UpdateDeploymentYAML(t.Context(), actions.DeploymentYAMLCommand{Target: actions.MutationTarget{Kind: "Deployment", Namespace: "payments", Name: "api"}, ExpectedUID: "uid-api", ExpectedResourceVersion: "17", YAML: editableDeployment})
	if !apierrors.IsConflict(err) {
		t.Fatalf("conflict lost: %v", err)
	}
}
