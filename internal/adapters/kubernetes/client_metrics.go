package kubernetes

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

type measuredTransport struct {
	next     http.RoundTripper
	registry *observability.Registry
	traffic  string
}

func (transport measuredTransport) WrappedRoundTripper() http.RoundTripper { return transport.next }
func (transport measuredTransport) CloseIdleConnections() {
	utilnet.CloseIdleConnectionsFor(transport.next)
}

func (transport measuredTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.next.RoundTrip(request)
	status := "transport_error"
	if response != nil {
		status = strconv.Itoa(response.StatusCode)
	}
	// Never retain URL, query, request headers, credentials or response bodies.
	transport.registry.IncCounter(observability.KubernetesRequestsTotalName, map[string]string{"traffic": transport.traffic, "status": status})
	return response, err
}

func observeTransport(config *rest.Config, registry *observability.Registry, traffic string) {
	if registry == nil {
		return
	}
	config.Wrap(func(next http.RoundTripper) http.RoundTripper {
		return measuredTransport{next: next, registry: registry, traffic: traffic}
	})
}

type measuredRateLimiter struct {
	flowcontrol.RateLimiter
	registry *observability.Registry
	traffic  string
}

func (limiter measuredRateLimiter) Wait(ctx context.Context) error {
	start := time.Now()
	err := limiter.RateLimiter.Wait(ctx)
	labels := map[string]string{"traffic": limiter.traffic}
	limiter.registry.IncCounter(observability.ClientThrottleTotalName, labels)
	limiter.registry.AddCounter(observability.ClientThrottleNanosecondsTotalName, labels, uint64(time.Since(start).Nanoseconds()))
	return err
}

// Preserve client-go's separate limiter per client family, and the configured
// limiter when supplied. Sharing a new limiter across families would change QPS.
func observeRateLimit(config *rest.Config, registry *observability.Registry, traffic string) *rest.Config {
	if registry == nil {
		return config
	}
	copy := rest.CopyConfig(config)
	limiter := copy.RateLimiter
	if limiter == nil && copy.QPS > 0 && copy.Burst > 0 {
		limiter = flowcontrol.NewTokenBucketRateLimiter(copy.QPS, copy.Burst)
	}
	if limiter != nil {
		copy.RateLimiter = measuredRateLimiter{RateLimiter: limiter, registry: registry, traffic: traffic}
	}
	return copy
}
