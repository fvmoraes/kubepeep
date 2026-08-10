package namespaces

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	InvalidNamespaceNameCode = "INVALID_NAMESPACE_NAME"
	NamespaceNotFoundCode    = "NAMESPACE_NOT_FOUND"
)

type InvalidNamespace struct {
	Input string `json:"input"`
	Code  string `json:"code"`
}

type ExistenceReport struct {
	Checked    bool   `json:"checked"`
	ReasonCode string `json:"reasonCode"`
}

type ValidationReport struct {
	Valid               []string           `json:"valid"`
	ValidCount          int                `json:"validCount"`
	DuplicateCount      int                `json:"duplicateCount"`
	DiscardedEmptyCount int                `json:"discardedEmptyCount"`
	Invalid             []InvalidNamespace `json:"invalid"`
	InvalidCount        int                `json:"invalidCount"`
	Existence           ExistenceReport    `json:"existence"`
}

// ParseRawInput implements the format commitment rules from docs/api.md. Once
// input looks like JSON or YAML, a parse failure is final and never falls back
// to the permissive text tokenizer.
func ParseRawInput(raw string) (ValidationReport, error) {
	if len(raw) > MaxScopeBodyBytes {
		return ValidationReport{}, ErrBodyTooLarge
	}
	if !utf8.ValidString(raw) {
		return ValidationReport{}, invalidInputError()
	}
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "\ufeff"))
	if trimmed == "" {
		return classifyItems(nil)
	}

	var (
		items []string
		err   error
	)
	switch {
	case strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{"):
		items, err = parseJSONItems(trimmed)
	case strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "namespaces:") || strings.HasPrefix(trimmed, "- "):
		items, err = parseYAMLItems(trimmed)
	default:
		items = tokenizeText(trimmed)
	}
	if err != nil {
		return ValidationReport{}, invalidInputError()
	}
	if len(items) > MaxNamespaceEntries {
		return ValidationReport{}, ErrNamespaceLimit
	}
	return classifyItems(items)
}

func ParseNamespaceList(items []string) (ValidationReport, error) {
	if len(items) > MaxNamespaceEntries {
		return ValidationReport{}, ErrNamespaceLimit
	}
	return classifyItems(items)
}

func invalidInputError() error {
	return fmt.Errorf("%w: INVALID_NAMESPACE_INPUT", ErrInvalidNamespaceInput)
}

func parseJSONItems(input string) ([]string, error) {
	decoder := json.NewDecoder(strings.NewReader(input))
	if strings.HasPrefix(input, "[") {
		var items []string
		if err := decoder.Decode(&items); err != nil || items == nil {
			return nil, ErrInvalidNamespaceInput
		}
		if err := requireJSONEOF(decoder); err != nil {
			return nil, err
		}
		return items, nil
	}

	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, ErrInvalidNamespaceInput
	}
	seenNamespaces := false
	var items []string
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok || name != "namespaces" || seenNamespaces {
			return nil, ErrInvalidNamespaceInput
		}
		seenNamespaces = true
		if err := decoder.Decode(&items); err != nil || items == nil {
			return nil, ErrInvalidNamespaceInput
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || !seenNamespaces {
		return nil, ErrInvalidNamespaceInput
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	return items, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidNamespaceInput
	}
	return nil
}

func parseYAMLItems(input string) ([]string, error) {
	decoder := yaml.NewDecoder(strings.NewReader(input))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidNamespaceInput
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, ErrInvalidNamespaceInput
	}
	root := document.Content[0]
	if hasForbiddenYAMLSyntax(root) {
		return nil, ErrInvalidNamespaceInput
	}
	switch root.Kind {
	case yaml.SequenceNode:
		return yamlStringSequence(root)
	case yaml.MappingNode:
		if len(root.Content) != 2 {
			return nil, ErrInvalidNamespaceInput
		}
		key, value := root.Content[0], root.Content[1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value != "namespaces" || value.Kind != yaml.SequenceNode {
			return nil, ErrInvalidNamespaceInput
		}
		return yamlStringSequence(value)
	default:
		return nil, ErrInvalidNamespaceInput
	}
}

func yamlStringSequence(sequence *yaml.Node) ([]string, error) {
	items := make([]string, 0, len(sequence.Content))
	for _, item := range sequence.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
			return nil, ErrInvalidNamespaceInput
		}
		items = append(items, item.Value)
	}
	return items, nil
}

func hasForbiddenYAMLSyntax(node *yaml.Node) bool {
	if node == nil || node.Kind == yaml.AliasNode || node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 {
		return true
	}
	for _, child := range node.Content {
		if hasForbiddenYAMLSyntax(child) {
			return true
		}
	}
	return false
}

func tokenizeText(input string) []string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n")
	// Preserve empty structural segments because the contract counts them,
	// while treating a run of spaces/tabs inside a segment as one separator.
	segments := splitStructural(normalized)
	items := make([]string, 0, len(segments))
	for _, segment := range segments {
		trimmed := strings.Trim(segment, " \t")
		if trimmed == "" {
			items = append(items, "")
			continue
		}
		fields := strings.FieldsFunc(trimmed, func(r rune) bool { return r == ' ' || r == '\t' })
		items = append(items, fields...)
	}
	return items
}

func splitStructural(input string) []string {
	var segments []string
	start := 0
	for index, character := range input {
		if character != ',' && character != ';' && character != '\n' {
			continue
		}
		segments = append(segments, input[start:index])
		start = index + 1
	}
	segments = append(segments, input[start:])
	return segments
}

func classifyItems(items []string) (ValidationReport, error) {
	report := ValidationReport{
		Valid:     make([]string, 0, len(items)),
		Invalid:   make([]InvalidNamespace, 0),
		Existence: ExistenceReport{Checked: false, ReasonCode: "NOT_CHECKED"},
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			report.DiscardedEmptyCount++
			continue
		}
		if _, duplicate := seen[item]; duplicate {
			report.DuplicateCount++
			continue
		}
		seen[item] = struct{}{}
		if !ValidNamespaceName(item) {
			report.Invalid = append(report.Invalid, InvalidNamespace{Input: item, Code: InvalidNamespaceNameCode})
			continue
		}
		report.Valid = append(report.Valid, item)
	}
	report.ValidCount = len(report.Valid)
	report.InvalidCount = len(report.Invalid)
	return report, nil
}
