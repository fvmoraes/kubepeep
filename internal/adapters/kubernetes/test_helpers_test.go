package kubernetes

import (
	"os"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func testKubeconfig(server, currentContext string) *clientcmdapi.Config {
	config := clientcmdapi.NewConfig()
	config.Clusters["cluster"] = &clientcmdapi.Cluster{Server: server}
	config.AuthInfos["user"] = &clientcmdapi.AuthInfo{}
	config.Contexts["current"] = &clientcmdapi.Context{Cluster: "cluster", AuthInfo: "user"}
	config.Contexts["persisted"] = &clientcmdapi.Context{Cluster: "cluster", AuthInfo: "user"}
	config.Contexts["explicit"] = &clientcmdapi.Context{Cluster: "cluster", AuthInfo: "user"}
	config.CurrentContext = currentContext
	return config
}

func writeTestKubeconfig(t *testing.T, path string, config *clientcmdapi.Config) {
	t.Helper()
	if err := clientcmd.WriteToFile(*config, path); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
}

func stringPointer(value string) *string {
	return &value
}

func safeCode(t *testing.T, err error) ErrorCode {
	t.Helper()
	if err == nil {
		t.Fatal("expected safe error")
	}
	code, _, _ := ErrorDetails(err)
	return code
}

// TestExecCredentialHelper is re-executed as a client-go exec credential
// plugin by the integration tests below.
func TestExecCredentialHelper(t *testing.T) {
	if os.Getenv("KUBEPEEP_EXEC_HELPER") != "1" {
		return
	}
	if os.Getenv("KUBEPEEP_EXEC_FAIL") == "1" {
		os.Exit(17)
	}
	token := os.Getenv("KUBEPEEP_EXEC_TOKEN")
	_, _ = os.Stdout.WriteString(`{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"` + token + `"}}`)
	os.Exit(0)
}
