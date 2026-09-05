package authorization

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
)

func TestAllowlistExactlyMatchesDocumentedMVPIDs(t *testing.T) {
	want := []string{
		"namespaces.list",
		"pods.list", "pods.get", "pods.watch", "pods.logs.get", "pods.delete", "pods.exec.create", "pods.portforward.create",
		"events.list", "events.watch",
		"deployments.list", "deployments.get", "deployments.watch", "deployments.restart", "deployments.scale",
		"statefulsets.list", "statefulsets.get", "statefulsets.watch", "statefulsets.scale",
		"daemonsets.list", "daemonsets.get", "daemonsets.watch",
		"jobs.list", "jobs.get", "jobs.watch",
		"cronjobs.list", "cronjobs.get", "cronjobs.watch",
		"services.list", "services.get", "services.watch",
		"ingresses.list", "ingresses.get", "ingresses.watch",
		"endpoint-slices.list", "endpoint-slices.get", "endpoint-slices.watch",
		"configmaps.list", "configmaps.get", "configmaps.watch",
		"secrets.list", "secrets.get",
		"nodes.list", "nodes.get",
		"persistentvolumes.list", "persistentvolumes.get",
		"persistentvolumeclaims.list", "persistentvolumeclaims.get",
		"storageclasses.list", "storageclasses.get",
		"volumeattachments.list", "volumeattachments.get",
		"csinodes.list", "csinodes.get",
		"csidrivers.list", "csidrivers.get",
		"leases.list", "leases.get",
		"namespaces.get",
		"metrics.pods.list",
	}
	allowlist := Allowlist()
	if len(allowlist) != len(want) {
		t.Fatalf("allowlist length = %d, want %d", len(allowlist), len(want))
	}
	seen := make(map[string]struct{}, len(allowlist))
	for index, specification := range allowlist {
		if specification.ID != want[index] {
			t.Fatalf("allowlist[%d] = %q, want %q", index, specification.ID, want[index])
		}
		if _, duplicate := seen[specification.ID]; duplicate {
			t.Fatalf("duplicate capability ID %q", specification.ID)
		}
		seen[specification.ID] = struct{}{}
		if specification.ResourceNamePolicy == ResourceNameEmpty && specification.Scope == ScopeCluster && specification.ID != "namespaces.list" && specification.ID != "nodes.list" && !isClusterListCapability(specification.ID) {
			t.Fatalf("unexpected cluster capability: %+v", specification)
		}
	}

	// The returned slice is a defensive copy.
	allowlist[0].ID = "changed"
	if got := Allowlist()[0].ID; got != "namespaces.list" {
		t.Fatalf("allowlist mutated through caller: %q", got)
	}
}

// isClusterListCapability enumerates the approved cluster-scoped empty-name
// list capabilities beyond namespaces.list.
func isClusterListCapability(id string) bool {
	switch id {
	case "nodes.list", "persistentvolumes.list", "storageclasses.list",
		"volumeattachments.list", "csinodes.list", "csidrivers.list":
		return true
	}
	return false
}

func TestKeyForCapabilitySeparatesResourceAndSubresource(t *testing.T) {
	key, err := KeyForCapability("gen_42", "payments", "pods.logs.get", "api-0")
	if err != nil {
		t.Fatal(err)
	}
	if key.Resource != "pods" || key.Subresource != "log" || key.Verb != "get" || key.ResourceName != "api-0" {
		t.Fatalf("key = %+v", key)
	}
	clusterKey, err := KeyForCapability("gen_42", "", "namespaces.list", "")
	if err != nil {
		t.Fatal(err)
	}
	if clusterKey.Namespace != "" || clusterKey.Resource != "namespaces" {
		t.Fatalf("cluster key = %+v", clusterKey)
	}

	invalid := []struct {
		namespace, id, name string
	}{
		{namespace: "payments", id: "not.allowlisted"},
		{namespace: "payments", id: "namespaces.list"},
		{id: "pods.list"},
		{namespace: "payments", id: "pods.list", name: "api"},
	}
	for _, test := range invalid {
		if _, err := KeyForCapability("gen_42", test.namespace, test.id, test.name); ErrorCodeOf(err) != CodeValidationFailed {
			t.Fatalf("KeyForCapability(%q,%q,%q) code = %q", test.namespace, test.id, test.name, ErrorCodeOf(err))
		}
	}
}

func TestExpandPermissionsFormsBoundedTargetProduct(t *testing.T) {
	request := PermissionsRequest{
		Generation:       "gen_42",
		ActiveNamespaces: []string{"payments", "billing", "ops"},
		Namespaces:       []string{"payments", "billing"},
		CapabilityIDs:    []string{"pods.get", "deployments.scale", "pods.list", "namespaces.list"},
		ResourceNames:    []string{"api-0", "worker-0"},
	}
	expanded, truncated, err := ExpandPermissions(request)
	if err != nil {
		t.Fatal(err)
	}
	// pods.get: 2x2, deployments.scale: 2x2, pods.list: 2, namespaces.list: 1.
	if len(expanded) != 11 || truncated {
		t.Fatalf("expanded=%d truncated=%v, want 11/false", len(expanded), truncated)
	}
	if got := expanded[0]; got.CapabilityID != "pods.get" || got.Key.Namespace != "payments" || got.Key.ResourceName != "api-0" {
		t.Fatalf("first expanded item = %+v", got)
	}
	if got := expanded[len(expanded)-1]; got.CapabilityID != "namespaces.list" || got.Key.Namespace != "" {
		t.Fatalf("last expanded item = %+v", got)
	}
}

