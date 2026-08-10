package dashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"sync"
	"time"
)

type LogService struct {
	reader     LogReader
	authorizer LogAuthorizer
	clock      Clock
	budget     LogBudget
}

func NewLogService(reader LogReader, authorizer LogAuthorizer, clock Clock, budget LogBudget) *LogService {
	if clock == nil {
		clock = realClock{}
	}
	return &LogService{reader: reader, authorizer: authorizer, clock: clock, budget: budget.normalized()}
}

type targetScanResult struct {
	index     int
	matches   []LogMatchDTO
	truncated bool
	err       error
}

var errLogScanByteBudgetExhausted = errors.New("log scan byte budget exhausted")

// Scan performs no writes and owns no storage dependency. Targets and results
// live only for the lifetime of this call.
func (s *LogService) Scan(ctx context.Context, request LogScanRequest, targets []LogTarget) DashboardBlockDTO[[]LogMatchDTO] {
	resolved, err := ResolveLogScanRequest(request)
	namespaces := targetNamespaces(targets)
	block := blockWithValue(make([]LogMatchDTO, 0), emptyCoverage(len(namespaces)))
	if err != nil {
		addBlockError(&block, "", err)
		return block
	}
	if s == nil || s.reader == nil || s.authorizer == nil {
		addBlockError(&block, "", NewFeatureUnavailableError())
		return block
	}
	limitedTargets, targetTruncated := limitExplicitTargets(targets, resolved.MaxPods, s.budget.MaxContainers)
	if targetTruncated {
		block.Truncated = true
		block.Complete = false
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	scanBytes := newLogScanByteBudget(s.budget.MaxScanBytes)
	capturedNow := s.clock.Now()
	allowed := make([]LogTarget, 0, len(limitedTargets))
	authorization := make(map[string]PermissionDecision)
	authorizationFailed := make(map[string]bool)
	for _, target := range limitedTargets {
		if err := requestContext.Err(); err != nil {
			addUniqueBlockError(&block, target.Namespace, err)
			break
		}
		key := target.Namespace + "\x00" + target.Pod
		decision, known := authorization[key]
		if !known {
			var permissionErr error
			decision, permissionErr = s.authorizer.CanReadPodLogs(requestContext, target.Namespace, target.Pod)
			if permissionErr != nil {
				addUniqueBlockError(&block, target.Namespace, permissionErr)
				authorization[key] = PermissionUnknown
				authorizationFailed[key] = true
				continue
			}
			authorization[key] = decision
		}
		if authorizationFailed[key] {
			continue
		}
		switch decision {
		case PermissionAllowed:
			allowed = append(allowed, target)
		case PermissionDenied:
			addUniqueBlockError(&block, target.Namespace, NewDeniedError())
		case PermissionUnknown:
			addUniqueBlockError(&block, target.Namespace, NewAuthorizationUnavailableError())
		default:
			addUniqueBlockError(&block, target.Namespace, NewAuthorizationUnavailableError())
		}
	}

	jobs := make(chan int)
	results := make(chan targetScanResult, len(allowed))
	var workers sync.WaitGroup
	workerCount := minInt(resolved.MaxConcurrentContainers, len(allowed))
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-requestContext.Done():
					return
				case <-scanBytes.Done():
					return
				case index, open := <-jobs:
					if !open {
						return
					}
					results <- s.scanTarget(requestContext, resolved, allowed[index], index, capturedNow, scanBytes)
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range allowed {
			select {
			case jobs <- index:
			case <-requestContext.Done():
				return
			case <-scanBytes.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	ordered := make([]targetScanResult, 0, len(allowed))
	for result := range results {
		ordered = append(ordered, result)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].index < ordered[right].index })
	completedTargets := make(map[string]int)
	expectedTargets := make(map[string]int)
	for _, target := range allowed {
		expectedTargets[target.Namespace]++
	}
	for _, result := range ordered {
		namespace := allowed[result.index].Namespace
		if result.err != nil {
			addUniqueBlockError(&block, namespace, result.err)
			continue
		}
		completedTargets[namespace]++
		if result.truncated {
			block.Truncated = true
			block.Complete = false
		}
		block.Value = append(block.Value, result.matches...)
	}
	if err := requestContext.Err(); err != nil && len(ordered) < len(allowed) {
		addUniqueBlockError(&block, "", err)
	}
	if scanBytes.Exhausted() || (len(ordered) < len(allowed) && requestContext.Err() == nil) {
		block.Truncated = true
		block.Complete = false
	}
	for _, namespace := range namespaces {
		if expectedTargets[namespace] > 0 && completedTargets[namespace] == expectedTargets[namespace] && !containsNamespaceError(block.Errors, namespace) {
			block.Coverage.CompletedNamespaces++
		}
	}
	block.Value, targetTruncated = capSerializedMatches(block.Value, s.budget.MaxScanBytes)
	if targetTruncated {
		block.Truncated = true
		block.Complete = false
	}
	return block
}

func (s *LogService) scanTarget(ctx context.Context, request ResolvedLogScanRequest, target LogTarget, index int, capturedNow time.Time, scanBytes *logScanByteBudget) targetScanResult {
	if scanBytes.Exhausted() {
		return targetScanResult{index: index, truncated: true}
	}
	reader, err := s.reader.ReadLogs(ctx, LogReadRequest{
		Namespace:  target.Namespace,
		Pod:        target.Pod,
		Container:  target.Container,
		Previous:   target.Previous,
		SinceTime:  capturedNow.Add(-request.Window),
		TailLines:  request.TailLines,
		Timestamps: true,
	})
	if err != nil {
		return targetScanResult{index: index, err: err}
	}
	defer reader.Close()
	stopClose := closeReaderOnCancel(ctx, scanBytes.Done(), reader)
	defer stopClose()
	limited := &logScanBudgetReader{ctx: ctx, source: reader, budget: scanBytes}
	lines, truncated, readErr := readBoundedLogLines(ctx, limited, s.budget, request.TailLines)
	if readErr != nil {
		return targetScanResult{index: index, truncated: truncated, err: readErr}
	}
	matches := make([]LogMatchDTO, 0)
	for _, line := range lines {
		detected, ok := DetectLogLine(line.value, line.truncated)
		if !ok {
			continue
		}
		matches = append(matches, LogMatchDTO{
			Namespace:  target.Namespace,
			Pod:        target.Pod,
			Container:  target.Container,
			Workload:   cloneResourceRef(target.Workload),
			Timestamp:  detected.Timestamp,
			Excerpt:    detected.Excerpt,
			ReasonCode: detected.ReasonCode,
			Redacted:   detected.Redacted,
			Truncated:  detected.Truncated,
		})
	}
	return targetScanResult{index: index, matches: matches, truncated: truncated}
}

// logScanByteBudget bounds the raw payload shared by every concurrent log
// reader in one scan. Reads reserve capacity before touching the upstream
// stream and refund only bytes the stream did not return, so concurrent
// readers can never overrun the aggregate ceiling.
type logScanByteBudget struct {
	mu        sync.Mutex
	remaining int64
	inFlight  int64
	exhausted bool
	done      chan struct{}
	changed   chan struct{}
}

func newLogScanByteBudget(maximum int64) *logScanByteBudget {
	return &logScanByteBudget{
		remaining: maximum,
		done:      make(chan struct{}),
		changed:   make(chan struct{}),
	}
}

func (budget *logScanByteBudget) Done() <-chan struct{} {
	return budget.done
}

func (budget *logScanByteBudget) Exhausted() bool {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.exhausted
}

func (budget *logScanByteBudget) reserve(ctx context.Context, maximum int64) (int64, error) {
	for {
		budget.mu.Lock()
		if budget.exhausted {
			budget.mu.Unlock()
			return 0, errLogScanByteBudgetExhausted
		}
		if budget.remaining > 0 {
			reserved := maximum
			if budget.remaining < reserved {
				reserved = budget.remaining
			}
			budget.remaining -= reserved
			budget.inFlight += reserved
			budget.mu.Unlock()
			return reserved, nil
		}
		changed := budget.changed
		budget.mu.Unlock()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-budget.done:
			return 0, errLogScanByteBudgetExhausted
		case <-changed:
		}
	}
}

