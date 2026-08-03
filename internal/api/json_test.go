package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeStrictClassifiesUnknownAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "unknown", body: `{"name":"ok","extra":true}`, code: CodeUnknownField},
		{name: "trailing", body: `{"name":"ok"} {}`, code: CodeInvalidJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(test.body))
			w := httptest.NewRecorder()
			var body struct {
				Name string `json:"name"`
			}
			err := DecodeStrict(w, r, &body, 1024)
			var httpError *HTTPError
			if !errors.As(err, &httpError) || string(httpError.AppError.Code) != test.code {
				t.Fatalf("error = %#v, want %s", err, test.code)
			}
		})
	}
}

func TestDecodeStrictCoversEmptyTypeAndSizeFailures(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		limit  int64
		status int
		code   string
	}{
		{name: "empty", body: "", limit: 1024, status: http.StatusBadRequest, code: CodeInvalidJSON},
		{name: "type", body: `{"name":42}`, limit: 1024, status: http.StatusBadRequest, code: CodeValidationFailed},
		{name: "oversized", body: `{"name":"larger than limit"}`, limit: 8, status: http.StatusRequestEntityTooLarge, code: CodeBodyTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			w := httptest.NewRecorder()
			var body struct {
				Name string `json:"name"`
			}
			err := DecodeStrict(w, r, &body, test.limit)
			var httpError *HTTPError
			if !errors.As(err, &httpError) || httpError.Status != test.status || string(httpError.AppError.Code) != test.code {
				t.Fatalf("error = %#v, want status=%d code=%s", err, test.status, test.code)
			}
		})
	}
}