func TestExpandPermissionsDefaultsAndTruncatesNamespaces(t *testing.T) {
	namespaces := make([]string, 21)
	for index := range namespaces {
		namespaces[index] = fmt.Sprintf("namespace-%02d", index)
	}
	expanded, truncated, err := ExpandPermissions(PermissionsRequest{
		Generation:       "gen_42",
		ActiveNamespaces: namespaces,
		CapabilityIDs:    []string{"pods.list"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(expanded) != 20 || !truncated {
		t.Fatalf("expanded=%d truncated=%v, want 20/true", len(expanded), truncated)
	}
	for index, item := range expanded {
		if item.Key.Namespace != namespaces[index] {
			t.Fatalf("expanded[%d].namespace = %q, want %q", index, item.Key.Namespace, namespaces[index])
		}
	}
}

func TestExpandPermissionsRejectsInvalidGrammarAndProducts(t *testing.T) {
	base := PermissionsRequest{
		Generation:       "gen_42",
		ActiveNamespaces: []string{"payments", "billing"},
		CapabilityIDs:    []string{"pods.list"},
	}
	tests := map[string]PermissionsRequest{
		"empty generation":        withPermissions(base, func(request *PermissionsRequest) { request.Generation = "" }),
		"namespace outside scope": withPermissions(base, func(request *PermissionsRequest) { request.Namespaces = []string{"ops"} }),
		"duplicate namespace":     withPermissions(base, func(request *PermissionsRequest) { request.Namespaces = []string{"payments", "payments"} }),
		"invalid namespace":       withPermissions(base, func(request *PermissionsRequest) { request.ActiveNamespaces = []string{"Bad_Name"} }),
		"invalid capability":      withPermissions(base, func(request *PermissionsRequest) { request.CapabilityIDs = []string{"pods.raw.sar"} }),
		"duplicate capability":    withPermissions(base, func(request *PermissionsRequest) { request.CapabilityIDs = []string{"pods.list", "pods.list"} }),
		"unused resource name":    withPermissions(base, func(request *PermissionsRequest) { request.ResourceNames = []string{"api"} }),
		"invalid resource name": withPermissions(base, func(request *PermissionsRequest) {
			request.CapabilityIDs = []string{"pods.get"}
			request.ResourceNames = []string{"Bad_Name"}
		}),
		"duplicate resource name": withPermissions(base, func(request *PermissionsRequest) {
			request.CapabilityIDs = []string{"pods.get"}
			request.ResourceNames = []string{"api", "api"}
		}),
	}
	tooManyCapabilities := make([]string, maxQueryCapabilities+1)
	for index := range tooManyCapabilities {
		tooManyCapabilities[index] = "pods.list"
	}
	tests["too many capability query values"] = withPermissions(base, func(request *PermissionsRequest) { request.CapabilityIDs = tooManyCapabilities })
	tooManyNamespaces := make([]string, maxQueryNamespaces+1)
	for index := range tooManyNamespaces {
		tooManyNamespaces[index] = fmt.Sprintf("ns-%02d", index)
	}
	tests["too many namespace query values"] = withPermissions(base, func(request *PermissionsRequest) { request.Namespaces = tooManyNamespaces })
	tooManyNames := make([]string, maxQueryResourceNames+1)
	for index := range tooManyNames {
		tooManyNames[index] = fmt.Sprintf("pod-%02d", index)
	}
	tests["too many resourceName query values"] = withPermissions(base, func(request *PermissionsRequest) {
		request.CapabilityIDs = []string{"pods.get"}
		request.ResourceNames = tooManyNames
	})

	for name, request := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ExpandPermissions(request); ErrorCodeOf(err) != CodeValidationFailed {
				t.Fatalf("code = %q, want validation failure", ErrorCodeOf(err))
			}
		})
	}
}

func TestExpandPermissionsEnforcesMaximumOneHundredDecisions(t *testing.T) {
	fiveNamespaces := []string{"one", "two", "three", "four", "five"}
	twentyNames := make([]string, 20)
	for index := range twentyNames {
		twentyNames[index] = fmt.Sprintf("pod-%02d", index)
	}
	request := PermissionsRequest{
		Generation:       "gen_42",
		ActiveNamespaces: fiveNamespaces,
		CapabilityIDs:    []string{"pods.get"},
		ResourceNames:    twentyNames,
	}
	expanded, _, err := ExpandPermissions(request)
	if err != nil || len(expanded) != maxExpandedDecisions {
		t.Fatalf("exact max expansion len=%d err=%v", len(expanded), err)
	}
	request.ActiveNamespaces = append(request.ActiveNamespaces, "six")
	if _, _, err := ExpandPermissions(request); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("over-max expansion code = %q", ErrorCodeOf(err))
	}
}

