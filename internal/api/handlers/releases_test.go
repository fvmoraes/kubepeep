package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/releases"
)

func TestReleaseCheckWorksWithoutClusterSelection(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	releaseCheck(releases.New("dev")).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/releases/latest", nil))
	var envelope struct {
		Data releases.Result `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || envelope.Data.Available || envelope.Data.CurrentVersion != "dev" {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body)
	}
	if methods, known := allowedMethods("/api/v1/releases/latest"); !known || methods != "GET, HEAD" {
		t.Fatalf("incorrect route contract: %q %v", methods, known)
	}
}
