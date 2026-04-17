// Package store 抽象任务/报告/Agent trace 的持久化层。
// v0.1 默认提供 Memory 实现；Postgres 实现保留接口位置（migrations/001_init.sql 已就绪）。
package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"pmhive/server/internal/agent"
	"pmhive/server/internal/stream"
	"pmhive/server/internal/tools/social"
)

// TaskStatus 状态机
type TaskStatus string

const (
	StatusQueued    TaskStatus = "queued"
	StatusRunning   TaskStatus = "running"
	StatusSucceeded TaskStatus = "succeeded"
	StatusFailed    TaskStatus = "failed"
	StatusCancelled TaskStatus = "cancelled"
)

// Scenario MVP 三件套
type Scenario string

const (
	ScenarioCompetitorResearch Scenario = "competitor_research"
	ScenarioInterviewAnalysis  Scenario = "interview_analysis"
	ScenarioPRDDrafting        Scenario = "prd_drafting"
	ScenarioSocialListening    Scenario = "social_listening"
)

// Task 一次研究任务
type Task struct {
	ID           uuid.UUID  `json:"id"`
	ProjectID    *uuid.UUID `json:"project_id,omitempty"`     // v0.7: 项目空间归属
	ParentTaskID *uuid.UUID `json:"parent_task_id,omitempty"` // v0.6: 追问产生的 child task
	Scenario     Scenario   `json:"scenario"`
	Input        string     `json:"input"`
	Status       TaskStatus `json:"status"`
	Stage        string     `json:"stage"`
	Progress     int        `json:"progress"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// Project 项目空间：聚合多个相关 task，沉淀长期记忆
type Project struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// KnowledgeEntry 项目级知识库条目（跨任务沉淀的事实/实体）
type KnowledgeEntry struct {
	ID         int64     `json:"id"`
	ProjectID  uuid.UUID `json:"project_id"`
	SourceTask uuid.UUID `json:"source_task"`
	Kind       string    `json:"kind"`     // entity / fact / quote
	Subject    string    `json:"subject"`  // 实体名 (如 "Notion") / 主题
	Content    string    `json:"content"`  // 描述
	URL        string    `json:"url,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Report 最终产物
type Report struct {
	TaskID    uuid.UUID            `json:"task_id"`
	Title     string               `json:"title"`
	Markdown  string               `json:"markdown"`
	Sources   []agent.Source       `json:"sources"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
	UpdatedAt time.Time            `json:"updated_at"`
}

// PostFilter 列表查询过滤器
type PostFilter struct {
	Platform string // ""=全部
	Keyword  string // 在 title/content 里子串匹配
	Limit    int    // 默认 100
	Offset   int
}

// Store 接口
type Store interface {
	CreateTask(ctx context.Context, t *Task) error
	GetTask(ctx context.Context, id uuid.UUID) (*Task, error)
	ListTasks(ctx context.Context, limit int) ([]*Task, error)
	UpdateTaskStatus(ctx context.Context, id uuid.UUID, status TaskStatus, stage string, progress int, errMsg string) error
	SaveReport(ctx context.Context, r *Report) error
	GetReport(ctx context.Context, taskID uuid.UUID) (*Report, error)
	UpdateReportMarkdown(ctx context.Context, taskID uuid.UUID, mutate func(md string) string) error
	SaveTraces(ctx context.Context, taskID uuid.UUID, events []stream.Event) error
	GetTraces(ctx context.Context, taskID uuid.UUID) ([]stream.Event, error)
	// v0.5+: posts
	SavePosts(ctx context.Context, taskID uuid.UUID, posts []social.Post) error
	ListPosts(ctx context.Context, taskID uuid.UUID, f PostFilter) ([]social.Post, int, error)
	CountPosts(ctx context.Context, taskID uuid.UUID) (int, error)
	// v0.7+: project + knowledge base
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id uuid.UUID) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	ListProjectTasks(ctx context.Context, projectID uuid.UUID) ([]*Task, error)
	AddKnowledge(ctx context.Context, entries []KnowledgeEntry) error
	QueryKnowledge(ctx context.Context, projectID uuid.UUID, keywords []string, limit int) ([]KnowledgeEntry, error)
}

var ErrNotFound = errors.New("not found")

// ---- Memory 实现 ----

type Memory struct {
	mu        sync.RWMutex
	tasks     map[uuid.UUID]*Task
	reports   map[uuid.UUID]*Report
	traces    map[uuid.UUID][]stream.Event
	posts     map[uuid.UUID][]social.Post
	order     []uuid.UUID // 按创建顺序
	projects  map[uuid.UUID]*Project
	projOrder []uuid.UUID
	knowledge []KnowledgeEntry
	knowSeq   int64
}

func NewMemory() *Memory {
	return &Memory{
		tasks:    make(map[uuid.UUID]*Task),
		reports:  make(map[uuid.UUID]*Report),
		traces:   make(map[uuid.UUID][]stream.Event),
		posts:    make(map[uuid.UUID][]social.Post),
		projects: make(map[uuid.UUID]*Project),
	}
}

func (m *Memory) CreateTask(_ context.Context, t *Task) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = StatusQueued
	}
	// 内部存副本，避免 caller 持有的指针被 worker 异步修改造成 race
	internal := *t
	m.mu.Lock()
	m.tasks[internal.ID] = &internal
	m.order = append(m.order, internal.ID)
	m.mu.Unlock()
	return nil
}

func (m *Memory) GetTask(_ context.Context, id uuid.UUID) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (m *Memory) ListTasks(_ context.Context, limit int) ([]*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	out := make([]*Task, 0, limit)
	for i := len(m.order) - 1; i >= 0 && len(out) < limit; i-- {
		t := m.tasks[m.order[i]]
		if t == nil {
			continue
		}
		cp := *t
		out = append(out, &cp)
	}
	return out, nil
}

func (m *Memory) UpdateTaskStatus(_ context.Context, id uuid.UUID, status TaskStatus, stage string, progress int, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return ErrNotFound
	}
	t.Status = status
	if stage != "" {
		t.Stage = stage
	}
	if progress >= 0 {
		t.Progress = progress
	}
	if errMsg != "" {
		t.Error = errMsg
	}
	t.UpdatedAt = time.Now()
	if status == StatusSucceeded || status == StatusFailed || status == StatusCancelled {
		now := time.Now()
		t.FinishedAt = &now
	}
	return nil
}

func (m *Memory) SaveReport(_ context.Context, r *Report) error {
	r.UpdatedAt = time.Now()
	m.mu.Lock()
	m.reports[r.TaskID] = r
	m.mu.Unlock()
	return nil
}

func (m *Memory) GetReport(_ context.Context, taskID uuid.UUID) (*Report, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.reports[taskID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (m *Memory) SaveTraces(_ context.Context, taskID uuid.UUID, events []stream.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]stream.Event(nil), events...)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].Seq < cp[j].Seq })
	m.traces[taskID] = cp
	return nil
}

func (m *Memory) GetTraces(_ context.Context, taskID uuid.UUID) ([]stream.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := append([]stream.Event(nil), m.traces[taskID]...)
	return out, nil
}

// SavePosts 追加保存（worker 可分批调用，dedup 由 caller 负责）
func (m *Memory) SavePosts(_ context.Context, taskID uuid.UUID, posts []social.Post) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]social.Post(nil), posts...)
	m.posts[taskID] = append(m.posts[taskID], cp...)
	return nil
}

func (m *Memory) ListPosts(_ context.Context, taskID uuid.UUID, f PostFilter) ([]social.Post, int, error) {
	m.mu.RLock()
	all := m.posts[taskID]
	m.mu.RUnlock()

	// 过滤
	plat := strings.ToLower(strings.TrimSpace(f.Platform))
	kw := strings.ToLower(strings.TrimSpace(f.Keyword))
	filtered := make([]social.Post, 0, len(all))
	for _, p := range all {
		if plat != "" && strings.ToLower(p.Platform) != plat {
			continue
		}
		if kw != "" {
			hay := strings.ToLower(p.Title + " " + p.Content + " " + p.Author)
			if !strings.Contains(hay, kw) {
				continue
			}
		}
		filtered = append(filtered, p)
	}

	total := len(filtered)
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	off := f.Offset
	if off < 0 {
		off = 0
	}
	if off >= total {
		return []social.Post{}, total, nil
	}
	end := off + limit
	if end > total {
		end = total
	}
	out := append([]social.Post(nil), filtered[off:end]...)
	return out, total, nil
}

func (m *Memory) CountPosts(_ context.Context, taskID uuid.UUID) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.posts[taskID]), nil
}

// UpdateReportMarkdown 原地修改 report markdown（追问 merge 用）
func (m *Memory) UpdateReportMarkdown(_ context.Context, taskID uuid.UUID, mutate func(string) string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reports[taskID]
	if !ok {
		return ErrNotFound
	}
	r.Markdown = mutate(r.Markdown)
	r.UpdatedAt = time.Now()
	return nil
}

// ===== Project =====

func (m *Memory) CreateProject(_ context.Context, p *Project) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	internal := *p
	m.mu.Lock()
	m.projects[internal.ID] = &internal
	m.projOrder = append(m.projOrder, internal.ID)
	m.mu.Unlock()
	return nil
}

func (m *Memory) GetProject(_ context.Context, id uuid.UUID) (*Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.projects[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *Memory) ListProjects(_ context.Context) ([]*Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Project, 0, len(m.projOrder))
	for i := len(m.projOrder) - 1; i >= 0; i-- {
		if p, ok := m.projects[m.projOrder[i]]; ok {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *Memory) ListProjectTasks(_ context.Context, projectID uuid.UUID) ([]*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Task, 0)
	for i := len(m.order) - 1; i >= 0; i-- {
		t := m.tasks[m.order[i]]
		if t == nil || t.ProjectID == nil {
			continue
		}
		if *t.ProjectID == projectID {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

// ===== Knowledge base =====

func (m *Memory) AddKnowledge(_ context.Context, entries []KnowledgeEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for _, e := range entries {
		m.knowSeq++
		e.ID = m.knowSeq
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
		m.knowledge = append(m.knowledge, e)
	}
	return nil
}

// QueryKnowledge 简单关键词匹配（无 embedding，v0.7 PG 升级时再上 pgvector）
func (m *Memory) QueryKnowledge(_ context.Context, projectID uuid.UUID, keywords []string, limit int) ([]KnowledgeEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	keys := make([]string, 0, len(keywords))
	for _, k := range keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if len(k) >= 2 {
			keys = append(keys, k)
		}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]KnowledgeEntry, 0, limit)
	for _, e := range m.knowledge {
		if e.ProjectID != projectID {
			continue
		}
		if len(keys) > 0 {
			hay := strings.ToLower(e.Subject + " " + e.Content)
			matched := false
			for _, k := range keys {
				if strings.Contains(hay, k) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
