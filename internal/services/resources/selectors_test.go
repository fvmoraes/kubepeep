package resources

import "testing"

func TestNormalizeSelectorsUsesConservativeResourceMatrix(t *testing.T) {
	cases := []struct {
		name       string
		collection Collection
		options    ListOptions
		wantErr    bool
		wantField  string
	}{
		{name: "pod node pushdown", collection: CollectionPods, options: ListOptions{Node: "worker-1"}, wantField: "spec.nodeName=worker-1"},
		{name: "event exact pushdown", collection: CollectionEvents, options: ListOptions{ObjectKind: "Pod", Reason: "Unhealthy"}, wantField: "involvedObject.kind=Pod,reason=Unhealthy"},
		{name: "unsupported service field", collection: CollectionServices, options: ListOptions{FieldSelector: "spec.clusterIP=10.0.0.1"}, wantErr: true},
		{name: "invalid label grammar", collection: CollectionPods, options: ListOptions{LabelSelector: "app in ("}, wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := NormalizeListOptions(testCase.collection, testCase.options)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("invalid selector was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.FieldSelector != testCase.wantField {
				t.Fatalf("field selector = %q, want %q", result.FieldSelector, testCase.wantField)
			}
		})
	}
}
