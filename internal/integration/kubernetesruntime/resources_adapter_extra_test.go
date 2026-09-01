package kubernetesruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

func detailTestBackend(client *kubefake.Clientset, authorizer *selectiveResourceAuthorization) *ResourceBackend {
	return &ResourceBackend{
		clients:    fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}},
		authorizer: authorizer, now: time.Now,
	}
}

func TestResourceBackendGetDetailEndpoints(t *testing.T) {
	t.Parallel()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIPs: []string{"10.0.0.15"}, Ports: []corev1.ServicePort{{Port: 8080}}}}
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web"}, Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "app.example.com"}}}}
	endpointSlice := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api-abc"}, AddressType: discoveryv1.AddressTypeIPv4}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "settings"}, Data: map[string]string{"key": "value"}}
	client := kubefake.NewSimpleClientset(service, ingress, endpointSlice, configMap)
	backend := detailTestBackend(client, &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}})
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
	ctx := context.Background()

	detail, err := backend.GetService(ctx, binding, resolution, "default", "api")
	if err != nil || detail.Summary.Name != "api" || detail.Summary.Type != "ClusterIP" || len(detail.Summary.Ports) != 1 {
		t.Fatalf("service detail = %#v err = %v", detail, err)
	}
	ingressDetail, err := backend.GetIngress(ctx, binding, resolution, "default", "web")
	if err != nil || ingressDetail.Summary.Name != "web" || len(ingressDetail.Summary.Hosts) != 1 || ingressDetail.Summary.Hosts[0] != "app.example.com" {
		t.Fatalf("ingress detail = %#v err = %v", ingressDetail, err)
	}
	sliceDetail, err := backend.GetEndpointSlice(ctx, binding, resolution, "default", "api-abc")
	if err != nil || sliceDetail.Summary.Name != "api-abc" || sliceDetail.Summary.AddressType != "IPv4" {
		t.Fatalf("endpoint slice detail = %#v err = %v", sliceDetail, err)
	}
	configMapDetail, err := backend.GetConfigMap(ctx, binding, resolution, "default", "settings")
	if err != nil || configMapDetail.Metadata.Name != "settings" || len(configMapDetail.Entries) != 1 || configMapDetail.Entries[0].Value != "value" {
		t.Fatalf("config map detail = %#v err = %v", configMapDetail, err)
	}

	if _, err := backend.GetService(ctx, binding, resolution, "default", "missing"); resources.ErrorCodeOf(err) != resources.CodeNotFound {
		t.Fatalf("missing service err = %v", err)
	}
}

func TestResourceBackendYAMLDocuments(t *testing.T) {
	t.Parallel()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}, Spec: appsv1.DeploymentSpec{Replicas: nil}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}}
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web"}}
	endpointSlice := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api-abc"}}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "settings"}}
	client := kubefake.NewSimpleClientset(pod, deployment, service, ingress, endpointSlice, configMap)
	backend := detailTestBackend(client, &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}})
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	ctx := context.Background()

	podYAML, err := backend.PodYAML(ctx, binding, "default", "api")
	if err != nil || !strings.Contains(string(podYAML), "name: api") {
		t.Fatalf("pod yaml = %q err = %v", podYAML, err)
	}
	workloadYAML, err := backend.WorkloadYAMLDocument(ctx, binding, "deployments", "default", "api")
	if err != nil || !strings.Contains(string(workloadYAML), "name: api") {
		t.Fatalf("workload yaml = %q err = %v", workloadYAML, err)
	}
	if _, err := backend.WorkloadYAMLDocument(ctx, binding, "bogus", "default", "api"); resources.ErrorCodeOf(err) != resources.CodeValidationFailed {
		t.Fatalf("invalid workload kind err = %v", err)
	}

	cases := []struct{ collection, name string }{
		{collection: "services", name: "api"},
		{collection: "ingresses", name: "web"},
		{collection: "endpointslices", name: "api-abc"},
		{collection: "configmaps", name: "settings"},
	}
	for _, test := range cases {
		document, err := backend.ResourceYAML(ctx, binding, test.collection, "default", test.name)
		if err != nil || !strings.Contains(string(document), "name: "+test.name) {
			t.Fatalf("yaml %s = %q err = %v", test.collection, document, err)
		}
	}
	if _, err := backend.ResourceYAML(ctx, binding, "bogus", "default", "api"); resources.ErrorCodeOf(err) != resources.CodeValidationFailed {
		t.Fatalf("invalid yaml collection err = %v", err)
	}
}

