package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"pmhive/server/internal/llm"
)

// Planner 拆解调研大纲并产生竞品候选清单
type Planner struct{}

func (Planner) Name() string { return "planner" }

const plannerSystem = `You are PLANNER_AGENT for a multi-agent product research system.
Given a research topic in Chinese (赛道/产品名), output JSON with:
- "outline": array of 5 调研维度 (string)
- "candidates": array of 5 candidate products, each:
  {
    "name": "中文产品名（主显示名）",
    "name_en": "official English brand name",  // 极其重要：社交平台搜索（Reddit/X/etc）依赖此字段
    "url": "官网或主页 URL（必须真实可访问）"
  }

规则：
1. **name_en 必填**：每个候选都给出该产品在 Reddit/Twitter 等英文社区的**官方品牌名**。
   例：思源笔记 → "SiYuan Note" 或 "siyuan-note"；幻塔 → "Tower of Fantasy"；
       Notion 直接是 Notion；中国出海产品给官方英文名（如"Lark"对飞书）。
   绝不要捏造英文名 — 不确定时给最常见的拼音 + 类目词（如"miaoji ai"）。
2. URL 必须真实可访问，不要编造（如 miaojiapp.com 这种不存在的）。
3. Output ONLY valid JSON, no commentary.`

func (Planner) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "planner", "start", map[string]string{"input": st.Input})

	userMsg := st.Input
	if st.PriorContext != "" {
		userMsg = "# 项目历史调研（参考，可复用）\n" + st.PriorContext + "\n\n# 本次需求\n" + st.Input
	}
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: plannerSystem},
			{Role: llm.RoleUser, Content: userMsg},
		},
		Temperature: 0.3,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "planner", "error", map[string]string{"err": err.Error()})
		return err
	}

	var parsed struct {
		Outline    []string    `json:"outline"`
		Candidates []Candidate `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "planner", "error", map[string]string{"err": "parse json: " + err.Error(), "raw": shortPreview(resp.Message.Content, 200)})
		return fmt.Errorf("planner parse: %w", err)
	}
	st.Outline = parsed.Outline
	st.Candidates = parsed.Candidates

	publish(d, st, "planner", "message", map[string]interface{}{
		"outline_count":    len(parsed.Outline),
		"candidate_count":  len(parsed.Candidates),
		"outline":          parsed.Outline,
		"candidates":       parsed.Candidates,
	})
	publish(d, st, "planner", "done", nil)
	return nil
}
