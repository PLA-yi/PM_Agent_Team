package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"pmhive/server/internal/llm"
)

// ============================================================================
// 需求分析模块（3 阶段）
//   1. RequirementDiscoverer  — 找需求（从输入产品/市场上下文挖潜在需求）
//   2. RequirementAnalyzer    — 深度分析（每条需求的 JTBD/痛点/用户群）
//   3. RequirementPrioritizer — RICE + Kano 排序
// ============================================================================

// ----- 1. Discoverer ------

type RequirementDiscoverer struct{}

func (RequirementDiscoverer) Name() string { return "planner" } // 复用 planner timeline 颜色

const discovererSystem = `You are PLANNER_AGENT (REQUIREMENT_DISCOVERER).
PM 给出产品/市场上下文，你的任务是**列出 6-10 条潜在需求**。

来源信号优先级：
- user_voice: 真实用户反馈 (访谈/社区)
- market_gap: 行业空白
- inferred: 基于 JTBD 推理

每条需求要可执行（不是"提升用户体验"这种空话）。

输出 JSON:
{
  "requirements": [
    {
      "id": "R001",
      "title": "需求一句话标题",
      "source": "user_voice|market_gap|inferred",
      "user_segment": "目标用户群（具体）",
      "jtbd": "When ___, I want to ___, so I can ___",
      "painpoint": "当前痛点（一句话）",
      "frequency": "daily|weekly|occasional"
    }
  ]
}

Output ONLY valid JSON.`

func (RequirementDiscoverer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "planner", "start", map[string]string{"input": st.Input})
	user := st.Input
	if st.PriorContext != "" {
		user = "# 项目历史调研\n" + st.PriorContext + "\n\n# 当前产品/市场上下文\n" + st.Input
	}
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: discovererSystem},
			{Role: llm.RoleUser, Content: user},
		},
		Temperature: 0.4,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "planner", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed struct {
		Requirements []Requirement `json:"requirements"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "planner", "error", map[string]string{"err": "parse: " + err.Error()})
		return fmt.Errorf("discoverer parse: %w", err)
	}
	st.Requirements = parsed.Requirements
	publish(d, st, "planner", "message", map[string]int{"requirements": len(parsed.Requirements)})
	publish(d, st, "planner", "done", nil)
	return nil
}

// ----- 2. Analyzer (RICE + Kano 维度评分) ------

type RequirementAnalyzer struct{}

func (RequirementAnalyzer) Name() string { return "analyzer" }

const reqAnalyzerSystem = `You are ANALYZER_AGENT (REQUIREMENT_ANALYZER).
对每条需求做 RICE 评分 + Kano 类型判断。

RICE 公式：(Reach × Impact × Confidence) / Effort
- Reach: 影响人数 0-100（每季度估算）
- Impact: 影响力 0.25/0.5/1/2/3 (minimal/low/medium/high/massive)
- Confidence: 自信度 0-1 (0.5/0.8/1.0)
- Effort: 工程量 person-month

Kano 模型：
- basic: 基本必须有，没有用户会不满
- performance: 越多越好的线性提升
- excitement: 惊喜功能，没有不会失望但有了会欣喜
- indifferent: 用户不关心

输出 JSON：
{
  "scored": [
    {
      "id": "R001",
      "reach": 0-100,
      "impact": 0.25/0.5/1/2/3,
      "confidence": 0-1,
      "effort": person-month float,
      "kano_type": "basic|performance|excitement|indifferent"
    }
  ]
}

Output ONLY valid JSON.`

func (RequirementAnalyzer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "analyzer", "start", map[string]int{"requirements": len(st.Requirements)})
	if len(st.Requirements) == 0 {
		return fmt.Errorf("analyzer: no requirements to score")
	}
	reqsJSON, _ := json.Marshal(st.Requirements)
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: reqAnalyzerSystem},
			{Role: llm.RoleUser, Content: string(reqsJSON)},
		},
		Temperature: 0.2,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "analyzer", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed struct {
		Scored []struct {
			ID         string  `json:"id"`
			Reach      int     `json:"reach"`
			Impact     float64 `json:"impact"`
			Confidence float64 `json:"confidence"`
			Effort     float64 `json:"effort"`
			KanoType   string  `json:"kano_type"`
		} `json:"scored"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "analyzer", "error", map[string]string{"err": "parse: " + err.Error()})
		return fmt.Errorf("analyzer parse: %w", err)
	}
	// merge 回 st.Requirements
	scoreMap := make(map[string]int)
	for i, r := range st.Requirements {
		scoreMap[r.ID] = i
	}
	for _, s := range parsed.Scored {
		if i, ok := scoreMap[s.ID]; ok {
			st.Requirements[i].Reach = s.Reach
			st.Requirements[i].Impact = s.Impact
			st.Requirements[i].Confidence = s.Confidence
			st.Requirements[i].Effort = s.Effort
			st.Requirements[i].KanoType = s.KanoType
			if s.Effort > 0 {
				st.Requirements[i].RICEScore = float64(s.Reach) * s.Impact * s.Confidence / s.Effort
			}
		}
	}
	publish(d, st, "analyzer", "message", map[string]int{"scored": len(parsed.Scored)})
	publish(d, st, "analyzer", "done", nil)
	return nil
}

// ----- 3. Prioritizer (排序 + 写报告) ------

type RequirementPrioritizer struct{}

func (RequirementPrioritizer) Name() string { return "writer" }

const prioritizerSystem = `You are WRITER_AGENT (REQUIREMENT_PRIORITIZER).
基于已 RICE 评分 + Kano 分类的需求列表，写一份**需求优先级报告**。

报告结构：
# 标题
## 一、需求总览（X 条需求，按优先级分 P0/P1/P2）
## 二、Top 5 需求详解（每条：JTBD / 用户群 / 痛点 / RICE 分数 / Kano 类型 / 推荐排期）
## 三、Kano 矩阵分析（哪些是 basic 必须做，哪些是 excitement 可选锦上添花）
## 四、本期建议（哪些 P0 必须立刻做，哪些 P2 可延后）
## 引用（如有 sources）

总长度 ≤1200 字。 Output ONLY Markdown.`

func (RequirementPrioritizer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "writer", "start", map[string]int{"requirements": len(st.Requirements)})

	// 按 RICE 排序
	sort.Slice(st.Requirements, func(i, j int) bool {
		return st.Requirements[i].RICEScore > st.Requirements[j].RICEScore
	})

	pb, _ := json.Marshal(map[string]interface{}{
		"input":        st.Input,
		"requirements": st.Requirements,
	})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: prioritizerSystem},
			{Role: llm.RoleUser, Content: string(pb)},
		},
		Temperature: 0.4,
	})
	if err != nil {
		publish(d, st, "writer", "error", map[string]string{"err": err.Error()})
		return err
	}
	if strings.TrimSpace(resp.Message.Content) == "" {
		return fmt.Errorf("prioritizer empty output")
	}
	st.Report = resp.Message.Content
	publish(d, st, "writer", "message", map[string]int{"length": len(st.Report)})
	publish(d, st, "writer", "done", nil)
	return nil
}
