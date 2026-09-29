package actions

import (
	"context"
	"time"
)

// AuditEvent is deliberately incapable of carrying request bodies, commands,
// tickets, stream data, ports, upstream error text, or Kubernetes object
// identities. Resource contains only a canonical kind from a closed set.
type AuditEvent struct {
	Timestamp time.Time
	Level     string
	Component string
	Operation string
	Resource  string
	Duration  time.Duration
	ErrorCode ErrorCode
}

type AuditSink interface {
	Record(context.Context, AuditEvent)
}

type NoopAuditSink struct{}

func (NoopAuditSink) Record(context.Context, AuditEvent) {}

func recordAudit(ctx context.Context, sink AuditSink, clock Clock, started time.Time, operation string, target MutationTarget, err error) {
	if sink == nil {
		return
	}
	level := "info"
	code := ErrorCodeOf(err)
	if err != nil {
		level = "error"
		if code == "" {
			code = CodeInternal
		}
	}
	now := clock.Now().UTC()
	sink.Record(ctx, AuditEvent{
		Timestamp: now,
		Level:     level,
		Component: "actions",
		Operation: safeMetadata(operation),
		Resource:  auditResourceKind(target.Kind),
		Duration:  nonNegativeDuration(now.Sub(started)),
		ErrorCode: code,
	})
}

func auditResourceKind(kind string) string {
	switch kind {
	case "Pod", "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "ReplicaSet", "Service":
		return kind
	default:
		return "Unknown"
	}
}

func nonNegativeDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}
