package resources

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	kvalidation "k8s.io/apimachinery/pkg/util/validation"
)

// SavedPortForward stores an inactive recipe, never a live session or credentials.
// Re-enabling must pass the normal selection, scope and Kubernetes checks.
type SavedPortForward struct {
	ClusterProfileID int64  `json:"clusterProfileId"`
	Context          string `json:"context"`
	Namespace        string `json:"namespace"`
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	RemotePort       int    `json:"remotePort"`
	LocalPort        int    `json:"localPort"`
}

func validateSavedPortForwards(values []SavedPortForward) error {
	if len(values) > 100 {
		return validationError("saved port forwards exceed 100 entries")
	}
	seen := map[SavedPortForward]bool{}
	for _, value := range values {
		if value.ClusterProfileID <= 0 || strings.TrimSpace(value.Context) == "" || len(value.Context) > 1024 || !utf8.ValidString(value.Context) || (value.Kind != "Pod" && value.Kind != "Service") {
			return validationError("saved port forward target is invalid")
		}
		if len(kvalidation.IsDNS1123Label(value.Namespace)) != 0 || len(kvalidation.IsDNS1123Subdomain(value.Name)) != 0 {
			return validationError("saved port forward namespace or name is invalid")
		}
		if value.LocalPort < 1024 || value.LocalPort > 65535 || value.RemotePort < 1 || value.RemotePort > 65535 {
			return validationError("saved port forward port is invalid")
		}
		identity := value
		identity.LocalPort, identity.RemotePort = 0, 0
		if seen[identity] {
			return validationError("saved port forward target must be unique")
		}
		seen[identity] = true
	}
	encoded, _ := json.Marshal(values)
	if len(encoded) > maximumPreferenceJSONBytes {
		return validationError("saved port forwards exceed 64 KiB")
	}
	return nil
}
