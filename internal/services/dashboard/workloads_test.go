package dashboard

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestDeploymentClassificationClosedOrderAndDefaults(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	tests := []struct {
		name   string
		mutate func(*appsv1.Deployment)
		want   WorkloadStatus
	}{
		{"healthy default replica", func(value *appsv1.Deployment) {
			value.Status.ReadyReplicas = 1
			value.Status.AvailableReplicas = 1
			value.Status.UpdatedReplicas = 1
		}, WorkloadHealthy},
		{"degraded available", func(value *appsv1.Deployment) {
			replicas := int32(3)
			value.Spec.Replicas = &replicas
			value.Status.ReadyReplicas = 2
			value.Status.AvailableReplicas = 2
			value.Status.UpdatedReplicas = 3
		}, WorkloadDegraded},
		{"degraded deadline", func(value *appsv1.Deployment) {
			value.Status.ReadyReplicas = 1
			value.Status.AvailableReplicas = 1
			value.Status.UpdatedReplicas = 1
			value.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"}}
		}, WorkloadDegraded},
		{"progressing", func(value *appsv1.Deployment) {
			replicas := int32(3)
			value.Spec.Replicas = &replicas
			value.Status.ReadyReplicas = 2
			value.Status.AvailableReplicas = 3
			value.Status.UpdatedReplicas = 2
		}, WorkloadProgressing},
		{"stale", func(value *appsv1.Deployment) {
			value.Status.ObservedGeneration = value.Generation - 1
			value.Status.ReadyReplicas = 1
			value.Status.AvailableReplicas = 1
			value.Status.UpdatedReplicas = 1
		}, WorkloadUnknown},
		{"missing generation", func(value *appsv1.Deployment) { value.Generation = 0; value.Status.ObservedGeneration = 0 }, WorkloadUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := appsv1.Deployment{ObjectMeta: workloadMeta("deployment", now), Status: appsv1.DeploymentStatus{ObservedGeneration: 2}}
			test.mutate(&value)
			got := ClassifyDeployment(&value, now)
			if got.Status != test.want {
				t.Fatalf("got %q, want %q: %+v", got.Status, test.want, got)
			}
			if test.name == "healthy default replica" && (got.Desired == nil || *got.Desired != 1) {
				t.Fatalf("default replicas not materialized: %+v", got)
			}
		})
	}
}

func TestStatefulSetClassificationPresencePriorityAndDefaults(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	value := appsv1.StatefulSet{ObjectMeta: workloadMeta("stateful", now), Status: appsv1.StatefulSetStatus{ObservedGeneration: 2, ReadyReplicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1}}
	if got := ClassifyStatefulSet(&value, now); got.Status != WorkloadHealthy || got.Desired == nil || *got.Desired != 1 {
		t.Fatalf("unexpected healthy StatefulSet: %+v", got)
	}
	if got := ClassifyStatefulSetWithAvailability(&value, false, now); got.Status != WorkloadUnknown || got.Available != nil {
		t.Fatalf("missing available field became healthy: %+v", got)
	}
	replicas := int32(3)
	value.Spec.Replicas = &replicas
	value.Status.ReadyReplicas = 2
	value.Status.UpdatedReplicas = 1
	if got := ClassifyStatefulSet(&value, now); got.Status != WorkloadDegraded {
		t.Fatalf("ready priority not applied: %+v", got)
	}
	value.Status.ReadyReplicas = 3
	if got := ClassifyStatefulSet(&value, now); got.Status != WorkloadProgressing {
		t.Fatalf("updated classification not applied: %+v", got)
	}
}

func TestDaemonSetClassification(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	value := appsv1.DaemonSet{ObjectMeta: workloadMeta("daemon", now), Status: appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 3, NumberReady: 3, NumberAvailable: 3, UpdatedNumberScheduled: 3}}
	if got := ClassifyDaemonSet(&value, now); got.Status != WorkloadHealthy {
		t.Fatalf("unexpected healthy DaemonSet: %+v", got)
	}
	value.Status.NumberUnavailable = 1
	if got := ClassifyDaemonSet(&value, now); got.Status != WorkloadDegraded {
		t.Fatalf("unavailable DaemonSet not degraded: %+v", got)
	}
	value.Status.NumberUnavailable = 0
	value.Status.UpdatedNumberScheduled = 2
	if got := ClassifyDaemonSet(&value, now); got.Status != WorkloadProgressing {
		t.Fatalf("DaemonSet not progressing: %+v", got)
	}
	value.Status.ObservedGeneration = 1
	if got := ClassifyDaemonSet(&value, now); got.Status != WorkloadUnknown {
		t.Fatalf("stale DaemonSet not unknown: %+v", got)
	}
}

