package kubernetesruntime

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kubeadapter "github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func TestDashboardAdapterCanReadPodLogsDecisions(t *testing.T) {
	t.Parallel()
	provider, _, _ := dashboardTestClients(t)
	adapter := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{}, binding: dashboardTestBinding()}
	decision, err := adapter.CanReadPodLogs(context.Background(), "payments", "api")
	if err != nil || decision != dashboard.PermissionAllowed {
		t.Fatalf("allowed decision = %q err = %v", decision, err)
	}

	denied := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{denyResource: "pods"}, binding: dashboardTestBinding()}
	decision, err = denied.CanReadPodLogs(context.Background(), "payments", "api")
	if err != nil || decision != dashboard.PermissionDenied {
		t.Fatalf("denied decision = %q err = %v", decision, err)
	}
}

func TestDashboardAdapterReadLogsStreamsAndCloses(t *testing.T) {
	t.Parallel()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api"}}
	provider, _, _ := dashboardTestClients(t, pod)
	adapter := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{}, binding: dashboardTestBinding()}
	request := dashboard.LogReadRequest{Namespace: "payments", Pod: "api", Container: "api", TailLines: 100, SinceTime: time.Now().Add(-time.Hour)}
	reader, err := adapter.ReadLogs(context.Background(), request)
	if err != nil || reader == nil {
		t.Fatalf("read logs = %v, err = %v", reader, err)
	}
	payload, readErr := io.ReadAll(reader)
	if readErr != nil || string(payload) != "fake logs" {
		t.Fatalf("log payload = %q err = %v", payload, readErr)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("second close = %v", err)
	}

	denied := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{denyResource: "pods"}, binding: dashboardTestBinding()}
	_, deniedErr := denied.ReadLogs(context.Background(), request)
	var safeDenied interface {
		Code() string
		Denied() bool
	}
	if !errors.As(deniedErr, &safeDenied) || safeDenied.Code() != dashboard.CodeForbidden || !safeDenied.Denied() {
		t.Fatalf("denied read logs err = %v", deniedErr)
	}

	streamingless := &fixedDashboardClients{set: dashboardClientSet{kubernetes: provider.set.kubernetes}}
	streamingAdapter := &dashboardAdapter{clients: streamingless, authorization: &dashboardAuthorizationStub{}, binding: dashboardTestBinding()}
	_, unavailableErr := streamingAdapter.ReadLogs(context.Background(), request)
	var safeUnavailable interface {
		Code() string
	}
	if !errors.As(unavailableErr, &safeUnavailable) || safeUnavailable.Code() != dashboard.CodeFeatureUnavailable {
		t.Fatalf("streamingless read logs err = %v", unavailableErr)
	}
}

