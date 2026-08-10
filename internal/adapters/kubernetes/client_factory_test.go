package kubernetes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestFactorySeparatesUnaryTimeoutFromStreamingTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		time.Sleep(100 * time.Millisecond)
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
	factory, err := NewClientFactory(FactoryOptions{UnaryTimeout: 35 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	clients, err := factory.Build(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	defer clients.CloseIdleConnections()
	if clients.unaryConfigCopy().Timeout != 35*time.Millisecond {
		t.Fatalf("unary timeout = %s", clients.unaryConfigCopy().Timeout)
	}
	if clients.streamingConfigCopy().Timeout != 0 {
		t.Fatalf("streaming timeout = %s", clients.streamingConfigCopy().Timeout)
	}
	if !impersonationIsEmpty(clients.unaryConfigCopy().Impersonate) || !impersonationIsEmpty(clients.streamingConfigCopy().Impersonate) {
		t.Fatal("factory enabled impersonation")
	}

	_, unaryErr := clients.UnaryKubernetes().Discovery().RESTClient().Get().AbsPath("/version").DoRaw(context.Background())
	if unaryErr == nil {
		t.Fatal("bounded unary request unexpectedly survived its timeout")
	}
	streamContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := clients.StreamingKubernetes().Discovery().RESTClient().Get().AbsPath("/version").DoRaw(streamContext); err != nil {
		t.Fatalf("streaming transport inherited unary timeout: %v", err)
	}
}

func TestGenerationUsesActivityIdleDeadlineAndCancelsPreviousWork(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	generation := newGeneration(parent, 1, 25*time.Millisecond)
	unary, cancelUnary, err := generation.Unary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelUnary()
	if _, hasDeadline := unary.Deadline(); !hasDeadline {
		t.Fatal("unary context has no finite deadline")
	}
	stream, err := generation.Stream(context.Background(), 45*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if _, hasDeadline := stream.Context().Deadline(); hasDeadline {
		t.Fatal("stream inherited a global deadline")
	}
	for range 4 {
		time.Sleep(20 * time.Millisecond)
		if !stream.Activity() {
			t.Fatal("active stream was canceled by unary timeout")
		}
	}
	select {
	case <-stream.Context().Done():
		t.Fatal("active stream ended prematurely")
	default:
	}
	select {
	case <-stream.Context().Done():
	case <-time.After(150 * time.Millisecond):
		t.Fatal("idle stream was not canceled")
	}
}

func TestOfficialExecPluginAuthenticationAndSanitizedFailure(t *testing.T) {
	const token = "EXEC_TOKEN_SHOULD_STAY_IN_MEMORY"
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"gitVersion":"v1.35.0"}`))
	}))
	defer server.Close()

	makeResolution := func(t *testing.T, fail bool) *Resolution {
		t.Helper()
		config := testKubeconfig(server.URL, "current")
		config.Clusters["cluster"].InsecureSkipTLSVerify = true
		environment := []clientcmdapi.ExecEnvVar{
			{Name: "KUBEPEEP_EXEC_HELPER", Value: "1"},
			{Name: "KUBEPEEP_EXEC_TOKEN", Value: token},
		}
		if fail {
			environment = append(environment,
				clientcmdapi.ExecEnvVar{Name: "KUBEPEEP_EXEC_FAIL", Value: "1"},
				clientcmdapi.ExecEnvVar{Name: "KUBEPEEP_EXEC_SECRET", Value: "PLUGIN_STDERR_SECRET_SHOULD_NOT_LEAK"},
			)
		}
		config.AuthInfos["user"].Exec = &clientcmdapi.ExecConfig{
			Command:         os.Args[0],
			Args:            []string{"-test.run=TestExecCredentialHelper", "--"},
			Env:             environment,
			APIVersion:      "client.authentication.k8s.io/v1",
			InteractiveMode: clientcmdapi.NeverExecInteractiveMode,
		}
		path := filepath.Join(t.TempDir(), "exec-config.yaml")
		writeTestKubeconfig(t, path, config)
		resolution, err := NewLoader(LoaderOptions{}).Resolve(context.Background(), ResolveRequest{ExplicitPath: &path, FirstReconcile: true})
		if err != nil {
			t.Fatal(err)
		}
		return resolution
	}
	factory, err := NewClientFactory(FactoryOptions{UnaryTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	clients, err := factory.Build(context.Background(), makeResolution(t, false))
	if err != nil {
		t.Fatal(err)
	}
	result := CheckConnectivity(context.Background(), clients)
	clients.CloseIdleConnections()
	if result.Status != ConnectivityHealthy || result.Version != "v1.35.0" {
		t.Fatalf("exec connectivity = %+v", result)
	}

	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStderr := os.Stderr
	os.Stderr = writeStderr
	failingClients, buildErr := factory.Build(context.Background(), makeResolution(t, true))
	if buildErr == nil {
		result = CheckConnectivity(context.Background(), failingClients)
		failingClients.CloseIdleConnections()
	}
	os.Stderr = previousStderr
	_ = writeStderr.Close()
	pluginStderr, readErr := io.ReadAll(readStderr)
	_ = readStderr.Close()
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	if result.Code != CodeAuthenticationUnavailable || result.Status != ConnectivityUnknown {
		t.Fatalf("failed exec connectivity = %+v", result)
	}
	if strings.Contains(result.Message, "PLUGIN_STDERR_SECRET") || strings.Contains(result.Message, token) {
		t.Fatalf("exec output leaked: %+v", result)
	}
	if strings.Contains(string(pluginStderr), "PLUGIN_STDERR_SECRET") || strings.Contains(string(pluginStderr), token) {
		t.Fatalf("exec plugin bypassed sanitized output boundary")
	}
}

func TestConnectivitySeparatesOfflineClusterFromLocalApplicationHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"gitVersion":"v1.35.0"}`))
	}))
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeTestKubeconfig(t, path, testKubeconfig(server.URL, "current"))
	resolution, err := NewLoader(LoaderOptions{}).Resolve(context.Background(), ResolveRequest{ExplicitPath: &path, FirstReconcile: true})
	if err != nil {
		t.Fatal(err)
	}
	factory, _ := NewClientFactory(FactoryOptions{UnaryTimeout: 100 * time.Millisecond})
	clients, err := factory.Build(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	server.Close()
	defer clients.CloseIdleConnections()
	result := CheckConnectivity(context.Background(), clients)
	if result.Status != ConnectivityDegraded || result.Code != CodeClusterUnavailable {
		t.Fatalf("offline cluster classification = %+v", result)
	}
}

