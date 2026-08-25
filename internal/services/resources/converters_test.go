package resources

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestWorkloadConvertersUseCompactDTOAndOmitPodTemplateSecrets(t *testing.T) {
	replicas := int32(2)
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments", UID: "uid", ResourceVersion: "9", Generation: 1, CreationTimestamp: metav1.NewTime(time.Unix(100, 0)), Annotations: map[string]string{"private": "do-not-return"}}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"kubectl.kubernetes.io/restartedAt": "2026-08-17T10:00:00Z", "secret": "do-not-return"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "example/api:1", Env: []corev1.EnvVar{{Name: "PASSWORD", Value: "TOP_SECRET"}}}}}}}, Status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 2, ReadyReplicas: 2, AvailableReplicas: 2, UpdatedReplicas: 2, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, Reason: "Ready"}}}}
	summary := ConvertDeployment(deployment, time.Unix(200, 0))
	if summary.Kind != "Deployment" || summary.Ready == nil || *summary.Ready != 2 || summary.Status != WorkloadHealthy {
		t.Fatalf("summary = %#v", summary)
	}
	detail := DeploymentDetail(deployment, nil, time.Unix(200, 0))
	encoded, _ := json.Marshal(detail)
	text := string(encoded)
	for _, prohibited := range []string{"TOP_SECRET", "PASSWORD", "do-not-return", "annotations", "managedFields"} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("detail leaked %q: %s", prohibited, text)
		}
	}
	if detail.RestartAt == nil || *detail.RestartAt != "2026-08-17T10:00:00Z" {
		t.Fatalf("restartAt = %#v", detail.RestartAt)
	}
}

func TestAllWorkloadKindsProduceTheirClosedKind(t *testing.T) {
	now := time.Now()
	stateful := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "s"}}
	daemon := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "d"}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j"}}
	cron := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "c"}}
	got := []string{ConvertStatefulSet(stateful, now).Kind, ConvertDaemonSet(daemon, now).Kind, ConvertJob(job, now).Kind, ConvertCronJob(cron, nil, now).Kind}
	want := []string{"StatefulSet", "DaemonSet", "Job", "CronJob"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("kind %d = %q", index, got[index])
		}
	}
}

func TestCronJobIncompleteHistoryNeverInventsPrioritizedStatus(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	last := metav1.NewTime(now.Add(-time.Hour))
	cron := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup", UID: "cron-uid"}, Status: batchv1.CronJobStatus{LastScheduleTime: &last}}
	if got := ConvertCronJobWithHistory(cron, nil, false, now); got.Status != WorkloadUnknown {
		t.Fatalf("incomplete history status=%s", got.Status)
	}
	if got := ConvertCronJob(cron, nil, now); got.Status != WorkloadUnknown {
		t.Fatalf("nil history status=%s", got.Status)
	}
	if got := ConvertCronJobWithHistory(cron, []batchv1.Job{}, true, now); got.Status != WorkloadHealthy {
		t.Fatalf("complete empty history status=%s", got.Status)
	}
}

func TestPodDetailKeepsOnlyAllowlistedContainerFields(t *testing.T) {
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api-1", Namespace: "payments", CreationTimestamp: metav1.NewTime(time.Unix(100, 0)),
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api", Controller: &controller}},
		},
		Spec: corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{
			{Name: "api", Image: "example:1", Command: []string{"private-command"}, Env: []corev1.EnvVar{{Name: "TOKEN", Value: "secret-value"}}},
			{Name: "sidecar", Image: "sidecar:1"},
		}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.1", ContainerStatuses: []corev1.ContainerStatus{
			{Name: "api", Ready: true, RestartCount: 3, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			{Name: "sidecar", Ready: false, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
		}},
	}
	detail := PodDetail(pod, nil, time.Unix(200, 0))
	if detail.Summary.Ready.Current != 1 || detail.Summary.Ready.Desired != 2 || !detail.Summary.Problematic || detail.Summary.Owner == nil {
		t.Fatalf("summary = %#v", detail.Summary)
	}
	encoded, _ := json.Marshal(detail)
	for _, prohibited := range []string{"private-command", "secret-value", "TOKEN", "env", "command"} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("pod detail leaked %q: %s", prohibited, encoded)
		}
	}
}

func TestEventConversionRedactsAndPreservesRealCountAndTimestamp(t *testing.T) {
	event := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "payments"}, InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api"}, Reason: "Failed", Message: "Bearer secret", Count: 3, Type: "Other", LastTimestamp: metav1.NewTime(time.Unix(10, 0)), Source: corev1.EventSource{Component: "kubelet"}}
	dto := ConvertEvent(event, TextRedactorFunc(func(string) string { return "[REDACTED]" }))
	if dto.Message != "[REDACTED]" || dto.Count != 3 || dto.Type != "Unknown" || dto.Timestamp == nil || dto.Source == nil {
		t.Fatalf("event = %#v", dto)
	}
}

