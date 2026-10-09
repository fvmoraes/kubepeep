package kubernetes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type HelmClient struct {
	configuration *action.Configuration
	namespace     string
	storage       *helmStorage
}

// Helm uses the same selected, generation-bound transport as other clients.
// It never reads Helm's global kubeconfig, env driver or current context.
func (clients *Clients) HelmClient(ctx context.Context, namespace, driver string) (*HelmClient, error) {
	if driver != "secrets" && driver != "configmaps" {
		return nil, errors.New("unsupported Helm storage")
	}
	config := clients.unaryConfigCopy()
	if config == nil {
		return nil, errActionsClientUnavailable
	}
	config.Wrap(func(base http.RoundTripper) http.RoundTripper { return helmTransport{base: base, ctx: ctx} })
	getter := &helmGetter{config: config, namespace: namespace}
	configuration := new(action.Configuration)
	if err := configuration.Init(getter, namespace, driver, func(string, ...interface{}) {}); err != nil {
		return nil, err
	}
	core, err := coreclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	metadataClient, err := metadata.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	boundedStorage := &helmStorage{ctx: ctx, core: core, metadata: metadataClient, namespace: namespace, resource: driver, seen: map[string]helmRecordIdentity{}}
	configuration.Releases = storage.Init(boundedStorage)
	return &HelmClient{configuration: configuration, namespace: namespace, storage: boundedStorage}, nil
}

func (client *HelmClient) RequireRevision(guard HelmRevisionGuard) error {
	if client == nil || client.storage == nil || guard.Name == "" || guard.Revision < 1 || guard.UID == "" || guard.ResourceVersion == "" {
		return ErrHelmReadOnly
	}
	client.storage.mu.Lock()
	defer client.storage.mu.Unlock()
	if client.storage.created {
		return ErrHelmReadOnly
	}
	client.storage.guard = &guard
	return nil
}
func (client *HelmClient) Get(name string, revision int) (*release.Release, error) {
	get := action.NewGet(client.configuration)
	get.Version = revision
	return get.Run(name)
}
func (client *HelmClient) Upgrade(ctx context.Context, name string, chart *chart.Chart, values map[string]interface{}) (*release.Release, error) {
	upgrade := action.NewUpgrade(client.configuration)
	upgrade.Namespace = client.namespace
	upgrade.Timeout = 2 * time.Minute
	upgrade.MaxHistory = 0
	upgrade.Wait = false
	return upgrade.RunWithContext(ctx, name, chart, values)
}
func (client *HelmClient) Rollback(name string, revision int) (*release.Release, error) {
	rollback := action.NewRollback(client.configuration)
	rollback.Version = revision
	rollback.Timeout = 2 * time.Minute
	rollback.MaxHistory = 0
	rollback.Wait = false
	if err := rollback.Run(name); err != nil {
		return nil, err
	}
	return client.Get(name, 0)
}

type helmGetter struct {
	config    *rest.Config
	namespace string
}

func (g *helmGetter) ToRESTConfig() (*rest.Config, error) { return rest.CopyConfig(g.config), nil }
func (g *helmGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	client, err := discovery.NewDiscoveryClientForConfig(g.config)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(client), nil
}
func (g *helmGetter) ToRESTMapper() (meta.RESTMapper, error) {
	client, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(client), nil
}
func (g *helmGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig { return helmRawConfig{g: g} }

type helmRawConfig struct{ g *helmGetter }

func (c helmRawConfig) RawConfig() (clientcmdapi.Config, error) { return clientcmdapi.Config{}, nil }
func (c helmRawConfig) ClientConfig() (*rest.Config, error)     { return c.g.ToRESTConfig() }
func (c helmRawConfig) Namespace() (string, bool, error)        { return c.g.namespace, true, nil }
func (c helmRawConfig) ConfigAccess() clientcmd.ConfigAccess    { return nil }

type helmTransport struct {
	base http.RoundTripper
	ctx  context.Context
}

func (t helmTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(request.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	cleanup := func() { stop(); cancel() }
	if err := t.ctx.Err(); err != nil {
		cleanup()
		return nil, err
	}
	response, err := t.base.RoundTrip(request.Clone(ctx))
	if err != nil {
		cleanup()
		return nil, err
	}
	response.Body = &helmResponseBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}

type helmResponseBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *helmResponseBody) Close() error { defer b.cleanup(); return b.ReadCloser.Close() }
