package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

const (
	jsonAccept     = "application/json"
	metadataAccept = "application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1,application/json"
	protobufAccept = "application/vnd.kubernetes.protobuf,application/json"
)

type endpointResult struct {
	Name            string  `json:"name"`
	P50MS           float64 `json:"p50Ms"`
	P95MS           float64 `json:"p95Ms"`
	WireBytes       int     `json:"wireBytes"`
	DecodedBytes    int     `json:"decodedBytes"`
	ContentType     string  `json:"contentType"`
	ContentEncoding string  `json:"contentEncoding,omitempty"`
}

type rateResult struct {
	QPS        int     `json:"qps"`
	Burst      int     `json:"burst"`
	Requests   int     `json:"requests"`
	WallMS     float64 `json:"wallMs"`
	P95MS      float64 `json:"p95Ms"`
	ThrottleMS float64 `json:"clientThrottleMs"`
	HTTP429    int     `json:"http429"`
	Errors     int     `json:"errors"`
}

type aimdResult struct {
	Mode       string  `json:"mode"`
	Requests   int     `json:"requests"`
	HTTP429    int     `json:"http429"`
	WallMS     float64 `json:"wallMs"`
	FinalLimit int     `json:"finalLimit"`
	Increases  uint64  `json:"increases"`
	Reductions uint64  `json:"reductions"`
}

type prefetchResult struct {
	NaiveVisibleMS       float64 `json:"naiveVisibleMs"`
	PrioritizedVisibleMS float64 `json:"prioritizedVisibleMs"`
	Deferred             uint64  `json:"deferred"`
}

type report struct {
	GeneratedAt string           `json:"generatedAt"`
	Commit      string           `json:"commit"`
	TreeState   string           `json:"treeState"`
	GoVersion   string           `json:"goVersion"`
	OS          string           `json:"os"`
	Arch        string           `json:"arch"`
	CPUs        int              `json:"cpus"`
	GOMAXPROCS  int              `json:"gomaxprocs"`
	MemoryBytes int64            `json:"memoryBytes,omitempty"`
	Kubernetes  string           `json:"kubernetesVersion"`
	DatasetPods int              `json:"datasetPods"`
	Requests    int              `json:"repetitions"`
	Transport   []endpointResult `json:"transport"`
	RateLimits  []rateResult     `json:"rateLimits"`
	AIMD        []aimdResult     `json:"aimd"`
	Prefetch    prefetchResult   `json:"prefetch"`
	Notes       []string         `json:"notes"`
}

type requestSample struct {
	elapsed  time.Duration
	wire     int
	decoded  int
	payload  []byte
	content  string
	encoding string
}

func percentile(values []time.Duration, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyOf := append([]time.Duration(nil), values...)
	sort.Slice(copyOf, func(i, j int) bool { return copyOf[i] < copyOf[j] })
	index := int(float64(len(copyOf)-1)*quantile + 0.5)
	return float64(copyOf[index].Microseconds()) / 1000
}

func readPayload(response *http.Response) (wire int, payload []byte, err error) {
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, nil, err
	}
	wire = len(data)
	if response.Header.Get("Content-Encoding") != "gzip" {
		return wire, data, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return 0, nil, err
	}
	defer reader.Close()
	plain, err := io.ReadAll(reader)
	return wire, plain, err
}

func request(ctx context.Context, client *http.Client, endpoint, accept, encoding string) (requestSample, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return requestSample{}, err
	}
	req.Header.Set("Accept", accept)
	if encoding != "" {
		req.Header.Set("Accept-Encoding", encoding)
	}
	started := time.Now()
	response, err := client.Do(req)
	if err != nil {
		return requestSample{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return requestSample{}, fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	wire, payload, err := readPayload(response)
	return requestSample{
		elapsed: time.Since(started), wire: wire, decoded: len(payload), payload: payload,
		content: response.Header.Get("Content-Type"), encoding: response.Header.Get("Content-Encoding"),
	}, err
}

func benchmarkEndpoint(ctx context.Context, name string, client *http.Client, endpoint, accept, encoding string, repetitions int) (endpointResult, []byte, error) {
	for range 2 {
		if _, err := request(ctx, client, endpoint, accept, encoding); err != nil {
			return endpointResult{}, nil, err
		}
	}
	latencies := make([]time.Duration, 0, repetitions)
	var last requestSample
	for range repetitions {
		sample, err := request(ctx, client, endpoint, accept, encoding)
		if err != nil {
			return endpointResult{}, nil, err
		}
		latencies = append(latencies, sample.elapsed)
		last = sample
	}
	return endpointResult{
		Name: name, P50MS: percentile(latencies, 0.50), P95MS: percentile(latencies, 0.95),
		WireBytes: last.wire, DecodedBytes: last.decoded, ContentType: last.content, ContentEncoding: last.encoding,
	}, last.payload, nil
}

type measuredLimiter struct {
	flowcontrol.RateLimiter
	nanos atomic.Int64
}

func (limiter *measuredLimiter) Wait(ctx context.Context) error {
	started := time.Now()
	err := limiter.RateLimiter.Wait(ctx)
	limiter.nanos.Add(time.Since(started).Nanoseconds())
	return err
}

func benchmarkRate(ctx context.Context, base *rest.Config, qps, burst, requests int) rateResult {
	config := rest.CopyConfig(base)
	config.ContentType = jsonAccept
	config.AcceptContentTypes = jsonAccept
	config.DisableCompression = true
	limiter := &measuredLimiter{RateLimiter: flowcontrol.NewTokenBucketRateLimiter(float32(qps), burst)}
	config.RateLimiter = limiter
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return rateResult{QPS: qps, Burst: burst, Requests: requests, Errors: requests}
	}
	latencies := make([]time.Duration, requests)
	var tooMany, failures atomic.Int64
	started := time.Now()
	var group sync.WaitGroup
	for index := range requests {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			callStarted := time.Now()
			_, callErr := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 1})
			latencies[index] = time.Since(callStarted)
			if apierrors.IsTooManyRequests(callErr) {
				tooMany.Add(1)
			} else if callErr != nil {
				failures.Add(1)
			}
		}(index)
	}
	group.Wait()
	return rateResult{
		QPS: qps, Burst: burst, Requests: requests, WallMS: float64(time.Since(started).Microseconds()) / 1000,
		P95MS: percentile(latencies, 0.95), ThrottleMS: float64(limiter.nanos.Load()) / float64(time.Millisecond),
		HTTP429: int(tooMany.Load()), Errors: int(failures.Load()),
	}
}

