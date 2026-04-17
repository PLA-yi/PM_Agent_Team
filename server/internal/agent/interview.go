package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pmhive/server/internal/llm"
)

// ===== Chunker =====
// 把访谈原始文本切成可处理的段，每段 ~200 字。LLM 这一步用规则切分（无需 LLM 调用）。

type Chunker struct {
	TargetSize int
}

func (Chunker) Name() string { return "chunker" }

func (c Chunker) Run(_ context.Context, st *State, d Deps) error {
	publish(d, st, "chunker", "start", map[string]int{"input_bytes": len(st.Input)})
	size := c.TargetSize
	if size <= 0 {
		size = 200
	}

	// 先按段落（空行）切，再按字数补刀
	paragraphs := strings.Split(st.Input, "\n\n")
	var chunks []string
	var cur strings.Builder
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			chunks = append(chunks, s)
		}
		cur.Reset()
	}
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if cur.Len()+len(p) > size && cur.Len() > 0 {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n")
		}
		cur.WriteString(p)
		if cur.Len() >= size {
			flush()
		}
	}
	flush()

	// 兜底：如果一段都没切出来（输入是单行长文本），强制按 size 切
	if len(chunks) == 0 && st.Input != "" {
		runes := []rune(st.Input)
		for i := 0; i < len(runes); i += size {
			end := i + size
			if end > len(runes) {
				end = len(runes)
			}
			chunks = append(chunks, string(runes[i:end]))
		}
	}

	st.InterviewChunks = chunks
	publish(d, st, "chunker", "message", map[string]int{"chunks": len(chunks)})
	publish(d, st, "chunker", "done", nil)
	return nil
}

// ===== Clusterer =====
// 把 chunks 喂给 LLM，提取主题聚类（含原话引用）

type Clusterer struct{}

func (Clusterer) Name() string { return "clusterer" }

const clustererSystem = `You are CLUSTERER_AGENT.
Given chunks of user interview transcripts (Chinese), cluster recurring themes.
Output JSON: {"insights":[{"theme":"主题","frequency":N,"quotes":["原话1","原话2"],"need_level":"critical|important|nice-to-have","confidence":0.0-1.0}]}
- 3-6 insights total
- Each theme should have at least 1 quote, at most 3
- Output ONLY valid JSON.`

func (Clusterer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "clusterer", "start", map[string]int{"chunks": len(st.InterviewChunks)})
	if len(st.InterviewChunks) == 0 {
		// fallback：直接用 input
		st.InterviewChunks = []string{st.Input}
	}

	var sb strings.Builder
	for i, c := range st.InterviewChunks {
		fmt.Fprintf(&sb, "## Chunk %d\n%s\n\n", i+1, c)
	}

	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: clustererSystem},
			{Role: llm.RoleUser, Content: sb.String()},
		},
		Temperature: 0.3,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "clusterer", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed struct {
		Insights []Insight `json:"insights"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "clusterer", "error", map[string]string{"err": "parse: " + err.Error(), "raw": shortPreview(resp.Message.Content, 200)})
		return fmt.Errorf("clusterer parse: %w", err)
	}
	st.Insights = parsed.Insights

	publish(d, st, "clusterer", "message", map[string]interface{}{
		"insight_count": len(parsed.Insights),
		"insights":      parsed.Insights,
	})
	publish(d, st, "clusterer", "done", nil)
	return nil
}

// ===== InsightSynthesizer =====
// 把 insights 渲染成 Markdown 报告（洞察主题 + 需求列表）

type InsightSynthesizer struct{}

func (InsightSynthesizer) Name() string { return "writer" } // 复用 writer 的 timeline 颜色

const synthSystem = `You are WRITER_AGENT for interview synthesis.
Given a list of insights, render a Chinese markdown report with sections:
1. # 标题 — 用户访谈洞察分析
2. ## 1. 概览 — 总样本量、主题数、关键发现 1 句话
3. ## 2. 主题洞察 — 每个 insight 一个 ### 子节，含频次/原话/置信度
4. ## 3. 需求列表 — 按 need_level 分组的可执行需求条目
5. ## 4. 后续建议 — 1-2 行下一步建议

Keep under 1000 字。 Output ONLY Markdown.`

func (InsightSynthesizer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "writer", "start", nil)
	payload, _ := json.Marshal(map[string]interface{}{
		"input":    st.Input,
		"chunks":   len(st.InterviewChunks),
		"insights": st.Insights,
	})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: synthSystem},
			{Role: llm.RoleUser, Content: string(payload)},
		},
		Temperature: 0.5,
	})
	if err != nil {
		publish(d, st, "writer", "error", map[string]string{"err": err.Error()})
		return err
	}
	st.Report = resp.Message.Content
	publish(d, st, "writer", "message", map[string]interface{}{"length": len(st.Report), "preview": shortPreview(st.Report, 240)})
	publish(d, st, "writer", "done", nil)
	return nil
}
