package resources

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// DynamicResource is a discovered resource identity, never an arbitrary URL.
type DynamicResource struct {
	Group      string   `json:"group"`
	Version    string   `json:"version"`
	Resource   string   `json:"resource"`
	Kind       string   `json:"kind"`
	Namespaced bool     `json:"namespaced"`
	ShortNames []string `json:"shortNames,omitempty"`
}

type CustomViewContext struct {
	ClusterProfileID int64             `json:"clusterProfileId"`
	Context          string            `json:"context"`
	Cluster          string            `json:"cluster"`
	Items            []DynamicResource `json:"items"`
}

type ResourceDiscoveryDTO struct {
	Resources []DynamicResource  `json:"resources"`
	Failures  []DiscoveryFailure `json:"failures"`
	Truncated bool               `json:"truncated"`
}
type DiscoveryFailure struct {
	GroupVersion string `json:"groupVersion"`
	Code         string `json:"code"`
	Message      string `json:"message"`
}
type DynamicColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Format   string `json:"format,omitempty"`
	Priority int32  `json:"priority,omitempty"`
}

// Rows contain only metadata and the server's printable scalar cells. Full
// resource documents never enter list, cursor or warm-inventory caches.
type DynamicRow struct {
	Name            string          `json:"name"`
	Namespace       string          `json:"namespace,omitempty"`
	Kind            string          `json:"kind"`
	UID             string          `json:"uid,omitempty"`
	ResourceVersion string          `json:"resourceVersion,omitempty"`
	CreatedAt       string          `json:"createdAt,omitempty"`
	AgeSeconds      int64           `json:"ageSeconds"`
	Columns         []DynamicColumn `json:"columns"`
	Cells           []string        `json:"cells"`
	Truncated       bool            `json:"truncated,omitempty"`
}

func (DynamicRow) resourceListItem()   {}
func (DynamicRow) resourceDetailItem() {}

type DynamicDocument struct {
	YAML       string `json:"yaml"`
	Generation string `json:"generation"`
}

func (DynamicDocument) resourceDetailItem() {}

var dynamicSegment = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var dynamicGroup = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)
var dynamicKind = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,127}$`)

func (r DynamicResource) Valid() bool {
	if r.Group != "" && (!dynamicGroup.MatchString(r.Group) || strings.Contains(r.Group, "..")) {
		return false
	}
	return dynamicSegment.MatchString(r.Version) && dynamicSegment.MatchString(r.Resource)
}
func (r DynamicResource) Collection() Collection {
	group, scope := r.Group, "n"
	if group == "" {
		group = "_"
	}
	if !r.Namespaced {
		scope = "c"
	}
	return Collection("dynamic:" + group + ":" + r.Version + ":" + r.Resource + ":" + scope)
}
func ParseDynamicCollection(collection Collection) (DynamicResource, bool) {
	parts := strings.Split(string(collection), ":")
	if len(parts) != 5 || parts[0] != "dynamic" || (parts[4] != "n" && parts[4] != "c") {
		return DynamicResource{}, false
	}
	r := DynamicResource{Group: parts[1], Version: parts[2], Resource: parts[3], Namespaced: parts[4] == "n"}
	if r.Group == "_" {
		r.Group = ""
	}
	return r, r.Valid()
}
func validateCustomViews(contexts []CustomViewContext) error {
	if len(contexts) > 100 {
		return validationError("custom views exceed 100 contexts")
	}
	seen := map[string]bool{}
	for _, context := range contexts {
		if context.ClusterProfileID <= 0 || strings.TrimSpace(context.Context) == "" || strings.TrimSpace(context.Cluster) == "" || len(context.Context) > 1024 || len(context.Cluster) > 1024 || !utf8.ValidString(context.Context+context.Cluster) || len(context.Items) > 32 {
			return validationError("custom view context is invalid or exceeds 32 views")
		}
		key := fmt.Sprintf("%d\x00%s\x00%s", context.ClusterProfileID, context.Context, context.Cluster)
		if seen[key] {
			return validationError("custom view context is duplicated")
		}
		seen[key] = true
		items := map[Collection]bool{}
		for _, item := range context.Items {
			if !item.Valid() || !dynamicKind.MatchString(item.Kind) || len(item.ShortNames) > 16 {
				return validationError("custom resource identity is invalid")
			}
			for _, name := range item.ShortNames {
				if !dynamicSegment.MatchString(name) {
					return validationError("custom resource short name is invalid")
				}
			}
			if items[item.Collection()] {
				return validationError("custom resource view is duplicated")
			}
			items[item.Collection()] = true
		}
	}
	encoded, err := json.Marshal(contexts)
	if err != nil {
		return fmt.Errorf("encode custom views: %w", err)
	}
	if len(encoded) > maximumPreferenceJSONBytes {
		return validationError("custom views exceed 64 KiB")
	}
	return nil
}
