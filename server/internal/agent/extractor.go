package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pmhive/server/internal/llm"
	"pmhive/server/internal/tools"
)

// Extractor 把搜索 + 抓取的原始数据，经 LLM 抽取为结构化竞品列表
type Extractor struct{}

func (Extractor) Name() string { return "extractor" }

const extractorSystem = `You are EXTRACTOR_AGENT.
Given raw competitor research material (search snippets + scraped page markdown + social posts),
produce JSON: {"competitors":[{"name":"","pricing":"","ai":true,"strengths":["","",""],"weaknesses":["","",""],"url":""}]}.
- Aim for 3-5 competitors.
- Keep strengths/weaknesses to <=3 short bullets each (Chinese).
- "ai" is boolean: 是否内置 AI 能力。
- 当社交平台原声（user voice）与官网营销话术冲突时，**优先采信社交原声**（真实用户体验比官方更可信）。
- Output ONLY valid JSON.`

func (Extractor) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "extractor", "start", nil)

	var sb strings.Builder
	sb.WriteString("# 调研材料\n\n")

	// 区分真实搜索结果 vs fallback 占位（避免污染 LLM 判断）
	var realResults, fallbackResults []tools.SearchResult
	for _, r := range st.SearchResults {
		if strings.Contains(r.Title, "[FALLBACK_MOCK]") || strings.Contains(r.Content, "[FALLBACK_MOCK]") {
			fallbackResults = append(fallbackResults, r)
		} else {
			realResults = append(realResults, r)
		}
	}
	if len(realResults) > 0 {
		sb.WriteString("## 真实搜索结果\n")
		for _, r := range realResults {
			fmt.Fprintf(&sb, "- [%s](%s): %s\n", r.Title, r.URL, shortPreview(r.Content, 240))
		}
	}
	if len(realResults) == 0 && len(fallbackResults) > 0 {
		sb.WriteString("⚠️ 真实搜索源不可用（限流/无 key）。请仅基于下方抓取到的真实页面 + 你的训练知识做判断，**不要捏造产品**。\n\n")
	}
	sb.WriteString("\n## 抓取页面（节选）\n")
	for u, p := range st.Pages {
		fmt.Fprintf(&sb, "### %s\n%s\n\n", u, shortPreview(p, 1200))
	}

	// 社交平台用户原声 —— 与官网营销话术对冲
	// cap 40 条防 LLM context 爆（全量在 store 中可单独查询）
	if len(st.Posts) > 0 {
		fmt.Fprintf(&sb, "\n## 社交平台用户原声（取 top 40 / 共 %d 条）\n", len(st.Posts))
		topN := st.Posts
		if len(topN) > 40 {
			topN = topN[:40]
		}
		for _, p := range topN {
			fmt.Fprintf(&sb, "- [%s · u/%s · %d↑ %d💬] %s\n  %s\n",
				p.Platform, p.Author, p.Engagement.Likes, p.Engagement.Comments,
				p.Title, shortPreview(p.Content, 240))
		}
	}

	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: extractorSystem},
			{Role: llm.RoleUser, Content: sb.String()},
		},
		Temperature: 0.2,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "extractor", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed struct {
		Competitors []Competitor `json:"competitors"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "extractor", "error", map[string]string{"err": "parse: " + err.Error(), "raw": shortPreview(resp.Message.Content, 200)})
		return fmt.Errorf("extractor parse: %w", err)
	}
	st.Competitors = parsed.Competitors

	publish(d, st, "extractor", "message", map[string]interface{}{
		"competitors_count": len(parsed.Competitors),
		"competitors":       parsed.Competitors,
	})
	publish(d, st, "extractor", "done", nil)
	return nil
}
