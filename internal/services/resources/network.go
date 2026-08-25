package resources

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type TypedValueDTO struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}
type ServicePortDTO struct {
	Name        *string       `json:"name"`
	Protocol    string        `json:"protocol"`
	Port        int32         `json:"port"`
	TargetPort  TypedValueDTO `json:"targetPort"`
	NodePort    *int32        `json:"nodePort"`
	AppProtocol *string       `json:"appProtocol"`
}
type ExternalEndpointDTO struct {
	Address  string `json:"address"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
}
type ServiceDTO struct {
	Namespace         string                `json:"namespace"`
	Name              string                `json:"name"`
	Type              string                `json:"type"`
	ClusterIPs        []string              `json:"clusterIPs"`
	Ports             []ServicePortDTO      `json:"ports"`
	Selector          map[string]string     `json:"selector"`
	ExternalEndpoints []ExternalEndpointDTO `json:"externalEndpoints"`
}

func (ServiceDTO) resourceListItem() {}

type ServiceDetailDTO struct {
	Metadata              ResourceMetadataDTO `json:"metadata"`
	Summary               ServiceDTO          `json:"summary"`
	SessionAffinity       string              `json:"sessionAffinity"`
	ExternalTrafficPolicy *string             `json:"externalTrafficPolicy"`
	IPFamilies            []string            `json:"ipFamilies"`
	HealthCheckNodePort   *int32              `json:"healthCheckNodePort"`
}

func (ServiceDetailDTO) resourceDetailItem() {}

func ConvertService(value *corev1.Service) ServiceDTO {
	clusterIPs := append([]string(nil), value.Spec.ClusterIPs...)
	if len(clusterIPs) == 0 && value.Spec.ClusterIP != "" && value.Spec.ClusterIP != corev1.ClusterIPNone {
		clusterIPs = []string{value.Spec.ClusterIP}
	}
	ports := make([]ServicePortDTO, 0, len(value.Spec.Ports))
	for _, port := range value.Spec.Ports {
		if port.Port < 1 || port.Port > 65535 {
			continue
		}
		var nodePort *int32
		if port.NodePort > 0 {
			copy := port.NodePort
			nodePort = &copy
		}
		ports = append(ports, ServicePortDTO{Name: nullableString(port.Name), Protocol: normalizeProtocol(port.Protocol), Port: port.Port, TargetPort: intOrString(port.TargetPort), NodePort: nodePort, AppProtocol: cloneString(port.AppProtocol)})
	}
	addresses := append([]string(nil), value.Spec.ExternalIPs...)
	for _, ingress := range value.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			addresses = append(addresses, ingress.IP)
		} else if ingress.Hostname != "" {
			addresses = append(addresses, ingress.Hostname)
		}
	}
	addresses = uniqueStrings(addresses)
	external := make([]ExternalEndpointDTO, 0, min(256, len(addresses)*max(1, len(ports))))
	for _, address := range addresses {
		for _, port := range ports {
			if len(external) == 256 {
				break
			}
			external = append(external, ExternalEndpointDTO{Address: address, Port: port.Port, Protocol: port.Protocol})
		}
	}
	return ServiceDTO{Namespace: value.Namespace, Name: value.Name, Type: normalizeServiceType(value.Spec.Type), ClusterIPs: clusterIPs, Ports: ports, Selector: limitedStringMap(value.Spec.Selector), ExternalEndpoints: external}
}

func ServiceDetail(value *corev1.Service) ServiceDetailDTO {
	session := string(value.Spec.SessionAffinity)
	if session != string(corev1.ServiceAffinityClientIP) {
		session = string(corev1.ServiceAffinityNone)
	}
	var traffic *string
	if value.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyCluster || value.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyLocal {
		copy := string(value.Spec.ExternalTrafficPolicy)
		traffic = &copy
	}
	families := make([]string, 0, len(value.Spec.IPFamilies))
	for _, family := range value.Spec.IPFamilies {
		if family == corev1.IPv4Protocol || family == corev1.IPv6Protocol {
			families = append(families, string(family))
		}
	}
	var health *int32
	if value.Spec.HealthCheckNodePort > 0 {
		copy := value.Spec.HealthCheckNodePort
		health = &copy
	}
	return ServiceDetailDTO{Metadata: ConvertMetadata(value), Summary: ConvertService(value), SessionAffinity: session, ExternalTrafficPolicy: traffic, IPFamilies: families, HealthCheckNodePort: health}
}

type IngressBackendDTO struct {
	ServiceName string        `json:"serviceName"`
	ServicePort TypedValueDTO `json:"servicePort"`
}
type IngressPathDTO struct {
	Host     string            `json:"host"`
	Path     string            `json:"path"`
	PathType string            `json:"pathType"`
	Backend  IngressBackendDTO `json:"backend"`
}
type IngressDTO struct {
	Namespace string           `json:"namespace"`
	Name      string           `json:"name"`
	ClassName *string          `json:"className"`
	Hosts     []string         `json:"hosts"`
	Paths     []IngressPathDTO `json:"paths"`
	TLSHosts  []string         `json:"tlsHosts"`
}

func (IngressDTO) resourceListItem() {}

type IngressDetailDTO struct {
	Metadata              ResourceMetadataDTO `json:"metadata"`
	Summary               IngressDTO          `json:"summary"`
	DefaultBackend        *IngressBackendDTO  `json:"defaultBackend"`
	LoadBalancerAddresses []string            `json:"loadBalancerAddresses"`
}

func (IngressDetailDTO) resourceDetailItem() {}

func ConvertIngress(value *networkingv1.Ingress) (IngressDTO, error) {
	result := IngressDTO{Namespace: value.Namespace, Name: value.Name, ClassName: cloneString(value.Spec.IngressClassName), Hosts: []string{}, Paths: []IngressPathDTO{}, TLSHosts: []string{}}
	for _, rule := range value.Spec.Rules {
		if rule.Host != "" {
			result.Hosts = append(result.Hosts, rule.Host)
		}
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			backend, err := ingressBackend(path.Backend)
			if err != nil {
				return IngressDTO{}, err
			}
			pathType := normalizePathType(path.PathType)
			result.Paths = append(result.Paths, IngressPathDTO{Host: rule.Host, Path: path.Path, PathType: pathType, Backend: backend})
		}
	}
	for _, tls := range value.Spec.TLS {
		result.TLSHosts = append(result.TLSHosts, tls.Hosts...)
	}
	result.Hosts = uniqueStrings(result.Hosts)
	result.TLSHosts = uniqueStrings(result.TLSHosts)
	return result, nil
}

func IngressDetail(value *networkingv1.Ingress) (IngressDetailDTO, error) {
	summary, err := ConvertIngress(value)
	if err != nil {
		return IngressDetailDTO{}, err
	}
	var defaultBackend *IngressBackendDTO
	if value.Spec.DefaultBackend != nil {
		converted, convertErr := ingressBackend(*value.Spec.DefaultBackend)
		if convertErr != nil {
			return IngressDetailDTO{}, convertErr
		}
		defaultBackend = &converted
	}
	addresses := make([]string, 0, len(value.Status.LoadBalancer.Ingress))
	for _, ingress := range value.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			addresses = append(addresses, ingress.IP)
		} else if ingress.Hostname != "" {
			addresses = append(addresses, ingress.Hostname)
		}
	}
	return IngressDetailDTO{Metadata: ConvertMetadata(value), Summary: summary, DefaultBackend: defaultBackend, LoadBalancerAddresses: uniqueStrings(addresses)}, nil
}

type EndpointSlicePortDTO struct {
	Name        *string `json:"name"`
	Protocol    *string `json:"protocol"`
	Port        *int32  `json:"port"`
	AppProtocol *string `json:"appProtocol"`
}
type EndpointConditionsDTO struct {
	Ready       *bool `json:"ready"`
	Serving     *bool `json:"serving"`
	Terminating *bool `json:"terminating"`
}
type EndpointDTO struct {
	Addresses  []string              `json:"addresses"`
	Hostname   *string               `json:"hostname"`
	NodeName   *string               `json:"nodeName"`
	Zone       *string               `json:"zone"`
	Conditions EndpointConditionsDTO `json:"conditions"`
	TargetRef  *ResourceRef          `json:"targetRef"`
}
type EndpointSliceDTO struct {
	Namespace   string                 `json:"namespace"`
	Name        string                 `json:"name"`
	AddressType string                 `json:"addressType"`
	Ports       []EndpointSlicePortDTO `json:"ports"`
	Endpoints   []EndpointDTO          `json:"endpoints"`
}

func (EndpointSliceDTO) resourceListItem() {}

type EndpointSliceDetailDTO struct {
	Metadata ResourceMetadataDTO `json:"metadata"`
	Summary  EndpointSliceDTO    `json:"summary"`
}

func (EndpointSliceDetailDTO) resourceDetailItem() {}

func ConvertEndpointSlice(value *discoveryv1.EndpointSlice) EndpointSliceDTO {
	ports := make([]EndpointSlicePortDTO, 0, len(value.Ports))
	for _, port := range value.Ports {
		var protocol *string
		if port.Protocol != nil {
			normalized := normalizeProtocol(*port.Protocol)
			protocol = &normalized
		}
		ports = append(ports, EndpointSlicePortDTO{Name: cloneString(port.Name), Protocol: protocol, Port: cloneInt32(port.Port), AppProtocol: cloneString(port.AppProtocol)})
	}
	maximum := min(len(value.Endpoints), 1000)
	endpoints := make([]EndpointDTO, 0, maximum)
	for _, endpoint := range value.Endpoints[:maximum] {
		endpoints = append(endpoints, EndpointDTO{Addresses: append([]string(nil), endpoint.Addresses...), Hostname: cloneString(endpoint.Hostname), NodeName: cloneString(endpoint.NodeName), Zone: cloneString(endpoint.Zone), Conditions: EndpointConditionsDTO{Ready: cloneBool(endpoint.Conditions.Ready), Serving: cloneBool(endpoint.Conditions.Serving), Terminating: cloneBool(endpoint.Conditions.Terminating)}, TargetRef: objectReference(endpoint.TargetRef)})
	}
	addressType := string(value.AddressType)
	if addressType != string(discoveryv1.AddressTypeIPv4) && addressType != string(discoveryv1.AddressTypeIPv6) && addressType != string(discoveryv1.AddressTypeFQDN) {
		addressType = "Unknown"
	}
	return EndpointSliceDTO{Namespace: value.Namespace, Name: value.Name, AddressType: addressType, Ports: ports, Endpoints: endpoints}
}

func EndpointSliceDetail(value *discoveryv1.EndpointSlice) EndpointSliceDetailDTO {
	return EndpointSliceDetailDTO{Metadata: ConvertMetadata(value), Summary: ConvertEndpointSlice(value)}
}

func normalizeServiceType(value corev1.ServiceType) string {
	switch value {
	case corev1.ServiceTypeClusterIP, corev1.ServiceTypeNodePort, corev1.ServiceTypeLoadBalancer, corev1.ServiceTypeExternalName:
		return string(value)
	default:
		return "Unknown"
	}
}
func intOrString(value intstr.IntOrString) TypedValueDTO {
	if value.Type == intstr.String {
		return TypedValueDTO{Type: "name", Value: value.StrVal}
	}
	return TypedValueDTO{Type: "number", Value: value.IntVal}
}
func ingressBackend(value networkingv1.IngressBackend) (IngressBackendDTO, error) {
	if value.Service == nil || value.Resource != nil {
		return IngressBackendDTO{}, domainError(CodeFeatureUnavailable, "This Ingress backend type is unavailable.", nil)
	}
	return IngressBackendDTO{ServiceName: value.Service.Name, ServicePort: backendPort(value.Service.Port)}, nil
}
func backendPort(value networkingv1.ServiceBackendPort) TypedValueDTO {
	if value.Name != "" {
		return TypedValueDTO{Type: "name", Value: value.Name}
	}
	return TypedValueDTO{Type: "number", Value: value.Number}
}
func normalizePathType(value *networkingv1.PathType) string {
	if value == nil {
		return string(networkingv1.PathTypeImplementationSpecific)
	}
	switch *value {
	case networkingv1.PathTypeExact, networkingv1.PathTypePrefix, networkingv1.PathTypeImplementationSpecific:
		return string(*value)
	default:
		return string(networkingv1.PathTypeImplementationSpecific)
	}
}
func objectReference(value *corev1.ObjectReference) *ResourceRef {
	if value == nil || value.Kind == "" || value.Name == "" {
		return nil
	}
	return &ResourceRef{APIGroup: apiGroup(value.APIVersion), Kind: value.Kind, Namespace: value.Namespace, Name: value.Name, UID: string(value.UID)}
}
func apiGroup(apiVersion string) string {
	for index, value := range apiVersion {
		if value == '/' {
			return apiVersion[:index]
		}
	}
	return ""
}
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
