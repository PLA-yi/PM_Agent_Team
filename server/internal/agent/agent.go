// Package agent 实现轻量级多 Agent 编排：每个 Agent 是一个 Run(ctx,state) 函数，
// Graph 按依赖顺序串行/并行执行。设计目标：可读、可测试、可替换为 Eino 而不破坏外层 API。
package agent

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"pmhive/server/internal/llm"
	socialpkg "pmhive/server/internal/tools/social"
	"pmhive/server/internal/stream"
	"pmhive/server/internal/tools"
)

// Source 在调研过程中收集到的引用
type Source struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// Competitor 提取出的结构化竞品
type Competitor struct {
	Name       string   `json:"name"`
	Pricing    string   `json:"pricing"`
	AI         bool     `json:"ai"`
	Strengths  []string `json:"strengths"`
	Weaknesses []string `json:"weaknesses"`
	URL        string   `json:"url,omitempty"`
}

// Candidate 候选竞品（Planner 输出 → Search/Scraper/Social 消费）
type Candidate struct {
	Name   string `json:"name"`              // 主显示名（通常中文）
	NameEN string `json:"name_en,omitempty"` // 英文名（社交平台 query 必备）
	URL    string `json:"url"`
}

// SearchAlias 返回搜索查询用的最佳别名（NameEN 优先，否则 Name）
func (c Candidate) SearchAlias() string {
	if c.NameEN != "" {
		return c.NameEN
	}
	return c.Name
}

// AllAliases 返回所有别名（用于相关性匹配）
func (c Candidate) AllAliases() []string {
	out := []string{c.Name}
	if c.NameEN != "" && c.NameEN != c.Name {
		out = append(out, c.NameEN)
	}
	return out
}

// Insight 用户访谈洞察聚类
type Insight struct {
	Theme      string   `json:"theme"`      // 主题
	Frequency  int      `json:"frequency"`  // 提及次数
	Quotes     []string `json:"quotes"`     // 原话引用
	NeedLevel  string   `json:"need_level"` // critical / important / nice-to-have
	Confidence float64  `json:"confidence"` // 0-1
}

// PRDSection PRD 段落
type PRDSection struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// State 在 Agent 之间流转的共享状态。
// 不同 scenario 用不同字段子集；未用字段保持零值。
type State struct {
	TaskID uuid.UUID
	Input  string

	// 竞品调研专用
	Outline       []string
	Candidates    []Candidate
	SearchResults []tools.SearchResult
	Pages         map[string]string // url -> markdown
	Sources       []Source
	Competitors   []Competitor
	Analysis      map[string]interface{}

	// 用户访谈分析专用
	InterviewChunks []string
	Insights        []Insight

	// 社交聆听（也用于 competitor_research 的"用户原声"补充）
	Posts []socialpkg.Post

	// PRD 起草专用
	PRDBackground string
	UserStories   []string
	PRDSections   []PRDSection

	// 共用：最终报告（Markdown）
	Report string

	// Reviewer 评分（v0.6+）
	Review *ReviewResult

	// v0.7+: 项目级长期记忆（来自 Project KB，跑任务前由 worker 注入）
	PriorContext string

	mu sync.Mutex
}

// ReviewResult Reviewer Agent 给报告打的分
type ReviewResult struct {
	OverallScore   float64  `json:"overall_score"`   // 0-10 综合
	FactScore      float64  `json:"fact_score"`      // 事实准确性
	CoverageScore  float64  `json:"coverage_score"`  // 覆盖度
	CitationScore  float64  `json:"citation_score"`  // 引用质量
	Strengths      []string `json:"strengths"`
	Issues         []string `json:"issues"`          // 缺陷列表（喂给 Writer 重写用）
	Verdict        string   `json:"verdict"`         // accept / revise / reject
	Iteration      int      `json:"iteration"`       // 第几轮 review（1 = 首版，2 = 重写后）
}

func (s *State) AddSource(src Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.Sources {
		if e.URL == src.URL {
			return
		}
	}
	s.Sources = append(s.Sources, src)
}

// Deps 注入运行时依赖
type Deps struct {
	LLM    llm.Client
	Search tools.Searcher
	Scrape tools.Scraper
	Social *socialpkg.Registry // v0.5+：社交聆听数据源
	Bus    *stream.Bus
	Model  string
}

// Agent 接口
type Agent interface {
	Name() string
	Run(ctx context.Context, st *State, d Deps) error
}

// publish 是 Agent 发布事件的便捷方法
func publish(d Deps, st *State, agentName, step string, payload interface{}) {
	d.Bus.Publish(st.TaskID, agentName, step, payload)
}

// shortPreview 截短 payload 给 timeline 显示
func shortPreview(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
