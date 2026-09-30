package kubernetesruntime

import (
	"context"
	"log/slog"

	gingerlogger "github.com/fvmoraes/ginger/pkg/logger"
	"github.com/fvmoraes/kubepeep/internal/services/actions"
)

// ActionAuditSink emits only the fixed metadata allowlisted by the local JSONL
// logger. It has no fields capable of carrying command, ticket, stream, body,
// port, upstream error content, or Kubernetes object identity.
type ActionAuditSink struct{ logger *gingerlogger.Logger }

func NewActionAuditSink(logger *gingerlogger.Logger) actions.AuditSink {
	if logger == nil || logger.Logger == nil {
		return actions.NoopAuditSink{}
	}
	return &ActionAuditSink{logger: logger}
}

func (sink *ActionAuditSink) Record(ctx context.Context, event actions.AuditEvent) {
	if sink == nil || sink.logger == nil || sink.logger.Logger == nil {
		return
	}
	level := slog.LevelInfo
	if event.Level == "error" {
		level = slog.LevelError
	}
	sink.logger.Logger.LogAttrs(ctx, level, event.Operation,
		slog.String("component", event.Component),
		slog.String("operation", event.Operation),
		slog.String("resource", event.Resource),
		slog.Duration("duration", event.Duration),
		slog.String("error_code", string(event.ErrorCode)),
	)
}

var _ actions.AuditSink = (*ActionAuditSink)(nil)
