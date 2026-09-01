package resources

import (
	"strings"

	"sigs.k8s.io/yaml"
)

// LastAppliedAnnotation stores the manifest kubectl last applied. Diffing the
// live document against it answers "what did I change since apply".
const LastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

const (
	// maximumDiffInputLines bounds either side before diffing; documents this
	// large exceed every realistic read-only view.
	maximumDiffInputLines = 6_000
	// MaximumDiffLines bounds the rendered result; beyond it the diff is
	// truncated instead of unbounded.
	MaximumDiffLines = 4_000
)

type DiffLineKind string

const (
	DiffSame    DiffLineKind = "same"
	DiffAdded   DiffLineKind = "added"
	DiffRemoved DiffLineKind = "removed"
)

type DiffLineDTO struct {
	Kind DiffLineKind `json:"kind"`
	Text string       `json:"text"`
}

type LastAppliedDiffDTO struct {
	// Absent is true when the object carries no last-applied annotation, so
	// no baseline exists to diff against. No zero value is implied.
	Absent    bool          `json:"absent"`
	Truncated bool          `json:"truncated"`
	Lines     []DiffLineDTO `json:"lines"`
}

type annotationReader interface {
	GetAnnotations() map[string]string
}

type annotationWriter interface {
	SetAnnotations(map[string]string)
}

// ExtractLastApplied returns the previous manifest as normalized YAML. The
// bool is false (with no error) when the annotation is absent.
func ExtractLastApplied(value any) ([]byte, bool, error) {
	reader, ok := value.(annotationReader)
	if !ok {
		return nil, false, nil
	}
	raw, ok := reader.GetAnnotations()[LastAppliedAnnotation]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, false, nil
	}
	if len(raw) > MaximumYAMLBytes {
		return nil, true, domainError(CodeLimitExceeded, "The last-applied configuration exceeds the response limit.", nil)
	}
	normalized, err := yaml.JSONToYAML([]byte(raw))
	if err != nil {
		return nil, true, domainError(CodeValidationFailed, "The last-applied configuration is not valid JSON.", err)
	}
	return normalized, true, nil
}

// StripLastAppliedAnnotation removes the last-applied annotation from the
// object so the current document does not shadow the whole diff with its own
// payload. The caller owns the object; mutation is safe on freshly fetched
// values.
func StripLastAppliedAnnotation(value any) any {
	writer, ok := value.(annotationWriter)
	if !ok {
		return value
	}
	var annotations map[string]string
	if reader, ok := value.(annotationReader); ok {
		annotations = reader.GetAnnotations()
	}
	if annotations == nil {
		return value
	}
	if _, ok := annotations[LastAppliedAnnotation]; !ok {
		return value
	}
	copied := make(map[string]string, len(annotations)-1)
	for key, text := range annotations {
		if key == LastAppliedAnnotation {
			continue
		}
		copied[key] = text
	}
	writer.SetAnnotations(copied)
	return value
}

// DiffYAML renders a bounded line diff from previous to current. Inputs must
// already be normalized YAML (JSONToYAML) so both sides compare fairly.
func DiffYAML(current, previous []byte) LastAppliedDiffDTO {
	currentLines := splitLines(current)
	previousLines := splitLines(previous)
	if len(currentLines) > maximumDiffInputLines {
		currentLines = currentLines[:maximumDiffInputLines]
	}
	if len(previousLines) > maximumDiffInputLines {
		previousLines = previousLines[:maximumDiffInputLines]
	}
	ops := myersDiff(previousLines, currentLines)
	lines := make([]DiffLineDTO, 0, len(ops))
	truncated := false
	for _, op := range ops {
		if len(lines) >= MaximumDiffLines {
			truncated = true
			break
		}
		switch op.kind {
		case opSame:
			lines = append(lines, DiffLineDTO{Kind: DiffSame, Text: op.text})
		case opInsert:
			lines = append(lines, DiffLineDTO{Kind: DiffAdded, Text: op.text})
		case opDelete:
			lines = append(lines, DiffLineDTO{Kind: DiffRemoved, Text: op.text})
		}
	}
	if lines == nil {
		lines = []DiffLineDTO{}
	}
	return LastAppliedDiffDTO{Lines: lines, Truncated: truncated}
}