func (budget *logScanByteBudget) complete(reserved, consumed int64) bool {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if consumed < 0 {
		consumed = 0
	}
	if consumed > reserved {
		consumed = reserved
	}
	budget.inFlight -= reserved
	budget.remaining += reserved - consumed
	if budget.remaining == 0 && budget.inFlight == 0 {
		if !budget.exhausted {
			budget.exhausted = true
			close(budget.done)
			close(budget.changed)
		}
		return true
	}
	close(budget.changed)
	budget.changed = make(chan struct{})
	return false
}

type logScanBudgetReader struct {
	ctx    context.Context
	source io.Reader
	budget *logScanByteBudget
}

func (reader *logScanBudgetReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	reserved, err := reader.budget.reserve(reader.ctx, int64(len(destination)))
	if err != nil {
		return 0, err
	}
	read, readErr := reader.source.Read(destination[:int(reserved)])
	exhausted := reader.budget.complete(reserved, int64(read))
	if exhausted {
		return read, errLogScanByteBudgetExhausted
	}
	return read, readErr
}

type boundedLine struct {
	value     []byte
	truncated bool
}

func readBoundedLogLines(ctx context.Context, source io.Reader, budget LogBudget, maximumLines int) ([]boundedLine, bool, error) {
	// Read at most the analyzable container budget. Reaching the exact ceiling
	// is conservatively reported as truncated, without inspecting one extra
	// payload byte merely to prove that upstream had more data.
	limited := &io.LimitedReader{R: source, N: budget.MaxContainerBytes}
	reader := bufio.NewReaderSize(limited, int(budget.MaxLineBytes)+1)
	result := make([]boundedLine, 0, minInt(maximumLines, 256))
	truncated := false
	for len(result) < maximumLines {
		if err := ctx.Err(); err != nil {
			return result, truncated, err
		}
		line, lineTruncated, eof, err := readBoundedLine(reader, int(budget.MaxLineBytes))
		if len(line) > 0 || !eof {
			copy := append([]byte(nil), line...)
			result = append(result, boundedLine{value: copy, truncated: lineTruncated})
		}
		if lineTruncated {
			truncated = true
		}
		if errors.Is(err, errLogScanByteBudgetExhausted) {
			return result, true, nil
		}
		if err != nil {
			return result, truncated, err
		}
		if eof || limited.N == 0 {
			break
		}
	}
	if len(result) == maximumLines {
		if _, err := reader.Peek(1); err == nil {
			truncated = true
		} else if errors.Is(err, errLogScanByteBudgetExhausted) {
			truncated = true
		} else if !errors.Is(err, io.EOF) {
			return result, truncated, err
		}
	}
	if limited.N == 0 {
		truncated = true
	}
	return result, truncated, nil
}

