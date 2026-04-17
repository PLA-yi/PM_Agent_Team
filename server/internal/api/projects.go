package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"pmhive/server/internal/store"
)

type createProjectReq struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	p := &store.Project{Name: req.Name, Description: req.Description}
	if err := s.Store.CreateProject(r.Context(), p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Store.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"projects": ps})
}

func (s *Server) handleListProjectTasks(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	ts, err := s.Store.ListProjectTasks(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": ts})
}
