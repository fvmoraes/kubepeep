package namespaces

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateDraftEnforcesEveryScopeModeInvariant(t *testing.T) {
	defaultPayments := "payments"
	base := ScopeDraft{ClusterProfileID: 1, Context: "development", Name: "Finance"}
	tests := []struct {
		name    string
		draft   ScopeDraft
		wantErr bool
	}{
		{name: "single", draft: mergeDraft(base, ScopeModeSingle, []string{"payments"}, &defaultPayments)},
		{name: "single too many", draft: mergeDraft(base, ScopeModeSingle, []string{"payments", "billing"}, nil), wantErr: true},
		{name: "list", draft: mergeDraft(base, ScopeModeList, []string{"payments", "billing"}, &defaultPayments)},
		{name: "list empty", draft: mergeDraft(base, ScopeModeList, nil, nil), wantErr: true},
		{name: "default absent", draft: mergeDraft(base, ScopeModeList, []string{"billing"}, &defaultPayments), wantErr: true},
		{name: "all", draft: mergeDraft(base, ScopeModeAll, nil, nil)},
		{name: "all wildcard", draft: mergeDraft(base, ScopeModeAll, []string{"*"}, nil), wantErr: true},
		{name: "duplicate", draft: mergeDraft(base, ScopeModeList, []string{"payments", "payments"}, nil), wantErr: true},
		{name: "bad context bytes", draft: ScopeDraft{ClusterProfileID: 1, Context: strings.Repeat("x", MaxContextBytes+1), Name: "Finance", Mode: ScopeModeAll}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateDraft(test.draft)
			if test.wantErr && !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v", err)
			}
			if !test.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mergeDraft(base ScopeDraft, mode ScopeMode, items []string, defaultNamespace *string) ScopeDraft {
	base.Mode = mode
	base.Namespaces = items
	base.DefaultNamespace = defaultNamespace
	return base
}
