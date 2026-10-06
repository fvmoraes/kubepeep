package resources

import (
	"sort"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
)

// Nil means no complete declared budget; it must never be rendered as zero.
type ResourceBudgetDTO struct {
	CPURequest    *int64 `json:"cpuRequestMillicores"`
	CPULimit      *int64 `json:"cpuLimitMillicores"`
	MemoryRequest *int64 `json:"memoryRequestBytes"`
	MemoryLimit   *int64 `json:"memoryLimitBytes"`
}

func podContainerBudgets(spec corev1.PodSpec) map[string]ResourceBudgetDTO {
	result := make(map[string]ResourceBudgetDTO, len(spec.Containers))
	for _, container := range spec.Containers {
		result[container.Name] = containerBudget(container.Resources)
	}
	for _, container := range spec.InitContainers {
		result[container.Name] = containerBudget(container.Resources)
	}
	return result
}

func containerBudget(value corev1.ResourceRequirements) ResourceBudgetDTO {
	quantity := func(values corev1.ResourceList, name corev1.ResourceName) *int64 {
		value, ok := values[name]
		if !ok || value.Sign() <= 0 {
			return nil
		}
		amount := value.Value()
		if name == corev1.ResourceCPU {
			amount = value.MilliValue()
		}
		return &amount
	}
	return ResourceBudgetDTO{CPURequest: quantity(value.Requests, corev1.ResourceCPU), CPULimit: quantity(value.Limits, corev1.ResourceCPU), MemoryRequest: quantity(value.Requests, corev1.ResourceMemory), MemoryLimit: quantity(value.Limits, corev1.ResourceMemory)}
}

func podBudget(spec corev1.PodSpec) ResourceBudgetDTO {
	// Usage contains app containers and restartable init sidecars. One missing
	// request/limit makes that total unknown rather than understating capacity.
	containers := append([]corev1.Container{}, spec.Containers...)
	for _, container := range spec.InitContainers {
		if container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			containers = append(containers, container)
		}
	}
	var cpuRequest, cpuLimit, memoryRequest, memoryLimit int64
	result := ResourceBudgetDTO{&cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit}
	add := func(total **int64, value *int64) {
		if value == nil || *total == nil {
			*total = nil
			return
		}
		if *value > 0 && **total > int64(^uint64(0)>>1)-*value {
			*total = nil
			return
		}
		**total += *value
	}
	for _, container := range containers {
		budget := containerBudget(container.Resources)
		add(&result.CPURequest, budget.CPURequest)
		add(&result.CPULimit, budget.CPULimit)
		add(&result.MemoryRequest, budget.MemoryRequest)
		add(&result.MemoryLimit, budget.MemoryLimit)
	}
	if len(containers) == 0 {
		return ResourceBudgetDTO{}
	}
	// Kubernetes pod-level resources override the corresponding container sum.
	if spec.Resources != nil {
		budget := containerBudget(*spec.Resources)
		if budget.CPURequest != nil {
			result.CPURequest = budget.CPURequest
		}
		if budget.CPULimit != nil {
			result.CPULimit = budget.CPULimit
		}
		if budget.MemoryRequest != nil {
			result.MemoryRequest = budget.MemoryRequest
		}
		if budget.MemoryLimit != nil {
			result.MemoryLimit = budget.MemoryLimit
		}
	}
	return result
}

