package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
)

type fakeClusterProfiles struct {
	profiles []clusterprofiles.DTO
	active   clusterprofiles.DTO
	err      error
}

func (f fakeClusterProfiles) List(context.Context) ([]clusterprofiles.DTO, error) {
	return f.profiles, f.err
}

func (f fakeClusterProfiles) Active(context.Context) (clusterprofiles.DTO, error) {
	return f.active, f.err
}

func TestClusterProfilesExposeOnlySanitizedDTOs(t *testing.T) {
	service := fakeClusterProfiles{profiles: []clusterprofiles.DTO{{
		ID: 1, Name: "Development", IsDefault: true,
		KubeconfigFiles: []clusterprofiles.FileDTO{{Position: 0, DisplayPath: "~/.kube/config"}},
	}}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/profiles", nil)
	recorder := httptest.NewRecorder()
	NewClusterProfiles(service).List(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d headers=%v", recorder.Code, recorder.Header())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	encoded := recorder.Body.String()
	for _, forbidden := range []string{"client-certificate-data", "token", "fingerprint"} {
		if contains(encoded, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestActiveClusterProfileMapsMissingToStable404(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/profile", nil)
	recorder := httptest.NewRecorder()
	NewClusterProfiles(fakeClusterProfiles{err: clusterprofiles.ErrNotFound}).Active(recorder, request)
	if recorder.Code != http.StatusNotFound || !contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	NewClusterProfiles(fakeClusterProfiles{err: errors.New("sensitive internal")}).List(recorder, request)
	if recorder.Code != http.StatusInternalServerError || contains(recorder.Body.String(), "sensitive internal") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func contains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}

type fakeProfileImporter struct {
	fakeClusterProfiles
	calls     int
	importErr error
}

func (f *fakeProfileImporter) Import(context.Context, clusterprofiles.ImportRequest) (clusterprofiles.DTO, error) {
	f.calls++
	return clusterprofiles.DTO{ID: 2, Name: "Imported", KubeconfigFiles: []clusterprofiles.FileDTO{{Position: 0, DisplayPath: "~/.kube/config"}}}, f.importErr
}

func TestKubeconfigImportRequestBoundary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body    string
		err           error
		status, calls int
	}{
		{"source", `{"path":"~/clusters/dev.yaml"}`, nil, 200, 1},
		{"content", `{"content":"private-value"}`, nil, 200, 1},
		{"conflict", `{"content":"private-value"}`, clusterprofiles.ErrImportConflict, 409, 1},
		{"relative references", `{"content":"private-value"}`, clusterprofiles.ErrImportRelativePaths, 400, 1},
		{"invalid config", `{"content":"private-value"}`, clusterprofiles.ErrImportInvalid, 400, 1},
		{"internal error", `{"content":"private-value"}`, errors.New("private-value"), 503, 1},
		{"unknown field", `{"path":"~/config", "arbitrary":"private-value"}`, nil, 400, 0},
		{"trailing body", `{"path":"~/config"} {}`, nil, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeProfileImporter{importErr: tc.err}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/profiles/import", strings.NewReader(tc.body))
			recorder := httptest.NewRecorder()
			NewClusterProfiles(service).Import(recorder, request)
			if recorder.Code != tc.status || service.calls != tc.calls {
				t.Fatalf("status=%d calls=%d", recorder.Code, service.calls)
			}
			if strings.Contains(recorder.Body.String(), "private-value") {
				t.Fatal("import response exposed private content")
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("import response must not be cached")
			}
		})
	}
	if methods, exists := allowedMethods("/api/v1/cluster/profiles/import"); !exists || methods != "POST" {
		t.Fatalf("import methods = %q, %v", methods, exists)
	}
}
