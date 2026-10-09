package resources

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"sync/atomic"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

const (
	DefaultListAttempts = 3
	DefaultRetryBase    = 200 * time.Millisecond
	MaximumRetryDelay   = 5 * time.Second
)

type RetryPolicy struct {
	Attempts   int
	Base       time.Duration
	Maximum    time.Duration
	Jitter     func(time.Duration) time.Duration
	Wait       func(context.Context, time.Duration) error
	OnThrottle func()
}

type apiPressure struct {
	throttles atomic.Int32
}

func (pressure *apiPressure) recordThrottle() {
	pressure.throttles.Add(1)
}

func (pressure *apiPressure) fanout(configured int) int {
	value := NormalizeFanout(configured)
	if pressure != nil && pressure.throttles.Load() >= 2 {
		value = max(1, value/2)
	}
	return value
}

func normalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	if policy.Attempts <= 0 {
		policy.Attempts = DefaultListAttempts
	}
	if policy.Base <= 0 {
		policy.Base = DefaultRetryBase
	}
	if policy.Maximum <= 0 || policy.Maximum > MaximumRetryDelay {
		policy.Maximum = MaximumRetryDelay
	}
	if policy.Jitter == nil {
		policy.Jitter = retryJitter
	}
	if policy.Wait == nil {
		policy.Wait = waitForRetry
	}
	return policy
}

func retryListPage[T ListItem](ctx context.Context, lister OriginLister[T], request PageRequest, policy RetryPolicy, pressure *apiPressure) (OriginPage[T], error) {
	policy = normalizeRetryPolicy(policy)
	var lastErr error
	for attempt := 0; attempt < policy.Attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return OriginPage[T]{Origin: request.Origin}, err
		}
		page, err := lister.ListPage(ctx, request)
		if err == nil {
			return page, nil
		}
		lastErr = err
		if ErrorCodeOf(err) == CodeRateLimited && policy.OnThrottle != nil {
			policy.OnThrottle()
		}
		if !retryableRead(err) || attempt+1 >= policy.Attempts {
			return page, err
		}
		if pressure != nil && ErrorCodeOf(err) == CodeRateLimited {
			pressure.recordThrottle()
		}
		delay := policy.Base << attempt
		var retryAfter time.Duration
		var suggested interface{ RetryAfter() time.Duration }
		if errors.As(err, &suggested) {
			retryAfter = suggested.RetryAfter()
		}
		// A server minimum outside this request's retry budget cannot be
		// shortened safely. Preserve the rate-limit error for the caller.
		if retryAfter > policy.Maximum {
			return page, err
		}
		if retryAfter > delay {
			delay = retryAfter
		}
		if delay > policy.Maximum {
			delay = policy.Maximum
		}
		delay = policy.Jitter(delay)
		if delay < 0 {
			delay = 0
		}
		if delay > policy.Maximum {
			delay = policy.Maximum
		}
		if delay < retryAfter {
			delay = retryAfter
		}
		if err := policy.Wait(ctx, delay); err != nil {
			return OriginPage[T]{Origin: request.Origin}, err
		}
	}
	return OriginPage[T]{Origin: request.Origin}, lastErr
}

func retryableRead(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var domain *DomainError
	if !errors.As(err, &domain) {
		failure := authorization.TranslateOperationError(err)
		return failure != nil && failure.Retryable
	}
	switch ErrorCodeOf(err) {
	case CodeRateLimited, CodeClusterUnavailable, CodeUpstreamTimeout, CodeAuthenticationUnavailable:
		return true
	default:
		return false
	}
}

// RetryRead retries only idempotent remote reads. Each API request remains
// bounded; the frontend's ten-second refresh owns continued recovery.
func RetryRead[T any](ctx context.Context, read func(context.Context) (T, error)) (T, error) {
	policy := normalizeRetryPolicy(RetryPolicy{})
	var zero T
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		value, err := read(ctx)
		if err == nil || !retryableRead(err) || attempt+1 >= policy.Attempts {
			return value, err
		}
		if err := policy.Wait(ctx, policy.Jitter(policy.Base<<attempt)); err != nil {
			return zero, err
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	var raw [8]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return delay
	}
	// 75%..125% avoids synchronized retry waves. The policy reapplies its hard
	// maximum after jitter. Overflow is impossible at the five-second cap.
	fraction := float64(binary.LittleEndian.Uint64(raw[:])) / float64(^uint64(0))
	return time.Duration(float64(delay) * (0.75 + 0.5*fraction))
}
