package kubernetes

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	// Register client-go authentication providers so kubeconfigs using
	// auth-provider flows (oidc, gcp, azure) resolve exactly as kubectl
	// does. Without these, real-world clusters fail with a sanitized
	// KUBERNETES_CLIENT_UNAVAILABLE instead of authenticating.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// client-go's official exec authenticator captures os.Stderr when its
// transport is constructed and exposes no public writer option. Serialize
// that brief construction boundary and give it the platform null device so
// untrusted plugin stderr can never bypass the sanitized application logger.
// The original process stderr is restored before Build returns.
var execAuthenticatorConstruction sync.Mutex

var discardedExecPluginStderr, discardedExecPluginStderrError = os.OpenFile(os.DevNull, os.O_WRONLY, 0)

const (
	DefaultUnaryTimeout = 15 * time.Second
	defaultUserAgent    = "kubePeep"
)

// ClientBuilder permits a cache to deduplicate construction without global
// mutable clientsets.
type ClientBuilder interface {
	Build(context.Context, *Resolution) (*Clients, error)
}

// FactoryOptions separates bounded unary traffic from long-running traffic.
type FactoryOptions struct {
	UnaryTimeout time.Duration
	QPS          float32
	Burst        int
	UserAgent    string
}

// ClientFactory builds independent client groups from an in-memory resolution.
type ClientFactory struct {
	unaryTimeout time.Duration
	qps          float32
	burst        int
	userAgent    string
}

func NewClientFactory(options FactoryOptions) (*ClientFactory, error) {
	timeout := options.UnaryTimeout
	if timeout == 0 {
		timeout = DefaultUnaryTimeout
	}
	if timeout < 0 {
		return nil, safeError(CodeClientUnavailable, "The unary Kubernetes timeout is invalid.", false)
	}
	userAgent := options.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	if len(userAgent) > 128 || strings.IndexFunc(userAgent, unicode.IsControl) >= 0 {
		return nil, safeError(CodeClientUnavailable, "The Kubernetes client identity is invalid.", false)
	}
	qps := options.QPS
	if qps == 0 {
		qps = 10
	}
	burst := options.Burst
	if burst == 0 {
		burst = 20
	}
	if qps < 0 || burst < 1 {
		return nil, safeError(CodeClientUnavailable, "The Kubernetes client rate limit is invalid.", false)
	}
	return &ClientFactory{unaryTimeout: timeout, qps: qps, burst: burst, userAgent: userAgent}, nil
}

type clientGroup struct {
	kubernetes kubeclient.Interface
	dynamic    dynamic.Interface
	metadata   metadata.Interface
	config     *rest.Config
	httpClient *http.Client
}

// Clients owns distinct transports for unary and streaming operations. Metrics
// uses the bounded unary transport and remains optional at the API layer.
type Clients struct {
	unary     *clientGroup
	streaming *clientGroup
	metrics   metricsclient.Interface
}

func (clients *Clients) UnaryKubernetes() kubeclient.Interface {
	if clients == nil || clients.unary == nil {
		return nil
	}
	return clients.unary.kubernetes
}

func (clients *Clients) UnaryDynamic() dynamic.Interface {
	if clients == nil || clients.unary == nil {
		return nil
	}
	return clients.unary.dynamic
}

// UnaryMetadata exposes the metadata-only client used for resources whose
// payload must never be materialized (notably Secrets). The underlying REST
// configuration and credentials remain private to this adapter package.
func (clients *Clients) UnaryMetadata() metadata.Interface {
	if clients == nil || clients.unary == nil {
		return nil
	}
	return clients.unary.metadata
}

func (clients *Clients) StreamingKubernetes() kubeclient.Interface {
	if clients == nil || clients.streaming == nil {
		return nil
	}
	return clients.streaming.kubernetes
}

func (clients *Clients) StreamingDynamic() dynamic.Interface {
	if clients == nil || clients.streaming == nil {
		return nil
	}
	return clients.streaming.dynamic
}

// StreamingMetadata is the long-running counterpart used by metadata-only
// watches (ConfigMaps today, and deliberately never a typed Secret watch).
func (clients *Clients) StreamingMetadata() metadata.Interface {
	if clients == nil || clients.streaming == nil {
		return nil
	}
	return clients.streaming.metadata
}

