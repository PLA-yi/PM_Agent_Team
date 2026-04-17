package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"pmhive/server/internal/store"
)

type createTaskRequest struct {
	Scenario  string `json:"scenario"`
	Input     string `json:"input"`
	ProjectID string `json:"project_id,omitempty"` // v0.7+
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Input) == "" {
		writeErr(w, http.StatusBadRequest, "input required")
		return
	}
	if req.Scenario == "" {
		req.Scenario = string(store.ScenarioCompetitorResearch)
	}
	switch req.Scenario {
	case string(store.ScenarioCompetitorResearch),
		string(store.ScenarioInterviewAnalysis),
		string(store.ScenarioPRDDrafting),
		string(store.ScenarioSocialListening),
		string(store.ScenarioRequirementAnalysis),
		string(store.ScenarioRequirementValidation):
		// ok
	default:
		writeErr(w, http.StatusBadRequest, "unknown scenario: "+req.Scenario)
		return
	}

	t := &store.Task{
		Scenario: store.Scenario(req.Scenario),
		Input:    req.Input,
		Status:   store.StatusQueued,
		Stage:    "queued",
	}
	if req.ProjectID != "" {
		pid, err := uuid.Parse(req.ProjectID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid project_id")
			return
		}
		t.ProjectID = &pid
	}
	if err := s.Store.CreateTask(r.Context(), t); err != nil {
		writeErr(w, http.StatusInternalServerError, "create: "+err.Error())
		return
	}
	s.Worker.Enqueue(t.ID)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tasks, err := s.Store.ListTasks(r.Context(), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tasks": tasks})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := s.Store.GetTask(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "task not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleGetReport(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	rep, err := s.Store.GetReport(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "report not ready")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleGetTraces(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	// 优先 bus 的实时 history；为空再读 store（已完成任务）
	hist := s.Bus.History(id)
	if len(hist) == 0 {
		got, err := s.Store.GetTraces(r.Context(), id)
		if err == nil {
			hist = got
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"traces": hist})
}

// FollowUp 在已完成任务上追问 → spawn 一个 child task
type followUpRequest struct {
	Input string `json:"input"`
}

func (s *Server) handleFollowUp(w http.ResponseWriter, r *http.Request) {
	parentID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	parent, err := s.Store.GetTask(r.Context(), parentID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "parent task not found")
		return
	}
	var req followUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.Input) == "" {
		writeErr(w, http.StatusBadRequest, "input required")
		return
	}

	// 复用同 scenario，组合成「父任务 input + 追问」
	combined := fmt.Sprintf("[追问 - 基于「%s」] %s", parent.Input, req.Input)
	t := &store.Task{
		ParentTaskID: &parentID,
		ProjectID:    parent.ProjectID, // 继承父任务 project
		Scenario:     parent.Scenario,
		Input:        combined,
		Status:       store.StatusQueued,
		Stage:        "queued",
	}
	if err := s.Store.CreateTask(r.Context(), t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Worker.Enqueue(t.ID)
	writeJSON(w, http.StatusCreated, t)
}

// 列出某任务抓取的 social posts，支持 platform/keyword/limit/offset
func (s *Server) handleGetPosts(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	posts, total, err := s.Store.ListPosts(r.Context(), id, store.PostFilter{
		Platform: q.Get("platform"),
		Keyword:  q.Get("q"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"posts":  posts,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// SSE 端点：实时推送 agent 协作事件
func (s *Server) handleStreamTask(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// since: 客户端断线重连时带上最后看到的 seq
	since := int64(0)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx 兼容
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsub := s.Bus.Subscribe(r.Context(), id, since)
	defer unsub()

	// 心跳，防止代理超时
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, _ := json.Marshal(ev)
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Step, payload)
			flusher.Flush()
			// 任务终态 → 关闭流
			if ev.Agent == "system" && (ev.Step == "task_succeeded" || ev.Step == "task_failed") {
				return
			}
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
