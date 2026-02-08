package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deicod/gopenusage/pkg/openusage"
)

type Server struct {
	manager *openusage.Manager
	mux     *http.ServeMux
}

func NewServer(manager *openusage.Manager) *Server {
	s := &Server{manager: manager, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/v1/usage", s.handleUsage)
	s.mux.HandleFunc("/v1/usage/", s.handleUsageByPlugin)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ids := parseIDs(strings.TrimSpace(r.URL.Query().Get("plugins")))
	outputs, err := s.manager.QueryAll(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, outputs)
}

func (s *Server) handleUsageByPlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	prefix := "/v1/usage/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	pluginID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if pluginID == "" {
		writeError(w, http.StatusBadRequest, "plugin id is required")
		return
	}
	if !s.manager.HasPlugin(pluginID) {
		writeError(w, http.StatusNotFound, "unknown plugin")
		return
	}

	output, err := s.manager.QueryOne(r.Context(), pluginID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, output)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
}
