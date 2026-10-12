package handlers

import (
	"net/http"

	"github.com/fvmoraes/ginger/pkg/response"
	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/releases"
)

func releaseCheck(checker *releases.Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := checker.Check(r.Context())
		if err != nil {
			api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeInternal, "Release check is temporarily unavailable.", nil, err))
			return
		}
		response.OK(w, result)
	}
}
