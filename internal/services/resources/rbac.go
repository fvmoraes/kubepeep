package resources

import (
	"strconv"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	nodev1 "k8s.io/api/node/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	maximumRBACRules         = 64
	maximumRuleStrings       = 16
	maximumSubjects          = 32
	maximumCRDVersions       = 16
	maximumWebhooks          = 32
	maximumWebhookRules      = 32
	maximumSubsets           = 32
	maximumEndpointAddresses = 512
)

// RuleDTO is one bounded, typed RBAC policy rule.
type RuleDTO struct {
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

// SubjectDTO is one bounded RBAC subject.
type SubjectDTO struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// RoleDTO lists a namespaced Role without resolving effective permissions.
type RoleDTO struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	RuleCount  int    `json:"ruleCount"`
	AgeSeconds int64  `json:"ageSeconds"`
}

func (RoleDTO) resourceListItem() {}

// RoleDetailDTO adds typed rules. Rules never imply effective permissions.
type RoleDetailDTO struct {
	Metadata  ResourceMetadataDTO `json:"metadata"`
	RuleCount int                 `json:"ruleCount"`
	Rules     []RuleDTO           `json:"rules"`
	Truncated bool                `json:"truncated"`
}

func (RoleDetailDTO) resourceDetailItem() {}