func TestResourceBackendAuthorizationDenialsOnDetailsAndYAML(t *testing.T) {
	t.Parallel()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}}
	client := kubefake.NewSimpleClientset(pod, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}})
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
	ctx := context.Background()

	denied := detailTestBackend(client, &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"/services/get": authorization.DecisionDenied, "/pods/get": authorization.DecisionDenied}})
	if _, err := denied.GetService(ctx, binding, resolution, "default", "api"); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("denied get service err = %v", err)
	}
	if _, err := denied.ResourceYAML(ctx, binding, "services", "default", "api"); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("denied resource yaml err = %v", err)
	}
	if _, err := denied.PodYAML(ctx, binding, "default", "api"); err == nil {
		t.Fatal("pod yaml accepted a denied get")
	}
	unknown := detailTestBackend(client, &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"/services/get": authorization.DecisionUnknown}})
	if _, err := unknown.GetService(ctx, binding, resolution, "default", "api"); resources.ErrorCodeOf(err) != resources.CodeAuthorizationUnavailable {
		t.Fatalf("unknown get service err = %v", err)
	}

	logDenied := &ResourceBackend{clients: fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"/pods/get": authorization.DecisionDenied}}, now: time.Now}
	if err := logDenied.ReauthorizeLogs(ctx, binding, "default", "api"); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("denied logs err = %v", err)
	}
	if err := (&ResourceBackend{authorizer: &allowResourceAuthorization{}}).AuthorizeLogs(ctx, binding, "default", "api"); err != nil {
		t.Fatalf("allowed logs err = %v", err)
	}
	if _, err := logDenied.ReadLogs(ctx, binding, resolution, "default", "api", resources.LogQuery{Container: "api"}); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("denied read logs err = %v", err)
	}
	if _, err := logDenied.FollowLogs(ctx, binding, resolution, "default", "api", resources.LogQuery{Container: "api"}, func(resources.LogLineDTO) error { return nil }); resources.ErrorCodeOf(err) != resources.CodeForbidden {
		t.Fatalf("denied follow logs err = %v", err)
	}
}

func TestRuntimeResourceClientProviderUnaryValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, _, _, err := (runtimeResourceClientProvider{}).Unary(ctx, namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}); resources.ErrorCodeOf(err) != resources.CodeFeatureUnavailable {
		t.Fatalf("nil runtime err = %v", err)
	}
	if _, _, _, err := (runtimeResourceClientProvider{runtime: &Runtime{}}).Unary(ctx, namespaces.SelectionBinding{}); resources.ErrorCodeOf(err) != resources.CodeFeatureUnavailable {
		t.Fatalf("invalid binding err = %v", err)
	}
	if _, _, _, err := (runtimeResourceClientProvider{runtime: &Runtime{}}).Unary(ctx, namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}); resources.ErrorCodeOf(err) != resources.CodeClusterUnavailable {
		t.Fatalf("missing lease err = %v", err)
	}
}

func TestResourceBackendListWorkloadPageKinds(t *testing.T) {
	t.Parallel()
	deployments := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}}
	statefulSets := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "db"}}
	daemonSets := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "agent"}}
	jobs := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "migration"}}
	client := kubefake.NewSimpleClientset(deployments, statefulSets, daemonSets, jobs)
	backend := detailTestBackend(client, &selectiveResourceAuthorization{denied: map[string]authorization.Decision{}})
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	ctx := context.Background()

	kinds := []string{"deployments", "statefulsets", "daemonsets", "jobs"}
	for _, kind := range kinds {
		origin := resources.Origin{Namespace: "default", Version: "v1", Resource: kind}
		if kind == "deployments" || kind == "statefulsets" || kind == "daemonsets" {
			origin.APIGroup = "apps"
		} else {
			origin.APIGroup = "batch"
		}
		page, err := backend.listWorkloadPage(ctx, binding, resources.PageRequest{Origin: origin, Limit: 10})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("workload page %s = %#v err = %v", kind, page.Items, err)
		}
	}
	if _, err := backend.listWorkloadPage(ctx, binding, resources.PageRequest{Origin: resources.Origin{Namespace: "default", Version: "v1", Resource: "bogus"}, Limit: 10}); resources.ErrorCodeOf(err) != resources.CodeValidationFailed {
		t.Fatalf("invalid workload page kind err = %v", err)
	}
}
