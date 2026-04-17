package api

import (
	"net/http"

	"pmhive/server/internal/agent"
)

func (s *Server) handleGetRoles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"roles": agent.RoleRegistry,
	})
}

func (s *Server) handleGetRolesByScenario(w http.ResponseWriter, r *http.Request) {
	sc := r.PathValue("scenario")
	roles := agent.RolesForScenario(sc)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scenario": sc,
		"roles":    roles,
	})
}
