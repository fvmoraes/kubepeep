package actions

import (
	"context"
	"time"
)

// AuditEvent is deliberately incapable of carrying request bodies, commands,
// tickets, stream data, ports, or upstream error text.
type AuditEvent struct {
	Timestamp time.Time
	Level     string
	Component string
	Operation string
	Context   string
	Namespace string
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
		Context:   safeMetadata(target.Context),
		Namespace: safeMetadata(target.Namespace),
		Resource:  safeMetadata(target.Kind + "/" + target.Name),
		Duration:  nonNegativeDuration(now.Sub(started)),
		ErrorCode: code,
	})
}

func nonNegativeDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}
