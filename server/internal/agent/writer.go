package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pmhive/server/internal/llm"
)

// Writer 渲染最终 Markdown 报告（含引用 [n]）
type Writer struct{}

func (Writer) Name() string { return "writer" }

const writerSystem = `You are WRITER_AGENT.
Render a Chinese competitor research report in Markdown given:
- input topic
- competitors (structured)
- analysis (SWOT + differentiation)
- sources (numbered references)
- social_quotes (用户在 Reddit/X/etc 上的原声)

⚠️ 关键质量门禁（必须遵守）：
- "## 用户真实声量" 这一段是**有条件的**：
  • 如果 social_quotes 数量 < 3 或质量明显是噪声（标题/内容与 competitors 完全无关，
    如调研 "AI 笔记" 却引用 "AITAH"/"AirPods"/"aiArt" 这种），**整段直接跳过，不要写**。
  • 不要为了凑结构而引用不相关的帖子。宁可少一段，不能误导用户。
  • 引用前自检：每条 quote 的 title/content 必须明显涉及 competitors 中的某个产品或本赛道。

Sections:
1. # 标题
2. ## 1. 调研范围
3. ## 2. 竞品矩阵 (Markdown table)
4. ## 3. SWOT 与差异化机会
5. ## 4. 用户真实声量（仅当数据可靠时；否则跳过本节，编号顺延）
6. ## 5. 建议切入点
7. ## 引用（仅列引用了的来源；不要列入未被正文引用的 source）

Use [1] [2] inline citations matching the sources list.
Keep total length under 1500 字。 Output ONLY Markdown, no preamble.`

// RewriteWriter 拿 Reviewer 的 critique 把报告重写一遍（self-correction loop）
type RewriteWriter struct {
	Critique []string // Reviewer.Issues
}

func (RewriteWriter) Name() string { return "writer" }

const rewriteWriterSystem = `You are WRITER_AGENT (RE-WRITE PASS).
之前你写了一份报告，REVIEWER 指出以下具体缺陷需要修复：

{{CRITIQUE}}

任务：保持原结构 + 风格不变，**仅针对每条缺陷**做精准修复 → 输出修复后的完整 Markdown。

规则：
- 不要为了"看起来改了"而无中生有 —— 没有 source 支持的内容不能加
- 引用必须能在 sources 列表里找到对应条目
- 仍保持中文 + Markdown，total ≤ 1500 字
- 输出 ONLY 完整修复后的 Markdown（不是 diff，是全文）

如果某条 critique 是错的（你检查后认为原报告没问题），可以保持原样，但要在末尾加一个 HTML 注释说明:
<!-- reviewer note: 第 N 条缺陷 -- 已检查，保持原写法因为 ... -->`

func (rw RewriteWriter) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "writer", "start", map[string]interface{}{
		"mode":     "rewrite",
		"critique": rw.Critique,
	})

	if len(rw.Critique) == 0 {
		// 无 critique 直接跳过
		publish(d, st, "writer", "done", map[string]string{"reason": "no_critique_skip"})
		return nil
	}

	// 把 critique 渲染成 system prompt
	var critiqueLines string
	for i, c := range rw.Critique {
		critiqueLines += fmt.Sprintf("%d. %s\n", i+1, c)
	}
	system := strings.Replace(rewriteWriterSystem, "{{CRITIQUE}}", critiqueLines, 1)

	// 把原报告 + 全部 grounding 数据塞回 user message
	postCap := 30
	posts := st.Posts
	if len(posts) > postCap {
		posts = posts[:postCap]
	}
	socialQuotes := make([]map[string]interface{}, 0, len(posts))
	for _, p := range posts {
		socialQuotes = append(socialQuotes, map[string]interface{}{
			"platform": p.Platform, "author": p.Author, "title": p.Title,
			"snippet": shortPreview(p.Content, 200), "url": p.URL,
		})
	}
	payload := map[string]interface{}{
		"input":          st.Input,
		"competitors":    st.Competitors,
		"analysis":       st.Analysis,
		"sources":        st.Sources,
		"social_quotes":  socialQuotes,
		"original_report": st.Report,
	}
	pb, _ := json.Marshal(payload)

	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: system},
			{Role: llm.RoleUser, Content: string(pb)},
		},
		Temperature: 0.3,
	})
	if err != nil {
		publish(d, st, "writer", "error", map[string]string{"err": err.Error()})
		return err
	}
	md := resp.Message.Content
	if len(strings.TrimSpace(md)) == 0 {
		return fmt.Errorf("rewrite empty")
	}
	st.Report = md
	publish(d, st, "writer", "message", map[string]interface{}{
		"mode":   "rewrite",
		"length": len(md),
	})
	publish(d, st, "writer", "done", nil)
	return nil
}

func (Writer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "writer", "start", nil)

	// 给来源编号
	for i := range st.Sources {
		// 编号 = 1-based index 由调用方使用
		_ = i
	}

	// 把社交原声单独传给 Writer（cap 30 防 context 爆）
	postCap := 30
	posts := st.Posts
	if len(posts) > postCap {
		posts = posts[:postCap]
	}
	socialQuotes := make([]map[string]interface{}, 0, len(posts))
	for _, p := range posts {
		socialQuotes = append(socialQuotes, map[string]interface{}{
			"platform": p.Platform,
			"author":   p.Author,
			"title":    p.Title,
			"snippet":  shortPreview(p.Content, 200),
			"url":      p.URL,
		})
	}
	payload := map[string]interface{}{
		"input":          st.Input,
		"outline":        st.Outline,
		"competitors":    st.Competitors,
		"analysis":       st.Analysis,
		"sources":        st.Sources,
		"social_quotes":  socialQuotes,
	}
	pb, _ := json.Marshal(payload)

	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: writerSystem},
			{Role: llm.RoleUser, Content: string(pb)},
		},
		Temperature: 0.5,
	})
	if err != nil {
		publish(d, st, "writer", "error", map[string]string{"err": err.Error()})
		return err
	}
	md := resp.Message.Content
	if strings.TrimSpace(md) == "" {
		return fmt.Errorf("writer empty output")
	}
	st.Report = md

	publish(d, st, "writer", "message", map[string]interface{}{
		"length":   len(md),
		"preview":  shortPreview(md, 240),
	})
	publish(d, st, "writer", "done", nil)
	return nil
}
