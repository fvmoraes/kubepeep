package resources

import (
	"strings"
	"testing"
)

func TestNormalizeListOptionsAppliesBoundedCanonicalDefaults(t *testing.T) {
	problematic := true
	options, err := NormalizeListOptions(CollectionPods, ListOptions{Namespaces: []string{"team-b", "team-a"}, Statuses: []string{"Unknown", "Running"}, Problematic: &problematic, Restarts: RestartGTE3})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if options.Limit != 100 || options.Sort != "identity" || options.Order != OrderAscending {
		t.Fatalf("unexpected defaults: %+v", options)
	}
	if got := strings.Join(options.Namespaces, ","); got != "team-a,team-b" {
		t.Fatalf("namespace order = %q", got)
	}
	if got := strings.Join(options.Statuses, ","); got != "Running,Unknown" {
		t.Fatalf("status order = %q", got)
	}
}

func TestNormalizeWorkloadKindsUsesExactCanonicalOrder(t *testing.T) {
	options, err := NormalizeListOptions(CollectionWorkloads, ListOptions{Kinds: []WorkloadKind{WorkloadJobs, WorkloadDeployments}})
	if err != nil {
		t.Fatal(err)
	}
	if got := options.Kinds; len(got) != 2 || got[0] != WorkloadDeployments || got[1] != WorkloadJobs {
		t.Fatalf("kinds = %#v", got)
	}
}

func TestNormalizeListOptionsRejectsUnboundedAndCrossCollectionFields(t *testing.T) {
	tests := []ListOptions{{Limit: 501}, {Search: strings.Repeat("x", 257)}, {Continue: strings.Repeat("x", MaximumCursorBytes+1)}, {Statuses: []string{"made-up"}}, {Kinds: []WorkloadKind{WorkloadJobs}}, {ObjectKind: "Pod"}}
	for index, options := range tests {
		if _, err := NormalizeListOptions(CollectionPods, options); ErrorCodeOf(err) != CodeValidationFailed {
			t.Fatalf("case %d: %v", index, err)
		}
	}
}

func TestResolveNamespacesCanOnlyNarrowActiveScope(t *testing.T) {
	got, err := ResolveNamespaces([]string{"zeta", "alpha"}, []string{"alpha"})
	if err != nil || len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("got %#v, %v", got, err)
	}
	if _, err = ResolveNamespaces([]string{"alpha"}, []string{"other"}); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("outside scope: %v", err)
	}
	large := make([]string, MaximumNamespaces+1)
	for index := range large {
		large[index] = "namespace-" + string(rune('a'+index%26)) + string(rune('a'+index/26))
	}
	got, err = ResolveNamespaces(large, []string{large[len(large)-1]})
	if err != nil || len(got) != 1 || got[0] != large[len(large)-1] {
		t.Fatalf("bounded subset of global scope = %#v, %v", got, err)
	}
	if _, err = ResolveNamespaces(large, nil); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("unbounded fan-out: %v", err)
	}
}

func TestContainsFoldedUsesUnicodeSimpleFolding(t *testing.T) {
	if !ContainsFolded("ΟΣ", "ος") {
		t.Fatal("expected Greek final sigma to match")
	}
	if ContainsFolded("payments", "PAYROLL") {
		t.Fatal("unexpected substring match")
	}
}
