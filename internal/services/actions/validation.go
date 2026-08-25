package actions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
)

var (
	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{16,128}$`)
	sessionIDPattern      = regexp.MustCompile(`^(pf|exec)_[A-Za-z0-9_-]{8,128}$`)
)

func validateBinding(binding namespaces.SelectionBinding) *Error {
	violations := make([]FieldViolation, 0, 3)
	if binding.ClusterProfileID <= 0 {
		violations = append(violations, FieldViolation{Field: "target.clusterProfileId", Rule: "positive"})
	}
	if binding.Context == "" || len(binding.Context) > 253 || hasControl(binding.Context) {
		violations = append(violations, FieldViolation{Field: "target.context", Rule: "active_context"})
	}
	if binding.Generation == "" || len(binding.Generation) > 256 || hasControl(binding.Generation) {
		violations = append(violations, FieldViolation{Field: "expectedGeneration", Rule: "active_generation"})
	}
	if len(violations) > 0 {
		return validationError(violations...)
	}
	return nil
}

func validateConfirmation(binding namespaces.SelectionBinding, route RouteTarget, confirmation Confirmation, expectedAction Action, expectedConsequence ConsequenceCode, routeKind, targetKind string) *Error {
	if err := validateBinding(binding); err != nil {
		return err
	}
	violations := make([]FieldViolation, 0, 12)
	if route.Kind != routeKind {
		violations = append(violations, FieldViolation{Field: "path.kind", Rule: routeKind})
	}
	if errs := kvalidation.IsDNS1123Label(route.Namespace); len(errs) > 0 {
		violations = append(violations, FieldViolation{Field: "path.namespace", Rule: "kubernetes_dns_label"})
	}
	if errs := kvalidation.IsDNS1123Subdomain(route.Name); len(errs) > 0 {
		violations = append(violations, FieldViolation{Field: "path.name", Rule: "kubernetes_dns_subdomain"})
	}
	if !confirmation.Confirmed {
		violations = append(violations, FieldViolation{Field: "confirmed", Rule: "must_be_true"})
	}
	if confirmation.Action != expectedAction {
		violations = append(violations, FieldViolation{Field: "action", Rule: string(expectedAction)})
	}
	if confirmation.ConsequenceCode != expectedConsequence {
		violations = append(violations, FieldViolation{Field: "consequenceCode", Rule: string(expectedConsequence)})
	}
	if confirmation.ExpectedGeneration != binding.Generation {
		violations = append(violations, FieldViolation{Field: "expectedGeneration", Rule: "matches_active_generation"})
	}
	target := confirmation.Target
	if target.ClusterProfileID != binding.ClusterProfileID {
		violations = append(violations, FieldViolation{Field: "target.clusterProfileId", Rule: "matches_active_selection"})
	}
	if target.Context != binding.Context {
		violations = append(violations, FieldViolation{Field: "target.context", Rule: "matches_active_selection"})
	}
	if target.Namespace != route.Namespace {
		violations = append(violations, FieldViolation{Field: "target.namespace", Rule: "matches_path"})
	}
	if target.Kind != targetKind {
		violations = append(violations, FieldViolation{Field: "target.kind", Rule: targetKind})
	}
	if target.Name != route.Name {
		violations = append(violations, FieldViolation{Field: "target.name", Rule: "matches_path"})
	}
	if len(violations) > 0 {
		return validationError(violations...)
	}
	return nil
}

func validateRestart(binding namespaces.SelectionBinding, route RouteTarget, request RestartRequest) *Error {
	if err := validateConfirmation(binding, route, request.Confirmation, ActionRestart, ConsequenceRecreateWorkloadPods, "deployments", "Deployment"); err != nil {
		return err
	}
	return validateOpaquePrecondition("expectedResourceVersion", request.ExpectedResourceVersion)
}

func validateScale(binding namespaces.SelectionBinding, route RouteTarget, request ScaleRequest) *Error {
	if route.Kind != "deployments" && route.Kind != "statefulsets" {
		return validationError(FieldViolation{Field: "path.kind", Rule: "deployments_or_statefulsets"})
	}
	targetKind := "Deployment"
	if route.Kind == "statefulsets" {
		targetKind = "StatefulSet"
	}
	if err := validateConfirmation(binding, route, request.Confirmation, ActionScale, ConsequenceChangeReplicaCount, route.Kind, targetKind); err != nil {
		return err
	}
	violations := make([]FieldViolation, 0, 2)
	if request.Replicas < 0 {
		violations = append(violations, FieldViolation{Field: "replicas", Rule: "minimum_0"})
	}
	if request.Replicas > math.MaxInt32 {
		violations = append(violations, FieldViolation{Field: "replicas", Rule: "maximum_2147483647"})
	}
	if err := validateOpaquePrecondition("expectedResourceVersion", request.ExpectedResourceVersion); err != nil {
		violations = append(violations, err.Details...)
	}
	if len(violations) > 0 {
		return validationError(violations...)
	}
	return nil
}

func validateDeletePod(binding namespaces.SelectionBinding, route RouteTarget, request PodDeleteRequest) *Error {
	if err := validateConfirmation(binding, route, request.Confirmation, ActionDeletePod, ConsequenceDeletePod, "pods", "Pod"); err != nil {
		return err
	}
	violations := make([]FieldViolation, 0, 2)
	if err := validateOpaquePrecondition("expectedUid", request.ExpectedUID); err != nil {
		violations = append(violations, err.Details...)
	}
	if err := validateOpaquePrecondition("expectedResourceVersion", request.ExpectedResourceVersion); err != nil {
		violations = append(violations, err.Details...)
	}
	if len(violations) > 0 {
		return validationError(violations...)
	}
	return nil
}

func validatePortForward(binding namespaces.SelectionBinding, route RouteTarget, request PortForwardCreateRequest) *Error {
	if err := validateConfirmation(binding, route, request.Confirmation, ActionPortForward, ConsequenceExposePodPortLocally, "pods", "Pod"); err != nil {
		return err
	}
	violations := make([]FieldViolation, 0, 2)
	if request.RemotePort < 1 || request.RemotePort > 65535 {
		violations = append(violations, FieldViolation{Field: "remotePort", Rule: "range_1_65535"})
	}
	if request.LocalPort != nil && (*request.LocalPort < 1024 || *request.LocalPort > 65535) {
		violations = append(violations, FieldViolation{Field: "localPort", Rule: "null_or_range_1024_65535"})
	}
	if len(violations) > 0 {
		return validationError(violations...)
	}
	return nil
}

func validateExec(binding namespaces.SelectionBinding, route RouteTarget, request ExecInit) *Error {
	if err := validateConfirmation(binding, route, request.Confirmation, ActionExec, ConsequenceOpenInteractiveProcess, "pods", "Pod"); err != nil {
		return err
	}
	violations := make([]FieldViolation, 0, 4)
	if errs := kvalidation.IsDNS1123Label(request.Container); len(errs) > 0 {
		violations = append(violations, FieldViolation{Field: "container", Rule: "kubernetes_dns_label"})
	}
	if len(request.Command) < 1 || len(request.Command) > MaximumExecArguments {
		violations = append(violations, FieldViolation{Field: "command", Rule: "argv_1_64"})
	}
	total := 0
	for index, argument := range request.Command {
		total += len(argument)
		if !utf8.ValidString(argument) || strings.IndexByte(argument, 0) >= 0 || len(argument) > MaximumExecArgumentBytes || (index == 0 && argument == "") {
			violations = append(violations, FieldViolation{Field: "command", Rule: "utf8_no_nul_item_max_4096"})
			break
		}
	}
	if total > MaximumExecCommandBytes {
		violations = append(violations, FieldViolation{Field: "command", Rule: "total_max_32768"})
	}
	if len(violations) > 0 {
		return validationError(violations...)
	}
	return nil
}

func validateOpaquePrecondition(field, value string) *Error {
	if value == "" || len(value) > 256 || hasControl(value) || !utf8.ValidString(value) {
		return validationError(FieldViolation{Field: field, Rule: "non_empty_opaque_max_256"})
	}
	return nil
}

func validateIdempotencyKey(value string) *Error {
	if !idempotencyKeyPattern.MatchString(value) {
		return validationError(FieldViolation{Field: "Idempotency-Key", Rule: "16_128_unreserved_characters"})
	}
	return nil
}

func validateSessionID(value, prefix string) *Error {
	if !sessionIDPattern.MatchString(value) || !strings.HasPrefix(value, prefix) {
		return validationError(FieldViolation{Field: "sessionId", Rule: "valid_identifier"})
	}
	return nil
}

func hasControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func canonicalBodyHash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", publicError(CodeInternal, http.StatusInternalServerError, false, err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func mutationTarget(binding namespaces.SelectionBinding, target ActionTargetDTO) MutationTarget {
	return MutationTarget{
		ClusterProfileID: binding.ClusterProfileID,
		Context:          binding.Context,
		Generation:       binding.Generation,
		Namespace:        target.Namespace,
		Kind:             target.Kind,
		Name:             target.Name,
	}
}

func canonicalWorkloadPath(route RouteTarget, suffix string) string {
	return "/api/v1/workloads/" + route.Kind + "/" + route.Namespace + "/" + route.Name + "/" + suffix
}

func canonicalPodPath(route RouteTarget, suffix string) string {
	path := "/api/v1/pods/" + route.Namespace + "/" + route.Name
	if suffix != "" {
		path += "/" + suffix
	}
	return path
}