func benchmarkAIMD(ctx context.Context, adaptive bool) aimdResult {
	var active atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		time.Sleep(8 * time.Millisecond)
		if current > 3 {
			response.WriteHeader(http.StatusTooManyRequests)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var scheduler *resources.RequestScheduler
	mode := "fixed-4"
	if adaptive {
		mode = "aimd-2-4-8"
		scheduler = resources.NewRequestSchedulerWithConfig(resources.SchedulerConfig{
			Minimum: 2, Initial: 4, Maximum: 8, Adaptive: true, HealthyThreshold: 8,
		}, nil)
	} else {
		scheduler = resources.NewRequestScheduler(4, nil)
	}
	const waves, perWave = 10, 4
	var tooMany atomic.Int64
	started := time.Now()
	for range waves {
		var group sync.WaitGroup
		for range perWave {
			group.Add(1)
			go func() {
				defer group.Done()
				release, err := scheduler.Acquire(ctx, resources.PriorityVisible)
				if err != nil {
					return
				}
				requestStarted := time.Now()
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
				response, err := http.DefaultClient.Do(req)
				throttled := err == nil && response.StatusCode == http.StatusTooManyRequests
				if response != nil {
					response.Body.Close()
				}
				if throttled {
					tooMany.Add(1)
				}
				scheduler.Observe(time.Since(requestStarted), throttled, false)
				release()
			}()
		}
		group.Wait()
	}
	stats := scheduler.Stats()
	return aimdResult{
		Mode: mode, Requests: waves * perWave, HTTP429: int(tooMany.Load()),
		WallMS: float64(time.Since(started).Microseconds()) / 1000, FinalLimit: stats.Limit,
		Increases: stats.Increases, Reductions: stats.Reductions,
	}
}

func benchmarkPrefetch(ctx context.Context) prefetchResult {
	const hold = 40 * time.Millisecond
	naive := make(chan struct{}, 4)
	for range 4 {
		naive <- struct{}{}
	}
	go func() {
		time.Sleep(hold)
		<-naive
	}()
	started := time.Now()
	naive <- struct{}{}
	naiveElapsed := time.Since(started)

	scheduler := resources.NewRequestScheduler(4, nil)
	releases := make([]func(), 0, 2)
	for range 2 {
		release, _ := scheduler.Acquire(ctx, resources.PriorityLikelyNext)
		releases = append(releases, release)
	}
	_, _ = scheduler.Acquire(ctx, resources.PriorityLikelyNext)
	started = time.Now()
	visible, _ := scheduler.Acquire(ctx, resources.PriorityVisible)
	prioritizedElapsed := time.Since(started)
	visible()
	for _, release := range releases {
		release()
	}
	return prefetchResult{
		NaiveVisibleMS:       float64(naiveElapsed.Microseconds()) / 1000,
		PrioritizedVisibleMS: float64(prioritizedElapsed.Microseconds()) / 1000,
		Deferred:             scheduler.Stats().Deferred,
	}
}

func benchmarkShapedRemote(payload []byte, repetitions int) (endpointResult, endpointResult, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(payload); err != nil {
		return endpointResult{}, endpointResult{}, err
	}
	if err := writer.Close(); err != nil {
		return endpointResult{}, endpointResult{}, err
	}
	serve := func(body []byte, encoding string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			time.Sleep(30*time.Millisecond + time.Duration(len(body))*time.Second/(10*1024*1024/8))
			response.Header().Set("Content-Type", jsonAccept)
			if encoding != "" {
				response.Header().Set("Content-Encoding", encoding)
			}
			_, _ = response.Write(body)
		}))
	}
	plainServer, gzipServer := serve(payload, ""), serve(compressed.Bytes(), "gzip")
	defer plainServer.Close()
	defer gzipServer.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	plain, _, err := benchmarkEndpoint(context.Background(), "remote-shaped-json", client, plainServer.URL, jsonAccept, "", repetitions)
	if err != nil {
		return endpointResult{}, endpointResult{}, err
	}
	compressedResult, _, err := benchmarkEndpoint(context.Background(), "remote-shaped-gzip", client, gzipServer.URL, jsonAccept, "", repetitions)
	return plain, compressedResult, err
}

