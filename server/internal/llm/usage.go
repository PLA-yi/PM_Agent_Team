// Package llm — usage tracking 基础设施 (#8 Token/Cost 追踪)
//
// 设计原则：
//   - LLM 调用必须可观测：每次记 model / agent / tokens / cost / elapsed
//   - 按 task_id 聚合 → 任务粒度的"我烧了多少"
//   - Cost 是估算（按公开 pricing 表），不是 provider 真实账单
//   - 无 key (mock) 时 cost = 0，但 token 仍记
package llm

import (
	"sync"
	"time"
)

// CallRecord 单次 LLM 调用记录
type CallRecord struct {
	TaskID       string    `json:"task_id"`
	Agent        string    `json:"agent"`         // 调用方 agent 名（planner/extractor/...）
	Model        string    `json:"model"`
	Provider     string    `json:"provider"`      // anthropic / openrouter / aihubmix / mock
	PromptTokens int       `json:"prompt_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	CostUSD      float64   `json:"cost_usd"`      // 估算
	ElapsedMs    int64     `json:"elapsed_ms"`
	At           time.Time `json:"at"`
	Err          string    `json:"err,omitempty"`
}

// TaskUsage 一个任务的累计用量
type TaskUsage struct {
	TaskID       string                  `json:"task_id"`
	Calls        int                     `json:"calls"`
	PromptTokens int                     `json:"prompt_tokens"`
	OutputTokens int                     `json:"output_tokens"`
	TotalTokens  int                     `json:"total_tokens"`
	CostUSD      float64                 `json:"cost_usd"`
	ElapsedMs    int64                   `json:"elapsed_ms"`
	ByAgent      map[string]*AgentUsage  `json:"by_agent"`
	ByModel      map[string]*ModelUsage  `json:"by_model"`
	Records      []CallRecord            `json:"records,omitempty"` // 详细历史
	BudgetUSD    float64                 `json:"budget_usd,omitempty"`
	BudgetExceeded bool                  `json:"budget_exceeded,omitempty"`
}

type AgentUsage struct {
	Calls       int     `json:"calls"`
	TotalTokens int     `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

type ModelUsage struct {
	Calls       int     `json:"calls"`
	TotalTokens int     `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// Recorder 全局并发安全的 usage 记录器
type Recorder struct {
	mu    sync.RWMutex
	tasks map[string]*TaskUsage
	// 全局任务级硬上限（USD）；> 0 时启用，超出抛 ErrBudgetExceeded
	defaultBudget float64
}

// ErrBudgetExceeded budget 超限错误，调用方应停止后续 LLM 调用
var ErrBudgetExceeded = &budgetErr{"budget exceeded"}

type budgetErr struct{ msg string }

func (e *budgetErr) Error() string { return e.msg }

// NewRecorder 创建（全局单例风格使用即可）
func NewRecorder(defaultBudgetUSD float64) *Recorder {
	return &Recorder{
		tasks:         make(map[string]*TaskUsage),
		defaultBudget: defaultBudgetUSD,
	}
}

// Record 记录一次调用；同时按 task 累加 + 检查 budget
// 返回 true 表示当前任务还在预算内，false 表示已超
func (r *Recorder) Record(rec CallRecord) bool {
	if rec.TaskID == "" {
		return true // task 维度无关的调用（如 server-level test）不影响 budget
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[rec.TaskID]
	if !ok {
		t = &TaskUsage{
			TaskID:    rec.TaskID,
			ByAgent:   make(map[string]*AgentUsage),
			ByModel:   make(map[string]*ModelUsage),
			BudgetUSD: r.defaultBudget,
		}
		r.tasks[rec.TaskID] = t
	}
	t.Calls++
	t.PromptTokens += rec.PromptTokens
	t.OutputTokens += rec.OutputTokens
	t.TotalTokens += rec.TotalTokens
	t.CostUSD += rec.CostUSD
	t.ElapsedMs += rec.ElapsedMs

	// per-agent
	if rec.Agent != "" {
		ag, ok := t.ByAgent[rec.Agent]
		if !ok {
			ag = &AgentUsage{}
			t.ByAgent[rec.Agent] = ag
		}
		ag.Calls++
		ag.TotalTokens += rec.TotalTokens
		ag.CostUSD += rec.CostUSD
	}
	// per-model
	if rec.Model != "" {
		m, ok := t.ByModel[rec.Model]
		if !ok {
			m = &ModelUsage{}
			t.ByModel[rec.Model] = m
		}
		m.Calls++
		m.TotalTokens += rec.TotalTokens
		m.CostUSD += rec.CostUSD
	}
	t.Records = append(t.Records, rec)

	// budget check
	if t.BudgetUSD > 0 && t.CostUSD > t.BudgetUSD {
		t.BudgetExceeded = true
		return false
	}
	return true
}

// Get 拿到任务用量快照
func (r *Recorder) Get(taskID string) *TaskUsage {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[taskID]
	if !ok {
		return nil
	}
	// shallow copy
	cp := *t
	cp.ByAgent = make(map[string]*AgentUsage, len(t.ByAgent))
	for k, v := range t.ByAgent {
		x := *v
		cp.ByAgent[k] = &x
	}
	cp.ByModel = make(map[string]*ModelUsage, len(t.ByModel))
	for k, v := range t.ByModel {
		x := *v
		cp.ByModel[k] = &x
	}
	cp.Records = append([]CallRecord(nil), t.Records...)
	return &cp
}

// SetBudget 单任务设置自定义 budget
func (r *Recorder) SetBudget(taskID string, usd float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[taskID]
	if !ok {
		t = &TaskUsage{TaskID: taskID, ByAgent: map[string]*AgentUsage{}, ByModel: map[string]*ModelUsage{}}
		r.tasks[taskID] = t
	}
	t.BudgetUSD = usd
}

// ==== Pricing 表（USD per 1K tokens；公开 list price，不是真实账单）====
// 数据来源：各家官方 pricing 页面。未匹配的模型按 0 估算（不影响功能，只是 cost = 0）

type modelPrice struct {
	in  float64 // per 1K input tokens
	out float64 // per 1K output tokens
}

var pricing = map[string]modelPrice{
	// Anthropic Claude (USD per 1M token / 1000 = per 1K)
	"claude-sonnet-4-5":          {in: 0.003, out: 0.015},
	"claude-sonnet-4.5":          {in: 0.003, out: 0.015},
	"claude-opus-4":              {in: 0.015, out: 0.075},
	"claude-3-5-sonnet-20241022": {in: 0.003, out: 0.015},
	"claude-3-5-haiku-20241022":  {in: 0.0008, out: 0.004},
	// OpenAI / OpenRouter passthrough（举例几个常用）
	"gpt-4o":                     {in: 0.0025, out: 0.01},
	"gpt-4o-mini":                {in: 0.00015, out: 0.0006},
	"openai/gpt-4o":              {in: 0.0025, out: 0.01},
	"anthropic/claude-sonnet-4.5": {in: 0.003, out: 0.015},
	"anthropic/claude-opus-4":    {in: 0.015, out: 0.075},
	// DeepSeek
	"deepseek-chat":              {in: 0.00027, out: 0.0011},
	"deepseek/deepseek-chat":     {in: 0.00027, out: 0.0011},
}

// EstimateCost 按 model + token 数估算 USD
func EstimateCost(model string, in, out int) float64 {
	p, ok := pricing[model]
	if !ok {
		return 0 // 未知模型 cost = 0（不阻断主流程）
	}
	return float64(in)/1000*p.in + float64(out)/1000*p.out
}

// PricingTable 暴露给前端展示
func PricingTable() map[string]modelPrice {
	out := make(map[string]modelPrice, len(pricing))
	for k, v := range pricing {
		out[k] = v
	}
	return out
}