func TestNetworkConvertersDoNotExposeTLSSecretAndRejectResourceBackend(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, ClusterIPs: []string{"10.0.0.1"}, Selector: map[string]string{"app": "api"}, Ports: []corev1.ServicePort{{Name: "http", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromString("http"), NodePort: 30080}}, ExternalIPs: []string{"203.0.113.1"}}}
	serviceDTO := ConvertService(service)
	if serviceDTO.Type != "NodePort" || len(serviceDTO.Ports) != 1 || serviceDTO.Ports[0].TargetPort.Type != "name" || len(serviceDTO.ExternalEndpoints) != 1 {
		t.Fatalf("service = %#v", serviceDTO)
	}
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"}, Spec: networkingv1.IngressSpec{TLS: []networkingv1.IngressTLS{{Hosts: []string{"example.test"}, SecretName: "tls-super-secret"}}, Rules: []networkingv1.IngressRule{{Host: "example.test", IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &pathType, Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "api", Port: networkingv1.ServiceBackendPort{Number: 80}}}}}}}}}}}
	dto, err := ConvertIngress(ingress)
	if err != nil || len(dto.Paths) != 1 {
		t.Fatalf("ingress=%#v err=%v", dto, err)
	}
	encoded, _ := json.Marshal(dto)
	if strings.Contains(string(encoded), "tls-super-secret") {
		t.Fatalf("TLS secret leaked: %s", encoded)
	}
	ingress.Spec.DefaultBackend = &networkingv1.IngressBackend{Resource: &corev1.TypedLocalObjectReference{Kind: "StorageBucket"}}
	if _, err = IngressDetail(ingress); ErrorCodeOf(err) != CodeFeatureUnavailable {
		t.Fatalf("resource backend error = %v", err)
	}
}

func TestEndpointSliceConverterClonesAllowlistedEndpointFields(t *testing.T) {
	ready := true
	port := int32(443)
	protocol := corev1.ProtocolTCP
	slice := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"}, AddressType: discoveryv1.AddressTypeIPv4, Ports: []discoveryv1.EndpointPort{{Port: &port, Protocol: &protocol}}, Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.2"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}, TargetRef: &corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "payments", Name: "api-1", UID: "pod-uid"}}}}
	dto := ConvertEndpointSlice(slice)
	if dto.AddressType != "IPv4" || len(dto.Endpoints) != 1 || dto.Endpoints[0].TargetRef == nil || dto.Endpoints[0].TargetRef.UID != "pod-uid" {
		t.Fatalf("slice = %#v", dto)
	}
}

func TestConfigAndSecretConvertersMaintainMetadataOnlyBoundary(t *testing.T) {
	config := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "payments"}, Data: map[string]string{"text": "hello"}, BinaryData: map[string][]byte{"binary": {0xff, 0x00}}}
	detail := ConvertConfigMapDetail(config)
	if len(detail.Entries) != 2 || detail.Entries[0].Key != "binary" || detail.Entries[0].Encoding != "base64" {
		t.Fatalf("config = %#v", detail)
	}
	deleted := metav1.NewTime(time.Unix(20, 0))
	secret := metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: "registry", Namespace: "payments", UID: types.UID("uid"), ResourceVersion: "must-not-return", Annotations: map[string]string{"token": "must-not-return"}, DeletionTimestamp: &deleted}}
	dto := ConvertSecretMetadata(secret)
	encoded, _ := json.Marshal(dto)
	text := string(encoded)
	if !strings.Contains(text, "deletionTimestamp") || strings.Contains(text, "resourceVersion") || strings.Contains(text, "annotations") || strings.Contains(text, "must-not-return") {
		t.Fatalf("secret metadata boundary failed: %s", text)
	}
}

func TestReadOnlyYAMLRejectsSecretAndDoesNotMutateSource(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{"password": []byte("value")}}
	if _, err := MarshalReadOnlyYAML(secret); !IsSecretYAMLError(err) {
		t.Fatalf("secret error = %v", err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "secret-manager"}}}}
	encoded, err := MarshalReadOnlyYAML(pod)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "managedFields") || strings.Contains(string(encoded), "secret-manager") {
		t.Fatalf("managed fields leaked: %s", encoded)
	}
	if len(pod.ManagedFields) != 1 {
		t.Fatal("source object was mutated")
	}
}
