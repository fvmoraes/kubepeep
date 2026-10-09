package resources

import "testing"

func TestEveryInventoryHasAnAuthorizedOrigin(t *testing.T) {
	t.Parallel()
	for collection := range rulesByCollection {
		t.Run(string(collection), func(t *testing.T) {
			t.Parallel()
			if isClusterScoped(collection) {
				origin, err := ClusterOriginFor(collection)
				if err != nil || origin.Namespace != "" || origin.Resource == "" || origin.Version == "" {
					t.Fatalf("cluster origin = %#v, %v", origin, err)
				}
				if _, err := NormalizeListOptions(collection, ListOptions{Namespaces: []string{"payments"}}); ErrorCodeOf(err) != CodeValidationFailed {
					t.Fatalf("cluster collection accepted a namespace: %v", err)
				}
				return
			}
			origins, err := OriginsFor(collection, []string{"payments"}, nil)
			if err != nil || len(origins) == 0 {
				t.Fatalf("origins = %#v, %v", origins, err)
			}
			for _, origin := range origins {
				if origin.Namespace != "payments" || origin.Version == "" || origin.Resource == "" {
					t.Fatalf("invalid origin: %#v", origin)
				}
			}
		})
	}
}

func TestExpandedInventoryOriginsMatchKubernetesAPIs(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		collection               Collection
		group, version, resource string
		cluster                  bool
	}{
		{CollectionHPAs, "autoscaling", "v2", "horizontalpodautoscalers", false},
		{CollectionPDBs, "policy", "v1", "poddisruptionbudgets", false},
		{CollectionServiceAccounts, "", "v1", "serviceaccounts", false},
		{CollectionResourceQuotas, "", "v1", "resourcequotas", false},
		{CollectionLimitRanges, "", "v1", "limitranges", false},
		{CollectionClusterRoles, "rbac.authorization.k8s.io", "v1", "clusterroles", true},
		{CollectionClusterRoleBindings, "rbac.authorization.k8s.io", "v1", "clusterrolebindings", true},
		{CollectionCRDs, "apiextensions.k8s.io", "v1", "customresourcedefinitions", true},
		{CollectionIngressClasses, "networking.k8s.io", "v1", "ingressclasses", true},
		{CollectionPriorityClasses, "scheduling.k8s.io", "v1", "priorityclasses", true},
		{CollectionRuntimeClasses, "node.k8s.io", "v1", "runtimeclasses", true},
		{CollectionMutatingWebhooks, "admissionregistration.k8s.io", "v1", "mutatingwebhookconfigurations", true},
		{CollectionValidatingWebhooks, "admissionregistration.k8s.io", "v1", "validatingwebhookconfigurations", true},
	} {
		t.Run(string(tt.collection), func(t *testing.T) {
			t.Parallel()
			var origin Origin
			if tt.cluster {
				value, err := ClusterOriginFor(tt.collection)
				if err != nil {
					t.Fatal(err)
				}
				origin = value
			} else {
				values, err := OriginsFor(tt.collection, []string{"payments"}, nil)
				if err != nil {
					t.Fatal(err)
				}
				origin = values[0]
			}
			if origin.APIGroup != tt.group || origin.Version != tt.version || origin.Resource != tt.resource {
				t.Fatalf("origin = %#v", origin)
			}
		})
	}
}