func gitIdentity(commitOverride string) (string, string) {
	commit := strings.TrimSpace(commitOverride)
	if commit == "" {
		if output, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
			commit = strings.TrimSpace(string(output))
		}
	}
	state := "unknown"
	if output, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		state = "clean"
		if len(bytes.TrimSpace(output)) > 0 {
			state = "dirty"
		}
	}
	return commit, state
}

func hostMemoryBytes() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kilobytes, parseErr := strconv.ParseInt(fields[1], 10, 64)
			if parseErr == nil {
				return kilobytes * 1024
			}
		}
	}
	return 0
}

func run(ctx context.Context, repetitions int, commitOverride string) (report, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	config, err := loader.ClientConfig()
	if err != nil {
		return report{}, err
	}
	config.DisableCompression = true
	client, err := rest.HTTPClientFor(config)
	if err != nil {
		return report{}, err
	}
	endpoint := config.Host + "/api/v1/pods?limit=500"
	transport := make([]endpointResult, 0, 7)
	jsonResult, payload, err := benchmarkEndpoint(ctx, "kind-json", client, endpoint, jsonAccept, "", repetitions)
	if err != nil {
		return report{}, err
	}
	transport = append(transport, jsonResult)
	for _, candidate := range []struct{ name, accept, encoding string }{
		{"kind-partial-metadata", metadataAccept, ""},
		{"kind-protobuf", protobufAccept, ""},
		{"kind-gzip", jsonAccept, "gzip"},
	} {
		result, _, measureErr := benchmarkEndpoint(ctx, candidate.name, client, endpoint, candidate.accept, candidate.encoding, repetitions)
		if measureErr != nil {
			return report{}, measureErr
		}
		transport = append(transport, result)
	}
	remoteJSON, remoteGzip, err := benchmarkShapedRemote(payload, repetitions)
	if err != nil {
		return report{}, err
	}
	transport = append(transport, remoteJSON, remoteGzip)

	discovery, err := kubernetes.NewForConfig(config)
	if err != nil {
		return report{}, err
	}
	version, err := discovery.Discovery().ServerVersion()
	if err != nil {
		return report{}, err
	}
	pods, err := discovery.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return report{}, err
	}
	rates := []rateResult{
		benchmarkRate(ctx, config, 5, 10, 40),
		benchmarkRate(ctx, config, 10, 20, 40),
		benchmarkRate(ctx, config, 20, 40, 40),
	}
	commit, treeState := gitIdentity(commitOverride)
	return report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339), Kubernetes: version.GitVersion,
		Commit: commit, TreeState: treeState, GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH,
		CPUs: runtime.NumCPU(), GOMAXPROCS: runtime.GOMAXPROCS(0), MemoryBytes: hostMemoryBytes(),
		DatasetPods: len(pods.Items), Requests: repetitions, Transport: transport, RateLimits: rates,
		AIMD:     []aimdResult{benchmarkAIMD(ctx, false), benchmarkAIMD(ctx, true)},
		Prefetch: benchmarkPrefetch(ctx),
		Notes: []string{
			"kind-* measurements use the current real Kubernetes context and contain no object identities",
			"remote-shaped-* uses the real JSON payload through a modeled 30 ms RTT / 10 Mbit/s link; it is not a real remote cluster",
			"AIMD uses a deterministic local endpoint that returns 429 above three concurrent requests",
		},
	}, nil
}

func main() {
	repetitions := flag.Int("repetitions", 12, "measured repetitions per transport variant")
	output := flag.String("output", "", "optional JSON output path")
	commit := flag.String("commit", "", "commit identity for the measured code")
	flag.Parse()
	if *repetitions < 3 || *repetitions > 100 {
		fmt.Fprintln(os.Stderr, "repetitions must be between 3 and 100")
		os.Exit(2)
	}
	result, err := run(context.Background(), *repetitions, *commit)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "protocol benchmark canceled")
		} else {
			fmt.Fprintln(os.Stderr, "protocol benchmark failed")
		}
		os.Exit(1)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "protocol benchmark encoding failed")
		os.Exit(1)
	}
	encoded = append(encoded, '\n')
	if *output != "" {
		if err := os.WriteFile(*output, encoded, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "protocol benchmark output failed")
			os.Exit(1)
		}
	}
	_, _ = os.Stdout.Write(encoded)
}
