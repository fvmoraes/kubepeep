package resources

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fvmoraes/kubepeep/internal/lifecycle"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	MaximumLogLineBytes      = 64 << 10
	MaximumLogEventBytes     = 68 << 10
	MaximumLogContainerBytes = 2 << 20
	MaximumLogResponseBytes  = 10 << 20
	MaximumLogFollowBytes    = 10 << 20
	MaximumLogFollowDuration = 4 * time.Hour
	logSSEEnvelopeReserve    = 64
)

var ErrSlowConsumer = errors.New("resources: slow log consumer")

type LogQuery struct {
	Container  string
	Previous   bool
	Timestamps *bool
	TailLines  int64
	Since      string
}

type LogLineDTO struct {
	Timestamp *string `json:"timestamp"`
	Text      string  `json:"text"`
	Truncated bool    `json:"truncated"`
}

type LogReadDTO struct {
	Container string       `json:"container"`
	Previous  bool         `json:"previous"`
	Lines     []LogLineDTO `json:"lines"`
	Truncated bool         `json:"truncated"`
}

type FollowTerminal struct {
	Reason     string `json:"reason"`
	Generation string `json:"generation"`
	Truncated  bool   `json:"truncated"`
}

type LogService struct {
	Port       LogPort
	Authorizer AuthorizationChecker
	Redactor   TextRedactor
	Now        func() time.Time
}

func NormalizeLogQuery(query LogQuery, follow bool) (LogQuery, error) {
	if len(validation.IsDNS1123Label(query.Container)) > 0 {
		return LogQuery{}, validationError("container must be a DNS label")
	}
	if query.TailLines == 0 {
		query.TailLines = 200
	}
	if query.Timestamps == nil {
		enabled := true
		query.Timestamps = &enabled
	}
	if query.TailLines < 1 || query.TailLines > 2000 {
		return LogQuery{}, validationError("tailLines must be between 1 and 2000")
	}
	if follow && query.Previous {
		return LogQuery{}, validationError("previous is not supported for follow")
	}
	if query.Since != "" {
		seconds, err := parseSince(query.Since)
		if err != nil {
			return LogQuery{}, err
		}
		if seconds > int64((4*time.Hour)/time.Second) {
			return LogQuery{}, validationError("since must not exceed 4h")
		}
	}
	return query, nil
}

