package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/actions"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestActionClientRestartUsesOnlyRestartAnnotationAndResourceVersion(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := &ActionClient{unary: clientset}
	var patch map[string]any
	clientset.PrependReactor("patch", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
		patchAction := action.(clienttesting.PatchAction)
		if patchAction.GetPatchType() != types.StrategicMergePatchType {
			t.Fatalf("patch type = %s", patchAction.GetPatchType())
		}
		if err := json.Unmarshal(patchAction.GetPatch(), &patch); err != nil {
			t.Fatal(err)
		}
		return true, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "124"}}, nil
	})

	result, err := client.RestartDeployment(context.Background(), actions.RestartDeploymentCommand{
		Target:                  actions.MutationTarget{Namespace: "payments", Name: "api"},
		ExpectedResourceVersion: "123",
		RestartedAt:             time.Date(2026, 8, 17, 15, 4, 5, 999, time.FixedZone("offset", -3*60*60)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResourceVersion != "124" {
		t.Fatalf("resourceVersion = %q", result.ResourceVersion)
	}
	metadata, ok := patch["metadata"].(map[string]any)
	if !ok || len(metadata) != 1 || metadata["resourceVersion"] != "123" {
		t.Fatalf("metadata patch = %#v", patch["metadata"])
	}
	spec, ok := patch["spec"].(map[string]any)
	if !ok || len(spec) != 1 {
		t.Fatalf("spec patch = %#v", patch["spec"])
	}
	template := spec["template"].(map[string]any)
	templateMetadata := template["metadata"].(map[string]any)
	annotations := templateMetadata["annotations"].(map[string]any)
	if len(annotations) != 1 || annotations["kubectl.kubernetes.io/restartedAt"] != "2026-08-17T18:04:05Z" {
		t.Fatalf("annotations = %#v", annotations)
	}
}

func TestActionClientScaleUsesUpdateScaleForBothKinds(t *testing.T) {
	for _, test := range []struct {
		kind     string
		resource string
	}{
		{kind: "deployments", resource: "deployments"},
		{kind: "statefulsets", resource: "statefulsets"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			client := &ActionClient{unary: clientset}
			clientset.PrependReactor("update", test.resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() != "scale" {
					t.Fatalf("subresource = %q", action.GetSubresource())
				}
				scale := action.(clienttesting.UpdateAction).GetObject().(*autoscalingv1.Scale)
				if scale.ResourceVersion != "rv-before" || scale.Spec.Replicas != 7 || scale.Name != "worker" || scale.Namespace != "jobs" {
					t.Fatalf("scale = %#v", scale)
				}
				copy := scale.DeepCopy()
				copy.ResourceVersion = "rv-after"
				return true, copy, nil
			})
			result, err := client.UpdateScale(context.Background(), actions.ScaleCommand{
				Target:   actions.MutationTarget{Kind: test.kind, Namespace: "jobs", Name: "worker"},
				Replicas: 7, ExpectedResourceVersion: "rv-before",
			})
			if err != nil || result.ResourceVersion != "rv-after" {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}

func TestActionClientDeletePodCarriesBothPreconditions(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := &ActionClient{unary: clientset}
	clientset.PrependReactor("delete", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		deleteAction := action.(clienttesting.DeleteAction)
		options := deleteAction.GetDeleteOptions()
		if options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID != "uid-1" {
			t.Fatalf("uid precondition = %#v", options.Preconditions)
		}
		if options.Preconditions.ResourceVersion == nil || *options.Preconditions.ResourceVersion != "rv-1" {
			t.Fatalf("resourceVersion precondition = %#v", options.Preconditions)
		}
		return true, nil, nil
	})
	_, err := client.DeletePod(context.Background(), actions.DeletePodCommand{
		Target:      actions.MutationTarget{Namespace: "payments", Name: "api-1"},
		ExpectedUID: "uid-1", ExpectedResourceVersion: "rv-1",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestActionClientInspectsRegularInitAndEphemeralRunningState(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns"},
		Spec: corev1.PodSpec{
			Containers:          []corev1.Container{{Name: "regular"}},
			InitContainers:      []corev1.Container{{Name: "init"}},
			EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug"}}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses:          []corev1.ContainerStatus{{Name: "regular", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
			InitContainerStatuses:      []corev1.ContainerStatus{{Name: "init", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}}},
			EphemeralContainerStatuses: []corev1.ContainerStatus{{Name: "debug", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
		},
	}
	client := &ActionClient{unary: fake.NewSimpleClientset(pod)}
	for _, test := range []struct {
		container string
		exists    bool
		running   bool
	}{
		{container: "regular", exists: true, running: true},
		{container: "init", exists: true, running: false},
		{container: "debug", exists: true, running: true},
		{container: "missing", exists: false, running: false},
	} {
		state, err := client.InspectExecTarget(context.Background(), actions.MutationTarget{Namespace: "ns", Name: "pod"}, test.container)
		if err != nil {
			t.Fatal(err)
		}
		if !state.PodExists || state.ContainerExists != test.exists || state.ContainerRunning != test.running {
			t.Fatalf("%s state = %#v", test.container, state)
		}
	}
}

func TestReadExecResultMapsSuccessExitAndGoneWithoutExposingStatusPayload(t *testing.T) {
	exitStatus := metav1.Status{
		Status:  metav1.StatusFailure,
		Reason:  "NonZeroExitCode",
		Details: &metav1.StatusDetails{Causes: []metav1.StatusCause{{Type: "ExitCode", Message: "23"}}},
	}
	notFound := metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonNotFound, Code: 404, Message: "sensitive upstream detail"}
	for _, test := range []struct {
		name   string
		status metav1.Status
		code   *int
		gone   bool
	}{
		{name: "success", status: metav1.Status{Status: metav1.StatusSuccess}, code: intPointer(0)},
		{name: "exit", status: exitStatus, code: intPointer(23)},
		{name: "gone", status: notFound, gone: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := json.Marshal(test.status)
			if err != nil {
				t.Fatal(err)
			}
			result := readExecResult(bytes.NewReader(payload))
			if test.gone {
				if !errors.Is(result.Err, actions.ErrExecTargetGone) {
					t.Fatalf("error = %v", result.Err)
				}
				return
			}
			if result.Err != nil || result.ExitCode == nil || *result.ExitCode != *test.code {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func intPointer(value int) *int { return &value }
