package resources

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodBudgetHandlesPartialBudgetsSidecarsAndPodOverride(t *testing.T) {
	requirements := corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}, Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")}}
	always := corev1.ContainerRestartPolicyAlways
	spec := corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Resources: requirements}}, InitContainers: []corev1.Container{{Name: "sidecar", RestartPolicy: &always, Resources: requirements}, {Name: "init", Resources: requirements}}}
	budget := podBudget(spec)
	if *budget.CPURequest != 200 || *budget.MemoryLimit != 256*1024*1024 || budget.CPULimit != nil || budget.MemoryRequest != nil {
		t.Fatalf("incorrect sidecar budget: %#v", budget)
	}
	spec.Containers = append(spec.Containers, corev1.Container{Name: "unbounded"})
	if value := podBudget(spec); value.CPURequest != nil || value.MemoryLimit != nil {
		t.Fatal("partial capacity represented as complete")
	}
	spec.Resources = &corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}}
	if value := podBudget(spec); value.CPURequest == nil || *value.CPURequest != 1000 {
		t.Fatal("pod budget did not override missing container values")
	}
}

func TestSecretReferencesCoverEnvironmentVolumesAndEphemeralContainers(t *testing.T) {
	spec := corev1.PodSpec{
		ImagePullSecrets:    []corev1.LocalObjectReference{{Name: "registry"}},
		Volumes:             []corev1.Volume{{VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "files"}}}, {VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "projected"}}}}}}}},
		Containers:          []corev1.Container{{EnvFrom: []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "files"}}}}}},
		EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Env: []corev1.EnvVar{{Name: "TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "debug"}, Key: "token"}}}}}}},
	}
	if got := podSecretRefs(spec); !reflect.DeepEqual(got, []string{"debug", "files", "projected", "registry"}) {
		t.Fatalf("missing or duplicate references: %v", got)
	}
	value := containerEnvironment(spec.EphemeralContainers[0].Env)[0]
	if value.Value != nil || value.Source.Name != "debug" || value.Source.Key != "token" {
		t.Fatal("environment must carry the reference, never resolve a secret implicitly")
	}
}
