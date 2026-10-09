package resources

import "testing"

func TestDynamicResourceIdentityAndPreferenceValidation(t *testing.T) {
	t.Parallel()
	for _, group := range []string{"", "core", "example.test"} {
		resource := DynamicResource{Group: group, Version: "v1beta1", Resource: "widgets", Kind: "Widget", Namespaced: true}
		parsed, ok := ParseDynamicCollection(resource.Collection())
		if !ok || parsed.Group != group || parsed.Resource != resource.Resource || !parsed.Namespaced {
			t.Fatalf("identity not reversible: %v", resource)
		}
		preferences := DefaultPreferences()
		preferences.CustomViews = []CustomViewContext{{ClusterProfileID: 1, Context: "dev", Cluster: "one", Items: []DynamicResource{resource}}}
		preferences.Columns.Hidden = map[string][]string{string(resource.Collection()): {"printer-1"}}
		if err := ValidatePreferences(preferences); err != nil {
			t.Fatal(err)
		}
		preferences.CustomViews[0].Items = append(preferences.CustomViews[0].Items, resource)
		if err := ValidatePreferences(preferences); ErrorCodeOf(err) != CodeValidationFailed {
			t.Fatal("duplicate view accepted")
		}
	}
	for _, identity := range []string{"dynamic:_:v1:pods/log:n", "dynamic:..:v1:pods:n", "dynamic:_:../v1:pods:n", "dynamic:_:v1:pods:all", "dynamic:http://host:v1:pods:n"} {
		if _, ok := ParseDynamicCollection(Collection(identity)); ok {
			t.Fatalf("invalid identity accepted: %s", identity)
		}
	}
	for _, collection := range []string{"deployments", "statefulsets", "gateways", "http-routes", "helm-releases", "helm-configmap-releases"} {
		preferences := DefaultPreferences()
		preferences.Columns.Order = map[string][]string{collection: {"namespace", "name", "status", "age"}}
		if err := ValidatePreferences(preferences); err != nil {
			t.Fatalf("column preference rejected: %s / %v", collection, err)
		}
	}
}