func (clients *Clients) Metrics() metricsclient.Interface {
	if clients == nil {
		return nil
	}
	return clients.metrics
}

// unaryConfigCopy and streamingConfigCopy are intentionally package-private:
// rest.Config contains credentials and must not enter handlers, DTOs, or logs.
func (clients *Clients) unaryConfigCopy() *rest.Config {
	if clients == nil || clients.unary == nil {
		return nil
	}
	return rest.CopyConfig(clients.unary.config)
}

func (clients *Clients) streamingConfigCopy() *rest.Config {
	if clients == nil || clients.streaming == nil {
		return nil
	}
	return rest.CopyConfig(clients.streaming.config)
}

func (clients *Clients) CloseIdleConnections() {
	if clients == nil {
		return
	}
	if clients.unary != nil && clients.unary.httpClient != nil {
		clients.unary.httpClient.CloseIdleConnections()
	}
	if clients.streaming != nil && clients.streaming.httpClient != nil {
		clients.streaming.httpClient.CloseIdleConnections()
	}
}

func (factory *ClientFactory) Build(ctx context.Context, resolution *Resolution) (*Clients, error) {
	if ctx == nil {
		return nil, safeError(CodeRequestCanceled, "The Kubernetes request was canceled.", true)
	}
	if err := ctx.Err(); err != nil {
		return nil, SanitizeError(err)
	}
	base, err := resolution.configCopy()
	if err != nil {
		return nil, err
	}
	base.UserAgent = factory.userAgent
	base.QPS = factory.qps
	base.Burst = factory.burst
	base.Impersonate = rest.ImpersonationConfig{}

	execAuthenticatorConstruction.Lock()
	previousStderr := os.Stderr
	if discardedExecPluginStderrError != nil || discardedExecPluginStderr == nil {
		execAuthenticatorConstruction.Unlock()
		return nil, safeError(CodeClientUnavailable, "The Kubernetes authentication output could not be isolated.", false)
	}
	os.Stderr = discardedExecPluginStderr
	defer func() {
		os.Stderr = previousStderr
		execAuthenticatorConstruction.Unlock()
	}()

	unaryConfig := rest.CopyConfig(base)
	unaryConfig.Timeout = factory.unaryTimeout
	unary, err := buildClientGroup(unaryConfig)
	if err != nil {
		return nil, SanitizeError(err)
	}

	streamingConfig := rest.CopyConfig(base)
	streamingConfig.Timeout = 0
	streaming, err := buildClientGroup(streamingConfig)
	if err != nil {
		unary.httpClient.CloseIdleConnections()
		return nil, SanitizeError(err)
	}
	metrics, err := metricsclient.NewForConfigAndClient(unaryConfig, unary.httpClient)
	if err != nil {
		unary.httpClient.CloseIdleConnections()
		streaming.httpClient.CloseIdleConnections()
		return nil, SanitizeError(err)
	}
	if err := ctx.Err(); err != nil {
		unary.httpClient.CloseIdleConnections()
		streaming.httpClient.CloseIdleConnections()
		return nil, SanitizeError(err)
	}
	return &Clients{unary: unary, streaming: streaming, metrics: metrics}, nil
}

func buildClientGroup(config *rest.Config) (*clientGroup, error) {
	if config == nil || !impersonationIsEmpty(config.Impersonate) {
		return nil, safeError(CodeClientUnavailable, "The Kubernetes client configuration is invalid.", false)
	}
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, err
	}
	kubernetesClient, err := kubeclient.NewForConfigAndClient(config, httpClient)
	if err != nil {
		httpClient.CloseIdleConnections()
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfigAndClient(config, httpClient)
	if err != nil {
		httpClient.CloseIdleConnections()
		return nil, err
	}
	metadataClient, err := metadata.NewForConfigAndClient(config, httpClient)
	if err != nil {
		httpClient.CloseIdleConnections()
		return nil, err
	}
	return &clientGroup{
		kubernetes: kubernetesClient,
		dynamic:    dynamicClient,
		metadata:   metadataClient,
		config:     rest.CopyConfig(config),
		httpClient: httpClient,
	}, nil
}
