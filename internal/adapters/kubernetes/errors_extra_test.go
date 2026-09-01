package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestSafeErrorFormattingAndDetails(t *testing.T) {
	if got := (*SafeError)(nil).Error(); got != "" {
		t.Fatalf("nil safe error = %q", got)
	}
	safe := &SafeError{Code: CodeClusterUnavailable, Message: "The Kubernetes cluster is temporarily unavailable.", Retryable: true}
	if got := safe.Error(); got != string(CodeClusterUnavailable)+": "+safe.Message {
		t.Fatalf("safe error = %q", got)
	}
	code, message, retryable := ErrorDetails(safe)
	if code != CodeClusterUnavailable || message != safe.Message || !retryable {
		t.Fatalf("details = (%q, %q, %v)", code, message, retryable)
	}
	wrapped := fmt.Errorf("transport: %w", safe)
	code, _, _ = ErrorDetails(wrapped)
	if code != CodeClusterUnavailable {
		t.Fatalf("wrapped details code = %q", code)
	}
	code, message, retryable = ErrorDetails(errors.New("TOKEN_SHOULD_NOT_APPEAR"))
	if code != CodeClientUnavailable || !retryable || message == "" {
		t.Fatalf("foreign error details = (%q, %q, %v)", code, message, retryable)
	}
	code, _, _ = ErrorDetails(nil)
	if code != CodeClientUnavailable {
		t.Fatalf("nil details code = %q", code)
	}
}

func TestSanitizeErrorClassifiesWithoutRetainingCause(t *testing.T) {
	unauthorized := apierrors.NewUnauthorized("AUTH_MARKER_SHOULD_NOT_LEAK")
	_, exitFailure := exec.Command("sh", "-c", "exit 3").Output()
	missingBinary := &exec.Error{Name: "kubepeep-missing-binary", Err: exec.ErrNotFound}
	credentialText := errors.New("getting credentials: exec plugin is configured to request a token")

	tests := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{name: "nil", err: nil, want: ""},
		{name: "canceled", err: context.Canceled, want: CodeRequestCanceled},
		{name: "deadline", err: fmt.Errorf("request: %w", context.DeadlineExceeded), want: CodeRequestTimeout},
		{name: "unauthorized", err: unauthorized, want: CodeAuthenticationUnavailable},
		{name: "exit error", err: exitFailure, want: CodeAuthenticationUnavailable},
		{name: "missing binary", err: missingBinary, want: CodeAuthenticationUnavailable},
		{name: "credential text", err: credentialText, want: CodeAuthenticationUnavailable},
		{name: "network", err: &net.OpError{Op: "dial", Err: errors.New("refused")}, want: CodeClusterUnavailable},
		{name: "plain", err: errors.New("TOKEN_MARKER_SHOULD_NOT_LEAK"), want: CodeClientUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sanitized := SanitizeError(test.err)
			if test.want == "" {
				if sanitized != nil {
					t.Fatalf("sanitized = %v", sanitized)
				}
				return
			}
			if code := safeCode(t, sanitized); code != test.want {
				t.Fatalf("code = %q, want %q", code, test.want)
			}
		})
	}

	alreadySafe := safeError(CodeContextNotFound, "The selected Kubernetes context does not exist.", false)
	if SanitizeError(alreadySafe) != error(alreadySafe) {
		t.Fatal("safe error was rewrapped")
	}
	if sanitized := SanitizeError(unauthorized); safeCode(t, sanitized) != CodeAuthenticationUnavailable || strings.Contains(sanitized.Error(), "AUTH_MARKER_SHOULD_NOT_LEAK") {
		t.Fatalf("sanitized text = %q", sanitized.Error())
	}
}

func TestIsExecCredentialErrorMatchesOnlyFixedCategories(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "", want: false},
		{text: "exec: executable kubepeep-helper not found", want: true},
		{text: "exec plugin: invalid apiVersion", want: true},
		{text: "error decoding stdout", want: true},
		{text: "ExecCredential is not valid", want: true},
		{text: "unrelated failure", want: false},
	}
	for _, test := range tests {
		var err error
		if test.text != "" {
			err = errors.New(test.text)
		}
		if got := isExecCredentialError(err); got != test.want {
			t.Fatalf("isExecCredentialError(%q) = %v, want %v", test.text, got, test.want)
		}
	}
}

func TestIsRebuildableAuthenticationErrorDetection(t *testing.T) {
	if IsRebuildableAuthenticationError(nil) {
		t.Fatal("nil error is rebuildable")
	}
	if IsRebuildableAuthenticationError(errors.New("plain failure")) {
		t.Fatal("plain error is rebuildable")
	}
	if !IsRebuildableAuthenticationError(apierrors.NewUnauthorized("marker")) {
		t.Fatal("unauthorized error is not rebuildable")
	}
	_, exitFailure := exec.Command("sh", "-c", "exit 3").Output()
	if !IsRebuildableAuthenticationError(exitFailure) {
		t.Fatal("exit error is not rebuildable")
	}
	if !IsRebuildableAuthenticationError(&exec.Error{Name: "kubepeep-missing-binary", Err: exec.ErrNotFound}) {
		t.Fatal("exec error is not rebuildable")
	}
	if !IsRebuildableAuthenticationError(errors.New("exec plugin is unavailable")) {
		t.Fatal("credential text error is not rebuildable")
	}
	wrapped := fmt.Errorf("transport: %w", errors.New("ExecCredential invalid"))
	if !IsRebuildableAuthenticationError(wrapped) {
		t.Fatal("wrapped credential text error is not rebuildable")
	}
}