func TestResolvePodOwnerFollowsAndPreservesOwners(t *testing.T) {
	t.Parallel()
	controller := true
	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments", Name: "api-rs", UID: types.UID("rs-uid"),
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: types.UID("deployment-uid"), Controller: &controller}},
		},
	}
	ownedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments", Name: "backup-1", UID: types.UID("job-uid"),
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "CronJob", Name: "backup", UID: types.UID("cron-uid"), Controller: &controller}},
		},
	}
	provider, _, _ := dashboardTestClients(t, replicaSet, ownedJob)
	adapter := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{}, binding: dashboardTestBinding()}
	ctx := context.Background()

	ownerless := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "bare"}}
	if owner, err := adapter.ResolvePodOwner(ctx, ownerless); err != nil || owner != nil {
		t.Fatalf("ownerless pod = %+v err = %v", owner, err)
	}
	deploymentOwned := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "payments", Name: "api",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: types.UID("deployment-uid"), Controller: &controller}},
	}}
	if owner, err := adapter.ResolvePodOwner(ctx, deploymentOwned); err != nil || owner == nil || owner.Kind != "Deployment" || owner.Name != "api" {
		t.Fatalf("deployment owner = %+v err = %v", owner, err)
	}

	replicaSetPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "payments", Name: "api-1",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-rs", UID: types.UID("rs-uid"), Controller: &controller}},
	}}
	owner, err := adapter.ResolvePodOwner(ctx, replicaSetPod)
	if err != nil || owner == nil || owner.Kind != "Deployment" || owner.Name != "api" {
		t.Fatalf("resolved owner = %+v err = %v", owner, err)
	}

	cronJobPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "payments", Name: "backup-1-abc",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: "backup-1", UID: types.UID("job-uid"), Controller: &controller}},
	}}
	owner, err = adapter.ResolvePodOwner(ctx, cronJobPod)
	if err != nil || owner == nil || owner.Kind != "CronJob" || owner.Name != "backup" {
		t.Fatalf("cron job owner = %+v err = %v", owner, err)
	}

	orphanReplicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api-rs", UID: types.UID("rs-uid")}}
	provider2, _, _ := dashboardTestClients(t, orphanReplicaSet)
	orphanAdapter := &dashboardAdapter{clients: provider2, authorization: &dashboardAuthorizationStub{}, binding: dashboardTestBinding()}
	owner, err = orphanAdapter.ResolvePodOwner(ctx, replicaSetPod)
	if err != nil || owner == nil || owner.Kind != "ReplicaSet" {
		t.Fatalf("orphan replicaSet owner = %+v err = %v", owner, err)
	}

	denied := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{denyResource: "replicasets"}, binding: dashboardTestBinding()}
	owner, err = denied.ResolvePodOwner(ctx, replicaSetPod)
	if err == nil || owner == nil || owner.Kind != "ReplicaSet" || owner.Name != "api-rs" {
		t.Fatalf("denied resolution = %+v err = %v", owner, err)
	}
	deniedJob := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{denyResource: "jobs"}, binding: dashboardTestBinding()}
	owner, err = deniedJob.ResolvePodOwner(ctx, cronJobPod)
	if err == nil || owner == nil || owner.Kind != "Job" {
		t.Fatalf("denied job resolution = %+v err = %v", owner, err)
	}
}

func TestToDashboardErrorTranslatesSafeClientErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    error
		expected error
	}{
		{name: "request timeout", input: &kubeadapter.SafeError{Code: kubeadapter.CodeRequestTimeout, Message: "safe"}, expected: context.DeadlineExceeded},
		{name: "request canceled", input: &kubeadapter.SafeError{Code: kubeadapter.CodeRequestCanceled, Message: "safe"}, expected: context.Canceled},
		{name: "generation changed", input: &kubeadapter.SafeError{Code: kubeadapter.CodeGenerationChanged, Message: "safe"}, expected: context.Canceled},
	}
	for _, test := range cases {
		converted := toDashboardError(test.input)
		if !errors.Is(converted, test.expected) {
			t.Fatalf("%s: converted %v, want %v", test.name, converted, test.expected)
		}
	}
	passthrough := &kubeadapter.SafeError{Code: "OTHER", Message: "safe"}
	if converted := toDashboardError(passthrough); !errors.Is(converted, passthrough) {
		t.Fatalf("unknown code was transformed: %v", converted)
	}
	timeout := toDashboardError(&authorization.PublicError{Code: authorization.CodeUpstreamTimeout, Message: "safe"})
	if !errors.Is(timeout, context.DeadlineExceeded) {
		t.Fatalf("authorization timeout = %v", timeout)
	}
	canceled := toDashboardError(&authorization.PublicError{Code: authorization.CodeClientCanceled, Message: "safe"})
	if !errors.Is(canceled, context.Canceled) {
		t.Fatalf("authorization canceled = %v", canceled)
	}
	unavailable := toDashboardError(&authorization.PublicError{Code: authorization.CodeAuthorizationUnavailable, Message: "safe"})
	var safeUnavailable interface {
		Code() string
	}
	if !errors.As(unavailable, &safeUnavailable) || safeUnavailable.Code() != dashboard.CodeAuthorizationUnavailable {
		t.Fatalf("authorization unavailable = %v", unavailable)
	}
	if toDashboardError(nil) != nil {
		t.Fatal("nil error was transformed")
	}
}

