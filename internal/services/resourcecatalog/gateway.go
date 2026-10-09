package resourcecatalog

const GatewayGroup = "gateway.networking.k8s.io"

func GatewayResources() []Resource {
	result := []Resource{}
	for _, resource := range resources {
		if resource.Group == GatewayGroup {
			result = append(result, resource)
		}
	}
	return result
}

// Try only published Gateway API versions, never a group or resource supplied
// by a YAML document. Older clusters can continue serving beta/alpha versions.
func Versions(resource Resource) []string {
	if resource.Group == GatewayGroup {
		return []string{"v1", "v1beta1", "v1alpha3", "v1alpha2"}
	}
	return []string{resource.Version}
}