func readBoundedLine(reader *bufio.Reader, maximum int) ([]byte, bool, bool, error) {
	line := make([]byte, 0, minInt(maximum, 4096))
	truncated := false
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > 0 && fragment[len(fragment)-1] == '\n' {
			fragment = fragment[:len(fragment)-1]
			if len(fragment) > 0 && fragment[len(fragment)-1] == '\r' {
				fragment = fragment[:len(fragment)-1]
			}
			before := len(line)
			line = appendAtMost(line, fragment, maximum)
			return line, truncated || before+len(fragment) > maximum, false, nil
		}
		before := len(line)
		line = appendAtMost(line, fragment, maximum)
		if before+len(fragment) > maximum || errors.Is(err, bufio.ErrBufferFull) {
			truncated = true
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, errLogScanByteBudgetExhausted):
			return line, true, true, err
		case errors.Is(err, io.EOF):
			return line, truncated, true, nil
		case err != nil:
			return line, truncated, false, err
		default:
			return line, truncated, false, nil
		}
	}
}

func appendAtMost(destination, source []byte, maximum int) []byte {
	remaining := maximum - len(destination)
	if remaining <= 0 {
		return destination
	}
	if len(source) > remaining {
		source = source[:remaining]
	}
	return append(destination, source...)
}

func closeReaderOnCancel(ctx context.Context, byteBudgetDone <-chan struct{}, reader io.Closer) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = reader.Close()
		case <-byteBudgetDone:
			_ = reader.Close()
		case <-done:
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

func targetNamespaces(targets []LogTarget) []string {
	values := make([]string, 0, len(targets))
	for _, target := range targets {
		values = append(values, target.Namespace)
	}
	return canonicalNamespaces(values)
}

func limitExplicitTargets(targets []LogTarget, maximumPods, maximumContainers int) ([]LogTarget, bool) {
	copy := append([]LogTarget(nil), targets...)
	sort.SliceStable(copy, func(left, right int) bool { return lessLogTarget(copy[left], copy[right]) })
	selected := make([]LogTarget, 0, minInt(len(copy), maximumContainers))
	pods := make(map[string]struct{})
	seen := make(map[logTargetKey]struct{})
	truncated := false
	for _, target := range copy {
		if target.Namespace == "" || target.Pod == "" || target.Container == "" {
			truncated = true
			continue
		}
		key := target.Namespace + "\x00" + target.Pod
		targetKey := logTargetKey{namespace: target.Namespace, pod: target.Pod, container: target.Container, previous: target.Previous}
		if _, exists := seen[targetKey]; exists {
			continue
		}
		if _, ok := pods[key]; !ok && len(pods) >= maximumPods {
			truncated = true
			continue
		}
		if len(selected) >= maximumContainers {
			truncated = true
			break
		}
		pods[key] = struct{}{}
		seen[targetKey] = struct{}{}
		selected = append(selected, target)
	}
	return selected, truncated
}

func addUniqueBlockError[T any](block *DashboardBlockDTO[T], namespace string, err error) {
	partial, _ := classifyPartialError(namespace, err)
	for _, existing := range block.Errors {
		if existing.Namespace == partial.Namespace && existing.Code == partial.Code {
			block.Complete = false
			return
		}
	}
	addBlockError(block, namespace, err)
}

func containsNamespaceError(errors []PartialError, namespace string) bool {
	for _, item := range errors {
		if item.Namespace == namespace || item.Namespace == "" {
			return true
		}
	}
	return false
}

func capSerializedMatches(values []LogMatchDTO, maximum int64) ([]LogMatchDTO, bool) {
	result := make([]LogMatchDTO, 0, len(values))
	used := int64(2) // opening and closing brackets of the JSON collection
	for _, value := range values {
		encoded, err := json.Marshal(value)
		separator := int64(0)
		if len(result) > 0 {
			separator = 1
		}
		if err != nil || int64(len(encoded))+separator > maximum-used {
			return result, true
		}
		used += int64(len(encoded)) + separator
		result = append(result, value)
	}
	return result, false
}