func TestNormalizedPageLimitAndWorkloadKindNameTables(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input int
		want  int
	}{
		{input: 0, want: dashboard.DefaultPageSize},
		{input: -5, want: dashboard.DefaultPageSize},
		{input: 501, want: dashboard.DefaultPageSize},
		{input: 1, want: 1},
		{input: 500, want: 500},
	} {
		if got := normalizedPageLimit(test.input); got != test.want {
			t.Fatalf("normalizedPageLimit(%d) = %d, want %d", test.input, got, test.want)
		}
	}
	for _, test := range []struct {
		kind int
		want string
	}{
		{kind: workloadDeployments, want: "Deployment"},
		{kind: workloadStatefulSets, want: "StatefulSet"},
		{kind: workloadDaemonSets, want: "DaemonSet"},
		{kind: workloadJobs, want: "Job"},
		{kind: workloadCronJobs, want: "CronJob"},
		{kind: 99, want: "Unknown"},
		{kind: -1, want: "Unknown"},
	} {
		if got := workloadKindName(test.kind); got != test.want {
			t.Fatalf("workloadKindName(%d) = %q, want %q", test.kind, got, test.want)
		}
	}
}

func TestRuntimeDashboardClientProviderRejectsMissingLease(t *testing.T) {
	t.Parallel()
	provider := runtimeDashboardClientProvider{runtime: &Runtime{}}
	if _, _, _, err := provider.Unary(context.Background(), dashboardTestBinding()); err == nil {
		t.Fatal("provider accepted a runtime without an active selection")
	}
}

func TestOwnerResourceRefHandlesGrouplessAPIVersions(t *testing.T) {
	t.Parallel()
	if owner := ownerResourceRef(nil, "payments"); owner != nil {
		t.Fatalf("nil owner produced %v", owner)
	}
	core := ownerResourceRef(&metav1.OwnerReference{APIVersion: "v1", Kind: "Pod", Name: "bare", UID: "uid-1"}, "payments")
	if core.APIGroup != "" || core.Kind != "Pod" || core.Namespace != "payments" || core.Name != "bare" || core.UID != "uid-1" {
		t.Fatalf("groupless ref = %+v", core)
	}
	grouped := ownerResourceRef(&metav1.OwnerReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "uid-2"}, "payments")
	if grouped.APIGroup != "apps" {
		t.Fatalf("grouped ref = %+v", grouped)
	}
}

func TestControllingOwnerPicksFirstController(t *testing.T) {
	t.Parallel()
	other := false
	controller := true
	if controllingOwner(nil) != nil {
		t.Fatal("nil owner references produced a controller")
	}
	noController := []metav1.OwnerReference{{Name: "a", Controller: &other}}
	if controllingOwner(noController) != nil {
		t.Fatal("non-controlling reference was selected")
	}
	references := []metav1.OwnerReference{
		{Name: "first", Controller: &other},
		{Name: "controller", Controller: &controller},
	}
	selected := controllingOwner(references)
	if selected == nil || selected.Name != "controller" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestDecodeWorkloadContinueRejectsConcatenatedJSON(t *testing.T) {
	t.Parallel()
	token, err := encodeWorkloadContinue(workloadContinue{Kind: workloadStatefulSets, Token: "upstream-token"})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeWorkloadContinue(token)
	if err != nil || decoded.Kind != workloadStatefulSets || decoded.Token != "upstream-token" {
		t.Fatalf("decoded = %+v err = %v", decoded, err)
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte("{\"kind\":1}{\"kind\":2}"))
	if _, err := decodeWorkloadContinue(encoded); err == nil {
		t.Fatalf("concatenated JSON token accepted: %v", err)
	}
}

func TestDashboardAdapterUnaryRejectsIncompleteBinding(t *testing.T) {
	t.Parallel()
	provider, _, _ := dashboardTestClients(t)
	incomplete := &dashboardAdapter{clients: provider, authorization: &dashboardAuthorizationStub{}, binding: namespaces.SelectionBinding{ClusterProfileID: 11, Context: "dev"}}
	if _, _, _, err := incomplete.unary(context.Background()); !isDashboardFeatureUnavailable(err) {
		t.Fatalf("incomplete binding err = %v", err)
	}
	var nilAdapter *dashboardAdapter
	if _, _, _, err := nilAdapter.unary(context.Background()); !isDashboardFeatureUnavailable(err) {
		t.Fatalf("nil adapter err = %v", err)
	}
}

func isDashboardFeatureUnavailable(err error) bool {
	var safe interface {
		Code() string
	}
	return errors.As(err, &safe) && safe.Code() == dashboard.CodeFeatureUnavailable
}