func TestJobClassificationClosedPriority(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	truth := true
	tests := []struct {
		name   string
		status batchv1.JobStatus
		spec   batchv1.JobSpec
		want   WorkloadStatus
	}{
		{"failed condition before suspend", batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}}, batchv1.JobSpec{Suspend: &truth}, WorkloadFailed},
		{"failed count", batchv1.JobStatus{Failed: 1}, batchv1.JobSpec{}, WorkloadFailed},
		{"suspended", batchv1.JobStatus{}, batchv1.JobSpec{Suspend: &truth}, WorkloadSuspended},
		{"active", batchv1.JobStatus{Active: 1}, batchv1.JobSpec{}, WorkloadProgressing},
		{"completed condition", batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}, batchv1.JobSpec{}, WorkloadCompleted},
		{"completed desired", batchv1.JobStatus{Succeeded: 2}, batchv1.JobSpec{Completions: int32TestPointer(2)}, WorkloadCompleted},
		{"unknown", batchv1.JobStatus{}, batchv1.JobSpec{}, WorkloadUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := batchv1.Job{ObjectMeta: workloadMeta("job", now), Spec: test.spec, Status: test.status}
			got := ClassifyJob(&value, now)
			if got.Status != test.want {
				t.Fatalf("got %q, want %q: %+v", got.Status, test.want, got)
			}
		})
	}
}

func TestCronJobLatestOwnedJobAndExact24HourBoundary(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	cron := batchv1.CronJob{ObjectMeta: workloadMeta("cron", now)}
	cron.UID = "cron-uid"
	cron.Status.LastScheduleTime = &metav1.Time{Time: now.Add(-time.Hour)}
	failed := failedOwnedJob("failed", cron.UID, now.Add(-24*time.Hour))
	if got := ClassifyCronJob(&cron, []batchv1.Job{failed}, now); got.Status != WorkloadFailed {
		t.Fatalf("exact 24h failed job not included: %+v", got)
	}
	failed.Status.Conditions[0].LastTransitionTime = metav1.NewTime(now.Add(-24*time.Hour - time.Nanosecond))
	if got := ClassifyCronJob(&cron, []batchv1.Job{failed}, now); got.Status != WorkloadHealthy {
		t.Fatalf("older than 24h job did not expire: %+v", got)
	}

	oldFailed := failedOwnedJob("old", cron.UID, now.Add(-time.Hour))
	newComplete := completeOwnedJob("new", cron.UID, now.Add(-time.Minute))
	if got := ClassifyCronJob(&cron, []batchv1.Job{oldFailed, newComplete}, now); got.Status != WorkloadHealthy {
		t.Fatalf("older failed job overrode latest successful job: %+v", got)
	}

	foreign := failedOwnedJob("foreign", types.UID("other"), now)
	if got := ClassifyCronJob(&cron, []batchv1.Job{foreign}, now); got.Status != WorkloadHealthy {
		t.Fatalf("foreign owner UID was correlated: %+v", got)
	}
}

func TestCronJobStatusPriority(t *testing.T) {
	t.Parallel()
	now := workloadNow()
	truth := true
	cron := batchv1.CronJob{ObjectMeta: workloadMeta("cron", now), Spec: batchv1.CronJobSpec{Suspend: &truth}}
	cron.UID = "cron"
	failed := failedOwnedJob("failed", cron.UID, now)
	if got := ClassifyCronJob(&cron, []batchv1.Job{failed}, now); got.Status != WorkloadFailed {
		t.Fatalf("failure did not precede suspension: %+v", got)
	}
	if got := ClassifyCronJob(&cron, nil, now); got.Status != WorkloadSuspended {
		t.Fatalf("suspension not classified: %+v", got)
	}
	cron.Spec.Suspend = nil
	cron.Status.Active = []corev1.ObjectReference{{Name: "active"}}
	if got := ClassifyCronJob(&cron, nil, now); got.Status != WorkloadProgressing || got.Ready == nil || *got.Ready != 1 {
		t.Fatalf("active CronJob not progressing: %+v", got)
	}
}

func workloadNow() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) }

func workloadMeta(name string, now time.Time) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: "payments", Name: name, UID: types.UID(name + "-uid"), Generation: 2, CreationTimestamp: metav1.NewTime(now.Add(-time.Hour))}
}

func int32TestPointer(value int32) *int32 { return &value }

func failedOwnedJob(name string, owner types.UID, at time.Time) batchv1.Job {
	controller := true
	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "payments", CreationTimestamp: metav1.NewTime(at.Add(-time.Minute)), OwnerReferences: []metav1.OwnerReference{{UID: owner, Controller: &controller}}},
		Status:     batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(at)}}},
	}
}

func completeOwnedJob(name string, owner types.UID, at time.Time) batchv1.Job {
	controller := true
	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "payments", CreationTimestamp: metav1.NewTime(at.Add(-time.Minute)), OwnerReferences: []metav1.OwnerReference{{UID: owner, Controller: &controller}}},
		Status:     batchv1.JobStatus{CompletionTime: &metav1.Time{Time: at}, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(at)}}},
	}
}
