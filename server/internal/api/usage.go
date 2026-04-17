package api

import (
	"net/http"

	"github.com/google/uuid"
)

func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if s.Usage == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": id.String(), "calls": 0})
		return
	}
	usage := s.Usage.Get(id.String())
	if usage == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": id.String(), "calls": 0})
		return
	}
	writeJSON(w, http.StatusOK, usage)
}