func (service *LogService) Read(ctx context.Context, selection Selection, target LogTarget, query LogQuery) (LogReadDTO, error) {
	result := LogReadDTO{Container: query.Container, Previous: query.Previous, Lines: []LogLineDTO{}}
	normalized, err := NormalizeLogQuery(query, false)
	if err != nil {
		return result, err
	}
	result.Container = normalized.Container
	result.Previous = normalized.Previous
	target.Container = normalized.Container
	if err = validateLogTarget(target); err != nil {
		return result, err
	}
	if err = service.authorize(ctx, selection, target); err != nil {
		return result, err
	}
	since, _ := sinceSeconds(normalized.Since)
	reader, err := service.Port.Open(ctx, target, LogSourceOptions{Previous: normalized.Previous, Timestamps: *normalized.Timestamps, TailLines: normalized.TailLines, SinceSeconds: since, LimitBytes: MaximumLogContainerBytes, Follow: false})
	if err != nil {
		return result, sanitizePortError(err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(io.LimitReader(reader, MaximumLogContainerBytes+1))
	if err != nil {
		return result, sanitizePortError(err)
	}
	if len(raw) > MaximumLogContainerBytes {
		raw = raw[:MaximumLogContainerBytes]
		result.Truncated = true
	}
	for _, line := range splitLogLines(raw) {
		dto := service.lineDTO(line, *normalized.Timestamps)
		if dto.Truncated {
			result.Truncated = true
		}
		trial := result
		trial.Lines = append(append([]LogLineDTO(nil), result.Lines...), dto)
		encoded, _ := json.Marshal(trial)
		if len(encoded) > MaximumLogResponseBytes {
			result.Truncated = true
			break
		}
		result.Lines = trial.Lines
	}
	return result, nil
}

// Follow reads incrementally and calls emit synchronously. An emitter can
// return ErrSlowConsumer when its bounded queue is full; no line is silently
// dropped. Heartbeat/meta/end SSE framing remains the handler's responsibility.
func (service *LogService) Follow(ctx context.Context, selection Selection, target LogTarget, query LogQuery, emit func(LogLineDTO) error) (FollowTerminal, error) {
	terminal := FollowTerminal{Reason: "upstream_eof", Generation: selection.Generation}
	normalized, err := NormalizeLogQuery(query, true)
	if err != nil {
		return terminal, err
	}
	if emit == nil {
		return terminal, validationError("follow emitter is required")
	}
	target.Container = normalized.Container
	if err = validateLogTarget(target); err != nil {
		return terminal, err
	}
	streamContext, cancel := context.WithTimeout(ctx, MaximumLogFollowDuration)
	defer cancel()
	if err = service.authorize(streamContext, selection, target); err != nil {
		return terminal, err
	}
	since, _ := sinceSeconds(normalized.Since)
	reader, err := service.Port.Open(streamContext, target, LogSourceOptions{Timestamps: *normalized.Timestamps, TailLines: normalized.TailLines, SinceSeconds: since, LimitBytes: MaximumLogFollowBytes, Follow: true})
	if err != nil {
		return terminal, sanitizePortError(err)
	}
	defer reader.Close()
	limited := &io.LimitedReader{R: reader, N: MaximumLogFollowBytes + 1}
	buffered := bufio.NewReaderSize(limited, 32<<10)
	used := 0
	for {
		raw, truncated, readErr := readBoundedLine(buffered, MaximumLogLineBytes)
		if len(raw) > 0 || truncated {
			line := service.lineDTO(raw, *normalized.Timestamps)
			line.Truncated = line.Truncated || truncated
			line = fitLogEvent(line)
			encoded, _ := json.Marshal(line)
			if used+len(encoded) > MaximumLogFollowBytes {
				terminal.Reason = "limit_reached"
				terminal.Truncated = true
				return terminal, nil
			}
			if emitErr := emit(line); emitErr != nil {
				if errors.Is(emitErr, ErrSlowConsumer) {
					return terminal, domainError(CodeLimitExceeded, "The log stream consumer is too slow.", ErrSlowConsumer)
				}
				return terminal, emitErr
			}
			used += len(encoded)
		}
		if readErr != nil {
			if errors.Is(streamContext.Err(), context.DeadlineExceeded) {
				terminal.Reason = "duration_reached"
				return terminal, nil
			}
			if errors.Is(context.Cause(streamContext), lifecycle.ErrServerShutdown) {
				terminal.Reason = "server_shutdown"
				return terminal, nil
			}
			if errors.Is(readErr, context.Canceled) || errors.Is(streamContext.Err(), context.Canceled) {
				terminal.Reason = "generation_changed"
				return terminal, nil
			}
			if errors.Is(readErr, io.EOF) {
				if limited.N == 0 {
					terminal.Reason = "limit_reached"
					terminal.Truncated = true
				} else if probe, ok := service.Port.(LogTerminationProbe); ok && probe.ContainerTerminated(streamContext, target) {
					terminal.Reason = "container_terminated"
				}
				return terminal, nil
			}
			return terminal, sanitizePortError(readErr)
		}
		select {
		case <-streamContext.Done():
			if errors.Is(streamContext.Err(), context.DeadlineExceeded) {
				terminal.Reason = "duration_reached"
			} else if errors.Is(context.Cause(streamContext), lifecycle.ErrServerShutdown) {
				terminal.Reason = "server_shutdown"
			} else {
				terminal.Reason = "generation_changed"
			}
			return terminal, nil
		default:
		}
	}
}

func (service *LogService) authorize(ctx context.Context, selection Selection, target LogTarget) error {
	if service == nil || service.Port == nil || service.Authorizer == nil {
		return domainError(CodeFeatureUnavailable, "Pod logs are unavailable.", nil)
	}
	capability := service.Authorizer.Check(ctx, authorization.Key{Generation: selection.Generation, Namespace: target.Namespace, Resource: "pods", Subresource: "log", Verb: "get", ResourceName: target.Pod})
	switch capability.Decision {
	case authorization.DecisionAllowed:
		return nil
	case authorization.DecisionDenied:
		return domainError(CodeForbidden, "Access to pod logs was denied.", nil)
	default:
		return domainError(CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
	}
}

func (service *LogService) lineDTO(raw []byte, timestamps bool) LogLineDTO {
	line := string(raw)
	timestamp, text := parseLogTimestamp(line, timestamps)
	if service.Redactor != nil {
		text = service.Redactor.Redact(text)
	}
	truncated := len(text) > MaximumLogLineBytes
	text = sanitizeText(text, MaximumLogLineBytes)
	return LogLineDTO{Timestamp: timestamp, Text: text, Truncated: truncated}
}

func parseLogTimestamp(line string, enabled bool) (*string, string) {
	if !enabled {
		return nil, line
	}
	separator := strings.IndexByte(line, ' ')
	if separator < 1 {
		return nil, line
	}
	parsed, err := time.Parse(time.RFC3339Nano, line[:separator])
	if err != nil {
		return nil, line
	}
	formatted := parsed.UTC().Format(time.RFC3339Nano)
	return &formatted, line[separator+1:]
}
func splitLogLines(raw []byte) [][]byte {
	if len(raw) == 0 {
		return nil
	}
	parts := strings.Split(string(raw), "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	result := make([][]byte, len(parts))
	for i := range parts {
		result[i] = []byte(parts[i])
	}
	return result
}
func validateLogTarget(target LogTarget) error {
	if len(validation.IsDNS1123Subdomain(target.Namespace)) > 0 || len(validation.IsDNS1123Subdomain(target.Pod)) > 0 {
		return validationError("pod target is invalid")
	}
	if target.Container == "" {
		return validationError("container is required")
	}
	return nil
}
func parseSince(value string) (int64, error) {
	if len(value) < 2 {
		return 0, validationError("since has an invalid value")
	}
	unit := value[len(value)-1]
	if unit != 's' && unit != 'm' && unit != 'h' {
		return 0, validationError("since has an invalid value")
	}
	number := value[:len(value)-1]
	if number[0] == '0' {
		return 0, validationError("since has an invalid value")
	}
	amount, err := strconv.ParseInt(number, 10, 64)
	if err != nil || amount < 1 {
		return 0, validationError("since has an invalid value")
	}
	multiplier := int64(1)
	if unit == 'm' {
		multiplier = 60
	}
	if unit == 'h' {
		multiplier = 3600
	}
	if amount > int64((4*time.Hour)/time.Second)/multiplier {
		return 0, validationError("since must not exceed 4h")
	}
	return amount * multiplier, nil
}
func sinceSeconds(value string) (*int64, error) {
	if value == "" {
		return nil, nil
	}
	seconds, err := parseSince(value)
	if err != nil {
		return nil, err
	}
	return &seconds, nil
}
func sanitizePortError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return domainError(CodeUpstreamTimeout, "The Kubernetes request timed out.", err)
	}
	if errors.Is(err, context.Canceled) {
		return domainError(CodeGenerationChanged, "The active selection changed.", err)
	}
	var domain *DomainError
	if errors.As(err, &domain) {
		return err
	}
	return domainError(CodeClusterUnavailable, "The Kubernetes API could not complete the request.", err)
}

func readBoundedLine(reader *bufio.Reader, maximum int) ([]byte, bool, error) {
	result := make([]byte, 0, min(maximum, 4096))
	truncated := false
	for {
		fragment, prefix, err := reader.ReadLine()
		if len(fragment) > 0 {
			remaining := maximum - len(result)
			if remaining > 0 {
				take := min(remaining, len(fragment))
				result = append(result, fragment[:take]...)
			}
			if len(fragment) > remaining {
				truncated = true
			}
		}
		if err != nil {
			return result, truncated, err
		}
		if !prefix {
			return result, truncated, nil
		}
		truncated = truncated || len(result) >= maximum
	}
}
func fitLogEvent(line LogLineDTO) LogLineDTO {
	encoded, _ := json.Marshal(line)
	if len(encoded) <= MaximumLogEventBytes-logSSEEnvelopeReserve {
		return line
	}
	line.Truncated = true
	low, high, best := 0, len(line.Text), 0
	for low <= high {
		middle := (low + high) / 2
		for middle > 0 && !utf8.ValidString(line.Text[:middle]) {
			middle--
		}
		candidate := line
		candidate.Text = line.Text[:middle]
		bytes, _ := json.Marshal(candidate)
		if len(bytes) <= MaximumLogEventBytes-logSSEEnvelopeReserve {
			best = middle
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	line.Text = line.Text[:best]
	return line
}
