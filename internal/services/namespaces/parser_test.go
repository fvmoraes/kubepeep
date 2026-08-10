package namespaces

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRawTextPreservesOrderAndCounters(t *testing.T) {
	report, err := ParseRawInput("\ufeff payments,billing\ninvoices; payments\n\nBad_Name,*;")
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, report.Valid, []string{"payments", "billing", "invoices"})
	if report.ValidCount != 3 || report.DuplicateCount != 1 || report.DiscardedEmptyCount != 2 || report.InvalidCount != 2 {
		t.Fatalf("unexpected counters: %#v", report)
	}
	if report.Invalid[0].Input != "Bad_Name" || report.Invalid[1].Input != "*" {
		t.Fatalf("invalid order = %#v", report.Invalid)
	}
}

func TestParseRawTextUsesRunsOfSpaceAndTabAsDelimiters(t *testing.T) {
	report, err := ParseRawInput("alpha   beta\tgamma")
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, report.Valid, []string{"alpha", "beta", "gamma"})
	if report.DiscardedEmptyCount != 0 {
		t.Fatalf("discarded empty = %d", report.DiscardedEmptyCount)
	}
}

func TestParseRawInputRemovesExternalWhitespaceThenBOM(t *testing.T) {
	report, err := ParseRawInput(" \n\ufeff payments,billing \n")
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, report.Valid, []string{"payments", "billing"})
}

func TestParseStrictJSONForms(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "array", input: `["payments","","payments","Bad_Name"]`},
		{name: "object", input: `{"namespaces":["payments","","payments","Bad_Name"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := ParseRawInput(test.input)
			if err != nil {
				t.Fatal(err)
			}
			assertStrings(t, report.Valid, []string{"payments"})
			if report.DuplicateCount != 1 || report.DiscardedEmptyCount != 1 || report.InvalidCount != 1 {
				t.Fatalf("unexpected report: %#v", report)
			}
		})
	}
}

func TestJSONLookingInputNeverFallsBackToText(t *testing.T) {
	inputs := []string{
		`["payments"] trailing`,
		`["payments", 42]`,
		`null`,
		`{"namespaces":["payments"],"extra":[]}`,
		`{"namespaces":["payments"],"namespaces":["billing"]}`,
		`{"other":["payments"]}`,
		`{"namespaces":null}`,
		`{"namespaces":["payments"]`,
	}
	for _, input := range inputs {
		if input == "null" {
			// The grammar commits to JSON only for '[' and '{'; bare null is
			// therefore the valid bare namespace name "null".
			report, err := ParseRawInput(input)
			if err != nil || report.ValidCount != 1 || report.Valid[0] != "null" {
				t.Fatalf("bare null report = %#v, %v", report, err)
			}
			continue
		}
		_, err := ParseRawInput(input)
		if !errors.Is(err, ErrInvalidNamespaceInput) {
			t.Fatalf("input %q error = %v", input, err)
		}
	}
}

func TestParseSimpleYAMLForms(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "top level sequence", input: "- payments\n- billing"},
		{name: "mapping", input: "namespaces:\n  - payments\n  - billing"},
		{name: "document marker", input: "---\nnamespaces:\n  - payments\n  - billing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := ParseRawInput(test.input)
			if err != nil {
				t.Fatal(err)
			}
			assertStrings(t, report.Valid, []string{"payments", "billing"})
		})
	}
}

func TestYAMLLookingInputRejectsComplexOrMalformedYAMLWithoutFallback(t *testing.T) {
	inputs := []string{
		"- &base payments\n- *base",
		"- !!str payments",
		"- 42",
		"- {name: payments}",
		"namespaces: payments",
		"namespaces:\n  - payments\nextra:\n  - billing",
		"---\n- payments\n---\n- billing",
		"namespaces: [",
		"---\nplain-scalar",
	}
	for _, input := range inputs {
		_, err := ParseRawInput(input)
		if !errors.Is(err, ErrInvalidNamespaceInput) {
			t.Fatalf("input %q error = %v", input, err)
		}
	}
}

func TestEmptyAndDuplicateInvalidItemsAreCountedOncePerRule(t *testing.T) {
	report, err := ParseNamespaceList([]string{"", " ", "Bad_Name", "Bad_Name", "payments", "payments"})
	if err != nil {
		t.Fatal(err)
	}
	if report.DiscardedEmptyCount != 2 || report.DuplicateCount != 2 || report.InvalidCount != 1 || report.ValidCount != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestNamespaceEntryAndByteLimits(t *testing.T) {
	_, err := ParseNamespaceList(make([]string, MaxNamespaceEntries+1))
	if !errors.Is(err, ErrNamespaceLimit) {
		t.Fatalf("entry limit error = %v", err)
	}
	_, err = ParseRawInput(strings.Repeat("a", MaxScopeBodyBytes+1))
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("byte limit error = %v", err)
	}
}

func TestValidNamespaceNameUsesKubernetesDNSLabelRules(t *testing.T) {
	valid := []string{"default", "payments-2", "a"}
	invalid := []string{"", "*", "Bad_Name", "contains.dot", "-leading", strings.Repeat("a", 64)}
	for _, name := range valid {
		if !ValidNamespaceName(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	for _, name := range invalid {
		if ValidNamespaceName(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func assertStrings(t *testing.T, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("strings = %#v, want %#v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("strings = %#v, want %#v", actual, expected)
		}
	}
}
