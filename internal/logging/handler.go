package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	redactedValue = "[REDACTED]"
	maxFieldBytes = 1024
)

var (
	allowedFields = map[string]struct{}{
		"component":  {},
		"operation":  {},
		"request_id": {},
		"context":    {},
		"namespace":  {},
		"resource":   {},
		"duration":   {},
		"error_code": {},
	}
	sensitiveContent = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[a-z0-9._~+/=-]+`),
		regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}(?:\.[a-zA-Z0-9_-]{10,})?\b`),
		regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`),
		regexp.MustCompile(`(?i)\b(?:authorization|password|passwd|token|secret|client[_-]?key)\s*[:=]\s*[^\s,;]+`),
		regexp.MustCompile(`(?s)-----BEGIN [^-]+-----.*?-----END [^-]+-----`),
	}
)

type jsonHandler struct {
	attrs []slog.Attr
	level slog.Leveler
	mu    *sync.Mutex
	out   io.Writer
}

func newJSONHandler(out io.Writer, level slog.Leveler) slog.Handler {
	if level == nil {
		level = slog.LevelInfo
	}
	return &jsonHandler{level: level, mu: &sync.Mutex{}, out: out}
}

func (h *jsonHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *jsonHandler) Handle(_ context.Context, record slog.Record) error {
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	entry := map[string]any{
		"timestamp": timestamp.UTC().Format(time.RFC3339Nano),
		"level":     strings.ToLower(record.Level.String()),
	}
	for _, attr := range h.attrs {
		addAllowed(entry, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		addAllowed(entry, attr)
		return true
	})
	if _, ok := entry["operation"]; !ok && record.Message != "" {
		entry["operation"] = sanitize(record.Message)
	}
	if _, ok := entry["component"]; !ok {
		operation, _ := entry["operation"].(string)
		if operation == "request_finished" || operation == "panic_recovered" || strings.HasPrefix(operation, "http.") {
			entry["component"] = "http"
		}
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode structured log: %w", err)
	}
	line = append(line, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	if _, err := h.out.Write(line); err != nil {
		return fmt.Errorf("write structured log: %w", err)
	}
	return nil
}

func (h *jsonHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cloned := *h
	cloned.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &cloned
}

func (h *jsonHandler) WithGroup(_ string) slog.Handler {
	// Nested groups would violate the flat, allowlisted observability schema.
	return h
}

func addAllowed(entry map[string]any, attr slog.Attr) {
	key := attr.Key
	if key == "http.duration" {
		key = "duration"
	}
	if _, ok := allowedFields[key]; !ok {
		return
	}
	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindDuration:
		entry[key] = value.Duration().String()
	case slog.KindInt64:
		entry[key] = value.Int64()
	case slog.KindUint64:
		entry[key] = value.Uint64()
	case slog.KindFloat64:
		entry[key] = value.Float64()
	case slog.KindBool:
		entry[key] = value.Bool()
	case slog.KindTime:
		entry[key] = value.Time().UTC().Format(time.RFC3339Nano)
	default:
		entry[key] = sanitize(fmt.Sprint(value.Any()))
	}
}

func sanitize(input string) string {
	cleaned := input
	for _, pattern := range sensitiveContent {
		cleaned = pattern.ReplaceAllString(cleaned, redactedValue)
	}
	cleaned = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, cleaned)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return truncateUTF8(cleaned, maxFieldBytes)
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