type EnvironmentSourceDTO struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Key      string `json:"key,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	Optional bool   `json:"optional"`
}

type EnvironmentVariableDTO struct {
	Name   string                `json:"name"`
	Value  *string               `json:"value"`
	Source *EnvironmentSourceDTO `json:"source,omitempty"`
}

func containerEnvironment(values []corev1.EnvVar) []EnvironmentVariableDTO {
	result := make([]EnvironmentVariableDTO, 0, len(values))
	for _, variable := range values {
		entry := EnvironmentVariableDTO{Name: variable.Name}
		if variable.ValueFrom == nil {
			value := variable.Value
			entry.Value = &value
		} else {
			source := variable.ValueFrom
			switch {
			case source.SecretKeyRef != nil:
				ref := source.SecretKeyRef
				entry.Source = &EnvironmentSourceDTO{Kind: "Secret", Name: ref.Name, Key: ref.Key, Optional: ref.Optional != nil && *ref.Optional}
			case source.ConfigMapKeyRef != nil:
				ref := source.ConfigMapKeyRef
				entry.Source = &EnvironmentSourceDTO{Kind: "ConfigMap", Name: ref.Name, Key: ref.Key, Optional: ref.Optional != nil && *ref.Optional}
			case source.FieldRef != nil:
				entry.Source = &EnvironmentSourceDTO{Kind: "Field", Name: source.FieldRef.FieldPath}
			case source.ResourceFieldRef != nil:
				entry.Source = &EnvironmentSourceDTO{Kind: "Resource", Name: source.ResourceFieldRef.ContainerName, Key: source.ResourceFieldRef.Resource}
			}
		}
		result = append(result, entry)
	}
	return result
}

func containerEnvFrom(values []corev1.EnvFromSource) []EnvironmentSourceDTO {
	result := make([]EnvironmentSourceDTO, 0, len(values))
	for _, source := range values {
		if source.SecretRef != nil {
			ref := source.SecretRef
			result = append(result, EnvironmentSourceDTO{Kind: "Secret", Name: ref.Name, Prefix: source.Prefix, Optional: ref.Optional != nil && *ref.Optional})
		}
		if source.ConfigMapRef != nil {
			ref := source.ConfigMapRef
			result = append(result, EnvironmentSourceDTO{Kind: "ConfigMap", Name: ref.Name, Prefix: source.Prefix, Optional: ref.Optional != nil && *ref.Optional})
		}
	}
	return result
}

func podSecretRefs(spec corev1.PodSpec) []string {
	names := map[string]struct{}{}
	add := func(name string) {
		if name != "" {
			names[name] = struct{}{}
		}
	}
	for _, volume := range spec.Volumes {
		if volume.Secret != nil {
			add(volume.Secret.SecretName)
		}
		if volume.Projected != nil {
			for _, source := range volume.Projected.Sources {
				if source.Secret != nil {
					add(source.Secret.Name)
				}
			}
		}
	}
	for _, ref := range spec.ImagePullSecrets {
		add(ref.Name)
	}
	containers := append(append([]corev1.Container{}, spec.Containers...), spec.InitContainers...)
	for _, ephemeral := range spec.EphemeralContainers {
		containers = append(containers, corev1.Container{Env: ephemeral.Env, EnvFrom: ephemeral.EnvFrom})
	}
	for _, container := range containers {
		for _, source := range container.EnvFrom {
			if source.SecretRef != nil {
				add(source.SecretRef.Name)
			}
		}
		for _, variable := range container.Env {
			if variable.ValueFrom != nil && variable.ValueFrom.SecretKeyRef != nil {
				add(variable.ValueFrom.SecretKeyRef.Name)
			}
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

type HPAResourceTargetDTO struct {
	Resource     string `json:"resource"`
	Container    string `json:"container,omitempty"`
	Utilization  *int32 `json:"utilization"`
	AverageValue *int64 `json:"averageValue"`
}

func hpaResourceTargets(metrics []autoscalingv2.MetricSpec) []HPAResourceTargetDTO {
	if len(metrics) > maximumHPAMetrics {
		metrics = metrics[:maximumHPAMetrics]
	}
	result := []HPAResourceTargetDTO{}
	for _, metric := range metrics {
		var name corev1.ResourceName
		var container string
		var target autoscalingv2.MetricTarget
		if metric.Resource != nil {
			name, target = metric.Resource.Name, metric.Resource.Target
		} else if metric.ContainerResource != nil {
			name, container, target = metric.ContainerResource.Name, metric.ContainerResource.Container, metric.ContainerResource.Target
		} else {
			continue
		}
		if name != corev1.ResourceCPU && name != corev1.ResourceMemory {
			continue
		}
		entry := HPAResourceTargetDTO{Resource: string(name), Container: container, Utilization: target.AverageUtilization}
		if target.AverageValue != nil {
			value := target.AverageValue.Value()
			if name == corev1.ResourceCPU {
				value = target.AverageValue.MilliValue()
			}
			entry.AverageValue = &value
		}
		result = append(result, entry)
	}
	return result
}
