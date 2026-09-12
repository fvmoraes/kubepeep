package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

func TestMatrixCoversRequiredDimensions(t *testing.T) {
	scenarios, err := scenariosFor("matrix")
	if err != nil {
		t.Fatal(err)
	}
	namespaces := map[int]bool{}
	pods := map[int]bool{}
	pages := map[int]bool{}
	latencies := map[int]bool{}
	faults := map[fault]bool{}
	profiles := map[profile]bool{}
	watch := map[bool]bool{}
	for _, candidate := range scenarios {
		namespaces[candidate.Namespaces] = true
		pods[candidate.PodsPerNamespace] = true
		pages[candidate.PageSize] = true
		latencies[candidate.LatencyMS] = true
		faults[candidate.Fault] = true
		profiles[candidate.Profile] = true
		watch[candidate.Watch] = true
	}
	for _, value := range []int{1, 10, 25, 50, 100, 200} {
		if !namespaces[value] {
			t.Errorf("namespace dimension %d is missing", value)
		}
	}
	for _, value := range []int{10, 50, 100, 250, 500} {
		if !pods[value] {
			t.Errorf("pod dimension %d is missing", value)
		}
	}
	for _, value := range []int{25, 50, 100} {
		if !pages[value] {
			t.Errorf("page dimension %d is missing", value)
		}
	}
	for _, value := range []int{0, 20, 50, 100, 200} {
		if !latencies[value] {
			t.Errorf("latency dimension %d is missing", value)
		}
	}
	for _, value := range []fault{faultNone, fault429, fault410, faultTimeout, faultReset} {
		if !faults[value] {
			t.Errorf("fault dimension %s is missing", value)
		}
	}
	for _, value := range []profile{profileGlobal, profileNamespace, profileMixed, profileUnavailable, profileInternal} {
		if !profiles[value] {
			t.Errorf("profile %s is missing", value)
		}
	}
	if !watch[false] || !watch[true] {
		t.Fatal("watch on/off dimensions are both required")
	}
}

func TestC02SeparatesGlobalRestrictedAndInternalTwoHundredNamespaceCases(t *testing.T) {
	candidates := []scenario{
		newScenario("global", profileGlobal, 200, 10, 100, 0, faultNone, false),
		newScenario("restricted", profileNamespace, 200, 10, 100, 0, faultNone, false),
		newScenario("internal", profileInternal, 200, 10, 100, 0, faultNone, false),
	}
	result := runReport("test", "deadbeef", "clean", 0, 1, candidates)
	if result.Results[0].Status != "ok" || result.Results[0].Scenario.ContractScope != "public-global-authorized" {
		t.Fatalf("global case = %#v", result.Results[0])
	}
	if result.Results[1].Status != "expected_error" || result.Results[1].ErrorCode != string(resources.CodeValidationFailed) || result.Results[1].Scenario.ContractScope != "public-limit-rejection" {
		t.Fatalf("restricted case = %#v", result.Results[1])
	}
	if result.Results[2].Status != "ok" || result.Results[2].Scenario.ContractScope != "internal-synthetic-only" {
		t.Fatalf("internal case = %#v", result.Results[2])
	}
	if result.Results[0].Metrics.ItemsReturned.P50 != 100 || result.Results[2].Metrics.ItemsReturned.P50 != 100 {
		t.Fatalf("page size was not preserved: global=%v internal=%v", result.Results[0].Metrics.ItemsReturned.P50, result.Results[2].Metrics.ItemsReturned.P50)
	}
}

func TestFaultsAndAuthorizationUnavailableHaveStableExpectedCodes(t *testing.T) {
	candidates := []scenario{
		newScenario("unavailable", profileUnavailable, 10, 10, 25, 0, faultNone, false),
		newScenario("expired", profileGlobal, 10, 10, 25, 0, fault410, false),
		newScenario("timeout", profileGlobal, 10, 10, 25, 0, faultTimeout, false),
		newScenario("reset", profileGlobal, 10, 10, 25, 0, faultReset, false),
		newScenario("throttle", profileGlobal, 10, 10, 25, 0, fault429, false),
	}
	result := runReport("test", "deadbeef", "clean", 0, 1, candidates)
	want := []resources.ErrorCode{
		resources.CodeAuthorizationUnavailable,
		resources.CodeCursorExpired,
		resources.CodeUpstreamTimeout,
		resources.CodeClusterUnavailable,
		resources.CodeClusterUnavailable,
	}
	for index, expected := range want {
		if result.Results[index].Status != "expected_error" || result.Results[index].ErrorCode != string(expected) {
			t.Errorf("result %d = status %q code %q, want expected_error/%q", index, result.Results[index].Status, result.Results[index].ErrorCode, expected)
		}
	}
}

func TestReportIsSanitizedAndDeclaresMeasurementProtocol(t *testing.T) {
	candidate := newScenario("sanitized", profileNamespace, 10, 10, 25, 0, faultNone, true)
	result := runReport("test", "deadbeef", "clean", 0, 1, []scenario{candidate})
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"pod-000000", "bench-ns-0000", "bearer ", "csrf", "uid"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Errorf("report contains forbidden identity/payload fragment %q", forbidden)
		}
	}
	if result.Metadata.Mode != "synthetic" || len(result.Protocol) < 4 {
		t.Fatalf("measurement mode/protocol missing: %#v", result.Metadata)
	}
	if result.Results[0].Metrics.WatchEvents.P50 < 1 || result.Results[0].Metrics.CursorBytes.P50 <= 0 {
		t.Fatalf("watch/cursor metrics missing: %#v", result.Results[0].Metrics)
	}
}

func TestSummarizeRequiresUnanimousOutcomes(t *testing.T) {
	candidate := newScenario("mixed-outcomes", profileGlobal, 10, 10, 25, 0, faultNone, false)
	result := summarize(candidate, []sample{
		{errorCode: ""},
		{errorCode: resources.CodeClusterUnavailable},
		{errorCode: ""},
	})
	if result.Status != "unstable" {
		t.Fatalf("status = %q, want unstable", result.Status)
	}
	if result.ErrorCode != "" {
		t.Fatalf("mixed samples must not publish a dominant error code: %q", result.ErrorCode)
	}
	if result.Outcomes["success"] != 2 || result.Outcomes[string(resources.CodeClusterUnavailable)] != 1 {
		t.Fatalf("outcomes = %#v", result.Outcomes)
	}
	if err := reportError(report{Results: []scenarioReport{result}}); err == nil {
		t.Fatal("an unstable scenario must make the benchmark command fail")
	}
}

func TestReportErrorRejectsUnanimousUnexpectedOutcome(t *testing.T) {
	candidate := newScenario("unexpected-outcome", profileGlobal, 10, 10, 25, 0, faultNone, false)
	result := summarize(candidate, []sample{
		{errorCode: resources.CodeClusterUnavailable},
		{errorCode: resources.CodeClusterUnavailable},
	})
	if result.Status != "unexpected_error" || result.ErrorCode != string(resources.CodeClusterUnavailable) {
		t.Fatalf("result = status %q code %q", result.Status, result.ErrorCode)
	}
	if result.Outcomes[string(resources.CodeClusterUnavailable)] != 2 {
		t.Fatalf("outcomes = %#v", result.Outcomes)
	}
	if err := reportError(report{Results: []scenarioReport{result}}); err == nil {
		t.Fatal("an unexpected outcome must make the benchmark command fail")
	}
}
