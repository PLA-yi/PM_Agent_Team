package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"pmhive/server/internal/integrations/jira"
	"pmhive/server/internal/integrations/slack"
	"pmhive/server/internal/store"
)

// ===== Slack =====

type slackNotifyReq struct {
	TaskID string `json:"task_id"`
}

func (s *Server) handleSlackNotify(w http.ResponseWriter, r *http.Request) {
	if !s.Slack.IsConfigured() {
		writeErr(w, http.StatusPreconditionFailed, "slack: webhook URL not configured (set SLACK_WEBHOOK_URL)")
		return
	}
	var req slackNotifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	tid, err := uuid.Parse(req.TaskID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid task_id")
		return
	}
	t, err := s.Store.GetTask(r.Context(), tid)
	if err != nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	rep, err := s.Store.GetReport(r.Context(), tid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "report not ready")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var score *float64
	if rep.Metadata != nil {
		if rev, ok := rep.Metadata["review"].(*store.Report); ok {
			_ = rev
		}
		// metadata.review 是 *agent.ReviewResult，做柔性提取
		if reviewMap, ok := rep.Metadata["review"].(map[string]interface{}); ok {
			if v, ok := reviewMap["overall_score"].(float64); ok {
				score = &v
			}
		}
	}
	err = s.Slack.PostTaskSummary(r.Context(), rep.Title, t.Input, rep.Markdown, "", score)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

// ===== Jira create issue =====

type jiraCreateReq struct {
	TaskID      string `json:"task_id"`
	ProjectKey  string `json:"project_key"`
	IssueType   string `json:"issue_type,omitempty"` // 默认 Task
}

func (s *Server) handleJiraCreateIssue(w http.ResponseWriter, r *http.Request) {
	if !s.Jira.IsConfigured() {
		writeErr(w, http.StatusPreconditionFailed, "jira: not configured (set JIRA_BASE_URL/JIRA_EMAIL/JIRA_API_TOKEN)")
		return
	}
	var req jiraCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	tid, err := uuid.Parse(req.TaskID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid task_id")
		return
	}
	if req.ProjectKey == "" {
		writeErr(w, http.StatusBadRequest, "project_key required")
		return
	}
	if req.IssueType == "" {
		req.IssueType = "Task"
	}
	t, _ := s.Store.GetTask(r.Context(), tid)
	rep, err := s.Store.GetReport(r.Context(), tid)
	if err != nil {
		writeErr(w, http.StatusNotFound, "report not ready")
		return
	}
	desc := fmt.Sprintf("PMHive Task: %s\n\n%s", t.Input, rep.Markdown)
	out, err := s.Jira.CreateIssue(r.Context(), jira.CreateIssueRequest{
		ProjectKey:  req.ProjectKey,
		IssueType:   req.IssueType,
		Summary:     rep.Title,
		Description: desc,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ===== Jira webhook receiver =====
// Atlassian → Webhook → POST 到 /api/webhooks/jira
// 收到 issue.created 时，自动 spawn PMHive task 调研 issue summary

type jiraWebhookEvent struct {
	WebhookEvent string `json:"webhookEvent"` // jira:issue_created / jira:issue_updated
	Issue        struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
		} `json:"fields"`
	} `json:"issue"`
}

func (s *Server) handleJiraWebhook(w http.ResponseWriter, r *http.Request) {
	var ev jiraWebhookEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if ev.WebhookEvent != "jira:issue_created" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": ev.WebhookEvent})
		return
	}
	// 把 issue summary 当 PRD 起草任务自动跑
	t := &store.Task{
		Scenario: store.ScenarioPRDDrafting,
		Input:    fmt.Sprintf("%s — %s", ev.Issue.Fields.Summary, ev.Issue.Fields.Description),
		Status:   store.StatusQueued,
		Stage:    "queued",
	}
	if err := s.Store.CreateTask(r.Context(), t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Worker.Enqueue(t.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":         "task_spawned",
		"task_id":        t.ID,
		"jira_issue_key": ev.Issue.Key,
	})
}

// ===== Slack helper for client side =====
// 留作返回 webhook 是否已配置的 capability check
type integrationStatus struct {
	Slack bool `json:"slack"`
	Jira  bool `json:"jira"`
}

// 公开能力（前端展示集成状态用）
func (s *Server) integrationStatus() integrationStatus {
	return integrationStatus{
		Slack: s.Slack != nil && s.Slack.IsConfigured(),
		Jira:  s.Jira != nil && s.Jira.IsConfigured(),
	}
}

var _ = slack.New // 避免 unused import