type diffOperationKind int

const (
	opSame diffOperationKind = iota
	opInsert
	opDelete
)

type diffOperation struct {
	kind diffOperationKind
	text string
}

func splitLines(value []byte) []string {
	text := strings.ReplaceAll(string(value), "\r\n", "\n")
	if text == "" {
		return []string{""}
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// myersDiff is the greedy O((N+M)D) diff from Myers' paper, adequate for the
// bounded read-only documents this feature serves.
func myersDiff(previous, current []string) []diffOperation {
	n, m := len(previous), len(current)
	max := n + m
	if max == 0 {
		return []diffOperation{}
	}
	// Trim the common prefix and suffix first; this shrinks D dramatically
	// for the common "edited a few fields" case.
	prefix := 0
	for prefix < n && prefix < m && previous[prefix] == current[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < n-prefix && suffix < m-prefix && previous[n-1-suffix] == current[m-1-suffix] {
		suffix++
	}
	head := make([]diffOperation, 0, prefix)
	for _, text := range previous[:prefix] {
		head = append(head, diffOperation{kind: opSame, text: text})
	}
	middle := myersMiddle(previous[prefix:n-suffix], current[prefix:m-suffix])
	tail := make([]diffOperation, 0, suffix)
	for _, text := range previous[n-suffix:] {
		tail = append(tail, diffOperation{kind: opSame, text: text})
	}
	return append(append(head, middle...), tail...)
}

func myersMiddle(previous, current []string) []diffOperation {
	n, m := len(previous), len(current)
	max := n + m
	if max == 0 {
		return []diffOperation{}
	}
	trace := make([][]int, 0, max+1)
	vertical := make([]int, 2*max+1)
	offset := max
	for d := 0; d <= max; d++ {
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && vertical[k-1+offset] < vertical[k+1+offset]) {
				x = vertical[k+1+offset]
			} else {
				x = vertical[k-1+offset] + 1
			}
			y := x - k
			for x < n && y < m && previous[x] == current[y] {
				x++
				y++
			}
			vertical[k+offset] = x
			if x >= n && y >= m {
				// The backtrack reads diagonal k±1 written during iteration d,
				// so the final state is captured before leaving.
				trace = append(trace, append([]int(nil), vertical...))
				return myersBacktrack(trace, previous, current)
			}
		}
		// Snapshot the state AFTER iteration d: backtracking relies on the
		// writes of this round being visible at trace[d].
		trace = append(trace, append([]int(nil), vertical...))
	}
	// Unreachable for complete traces; kept as a safe fallback.
	operations := make([]diffOperation, 0, n+m)
	for _, text := range previous {
		operations = append(operations, diffOperation{kind: opDelete, text: text})
	}
	for _, text := range current {
		operations = append(operations, diffOperation{kind: opInsert, text: text})
	}
	return operations
}

func myersBacktrack(trace [][]int, previous, current []string) []diffOperation {
	operations := make([]diffOperation, 0, len(previous)+len(current))
	x, y := len(previous), len(current)
	offset := len(previous) + len(current)
	for d := len(trace) - 2; d >= 0; d-- {
		snapshot := trace[d]
		k := x - y
		var previousK int
		if k == -d || (k != d && snapshot[k-1+offset] < snapshot[k+1+offset]) {
			previousK = k + 1
		} else {
			previousK = k - 1
		}
		previousX := snapshot[previousK+offset]
		previousY := previousX - previousK
		for x > previousX && y > previousY {
			operations = append(operations, diffOperation{kind: opSame, text: previous[x-1]})
			x--
			y--
		}
		if x == previousX {
			operations = append(operations, diffOperation{kind: opInsert, text: current[previousY]})
			y--
		} else {
			operations = append(operations, diffOperation{kind: opDelete, text: previous[x-1]})
			x--
		}
	}
	// Reverse in place.
	for left, right := 0, len(operations)-1; left < right; left, right = left+1, right-1 {
		operations[left], operations[right] = operations[right], operations[left]
	}
	return operations
}
