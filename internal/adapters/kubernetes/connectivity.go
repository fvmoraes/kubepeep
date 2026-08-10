package kubernetes

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/version"
)

type ConnectivityStatus string

const (
	ConnectivityHealthy  ConnectivityStatus = "healthy"
	ConnectivityDegraded ConnectivityStatus = "degraded"
	ConnectivityUnknown  ConnectivityStatus = "unknown"
)

// ConnectivityResult describes an external dependency only. Degraded or
// unknown cluster state must not be promoted to local application failure.
type ConnectivityResult struct {
	Status  ConnectivityStatus
	Code    ErrorCode
	Message string
	Version string
}

// CheckConnectivity performs a bounded /version request with the unary client.
// Every returned message is constant and credential-safe.
func CheckConnectivity(ctx context.Context, clients *Clients) ConnectivityResult {
	if ctx == nil || clients == nil || clients.UnaryKubernetes() == nil {
		return ConnectivityResult{
			Status:  ConnectivityUnknown,
			Code:    CodeClientUnavailable,
			Message: "The Kubernetes client is unavailable.",
		}
	}
	raw, err := clients.UnaryKubernetes().Discovery().RESTClient().Get().AbsPath("/version").DoRaw(ctx)
	if err != nil {
		code, message, _ := ErrorDetails(SanitizeError(err))
		status := ConnectivityDegraded
		if code == CodeAuthenticationUnavailable || code == CodeRequestCanceled || code == CodeRequestTimeout {
			status = ConnectivityUnknown
		}
		return ConnectivityResult{Status: status, Code: code, Message: message}
	}
	var info version.Info
	if err := json.Unmarshal(raw, &info); err != nil || info.GitVersion == "" {
		return ConnectivityResult{
			Status:  ConnectivityUnknown,
			Code:    CodeClientUnavailable,
			Message: "The Kubernetes API returned an invalid version response.",
		}
	}
	return ConnectivityResult{
		Status:  ConnectivityHealthy,
		Code:    "OK",
		Message: "The Kubernetes cluster is reachable.",
		Version: info.GitVersion,
	}
}
