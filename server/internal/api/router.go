// Package api 提供 HTTP 接口：任务 CRUD、SSE 流、报告查询。
package api

import (
	"net/http"
	"strings"

	"pmhive/server/internal/integrations/jira"
	"pmhive/server/internal/integrations/slack"
	"pmhive/server/internal/jobs"
	"pmhive/server/internal/llm"
	"pmhive/server/internal/store"
	"pmhive/server/internal/stream"
)

type Server struct {
	Store       store.Store
	Bus         *stream.Bus
	Worker      *jobs.Worker
	Slack       *slack.Client
	Jira        *jira.Client
	Usage       *llm.Recorder // #8 token/cost 追踪
	CORSAllowed []string
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("GET /api/tasks/{id}/stream", s.handleStreamTask)
	mux.HandleFunc("GET /api/tasks/{id}/report", s.handleGetReport)
	mux.HandleFunc("GET /api/tasks/{id}/traces", s.handleGetTraces)
	mux.HandleFunc("GET /api/tasks/{id}/posts", s.handleGetPosts)
	mux.HandleFunc("POST /api/tasks/{id}/followup", s.handleFollowUp)

	// Projects
	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("GET /api/projects", s.handleListProjects)
	mux.HandleFunc("GET /api/projects/{id}/tasks", s.handleListProjectTasks)

	// Integrations (Jira / Slack)
	mux.HandleFunc("POST /api/integrations/slack/notify", s.handleSlackNotify)
	mux.HandleFunc("POST /api/integrations/jira/issue", s.handleJiraCreateIssue)
	mux.HandleFunc("POST /api/webhooks/jira", s.handleJiraWebhook)
	mux.HandleFunc("GET /api/integrations/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.integrationStatus())
	})

	// v0.5: agent role registry
	mux.HandleFunc("GET /api/agents/roles", s.handleGetRoles)
	mux.HandleFunc("GET /api/agents/roles/{scenario}", s.handleGetRolesByScenario)
	mux.HandleFunc("GET /api/tasks/{id}/usage", s.handleGetUsage)

	return s.withCORS(mux)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := ""
		for _, a := range s.CORSAllowed {
			if a == "*" || strings.EqualFold(a, origin) {
				allowed = origin
				if a == "*" {
					allowed = "*"
				}
				break
			}
		}
		if allowed != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Last-Event-ID")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
