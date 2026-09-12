// Command benchmark runs the deterministic synthetic half of the Phase 0
// performance laboratory. It exercises the production collector and cursor
// store without claiming that injected faults or 200-origin fan-out came from
// a real Kind cluster. Real-cluster evidence remains a separate harness gate.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

const schemaVersion = "kubepeep-performance-baseline/v1"

type profile string

type fault string

const (
	profileGlobal      profile = "global"
	profileNamespace   profile = "namespace-only"
	profileMixed       profile = "mixed"
	profileUnavailable profile = "authorization-unavailable"
	profileInternal    profile = "internal-synthetic-origins"

	faultNone    fault = "none"
	fault429     fault = "429"
	fault410     fault = "410"
	faultTimeout fault = "timeout"
	faultReset   fault = "reset"
)

type scenario struct {
	ID               string  `json:"id"`
	Profile          profile `json:"profile"`
	Namespaces       int     `json:"namespaces"`
	PodsPerNamespace int     `json:"pods_per_namespace"`
	PageSize         int     `json:"page_size"`
	LatencyMS        int     `json:"latency_ms"`
	Fault            fault   `json:"fault"`
	Watch            bool    `json:"watch"`
	ContractScope    string  `json:"contract_scope"`
}

type metadata struct {
	SchemaVersion  string `json:"schema_version"`
	GeneratedAt    string `json:"generated_at"`
	Commit         string `json:"commit"`
	WorkingTree    string `json:"working_tree"`
	Suite          string `json:"suite"`
	Mode           string `json:"mode"`
	WarmupRuns     int    `json:"warmup_runs"`
	Repetitions    int    `json:"repetitions"`
	GoVersion      string `json:"go_version"`
	GOOS           string `json:"goos"`
	GOARCH         string `json:"goarch"`
	CPUCount       int    `json:"cpu_count"`
	GOMAXPROCS     int    `json:"gomaxprocs"`
	HostMemoryByte uint64 `json:"host_memory_bytes,omitempty"`
}