func withPermissions(request PermissionsRequest, mutate func(*PermissionsRequest)) PermissionsRequest {
	request.ActiveNamespaces = append([]string(nil), request.ActiveNamespaces...)
	request.Namespaces = append([]string(nil), request.Namespaces...)
	request.CapabilityIDs = append([]string(nil), request.CapabilityIDs...)
	request.ResourceNames = append([]string(nil), request.ResourceNames...)
	mutate(&request)
	return request
}

func TestMatrixReturnsPartialUnknownWithoutInventingDenial(t *testing.T) {
	reviewer := &fakeAccessReviewer{fn: func(_ context.Context, key Key) (AccessReviewResult, error) {
		if key.Namespace == "payments" {
			return AccessReviewResult{Allowed: true, Complete: true}, nil
		}
		return AccessReviewResult{}, errors.New("review timeout with token=secret")
	}}
	service := newTestService(t, reviewer, Options{})
	matrix, err := service.Matrix(context.Background(), PermissionsRequest{
		Generation:       "gen_42",
		ActiveNamespaces: []string{"payments", "billing"},
		CapabilityIDs:    []string{"pods.list"},
	})
	if err != nil {
		t.Fatalf("partial matrix error = %v", err)
	}
	if matrix.Complete || matrix.Truncated || len(matrix.Decisions) != 2 || len(matrix.Errors) != 1 {
		t.Fatalf("matrix = %+v", matrix)
	}
	if matrix.Decisions[0].Decision != DecisionAllowed || matrix.Decisions[1].Decision != DecisionUnknown {
		t.Fatalf("decisions = %+v", matrix.Decisions)
	}
	if matrix.Decisions[1].Decision == DecisionDenied || matrix.Errors[0].Code != CodeAuthorizationUnavailable {
		t.Fatalf("unknown review was converted to denial: %+v", matrix)
	}
	if contains(matrix.Errors[0].Message, "secret") {
		t.Fatalf("partial error leaked upstream text: %+v", matrix.Errors[0])
	}
}

func TestMatrixReturnsUnavailableWhenEveryDecisionIsUnknown(t *testing.T) {
	reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
		return AccessReviewResult{}, context.DeadlineExceeded
	}}
	service := newTestService(t, reviewer, Options{})
	matrix, err := service.Matrix(context.Background(), PermissionsRequest{
		Generation:       "gen_42",
		ActiveNamespaces: []string{"payments"},
		CapabilityIDs:    []string{"pods.list", "pods.watch"},
	})
	if ErrorCodeOf(err) != CodeAuthorizationUnavailable || len(matrix.Decisions) != 2 || len(matrix.Errors) != 2 {
		t.Fatalf("matrix=%+v code=%q", matrix, ErrorCodeOf(err))
	}
	for _, capability := range matrix.Decisions {
		if capability.Decision != DecisionUnknown {
			t.Fatalf("decision = %q, want unknown", capability.Decision)
		}
	}
}

func TestMatrixRefreshIgnoresOnlyRequestedCachedKeys(t *testing.T) {
	var allowed atomicDecision
	allowed.Store(DecisionAllowed)
	reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
		if allowed.Load() == DecisionAllowed {
			return AccessReviewResult{Allowed: true, Complete: true}, nil
		}
		return AccessReviewResult{Denied: true, Complete: true}, nil
	}}
	service := newTestService(t, reviewer, Options{})
	request := PermissionsRequest{
		Generation:       "gen_42",
		ActiveNamespaces: []string{"payments"},
		CapabilityIDs:    []string{"pods.list"},
	}
	first, err := service.Matrix(context.Background(), request)
	if err != nil || first.Decisions[0].Decision != DecisionAllowed {
		t.Fatalf("first matrix=%+v err=%v", first, err)
	}
	allowed.Store(DecisionDenied)
	cached, err := service.Matrix(context.Background(), request)
	if err != nil || cached.Decisions[0].Decision != DecisionAllowed {
		t.Fatalf("cached matrix=%+v err=%v", cached, err)
	}
	request.Refresh = true
	refreshed, err := service.Matrix(context.Background(), request)
	if err != nil || refreshed.Decisions[0].Decision != DecisionDenied {
		t.Fatalf("refreshed matrix=%+v err=%v", refreshed, err)
	}
	key, _ := KeyForCapability("gen_42", "payments", "pods.list", "")
	if got := reviewer.callCount(key); got != 2 {
		t.Fatalf("review calls = %d, want 2", got)
	}
}

type atomicDecision struct {
	value atomic.Value
}

func (decision *atomicDecision) Store(value Decision) { decision.value.Store(string(value)) }
func (decision *atomicDecision) Load() Decision       { return Decision(decision.value.Load().(string)) }
