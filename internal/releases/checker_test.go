package releases

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckVersionsAndCache(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, current, latest string
		available             bool
	}{
		{"upgrade", "0.11.0", "v0.12.0", true},
		{"numeric order", "v0.9.0", "v0.10.0", true},
		{"same", "0.12.0", "v0.12.0", false},
		{"older", "0.13.0", "v0.12.0", false},
		{"stable replaces prerelease", "0.12.0-rc.1", "v0.12.0", true},
		{"prerelease ignored", "0.11.0", "v0.12.0-rc.1", false},
		{"development build", "dev", "v0.12.0", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			checker := New(tt.current)
			checker.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != latestURL || r.Header.Get("Authorization") != "" {
					t.Fatal("unexpected release request")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"tag_name":%q,"draft":false,"prerelease":false}`, tt.latest)))}, nil
			})
			for range 3 {
				result, err := checker.Check(context.Background())
				if err != nil || result.Available != tt.available {
					t.Fatalf("result=%+v, error=%v", result, err)
				}
			}
			if tt.current == "dev" {
				if calls != 0 {
					t.Fatal("dev version queried GitHub")
				}
				return
			}
			if calls != 1 {
				t.Fatalf("cached check made %d calls", calls)
			}
			checker.nextCheck = time.Now().Add(-time.Second)
			if _, err := checker.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatal("expired cache was not refreshed")
			}
		})
	}
}

func TestCheckFailuresAreBoundedAndCached(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{"rate limited", "", 403, true}, {"offline server", "", 503, true}, {"no releases", "", 404, false},
		{"invalid json", "{", 200, true}, {"invalid version", `{"tag_name":"latest"}`, 200, true},
		{"oversized", strings.Repeat(" ", (1<<20)+1), 200, true},
		{"draft", `{"tag_name":"v99.0.0","draft":true}`, 200, false},
		{"flagged prerelease", `{"tag_name":"v99.0.0","prerelease":true}`, 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checker := New("0.11.0")
			calls := 0
			checker.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})
			for range 2 {
				result, err := checker.Check(context.Background())
				if (err != nil) != tt.wantError || result.Available {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
			if calls != 1 {
				t.Fatal("unsuccessful check was retried before two hours")
			}
		})
	}
}

func TestCanceledCheckDoesNotSuppressTheNextWindow(t *testing.T) {
	t.Parallel()
	checker := New("0.11.0")
	checker.client.Transport = transportFunc(func(request *http.Request) (*http.Response, error) {
		if err := request.Context().Err(); err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"0.12.0"}`))}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := checker.Check(ctx); err == nil {
		t.Fatal("canceled request succeeded")
	}
	result, err := checker.Check(context.Background())
	if err != nil || !result.Available {
		t.Fatalf("cancellation poisoned release cache: %+v %v", result, err)
	}
}

func TestConcurrentCheckSharesOnlyOneRequest(t *testing.T) {
	t.Parallel()
	checker := New("0.11.0")
	started, finish := make(chan struct{}), make(chan struct{})
	checker.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		close(started)
		<-finish
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"0.12.0"}`))}, nil
	})
	result := make(chan error, 1)
	go func() { _, err := checker.Check(context.Background()); result <- err }()
	<-started
	if _, err := checker.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(finish)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if latest, err := checker.Check(context.Background()); err != nil || !latest.Available {
		t.Fatalf("completed check not cached: %+v %v", latest, err)
	}
}
