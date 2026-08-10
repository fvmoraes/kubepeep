package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"sort"
	"strings"
	"time"
)

type DetectedLogMatch struct {
	Timestamp  *string
	Excerpt    string
	ReasonCode LogReasonCode
	Redacted   bool
	Truncated  bool
}

type detectionCandidate struct {
	reason    LogReasonCode
	fieldRank int
	excerpt   string
}

var textKeywords = []struct {
	keyword string
	reason  LogReasonCode
}{
	{"panic", LogPanic},
	{"oom", LogOOM},
	{"segmentation fault", LogErrorKeyword},
	{"exception", LogErrorKeyword},
	{"fatal", LogErrorKeyword},
	{"error", LogErrorKeyword},
	{"failed", LogErrorKeyword},
	{"failure", LogErrorKeyword},
	{"timeout", LogErrorKeyword},
	{"refused", LogErrorKeyword},
	{"unavailable", LogErrorKeyword},
	{"killed", LogErrorKeyword},
}

// DetectLogLine applies the closed detector to one already bounded line. It
// returns at most one match and redacts before constructing the result.
func DetectLogLine(line []byte, sourceTruncated bool) (DetectedLogMatch, bool) {
	content, streamTimestamp := splitStreamTimestamp(line)
	candidate, jsonTimestamp, ok := detectCandidate(content)
	if !ok {
		return DetectedLogMatch{}, false
	}
	timestamp := jsonTimestamp
	if timestamp == nil {
		timestamp = streamTimestamp
	}
	excerpt, redacted := Redact(candidate.excerpt)
	excerpt = sanitizeText(excerpt, len(excerpt))
	excerpt, excerptTruncated := truncateUTF8(excerpt, MaximumExcerptBytes)
	return DetectedLogMatch{
		Timestamp:  timestamp,
		Excerpt:    excerpt,
		ReasonCode: candidate.reason,
		Redacted:   redacted,
		Truncated:  sourceTruncated || excerptTruncated,
	}, true
}

func detectCandidate(line []byte) (detectionCandidate, *string, bool) {
	object, validObject := decodeJSONObject(line)
	if !validObject {
		candidate, ok := textCandidate(string(line), 6)
		return candidate, nil, ok
	}
	candidates := make([]detectionCandidate, 0, 8)
	for _, field := range []struct {
		name string
		rank int
	}{{"message", 0}, {"msg", 1}} {
		if value, ok := object[field.name].(string); ok {
			if candidate, matched := textCandidate(value, field.rank); matched {
				candidates = append(candidates, candidate)
			}
		}
	}
	if value, exists := object["error"]; exists && meaningfulJSONError(value) {
		excerpt, marshalOK := canonicalJSONValue(value)
		if marshalOK {
			candidates = append(candidates, detectionCandidate{reason: LogJSONErrorField, fieldRank: 2, excerpt: excerpt})
		}
	}
	if value, ok := object["stack"].(string); ok && strings.TrimSpace(value) != "" {
		candidates = append(candidates, detectionCandidate{reason: LogStackTrace, fieldRank: 3, excerpt: value})
	}
	for _, field := range []struct {
		name string
		rank int
	}{{"level", 4}, {"severity", 5}} {
		if value, ok := object[field.name].(string); ok && jsonErrorLevel(value) {
			candidates = append(candidates, detectionCandidate{reason: LogJSONErrorLevel, fieldRank: field.rank, excerpt: value})
		}
	}
	if len(candidates) == 0 {
		return detectionCandidate{}, jsonTimestamp(object), false
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if reasonRank(candidates[left].reason) != reasonRank(candidates[right].reason) {
			return reasonRank(candidates[left].reason) < reasonRank(candidates[right].reason)
		}
		return candidates[left].fieldRank < candidates[right].fieldRank
	})
	return candidates[0], jsonTimestamp(object), true
}

func textCandidate(value string, fieldRank int) (detectionCandidate, bool) {
	lower := asciiLower(value)
	for _, entry := range textKeywords {
		if containsBoundedKeyword(lower, entry.keyword) {
			return detectionCandidate{reason: entry.reason, fieldRank: fieldRank, excerpt: value}, true
		}
	}
	return detectionCandidate{}, false
}

func asciiLower(value string) string {
	buffer := []byte(value)
	for index, current := range buffer {
		if current >= 'A' && current <= 'Z' {
			buffer[index] = current + ('a' - 'A')
		}
	}
	return string(buffer)
}

func containsBoundedKeyword(value, keyword string) bool {
	for offset := 0; offset <= len(value)-len(keyword); {
		index := strings.Index(value[offset:], keyword)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(keyword)
		leftOK := index == 0 || !asciiAlphaNumeric(value[index-1])
		rightOK := end == len(value) || !asciiAlphaNumeric(value[end])
		if leftOK && rightOK {
			return true
		}
		offset = index + 1
	}
	return false
}

func asciiAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func decodeJSONObject(line []byte) (map[string]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, false
	}
	object, ok := value.(map[string]any)
	if !ok || jsonDepth(object, 1) > 8 {
		return nil, false
	}
	return object, true
}

func jsonDepth(value any, depth int) int {
	maximum := depth
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			childDepth := jsonDepth(child, depth+1)
			if childDepth > maximum {
				maximum = childDepth
			}
		}
	case []any:
		for _, child := range typed {
			childDepth := jsonDepth(child, depth+1)
			if childDepth > maximum {
				maximum = childDepth
			}
		}
	}
	return maximum
}

func meaningfulJSONError(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case bool:
		return typed
	case json.Number:
		floatValue, _, err := big.ParseFloat(string(typed), 10, 256, big.ToNearestEven)
		return err == nil && floatValue.Sign() != 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return false
	}
}

func canonicalJSONValue(value any) (string, bool) {
	if text, ok := value.(string); ok {
		return text, true
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func jsonErrorLevel(value string) bool {
	switch strings.ToLower(value) {
	case "error", "fatal", "panic", "critical":
		return true
	default:
		return false
	}
}

func reasonRank(value LogReasonCode) int {
	switch value {
	case LogPanic:
		return 0
	case LogOOM:
		return 1
	case LogStackTrace:
		return 2
	case LogJSONErrorLevel:
		return 3
	case LogJSONErrorField:
		return 4
	case LogErrorKeyword:
		return 5
	default:
		return 6
	}
}

func jsonTimestamp(object map[string]any) *string {
	for _, field := range []string{"timestamp", "time"} {
		value, ok := object[field].(string)
		if !ok {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err == nil {
			formatted := parsed.UTC().Format(time.RFC3339Nano)
			return &formatted
		}
	}
	return nil
}

func splitStreamTimestamp(line []byte) ([]byte, *string) {
	space := bytes.IndexByte(line, ' ')
	if space <= 0 {
		return line, nil
	}
	token := string(line[:space])
	parsed, err := time.Parse(time.RFC3339Nano, token)
	if err != nil {
		return line, nil
	}
	formatted := parsed.UTC().Format(time.RFC3339Nano)
	return bytes.TrimLeft(line[space+1:], " \t"), &formatted
}
