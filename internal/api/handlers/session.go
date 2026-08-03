package handlers

import (
	"net/http"

	"github.com/fvmoraes/ginger/pkg/response"

	"github.com/fvmoraes/kubepeep/internal/api"
)

type Session struct {
	sessions   *api.SessionStore
	generation api.GenerationSource
	origin     string
}

func NewSession(sessions *api.SessionStore, generation api.GenerationSource, origin string) *Session {
	return &Session{sessions: sessions, generation: generation, origin: origin}
}

func (h *Session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	session, err := h.sessions.Current(h.origin, h.generation.Current())
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	response.OK(w, session)
}