type distribution struct {
	Unit  string  `json:"unit"`
	Count int     `json:"count"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type metrics struct {
	TTFB                distribution `json:"ttfb"`
	FirstRow            distribution `json:"first_row"`
	FullPage            distribution `json:"full_page"`
	Requests            distribution `json:"requests"`
	BytesReceived       distribution `json:"bytes_received"`
	ItemsReceived       distribution `json:"items_received"`
	ItemsReturned       distribution `json:"items_returned"`
	OverfetchRatio      distribution `json:"overfetch_ratio"`
	TotalAllocDelta     distribution `json:"total_alloc_delta"`
	HeapAllocDelta      distribution `json:"heap_alloc_delta"`
	PeakGoroutineDelta  distribution `json:"peak_goroutine_delta"`
	FinalGoroutineDelta distribution `json:"final_goroutine_delta"`
	CursorBytes         distribution `json:"cursor_bytes"`
	CursorEntries       distribution `json:"cursor_entries"`
	PartialFailures     distribution `json:"partial_failures"`
	WatchEvents         distribution `json:"watch_events"`
	WatchLag            distribution `json:"watch_lag"`
}

type scenarioReport struct {
	Scenario  scenario       `json:"scenario"`
	Status    string         `json:"status"`
	ErrorCode string         `json:"error_code,omitempty"`
	Outcomes  map[string]int `json:"outcomes"`
	Samples   int            `json:"samples"`
	Metrics   metrics        `json:"metrics"`
}

type report struct {
	Metadata metadata         `json:"metadata"`
	Protocol []string         `json:"protocol"`
	Results  []scenarioReport `json:"results"`
}

type sample struct {
	ttfbMS              *float64
	firstRowMS          *float64
	fullPageMS          float64
	requests            float64
	bytesReceived       float64
	itemsReceived       float64
	itemsReturned       float64
	overfetchRatio      float64
	totalAllocDelta     float64
	heapAllocDelta      float64
	peakGoroutineDelta  float64
	finalGoroutineDelta float64
	cursorBytes         float64
	cursorEntries       float64
	partialFailures     float64
	watchEvents         float64
	watchLagMS          *float64
	errorCode           resources.ErrorCode
}

type syntheticAuthorizer struct {
	profile profile
}

func (authorizer syntheticAuthorizer) Check(ctx context.Context, key authorization.Key) authorization.Capability {
	decision := authorization.DecisionAllowed
	if ctx.Err() != nil {
		decision = authorization.DecisionUnknown
	} else {
		switch authorizer.profile {
		case profileUnavailable:
			decision = authorization.DecisionUnknown
		case profileMixed:
			index := namespaceIndex(key.Namespace)
			switch index % 5 {
			case 3:
				decision = authorization.DecisionDenied
			case 4:
				decision = authorization.DecisionUnknown
			}
		}
	}
	return authorization.Capability{Decision: decision}
}

type listerSnapshot struct {
	requests      int
	bytesReceived int64
	itemsReceived int
	firstResponse time.Duration
	peakGoroutine int
}

type syntheticLister struct {
	scenario scenario
	started  time.Time
	mu       sync.Mutex
	stats    listerSnapshot
}

func (lister *syntheticLister) ListPage(ctx context.Context, request resources.PageRequest) (resources.OriginPage[resources.PodDTO], error) {
	lister.mu.Lock()
	lister.stats.requests++
	lister.stats.peakGoroutine = max(lister.stats.peakGoroutine, runtime.NumGoroutine())
	lister.mu.Unlock()

	if lister.scenario.LatencyMS > 0 {
		timer := time.NewTimer(time.Duration(lister.scenario.LatencyMS) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			lister.observeResponse(nil)
			return resources.OriginPage[resources.PodDTO]{Origin: request.Origin}, ctx.Err()
		case <-timer.C:
		}
	}
	if lister.injectsFault(request.Origin) {
		lister.observeResponse(nil)
		switch lister.scenario.Fault {
		case fault429:
			return resources.OriginPage[resources.PodDTO]{Origin: request.Origin}, errors.New("synthetic HTTP 429")
		case fault410:
			return resources.OriginPage[resources.PodDTO]{Origin: request.Origin}, resources.ErrResourceExpired
		case faultTimeout:
			return resources.OriginPage[resources.PodDTO]{Origin: request.Origin}, context.DeadlineExceeded
		case faultReset:
			return resources.OriginPage[resources.PodDTO]{Origin: request.Origin}, syscall.ECONNRESET
		}
	}

	offset, err := parseContinue(request.Continue)
	if err != nil {
		lister.observeResponse(nil)
		return resources.OriginPage[resources.PodDTO]{Origin: request.Origin}, err
	}
	total := lister.scenario.PodsPerNamespace
	if request.Origin.Namespace == "" {
		total *= lister.scenario.Namespaces
	}
	end := min(offset+int(request.Limit), total)
	items := make([]resources.PodDTO, 0, max(0, end-offset))
	for index := offset; index < end; index++ {
		namespace := request.Origin.Namespace
		podIndex := index
		if namespace == "" {
			namespace = fmt.Sprintf("bench-ns-%04d", index/lister.scenario.PodsPerNamespace)
			podIndex = index % lister.scenario.PodsPerNamespace
		}
		items = append(items, resources.PodDTO{
			Namespace: namespace,
			Name:      fmt.Sprintf("pod-%06d", podIndex),
			Status:    "Running",
			Ready:     resources.ReadyDTO{Current: 1, Desired: 1},
		})
	}
	continuation := ""
	if end < total {
		continuation = strconv.Itoa(end)
	}
	lister.observeResponse(items)
	return resources.OriginPage[resources.PodDTO]{
		Origin:          request.Origin,
		Items:           items,
		Continue:        continuation,
		ResourceVersion: "synthetic-rv",
	}, nil
}

func (lister *syntheticLister) injectsFault(origin resources.Origin) bool {
	if lister.scenario.Fault == faultNone {
		return false
	}
	return origin.Namespace == "" || origin.Namespace == "bench-ns-0000"
}

func (lister *syntheticLister) observeResponse(items []resources.PodDTO) {
	encoded, _ := json.Marshal(items)
	lister.mu.Lock()
	defer lister.mu.Unlock()
	lister.stats.bytesReceived += int64(len(encoded))
	lister.stats.itemsReceived += len(items)
	elapsed := time.Since(lister.started)
	if lister.stats.firstResponse == 0 || elapsed < lister.stats.firstResponse {
		lister.stats.firstResponse = elapsed
	}
	lister.stats.peakGoroutine = max(lister.stats.peakGoroutine, runtime.NumGoroutine())
}

func (lister *syntheticLister) snapshot() listerSnapshot {
	lister.mu.Lock()
	defer lister.mu.Unlock()
	return lister.stats
}

type syntheticWatch struct {
	mu     sync.Mutex
	events int
	lags   []float64
	stop   chan struct{}
	done   chan struct{}
}

func startSyntheticWatch(enabled bool) *syntheticWatch {
	if !enabled {
		return nil
	}
	watch := &syntheticWatch{stop: make(chan struct{}), done: make(chan struct{})}
	watch.record(time.Now())
	go func() {
		defer close(watch.done)
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case emitted := <-ticker.C:
				watch.record(emitted)
			case <-watch.stop:
				return
			}
		}
	}()
	return watch
}

func (watch *syntheticWatch) record(emitted time.Time) {
	watch.mu.Lock()
	defer watch.mu.Unlock()
	watch.events++
	watch.lags = append(watch.lags, max(0, float64(time.Since(emitted).Microseconds())/1000))
}

func (watch *syntheticWatch) finish() (float64, *float64) {
	if watch == nil {
		return 0, nil
	}
	close(watch.stop)
	<-watch.done
	watch.mu.Lock()
	defer watch.mu.Unlock()
	value := percentile(watch.lags, 0.95)
	return float64(watch.events), &value
}

func main() {
	suite := flag.String("suite", "representative", "scenario suite: representative or matrix")
	output := flag.String("output", "-", "JSON output path, or - for stdout")
	commit := flag.String("commit", envOr("KUBEPEEP_BENCHMARK_COMMIT", "unknown"), "source commit measured")
	workingTree := flag.String("working-tree", envOr("KUBEPEEP_BENCHMARK_WORKTREE", "unknown"), "clean or dirty")
	warmup := flag.Int("warmup", 1, "discarded warmup runs per scenario")
	repetitions := flag.Int("repetitions", 5, "measured runs per scenario")
	flag.Parse()
	if *warmup < 0 || *repetitions < 1 || *repetitions > 100 {
		fatalf("warmup must be >= 0 and repetitions must be between 1 and 100")
	}
	scenarios, err := scenariosFor(*suite)
	if err != nil {
		fatalf("%v", err)
	}
	result := runReport(*suite, *commit, *workingTree, *warmup, *repetitions, scenarios)
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatalf("encode report: %v", err)
	}
	encoded = append(encoded, '\n')
	if *output == "-" {
		if _, err := os.Stdout.Write(encoded); err != nil {
			fatalf("write report: %v", err)
		}
	} else if err := os.WriteFile(*output, encoded, 0o600); err != nil {
		fatalf("write report: %v", err)
	}
	if err := reportError(result); err != nil {
		fatalf("%v", err)
	}
}

func runReport(suite, commit, workingTree string, warmup, repetitions int, scenarios []scenario) report {
	results := make([]scenarioReport, 0, len(scenarios))
	for _, candidate := range scenarios {
		for index := 0; index < warmup; index++ {
			_ = measure(candidate)
		}
		samples := make([]sample, repetitions)
		for index := range samples {
			samples[index] = measure(candidate)
		}
		results = append(results, summarize(candidate, samples))
	}
	return report{
		Metadata: metadata{
			SchemaVersion: schemaVersion, GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Commit: commit, WorkingTree: workingTree, Suite: suite, Mode: "synthetic",
			WarmupRuns: warmup, Repetitions: repetitions, GoVersion: runtime.Version(),
			GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CPUCount: runtime.NumCPU(),
			GOMAXPROCS: runtime.GOMAXPROCS(0), HostMemoryByte: hostMemoryBytes(),
		},
		Protocol: []string{
			"Each scenario uses the production resources.Collect and api.CursorStore paths.",
			"TTFB is the earliest synthetic upstream LIST response; first_row is when Collect returns a non-empty page; full_page is Collect completion.",
			"The watch dimension is an explicitly synthetic scheduler probe and never emits production telemetry.",
			"The global 200-namespace case uses one authorized all-namespaces origin; restricted fan-out remains capped at 100; 200 direct origins are internal stress only.",
			"Output contains counts, closed error codes and environment resources only; resource identities and payloads are omitted.",
		},
		Results: results,
	}
}

func measure(candidate scenario) sample {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	goroutinesBefore := runtime.NumGoroutine()
	started := time.Now()
	origins, setupErr := originsFor(candidate)
	if setupErr != nil {
		elapsed := milliseconds(time.Since(started))
		runtime.ReadMemStats(&after)
		return sample{
			fullPageMS: elapsed, totalAllocDelta: float64(after.TotalAlloc - before.TotalAlloc),
			heapAllocDelta:      float64(int64(after.HeapAlloc) - int64(before.HeapAlloc)),
			finalGoroutineDelta: float64(runtime.NumGoroutine() - goroutinesBefore), errorCode: resources.ErrorCodeOf(setupErr),
		}
	}
	watch := startSyntheticWatch(candidate.Watch)
	lister := &syntheticLister{scenario: candidate, started: started}
	result, err := resources.Collect(context.Background(), resources.CollectionRequest[resources.PodDTO]{
		Selection: resources.Selection{Generation: "benchmark-generation", Context: "benchmark-context", Scope: "benchmark-scope"},
		Options:   resources.ListOptions{Limit: candidate.PageSize}, Origins: origins,
		Lister: lister, Authorizer: syntheticAuthorizer{profile: candidate.Profile}, Less: podLess,
		Timeout: 5 * time.Second, RequestedNamespaces: candidate.Namespaces,
	})
	complete := time.Since(started)
	watchEvents, watchLag := watch.finish()
	stats := lister.snapshot()
	peak := max(stats.peakGoroutine, goroutinesBefore)
	cursorBytes := float64(0)
	cursorEntries := float64(0)
	if result.Cursor != nil {
		store := api.NewCursorStore(nil)
		if reference, storeErr := store.Put(result.Cursor); storeErr == nil {
			cursorBytes = float64(store.Bytes())
			cursorEntries = float64(store.Len())
			var decoded resources.CompositeCursor[resources.PodDTO]
			_ = store.Get(reference, &decoded)
			store.Delete(reference)
		}
	}
	runtime.ReadMemStats(&after)
	var ttfb *float64
	if stats.firstResponse > 0 {
		value := milliseconds(stats.firstResponse)
		ttfb = &value
	}
	var firstRow *float64
	if len(result.Items) > 0 {
		value := milliseconds(complete)
		firstRow = &value
	}
	overfetch := float64(0)
	if len(result.Items) > 0 {
		overfetch = float64(stats.itemsReceived) / float64(len(result.Items))
	}
	errorCode := resources.ErrorCode("")
	if err != nil {
		errorCode = resources.ErrorCodeOf(err)
	}
	return sample{
		ttfbMS: ttfb, firstRowMS: firstRow, fullPageMS: milliseconds(complete),
		requests: float64(stats.requests), bytesReceived: float64(stats.bytesReceived),
		itemsReceived: float64(stats.itemsReceived), itemsReturned: float64(len(result.Items)), overfetchRatio: overfetch,
		totalAllocDelta:     float64(after.TotalAlloc - before.TotalAlloc),
		heapAllocDelta:      float64(int64(after.HeapAlloc) - int64(before.HeapAlloc)),
		peakGoroutineDelta:  float64(peak - goroutinesBefore),
		finalGoroutineDelta: float64(runtime.NumGoroutine() - goroutinesBefore),
		cursorBytes:         cursorBytes, cursorEntries: cursorEntries,
		partialFailures: float64(len(result.Coverage.Failed)), watchEvents: watchEvents, watchLagMS: watchLag,
		errorCode: errorCode,
	}
}

func summarize(candidate scenario, samples []sample) scenarioReport {
	report := scenarioReport{Scenario: candidate, Samples: len(samples), Outcomes: make(map[string]int)}
	codes := make(map[resources.ErrorCode]int)
	for _, measured := range samples {
		codes[measured.errorCode]++
		report.Outcomes[outcomeName(measured.errorCode)]++
	}

	expected := expectedError(candidate)
	if len(codes) != 1 {
		report.Status = "unstable"
	} else {
		var code resources.ErrorCode
		for measuredCode := range codes {
			code = measuredCode
		}
		if code != "" {
			report.ErrorCode = string(code)
		}
		switch {
		case code == "" && expected == "":
			report.Status = "ok"
		case code == expected:
			report.Status = "expected_error"
		default:
			report.Status = "unexpected_error"
		}
	}
	report.Metrics = metrics{
		TTFB:                distributionOf("milliseconds", pointerValues(samples, func(value sample) *float64 { return value.ttfbMS })),
		FirstRow:            distributionOf("milliseconds", pointerValues(samples, func(value sample) *float64 { return value.firstRowMS })),
		FullPage:            distributionOf("milliseconds", values(samples, func(value sample) float64 { return value.fullPageMS })),
		Requests:            distributionOf("requests", values(samples, func(value sample) float64 { return value.requests })),
		BytesReceived:       distributionOf("bytes", values(samples, func(value sample) float64 { return value.bytesReceived })),
		ItemsReceived:       distributionOf("items", values(samples, func(value sample) float64 { return value.itemsReceived })),
		ItemsReturned:       distributionOf("items", values(samples, func(value sample) float64 { return value.itemsReturned })),
		OverfetchRatio:      distributionOf("ratio", values(samples, func(value sample) float64 { return value.overfetchRatio })),
		TotalAllocDelta:     distributionOf("bytes", values(samples, func(value sample) float64 { return value.totalAllocDelta })),
		HeapAllocDelta:      distributionOf("bytes", values(samples, func(value sample) float64 { return value.heapAllocDelta })),
		PeakGoroutineDelta:  distributionOf("goroutines", values(samples, func(value sample) float64 { return value.peakGoroutineDelta })),
		FinalGoroutineDelta: distributionOf("goroutines", values(samples, func(value sample) float64 { return value.finalGoroutineDelta })),
		CursorBytes:         distributionOf("bytes", values(samples, func(value sample) float64 { return value.cursorBytes })),
		CursorEntries:       distributionOf("entries", values(samples, func(value sample) float64 { return value.cursorEntries })),
		PartialFailures:     distributionOf("origins", values(samples, func(value sample) float64 { return value.partialFailures })),
		WatchEvents:         distributionOf("events", values(samples, func(value sample) float64 { return value.watchEvents })),
		WatchLag:            distributionOf("milliseconds", pointerValues(samples, func(value sample) *float64 { return value.watchLagMS })),
	}
	return report
}

func originsFor(candidate scenario) ([]resources.Origin, error) {
	if candidate.Profile == profileGlobal {
		return resources.GlobalOriginsFor(resources.CollectionPods, nil)
	}
	namespaces := make([]string, candidate.Namespaces)
	for index := range namespaces {
		namespaces[index] = fmt.Sprintf("bench-ns-%04d", index)
	}
	if candidate.Profile != profileInternal {
		return resources.OriginsFor(resources.CollectionPods, namespaces, nil)
	}
	origins := make([]resources.Origin, len(namespaces))
	for index, namespace := range namespaces {
		origins[index] = resources.Origin{Namespace: namespace, Version: "v1", Resource: "pods"}
	}
	return origins, nil
}

func podLess(left, right resources.PodDTO) bool {
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	return left.Name < right.Name
}

func expectedError(candidate scenario) resources.ErrorCode {
	if candidate.Profile != profileGlobal && candidate.Profile != profileInternal && candidate.Namespaces > resources.MaximumNamespaces {
		return resources.CodeValidationFailed
	}
	if candidate.Profile == profileUnavailable {
		return resources.CodeAuthorizationUnavailable
	}
	if candidate.Profile == profileGlobal {
		switch candidate.Fault {
		case fault410:
			return resources.CodeCursorExpired
		case faultTimeout:
			return resources.CodeUpstreamTimeout
		case fault429, faultReset:
			return resources.CodeClusterUnavailable
		}
	}
	return ""
}

func scenariosFor(name string) ([]scenario, error) {
	representative := []scenario{
		newScenario("global-200", profileGlobal, 200, 100, 100, 0, faultNone, true),
		newScenario("namespace-100", profileNamespace, 100, 100, 100, 20, faultNone, false),
		newScenario("mixed-100", profileMixed, 100, 100, 50, 50, faultNone, true),
		newScenario("authorization-unavailable", profileUnavailable, 10, 10, 25, 0, faultNone, false),
		newScenario("restricted-200-rejected", profileNamespace, 200, 10, 100, 0, faultNone, false),
		newScenario("internal-200-origins", profileInternal, 200, 10, 100, 0, faultNone, false),
		newScenario("global-429", profileGlobal, 25, 500, 100, 100, fault429, false),
		newScenario("global-410", profileGlobal, 25, 500, 100, 200, fault410, true),
		newScenario("global-timeout", profileGlobal, 10, 100, 50, 50, faultTimeout, false),
		newScenario("global-reset", profileGlobal, 10, 100, 50, 20, faultReset, false),
	}
	if name == "representative" {
		return representative, nil
	}
	if name != "matrix" {
		return nil, fmt.Errorf("unknown suite %q", name)
	}
	matrix := append([]scenario{}, representative...)
	namespaceCounts := []int{1, 10, 25, 50, 100, 200}
	podCounts := []int{10, 50, 100, 250, 500}
	pageSizes := []int{25, 50, 100}
	profiles := []profile{profileGlobal, profileNamespace, profileMixed, profileUnavailable}
	for profileIndex, value := range profiles {
		for index, namespaces := range namespaceCounts {
			matrix = append(matrix, newScenario(
				fmt.Sprintf("matrix-%s-%03d", value, namespaces), value, namespaces,
				podCounts[(profileIndex+index)%len(podCounts)], pageSizes[index%len(pageSizes)], 0, faultNone, index%2 == 0,
			))
		}
	}
	for _, latency := range []int{0, 20, 50, 100, 200} {
		matrix = append(matrix, newScenario(fmt.Sprintf("latency-%03d", latency), profileGlobal, 50, 100, 100, latency, faultNone, latency%40 == 0))
	}
	for _, injected := range []fault{fault429, fault410, faultTimeout, faultReset} {
		matrix = append(matrix, newScenario("fault-"+string(injected), profileGlobal, 25, 100, 50, 20, injected, true))
	}
	for _, origins := range []int{10, 50, 100, 200} {
		matrix = append(matrix, newScenario(fmt.Sprintf("internal-origins-%03d", origins), profileInternal, origins, 10, 100, 0, faultNone, false))
	}
	return deduplicate(matrix), nil
}

func newScenario(id string, selected profile, namespaces, pods, pageSize, latency int, injected fault, watch bool) scenario {
	scope := "public-restricted"
	if selected == profileGlobal {
		scope = "public-global-authorized"
	} else if selected == profileInternal {
		scope = "internal-synthetic-only"
	} else if namespaces > resources.MaximumNamespaces {
		scope = "public-limit-rejection"
	}
	return scenario{ID: id, Profile: selected, Namespaces: namespaces, PodsPerNamespace: pods, PageSize: pageSize, LatencyMS: latency, Fault: injected, Watch: watch, ContractScope: scope}
}

func deduplicate(values []scenario) []scenario {
	seen := make(map[string]struct{}, len(values))
	result := make([]scenario, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value.ID]; exists {
			continue
		}
		seen[value.ID] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseContinue(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid synthetic continue token")
	}
	return parsed, nil
}

func namespaceIndex(namespace string) int {
	position := strings.LastIndexByte(namespace, '-')
	if position < 0 {
		return 0
	}
	value, _ := strconv.Atoi(namespace[position+1:])
	return value
}

func values(samples []sample, selectValue func(sample) float64) []float64 {
	result := make([]float64, len(samples))
	for index, measured := range samples {
		result[index] = selectValue(measured)
	}
	return result
}

func pointerValues(samples []sample, selectValue func(sample) *float64) []float64 {
	result := make([]float64, 0, len(samples))
	for _, measured := range samples {
		if value := selectValue(measured); value != nil {
			result = append(result, *value)
		}
	}
	return result
}

func distributionOf(unit string, values []float64) distribution {
	if len(values) == 0 {
		return distribution{Unit: unit}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return distribution{Unit: unit, Count: len(sorted), P50: percentile(sorted, 0.50), P95: percentile(sorted, 0.95), Min: sorted[0], Max: sorted[len(sorted)-1]}
}

func percentile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	position := quantile * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return round(sorted[lower])
	}
	weight := position - float64(lower)
	return round(sorted[lower]*(1-weight) + sorted[upper]*weight)
}

func outcomeName(code resources.ErrorCode) string {
	if code == "" {
		return "success"
	}
	return string(code)
}

func reportError(result report) error {
	failures := make([]string, 0)
	for _, measured := range result.Results {
		if measured.Status != "ok" && measured.Status != "expected_error" {
			failures = append(failures, fmt.Sprintf("%s=%s", measured.Scenario.ID, measured.Status))
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return fmt.Errorf("benchmark has unexpected or unstable outcomes: %s", strings.Join(failures, ", "))
}

func round(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func milliseconds(value time.Duration) float64 {
	return round(float64(value.Microseconds()) / 1000)
}

func hostMemoryBytes() uint64 {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "MemTotal:" && fields[2] == "kB" {
			value, parseErr := strconv.ParseUint(fields[1], 10, 64)
			if parseErr == nil {
				return value * 1024
			}
		}
	}
	return 0
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fatalf(format string, values ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "benchmark: "+format+"\n", values...)
	os.Exit(2)
}

var _ resources.OriginLister[resources.PodDTO] = (*syntheticLister)(nil)
var _ resources.AuthorizationChecker = syntheticAuthorizer{}
