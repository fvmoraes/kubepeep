package kubernetes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestNewClientFactoryValidatesAndDefaultsOptions(t *testing.T) {
	tests := []struct {
		name        string
		options     FactoryOptions
		wantErr     bool
		wantTimeout time.Duration
		wantQPS     float32
		wantBurst   int
		wantAgent   string
	}{
		{
			name:        "defaults",
			options:     FactoryOptions{},
			wantTimeout: DefaultUnaryTimeout,
			wantQPS:     10,
			wantBurst:   20,
			wantAgent:   "kubePeep",
		},
		{
			name:        "custom",
			options:     FactoryOptions{UnaryTimeout: time.Second, QPS: 5, Burst: 9, UserAgent: "custom-agent"},
			wantTimeout: time.Second,
			wantQPS:     5,
			wantBurst:   9,
			wantAgent:   "custom-agent",
		},
		{name: "negative timeout", options: FactoryOptions{UnaryTimeout: -time.Second}, wantErr: true},
		{name: "overlong user agent", options: FactoryOptions{UserAgent: strings.Repeat("a", 129)}, wantErr: true},
		{name: "control character user agent", options: FactoryOptions{UserAgent: "agent\ninjected"}, wantErr: true},
		{name: "negative qps", options: FactoryOptions{QPS: -1}, wantErr: true},
		{name: "zero burst", options: FactoryOptions{Burst: -1}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, err := NewClientFactory(test.options)
			if test.wantErr {
				if err == nil {
					t.Fatal("invalid options were accepted")
				}
				if code := safeCode(t, err); code != CodeClientUnavailable {
					t.Fatalf("code = %q", code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if factory.unaryTimeout != test.wantTimeout || factory.qps != test.wantQPS || factory.burst != test.wantBurst || factory.userAgent != test.wantAgent {
				t.Fatalf("factory = %+v", factory)
			}
		})
	}
}

func TestClientsAccessorsExposeOnlyGroupInterfaces(t *testing.T) {
	nilClients := (*Clients)(nil)
	if nilClients.UnaryKubernetes() != nil || nilClients.UnaryDynamic() != nil || nilClients.UnaryMetadata() != nil {
		t.Fatal("nil clients exposed unary interfaces")
	}
	if nilClients.StreamingKubernetes() != nil || nilClients.StreamingDynamic() != nil || nilClients.StreamingMetadata() != nil {
		t.Fatal("nil clients exposed streaming interfaces")
	}
	if nilClients.Metrics() != nil {
		t.Fatal("nil clients exposed metrics")
	}
	nilClients.CloseIdleConnections()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"gitVersion":"v1.35.0"}`))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeTestKubeconfig(t, path, testKubeconfig(server.URL, "current"))
	resolution, err := NewLoader(LoaderOptions{}).Resolve(context.Background(), ResolveRequest{ExplicitPath: &path, FirstReconcile: true})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewClientFactory(FactoryOptions{UnaryTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	clients, err := factory.Build(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	defer clients.CloseIdleConnections()
	if clients.UnaryKubernetes() == nil || clients.UnaryDynamic() == nil || clients.UnaryMetadata() == nil {
		t.Fatal("unary accessors missing")
	}
	if clients.StreamingKubernetes() == nil || clients.StreamingDynamic() == nil || clients.StreamingMetadata() == nil {
		t.Fatal("streaming accessors missing")
	}
	if clients.Metrics() == nil {
		t.Fatal("metrics accessor missing")
	}
	(&Clients{}).CloseIdleConnections()
	(&Clients{unary: &clientGroup{}}).CloseIdleConnections()
	(&Clients{streaming: &clientGroup{}}).CloseIdleConnections()
}

func TestClientsConfigCopiesGuardNilGroups(t *testing.T) {
	if got := (*Clients)(nil).unaryConfigCopy(); got != nil {
		t.Fatalf("nil unary copy = %#v", got)
	}
	if got := (*Clients)(nil).streamingConfigCopy(); got != nil {
		t.Fatalf("nil streaming copy = %#v", got)
	}
	if got := (&Clients{}).unaryConfigCopy(); got != nil {
		t.Fatalf("empty unary copy = %#v", got)
	}
	clients := &Clients{unary: &clientGroup{config: &rest.Config{Host: "https://unary.invalid"}}}
	if got := clients.unaryConfigCopy(); got == nil || got.Host != "https://unary.invalid" {
		t.Fatalf("unary copy = %#v", got)
	}
}

func TestFactoryBuildRejectsInvalidContextsAndResolutions(t *testing.T) {
	factory, err := NewClientFactory(FactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = factory.Build(nil, &Resolution{})
	if code := safeCode(t, err); code != CodeRequestCanceled {
		t.Fatalf("nil context code = %q", code)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = factory.Build(canceled, &Resolution{})
	if code := safeCode(t, err); code != CodeRequestCanceled {
		t.Fatalf("canceled context code = %q", code)
	}
	_, err = factory.Build(context.Background(), &Resolution{})
	if code := safeCode(t, err); code != CodeContextRequired {
		t.Fatalf("empty resolution code = %q", code)
	}
	impersonated := &Resolution{restConfig: &rest.Config{Host: "https://cluster.invalid", Impersonate: rest.ImpersonationConfig{UserName: "forbidden-admin"}}}
	_, err = factory.Build(context.Background(), impersonated)
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("impersonated resolution code = %q", code)
	}
}

func TestBuildClientGroupRejectsInvalidConfigurations(t *testing.T) {
	_, err := buildClientGroup(nil)
	if code := safeCode(t, err); code != CodeClientUnavailable {
		t.Fatalf("nil config code = %q", code)
	}
	impersonated := &rest.Config{Host: "https://cluster.invalid", Impersonate: rest.ImpersonationConfig{UserName: "forbidden-admin"}}
	_, err = buildClientGroup(impersonated)
	if code := safeCode(t, err); code != CodeClientUnavailable {
		t.Fatalf("impersonated config code = %q", code)
	}
	badTLS := &rest.Config{Host: "https://cluster.invalid", TLSClientConfig: rest.TLSClientConfig{CAData: []byte("not-a-pem-certificate")}}
	if _, err := buildClientGroup(badTLS); err == nil || safeCode(t, err) != CodeClientUnavailable {
		t.Fatalf("bad TLS config err = %v", err)
	}
	unparsable := &rest.Config{Host: "://invalid-host"}
	if _, err := buildClientGroup(unparsable); err == nil {
		t.Fatal("unparsable host was accepted")
	}
}
