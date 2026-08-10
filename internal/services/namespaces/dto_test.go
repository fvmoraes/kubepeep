package namespaces

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDecodeScopeWriteRequestIsStrictAndTracksInputPresence(t *testing.T) {
	request, err := DecodeScopeWriteRequest(strings.NewReader(`{
		"clusterProfileId":1,
		"name":"Finance",
		"context":"development",
		"mode":"list",
		"namespaces":[],
		"defaultNamespace":null
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !request.NamespacesPresent || request.Namespaces == nil || request.RawInputPresent {
		t.Fatalf("presence = %#v", request)
	}

	tests := []struct {
		name string
		body string
		want error
	}{
		{name: "unknown field", body: `{"clusterProfileId":1,"unknown":true}`, want: ErrUnknownField},
		{name: "trailing content", body: `{} {}`, want: ErrInvalidJSON},
		{name: "empty", body: ` `, want: ErrInvalidJSON},
		{name: "null namespaces", body: `{"namespaces":null}`, want: ErrInvalidJSON},
		{name: "wrong raw input type", body: `{"rawInput":[]}`, want: ErrInvalidJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeScopeWriteRequest(strings.NewReader(test.body))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDecodeScopeRequestsEnforceOneMiBBodyLimit(t *testing.T) {
	_, err := DecodeScopeWriteRequest(strings.NewReader(strings.Repeat(" ", MaxScopeBodyBytes+1)))
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestScopeDTOUsesIndependentSlicesAndRFC3339Times(t *testing.T) {
	created := time.Date(2026, 7, 27, 12, 0, 0, 0, time.FixedZone("offset", -3*60*60))
	scope := Scope{ID: 7, Namespaces: []string{"payments"}, CreatedAt: created, UpdatedAt: created}
	dto := NewScopeDTO(scope)
	scope.Namespaces[0] = "changed"
	if dto.Namespaces[0] != "payments" || dto.CreatedAt != "2026-07-27T15:00:00Z" {
		t.Fatalf("dto = %#v", dto)
	}
}

func TestScopeDTORepresentsAllItemsAsAnEmptyArray(t *testing.T) {
	dto := NewScopeDTO(Scope{})
	if dto.Namespaces == nil || len(dto.Namespaces) != 0 {
		t.Fatalf("namespaces = %#v", dto.Namespaces)
	}
}
