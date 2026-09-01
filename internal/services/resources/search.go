package resources

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SearchQuery represents a parsed compound search expression.
// Syntax supported:
//   - term            positive word match
//   - -term or !term negative word match
//   - "exact phrase"  positive phrase match
//   - -"phrase"       negative phrase match
// Multiple terms/phrases are combined with AND semantics for includes and
// excludes. All includes must match and no exclude may match.
type SearchQuery struct {
	Include        []string
	Exclude        []string
	IncludePhrases []string
	ExcludePhrases []string
}

// ParseSearch parses a raw search string into a compound query.
// It preserves the original case-folding behavior used by ContainsFolded.
func ParseSearch(raw string) SearchQuery {
	var (
		include        []string
		exclude        []string
		includePhrases []string
		excludePhrases []string
	)

	remaining := strings.TrimSpace(raw)
	for len(remaining) > 0 {
		remaining = strings.TrimLeftFunc(remaining, unicode.IsSpace)
		if len(remaining) == 0 {
			break
		}

		negated := false
		if remaining[0] == '-' || remaining[0] == '!' {
			negated = true
			remaining = remaining[1:]
			remaining = strings.TrimLeftFunc(remaining, unicode.IsSpace)
			if len(remaining) == 0 {
				break
			}
		}

		var token string
		if remaining[0] == '"' {
			token, remaining = readQuoted(remaining)
		} else {
			token, remaining = readWord(remaining)
		}

		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		if negated {
			if strings.Contains(token, " ") {
				excludePhrases = append(excludePhrases, token)
			} else {
				exclude = append(exclude, token)
			}
		} else {
			if strings.Contains(token, " ") {
				includePhrases = append(includePhrases, token)
			} else {
				include = append(include, token)
			}
		}
	}

	return SearchQuery{
		Include:        include,
		Exclude:        exclude,
		IncludePhrases: includePhrases,
		ExcludePhrases: excludePhrases,
	}
}

func readWord(input string) (string, string) {
	end := strings.IndexFunc(input, unicode.IsSpace)
	if end == -1 {
		return input, ""
	}
	return input[:end], input[end:]
}

func readQuoted(input string) (string, string) {
	if len(input) == 0 || input[0] != '"' {
		return readWord(input)
	}
	input = input[1:]
	var builder strings.Builder
	for index := 0; index < len(input); {
		runeValue, width := utf8.DecodeRuneInString(input[index:])
		if runeValue == '"' {
			return builder.String(), input[index+width:]
		}
		if runeValue == '\\' && index+width < len(input) {
			next, nextWidth := utf8.DecodeRuneInString(input[index+width:])
			if next == '"' || next == '\\' {
				builder.WriteRune(next)
				index += width + nextWidth
				continue
			}
		}
		builder.WriteRune(runeValue)
		index += width
	}
	return builder.String(), ""
}

// IsEmpty reports whether the query has no constraints.
func (query SearchQuery) IsEmpty() bool {
	return len(query.Include)+len(query.Exclude)+len(query.IncludePhrases)+len(query.ExcludePhrases) == 0
}

// Matches reports whether all fields (concatenated with spaces) satisfy the
// query. Matching uses the same Unicode simple case folding as ContainsFolded.
func (query SearchQuery) Matches(fields ...string) bool {
	if query.IsEmpty() {
		return true
	}
	haystack := foldSimple(strings.Join(fields, " "))

	for _, term := range query.Include {
		if !strings.Contains(haystack, foldSimple(term)) {
			return false
		}
	}
	for _, phrase := range query.IncludePhrases {
		if !strings.Contains(haystack, foldSimple(phrase)) {
			return false
		}
	}
	for _, term := range query.Exclude {
		if strings.Contains(haystack, foldSimple(term)) {
			return false
		}
	}
	for _, phrase := range query.ExcludePhrases {
		if strings.Contains(haystack, foldSimple(phrase)) {
			return false
		}
	}
	return true
}
