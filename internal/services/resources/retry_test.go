package resources

import (
	"context"
	"testing"
	"time"
)

type retryTestLister struct{ calls int }

func (lister *retryTestLister) ListPage(_ context.Context, request PageRequest) (OriginPage[testListItem], error) {
	lister.calls++
	if lister.calls < 3 {
		return OriginPage[testListItem]{Origin: request.Origin}, NewRateLimitedError(time.Second, nil)
	}
	return OriginPage[testListItem]{Origin: request.Origin, Items: []testListItem{"ok"}}, nil
}

func TestRetryListPageRespectsRetryAfterAndReducesPressure(t *testing.T) {
	lister := &retryTestLister{}
	pressure := &apiPressure{}
	waits := []time.Duration{}
	page, err := retryListPage(t.Context(), lister, PageRequest{Origin: Origin{Version: "v1", Resource: "pods"}}, RetryPolicy{
		Attempts: 3,
		Base:     time.Millisecond,
		Maximum:  2 * time.Second,
		Jitter:   func(value time.Duration) time.Duration { return value },
		Wait: func(_ context.Context, value time.Duration) error {
			waits = append(waits, value)
			return nil
		},
	}, pressure)
	if err != nil || len(page.Items) != 1 || lister.calls != 3 {
		t.Fatalf("page=%+v calls=%d err=%v", page, lister.calls, err)
	}
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != time.Second {
		t.Fatalf("Retry-After waits = %v", waits)
	}
	if got := pressure.fanout(4); got != 2 {
		t.Fatalf("fanout after repeated 429 = %d", got)
	}
}

func TestRetryListPageCapsJitterAndStopsOnCancellation(t *testing.T) {
	lister := &retryTestLister{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	waited := time.Duration(-1)
	_, err := retryListPage(ctx, lister, PageRequest{Origin: Origin{Version: "v1", Resource: "pods"}}, RetryPolicy{
		Attempts: 3,
		Base:     time.Second,
		Maximum:  2 * time.Second,
		Jitter:   func(value time.Duration) time.Duration { return value * 10 },
		Wait: func(waitContext context.Context, value time.Duration) error {
			waited = value
			return waitContext.Err()
		},
	}, &apiPressure{})
	if err != context.Canceled || lister.calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, lister.calls)
	}
	if waited != 2*time.Second {
		t.Fatalf("capped jitter delay = %v", waited)
	}
}