func TestResolutionIsNotSerializableAndImpersonationIsRejected(t *testing.T) {
	config := testKubeconfig("https://cluster.invalid", "current")
	config.AuthInfos["user"].Token = "TOKEN_SHOULD_NOT_SERIALIZE"
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeTestKubeconfig(t, path, config)
	loader := NewLoader(LoaderOptions{})
	resolution, err := loader.Resolve(context.Background(), ResolveRequest{ExplicitPath: &path, FirstReconcile: true})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(resolution)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "{}" || strings.Contains(string(encoded), "TOKEN_SHOULD_NOT_SERIALIZE") {
		t.Fatalf("serialized resolution = %s", encoded)
	}

	config.AuthInfos["user"].Impersonate = "forbidden-admin"
	writeTestKubeconfig(t, path, config)
	_, err = loader.Resolve(context.Background(), ResolveRequest{ExplicitPath: &path, FirstReconcile: true})
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("impersonation code = %q", code)
	}
}

func TestProductionAdapterContainsNoCredentialInjectionFlow(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate adapter sources")
	}
	entries, err := os.ReadDir(filepath.Dir(currentFile))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"BearerToken =", "Password =", "Username =", "Impersonate.UserName =", "Impersonate.Groups ="}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, pattern := range forbidden {
			if strings.Contains(string(contents), pattern) {
				t.Fatalf("%s contains forbidden credential injection %q", entry.Name(), pattern)
			}
		}
	}
}
