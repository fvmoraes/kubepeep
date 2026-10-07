package resources

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	MaximumLogDownloadBytes    = 64 << 20
	MaximumLogDownloadDuration = 5 * time.Minute
)

// Download reads the complete retained container log, without a tail or time
// filter. It streams sanitized lines instead of accumulating a response in RAM.
func (service *LogService) Download(ctx context.Context, selection Selection, target LogTarget, query LogQuery, emit func(LogLineDTO) error) (FollowTerminal, error) {
	terminal := FollowTerminal{Reason: "completed", Generation: selection.Generation}
	if query.Since != "" || query.TailLines != 0 {
		return terminal, validationError("log downloads do not accept tailLines or since")
	}
	normalized, err := NormalizeLogQuery(query, false)
	if err != nil {
		return terminal, err
	}
	if emit == nil {
		return terminal, validationError("download emitter is required")
	}
	target.Container = normalized.Container
	if err = validateLogTarget(target); err != nil {
		return terminal, err
	}
	ctx, cancel := context.WithTimeout(ctx, MaximumLogDownloadDuration)
	defer cancel()
	if err = service.authorize(ctx, selection, target); err != nil {
		return terminal, err
	}
	// A negative tail is internal only: the adapter omits TailLines altogether.
	reader, err := service.Port.Open(ctx, target, LogSourceOptions{Previous: normalized.Previous, Timestamps: *normalized.Timestamps, TailLines: -1})
	if err != nil {
		return terminal, sanitizePortError(err)
	}
	defer reader.Close()
	limited := &io.LimitedReader{R: reader, N: MaximumLogDownloadBytes + 1}
	buffered := bufio.NewReaderSize(limited, 32<<10)
	used := 0
	for {
		if err = ctx.Err(); err != nil {
			return terminal, sanitizePortError(err)
		}
		raw, truncated, readErr := readBoundedLine(buffered, MaximumLogLineBytes)
		if len(raw) > 0 || truncated {
			line := fitLogEvent(service.lineDTO(raw, *normalized.Timestamps))
			line.Truncated = line.Truncated || truncated
			encoded, encodeErr := json.Marshal(line)
			if encodeErr != nil {
				return terminal, encodeErr
			}
			if used+len(encoded) > MaximumLogDownloadBytes {
				terminal.Reason, terminal.Truncated = "limit_reached", true
				return terminal, nil
			}
			if err = emit(line); err != nil {
				return terminal, err
			}
			used += len(encoded)
			terminal.Truncated = terminal.Truncated || line.Truncated
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return terminal, sanitizePortError(readErr)
			}
			if limited.N == 0 {
				terminal.Reason, terminal.Truncated = "limit_reached", true
			}
			return terminal, nil
		}
	}
}
