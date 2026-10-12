// Package releases checks for published updates without downloading or installing them.
package releases

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver/v4"
)

const CheckInterval = 2 * time.Hour
const latestURL = "https://api.github.com/repos/fvmoraes/kubepeep/releases/latest"

type Result struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion,omitempty"`
	Available      bool   `json:"available"`
}

type Checker struct {
	mu        sync.Mutex
	client    *http.Client
	current   string
	nextCheck time.Time
	result    Result
	err       error
	checking  bool
}

func New(current string) *Checker {
	return &Checker{current: current, result: Result{CurrentVersion: current}, client: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Check shares a bounded cache across all windows and contexts, including
// failures. A slow GitHub request never holds a lock needed by another request.
func (c *Checker) Check(ctx context.Context) (Result, error) {
	current, err := semver.Parse(strings.TrimPrefix(c.current, "v"))
	if err != nil {
		return Result{CurrentVersion: c.current}, nil
	}
	c.mu.Lock()
	if c.checking || time.Now().Before(c.nextCheck) {
		result, cachedErr := c.result, c.err
		c.mu.Unlock()
		return result, cachedErr
	}
	c.checking = true
	c.mu.Unlock()
	result, checkErr := c.fetch(ctx, current)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checking = false
	if ctx.Err() != nil {
		return c.result, checkErr
	}
	c.nextCheck = time.Now().Add(CheckInterval)
	c.err = checkErr
	if checkErr == nil {
		c.result = result
	}
	return c.result, checkErr
}

func (c *Checker) fetch(ctx context.Context, current semver.Version) (Result, error) {
	result := Result{CurrentVersion: c.current}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return result, fmt.Errorf("create release check: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "KubePeep-release-check")
	response, err := c.client.Do(request)
	if err != nil {
		return result, fmt.Errorf("check latest release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return result, nil
	}
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("check latest release: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return result, fmt.Errorf("read release metadata: %w", err)
	}
	if len(data) > 1<<20 {
		return result, fmt.Errorf("release metadata exceeds size limit")
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return result, fmt.Errorf("decode release metadata: %w", err)
	}
	latest, err := semver.Parse(strings.TrimPrefix(release.Tag, "v"))
	if err != nil {
		return result, fmt.Errorf("parse release version: %w", err)
	}
	if release.Draft || release.Prerelease || len(latest.Pre) > 0 {
		return result, nil
	}
	result.LatestVersion = latest.String()
	result.Available = latest.GT(current)
	return result, nil
}
