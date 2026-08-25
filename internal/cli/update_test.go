package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	"github.com/fvmoraes/kubepeep/internal/updater"
)

type fakeUpdateService struct {
	request updater.Request
	result  updater.Result
	err     error
	calls   int
}

func (service *fakeUpdateService) Update(_ context.Context, request updater.Request) (updater.Result, error) {
	service.calls++
	service.request = request
	return service.result, service.err
}

func TestUpdateRequiresExactVersionBeforeDownload(t *testing.T) {
	for _, arguments := range [][]string{{"update"}, {"update", "--version", "latest"}, {"update", "--version", "1.2"}} {
		service := &fakeUpdateService{err: updater.ErrInvalidVersion}
		code := ExecuteContext(context.Background(), arguments, Dependencies{
			ResolveLayout: func() (userdirs.Layout, error) { return cliLayout(t), nil },
			Updater:       service, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		})
		if code != ExitInvalid {
			t.Fatalf("arguments=%v exit=%d", arguments, code)
		}
		if len(arguments) == 1 && service.calls != 0 {
			t.Fatalf("missing version called updater %d times", service.calls)
		}
	}
}

func TestUpdateReportsVerifiedSynchronousReplacement(t *testing.T) {
	service := &fakeUpdateService{result: updater.Result{
		CurrentVersion: "0.1.0", TargetVersion: "0.2.0", Archive: "kubePeep_0.2.0_linux_amd64.tar.gz",
	}}
	stdout := &bytes.Buffer{}
	code := ExecuteContext(context.Background(), []string{"update", "--version", "v0.2.0"}, Dependencies{
		ResolveLayout: func() (userdirs.Layout, error) { return cliLayout(t), nil },
		Updater:       service, Stdout: stdout, Stderr: &bytes.Buffer{},
	})
	if code != ExitSuccess || service.calls != 1 {
		t.Fatalf("exit=%d calls=%d", code, service.calls)
	}
	if service.request.TargetVersion != "v0.2.0" || service.request.CurrentVersion == "" {
		t.Fatalf("request=%#v", service.request)
	}
	if stdout.String() != "Updated kubePeep 0.1.0 -> 0.2.0\n" {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestUpdateMapsChecksumFailureWithoutLeakingDetails(t *testing.T) {
	service := &fakeUpdateService{err: errors.Join(updater.ErrChecksum, errors.New("synthetic detail"))}
	stderr := &bytes.Buffer{}
	code := ExecuteContext(context.Background(), []string{"update", "--version", "0.2.0"}, Dependencies{
		ResolveLayout: func() (userdirs.Layout, error) { return cliLayout(t), nil },
		Updater:       service, Stdout: &bytes.Buffer{}, Stderr: stderr,
	})
	if code != ExitOperational || !bytes.Contains(stderr.Bytes(), []byte("SHA-256 verification failed")) {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
}