// ConvertRole projects one Role onto the bounded list DTO.
func ConvertRole(value *rbacv1.Role, now time.Time) RoleDTO {
	return RoleDTO{
		Namespace: value.Namespace, Name: value.Name, RuleCount: len(value.Rules),
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertRoleDetail projects one Role onto the bounded detail DTO.
func ConvertRoleDetail(value *rbacv1.Role) RoleDetailDTO {
	return RoleDetailDTO{
		Metadata: ConvertMetadata(value), RuleCount: len(value.Rules),
		Rules: boundedRules(value.Rules), Truncated: len(value.Rules) > maximumRBACRules,
	}
}

// ConvertClusterRole projects one ClusterRole onto the bounded list DTO
// (namespace stays empty).
func ConvertClusterRole(value *rbacv1.ClusterRole, now time.Time) RoleDTO {
	return RoleDTO{Name: value.Name, RuleCount: len(value.Rules), AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second)}
}

// ConvertClusterRoleDetail projects one ClusterRole onto the bounded detail DTO.
func ConvertClusterRoleDetail(value *rbacv1.ClusterRole) RoleDetailDTO {
	return RoleDetailDTO{
		Metadata: ConvertMetadata(value), RuleCount: len(value.Rules),
		Rules: boundedRules(value.Rules), Truncated: len(value.Rules) > maximumRBACRules,
	}
}

func boundedRules(rules []rbacv1.PolicyRule) []RuleDTO {
	result := make([]RuleDTO, 0, min(len(rules), maximumRBACRules))
	for _, rule := range rules {
		if len(result) == maximumRBACRules {
			break
		}
		result = append(result, RuleDTO{
			APIGroups: boundedStrings(rule.APIGroups),
			Resources: boundedStrings(rule.Resources),
			Verbs:     boundedStrings(rule.Verbs),
		})
	}
	return result
}

func boundedStrings(values []string) []string {
	if len(values) > maximumRuleStrings {
		values = values[:maximumRuleStrings]
	}
	return append([]string(nil), values...)
}

// BindingDTO carries roleRef and bounded subjects for both binding kinds.
type BindingDTO struct {
	Namespace   string       `json:"namespace"`
	Name        string       `json:"name"`
	RoleRefKind string       `json:"roleRefKind"`
	RoleRefName string       `json:"roleRefName"`
	Subjects    []SubjectDTO `json:"subjects"`
	Truncated   bool         `json:"truncated"`
	AgeSeconds  int64        `json:"ageSeconds"`
}

func (BindingDTO) resourceListItem() {}

func (BindingDTO) resourceDetailItem() {}

func bindingSubjects(subjects []rbacv1.Subject) ([]SubjectDTO, bool) {
	result := make([]SubjectDTO, 0, min(len(subjects), maximumSubjects))
	for _, subject := range subjects {
		if len(result) == maximumSubjects {
			break
		}
		result = append(result, SubjectDTO{Kind: subject.Kind, Name: subject.Name, Namespace: subject.Namespace})
	}
	return result, len(subjects) > maximumSubjects
}

// ConvertRoleBinding projects one RoleBinding onto the bounded DTO.
func ConvertRoleBinding(value *rbacv1.RoleBinding, now time.Time) BindingDTO {
	subjects, truncated := bindingSubjects(value.Subjects)
	return BindingDTO{
		Namespace: value.Namespace, Name: value.Name,
		RoleRefKind: value.RoleRef.Kind, RoleRefName: value.RoleRef.Name,
		Subjects: subjects, Truncated: truncated,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertClusterRoleBinding projects one ClusterRoleBinding onto the bounded DTO.
func ConvertClusterRoleBinding(value *rbacv1.ClusterRoleBinding, now time.Time) BindingDTO {
	subjects, truncated := bindingSubjects(value.Subjects)
	return BindingDTO{
		Name: value.Name, RoleRefKind: value.RoleRef.Kind, RoleRefName: value.RoleRef.Name,
		Subjects: subjects, Truncated: truncated,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// CustomResourceDefinitionDTO reports identity and served versions only.
// Schemas, defaults and examples never enter the DTO (V4-03).
type CustomResourceDefinitionDTO struct {
	Name       string          `json:"name"`
	Group      string          `json:"group"`
	Kind       string          `json:"kind"`
	Scope      string          `json:"scope"`
	Versions   []CRDVersionDTO `json:"versions"`
	Conditions []ConditionDTO  `json:"conditions"`
	Truncated  bool            `json:"truncated"`
	AgeSeconds int64           `json:"ageSeconds"`
}

func (CustomResourceDefinitionDTO) resourceListItem() {}

func (CustomResourceDefinitionDTO) resourceDetailItem() {}

// CRDVersionDTO names one served/storage version pair.
type CRDVersionDTO struct {
	Name    string `json:"name"`
	Served  bool   `json:"served"`
	Storage bool   `json:"storage"`
}

// ConvertCustomResourceDefinition projects one CRD onto the bounded DTO.
func ConvertCustomResourceDefinition(value *apiextensionsv1.CustomResourceDefinition, now time.Time) CustomResourceDefinitionDTO {
	versions := make([]CRDVersionDTO, 0, min(len(value.Spec.Versions), maximumCRDVersions))
	for _, version := range value.Spec.Versions {
		if len(versions) == maximumCRDVersions {
			break
		}
		versions = append(versions, CRDVersionDTO{Name: version.Name, Served: version.Served, Storage: version.Storage})
	}
	conditions := make([]ConditionDTO, 0, 4)
	for _, condition := range value.Status.Conditions {
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	return CustomResourceDefinitionDTO{
		Name: value.Name, Group: value.Spec.Group,
		Kind: value.Spec.Names.Kind, Scope: string(value.Spec.Scope),
		Versions: versions, Conditions: conditions,
		Truncated:  len(value.Spec.Versions) > maximumCRDVersions,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// PriorityClassDTO reports scheduling identity fields.
type PriorityClassDTO struct {
	Name             string  `json:"name"`
	Value            int32   `json:"value"`
	GlobalDefault    bool    `json:"globalDefault"`
	PreemptionPolicy *string `json:"preemptionPolicy"`
	AgeSeconds       int64   `json:"ageSeconds"`
}

func (PriorityClassDTO) resourceListItem() {}

func (PriorityClassDTO) resourceDetailItem() {}

// ConvertPriorityClass projects one PriorityClass onto the bounded DTO.
func ConvertPriorityClass(value *schedulingv1.PriorityClass, now time.Time) PriorityClassDTO {
	var preemption *string
	if value.PreemptionPolicy != nil {
		policy := string(*value.PreemptionPolicy)
		preemption = &policy
	}
	return PriorityClassDTO{
		Name: value.Name, Value: value.Value, GlobalDefault: value.GlobalDefault,
		PreemptionPolicy: preemption,
		AgeSeconds:       int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// RuntimeClassDTO reports the handler; overhead quantities are bounded.
type RuntimeClassDTO struct {
	Name       string            `json:"name"`
	Handler    string            `json:"handler"`
	Overhead   map[string]string `json:"overhead"`
	AgeSeconds int64             `json:"ageSeconds"`
}

func (RuntimeClassDTO) resourceListItem() {}

func (RuntimeClassDTO) resourceDetailItem() {}

// ConvertRuntimeClass projects one RuntimeClass onto the bounded DTO.
func ConvertRuntimeClass(value *nodev1.RuntimeClass, now time.Time) RuntimeClassDTO {
	overhead := map[string]string(nil)
	if value.Overhead != nil {
		overhead = boundedQuantity(value.Overhead.PodFixed)
	}
	return RuntimeClassDTO{
		Name: value.Name, Handler: value.Handler, Overhead: overhead,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// WebhookConfigurationDTO is the bounded list view shared by mutating and
// validating webhook configurations. CA bundles, URLs and service references
// are never projected (V4-05).
type WebhookConfigurationDTO struct {
	Name         string           `json:"name"`
	WebhookCount int              `json:"webhookCount"`
	Webhooks     []WebhookRuleDTO `json:"webhooks"`
	Truncated    bool             `json:"truncated"`
	AgeSeconds   int64            `json:"ageSeconds"`
}

func (WebhookConfigurationDTO) resourceListItem() {}

func (WebhookConfigurationDTO) resourceDetailItem() {}

// WebhookRuleDTO names one webhook and its approved rule surface.
type WebhookRuleDTO struct {
	Name          string    `json:"name"`
	FailurePolicy *string   `json:"failurePolicy"`
	Rules         []RuleDTO `json:"rules"`
	Truncated     bool      `json:"truncated"`
}

type webhookSummarySource struct {
	name          string
	failurePolicy *string
	rules         []admissionregistrationv1.RuleWithOperations
}

// convertWebhookConfiguration bounds webhook identity and rules; client
// configs (URLs, CA bundles, service references) never leave the cluster
// adapter.
func convertWebhookConfiguration(metadata metav1.Object, webhooks []webhookSummarySource, now time.Time) WebhookConfigurationDTO {
	summaries := make([]WebhookRuleDTO, 0, min(len(webhooks), maximumWebhooks))
	for _, webhook := range webhooks {
		if len(summaries) == maximumWebhooks {
			break
		}
		bounded := make([]RuleDTO, 0, min(len(webhook.rules), maximumWebhookRules))
		for _, rule := range webhook.rules {
			if len(bounded) == maximumWebhookRules {
				break
			}
			operations := make([]string, 0, len(rule.Operations))
			for _, operation := range rule.Operations {
				operations = append(operations, string(operation))
			}
			bounded = append(bounded, RuleDTO{
				APIGroups: boundedStrings(rule.APIGroups),
				Resources: boundedStrings(rule.Resources),
				Verbs:     boundedStrings(operations),
			})
		}
		summaries = append(summaries, WebhookRuleDTO{
			Name: webhook.name, FailurePolicy: webhook.failurePolicy,
			Rules: bounded, Truncated: len(webhook.rules) > maximumWebhookRules,
		})
	}
	return WebhookConfigurationDTO{
		Name: metadata.GetName(), WebhookCount: len(webhooks), Webhooks: summaries,
		Truncated:  len(webhooks) > maximumWebhooks,
		AgeSeconds: int64(now.Sub(metadata.GetCreationTimestamp().Time) / time.Second),
	}
}

// ConvertMutatingWebhookConfiguration projects one mutating configuration.
func ConvertMutatingWebhookConfiguration(value *admissionregistrationv1.MutatingWebhookConfiguration, now time.Time) WebhookConfigurationDTO {
	sources := make([]webhookSummarySource, 0, len(value.Webhooks))
	for index := range value.Webhooks {
		webhook := &value.Webhooks[index]
		var policy *string
		if webhook.FailurePolicy != nil {
			policyValue := string(*webhook.FailurePolicy)
			policy = &policyValue
		}
		sources = append(sources, webhookSummarySource{name: webhook.Name, failurePolicy: policy, rules: webhook.Rules})
	}
	return convertWebhookConfiguration(value, sources, now)
}

// ConvertValidatingWebhookConfiguration projects one validating configuration.
func ConvertValidatingWebhookConfiguration(value *admissionregistrationv1.ValidatingWebhookConfiguration, now time.Time) WebhookConfigurationDTO {
	sources := make([]webhookSummarySource, 0, len(value.Webhooks))
	for index := range value.Webhooks {
		webhook := &value.Webhooks[index]
		var policy *string
		if webhook.FailurePolicy != nil {
			policyValue := string(*webhook.FailurePolicy)
			policy = &policyValue
		}
		sources = append(sources, webhookSummarySource{name: webhook.Name, failurePolicy: policy, rules: webhook.Rules})
	}
	return convertWebhookConfiguration(value, sources, now)
}

// IngressClassDTO is the bounded cluster-scoped class view.
type IngressClassDTO struct {
	Name       string  `json:"name"`
	Controller string  `json:"controller"`
	Default    bool    `json:"default"`
	Parameters *string `json:"parameters"`
	AgeSeconds int64   `json:"ageSeconds"`
}

func (IngressClassDTO) resourceListItem() {}

func (IngressClassDTO) resourceDetailItem() {}

// ConvertIngressClass projects one IngressClass onto the bounded DTO.
func ConvertIngressClass(value *networkingv1.IngressClass, now time.Time) IngressClassDTO {
	var parameters *string
	if value.Spec.Parameters != nil {
		label := value.Spec.Parameters.Kind
		if value.Spec.Parameters.Name != "" {
			label = value.Spec.Parameters.Kind + "/" + value.Spec.Parameters.Name
		}
		parameters = &label
	}
	return IngressClassDTO{
		Name: value.Name, Controller: value.Spec.Controller,
		Default:    value.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true",
		Parameters: parameters,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// NetworkPolicyDTO reports selectors, policy types and bounded rules.
type NetworkPolicyDTO struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	PodSelector string   `json:"podSelector"`
	PolicyTypes []string `json:"policyTypes"`
	RuleSummary []string `json:"ruleSummary"`
	AgeSeconds  int64    `json:"ageSeconds"`
}

func (NetworkPolicyDTO) resourceListItem() {}

func (NetworkPolicyDTO) resourceDetailItem() {}

func labelSelectorString(selector metav1.LabelSelector) string {
	parsed, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return ""
	}
	return parsed.String()
}

// ConvertNetworkPolicy projects one NetworkPolicy onto the bounded DTO.
func ConvertNetworkPolicy(value *networkingv1.NetworkPolicy, now time.Time) NetworkPolicyDTO {
	summary := make([]string, 0, maximumRuleStrings)
	for range value.Spec.Ingress {
		if len(summary) == maximumRuleStrings {
			break
		}
		summary = append(summary, "ingress rule")
	}
	for range value.Spec.Egress {
		if len(summary) == maximumRuleStrings {
			break
		}
		summary = append(summary, "egress rule")
	}
	types := make([]string, 0, len(value.Spec.PolicyTypes))
	for _, policyType := range value.Spec.PolicyTypes {
		types = append(types, string(policyType))
	}
	return NetworkPolicyDTO{
		Namespace: value.Namespace, Name: value.Name,
		PodSelector: labelSelectorString(value.Spec.PodSelector),
		PolicyTypes: types, RuleSummary: summary,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// EndpointsDTO is the bounded legacy Endpoints view with a visible
// truncation signal (V4-07).
type EndpointsDTO struct {
	Namespace     string   `json:"namespace"`
	Name          string   `json:"name"`
	ReadyCount    int      `json:"readyCount"`
	NotReadyCount int      `json:"notReadyCount"`
	Ports         []string `json:"ports"`
	Truncated     bool     `json:"truncated"`
	AgeSeconds    int64    `json:"ageSeconds"`
}

func (EndpointsDTO) resourceListItem() {}

func (EndpointsDTO) resourceDetailItem() {}

// ConvertEndpoints projects one Endpoints object onto the bounded DTO.
func ConvertEndpoints(value *corev1.Endpoints, now time.Time) EndpointsDTO {
	ready, notReady := 0, 0
	ports := make([]string, 0, 8)
	addressTotal := 0
	for _, subset := range value.Subsets {
		for _, port := range subset.Ports {
			if len(ports) < 8 {
				ports = append(ports, string(port.Protocol)+"/"+strconv.Itoa(int(port.Port)))
			}
		}
		ready += len(subset.Addresses)
		notReady += len(subset.NotReadyAddresses)
		addressTotal += len(subset.Addresses) + len(subset.NotReadyAddresses)
	}
	return EndpointsDTO{
		Namespace: value.Namespace, Name: value.Name,
		ReadyCount: ready, NotReadyCount: notReady, Ports: ports,
		Truncated:  addressTotal > maximumEndpointAddresses || len(value.Subsets) > maximumSubsets,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}
