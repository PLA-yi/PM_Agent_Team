package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"pmhive/server/internal/llm"
)

// Reviewer 给 Writer 输出的报告打分。
// 三维度：fact / coverage / citation；输出 0-10 综合分 + 缺陷列表。
//
// 缺陷列表会被 Coordinator 用作 Writer self-correction 的 critique 输入。
type Reviewer struct {
	Iteration int // 第几轮 review（1 = 首版）
}

func (Reviewer) Name() string { return "reviewer" }

const reviewerSystem = `You are REVIEWER_AGENT —— a strict but fair reviewer for product research reports.

You will be given:
- input topic (用户原需求)
- competitors (Extractor 提取的结构化数据)
- analysis (Analyzer 的 SWOT)
- sources (引用库)
- social_quotes (社交平台原声)
- report (Writer 写的 Markdown 报告)

请从三个维度给报告打 0-10 分（保留 1 位小数，不要给 10 分这种水分）：
1. fact_score: 报告里的事实陈述（产品定价/功能/数据）是否能在 sources/social_quotes 里找到依据
2. coverage_score: 是否覆盖了用户需求的所有维度（如调研竞品矩阵 / SWOT / 切入点）
3. citation_score: 引用是否准确（[N] 标号是否对应到真实有内容的 source、有没有空引或乱引）

输出 JSON：
{
  "overall_score": 综合分（三项加权 fact:0.5 coverage:0.3 citation:0.2 的平均）,
  "fact_score": ...,
  "coverage_score": ...,
  "citation_score": ...,
  "strengths": ["亮点1", "亮点2"],         // 1-3 条
  "issues": ["具体问题1", "具体问题2"],     // 0-5 条；每条要可执行（"补充 X 产品的定价"，不要"建议改进"这种空话）
  "verdict": "accept | revise | reject"   // accept ≥7 / revise 5-7 / reject <5
}

严格标准：
- 报告引用了不相关的 source（比如调研 AI 笔记却引用 r/AITAH）→ citation_score ≤ 4
- 关键产品没出现在矩阵里（如调研 AI 笔记没列 Notion）→ coverage_score ≤ 5
- 报告里出现 sources 里找不到的具体数字 / 价格 → fact_score ≤ 6

Output ONLY valid JSON.`

func (r Reviewer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "reviewer", "start", map[string]interface{}{
		"iteration":   r.Iteration,
		"report_len":  len(st.Report),
		"competitors": len(st.Competitors),
		"sources":     len(st.Sources),
		"posts":       len(st.Posts),
	})

	if st.Report == "" {
		err := fmt.Errorf("reviewer: empty report")
		publish(d, st, "reviewer", "error", map[string]string{"err": err.Error()})
		return err
	}

	// cap social_quotes 给 Reviewer 看 top 20 就够判断
	postCap := 20
	postsSlice := st.Posts
	if len(postsSlice) > postCap {
		postsSlice = postsSlice[:postCap]
	}

	payload := map[string]interface{}{
		"input":         st.Input,
		"competitors":   st.Competitors,
		"analysis":      st.Analysis,
		"sources":       st.Sources,
		"social_quotes": postsSlice,
		"report":        st.Report,
	}
	pb, _ := json.Marshal(payload)

	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: reviewerSystem},
			{Role: llm.RoleUser, Content: string(pb)},
		},
		Temperature: 0.2,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "reviewer", "error", map[string]string{"err": err.Error()})
		return err
	}

	var review ReviewResult
	if err := json.Unmarshal([]byte(resp.Message.Content), &review); err != nil {
		publish(d, st, "reviewer", "error", map[string]string{
			"err": "parse: " + err.Error(),
			"raw": shortPreview(resp.Message.Content, 200),
		})
		return fmt.Errorf("reviewer parse: %w", err)
	}
	review.Iteration = r.Iteration
	st.Review = &review

	publish(d, st, "reviewer", "message", map[string]interface{}{
		"overall":   review.OverallScore,
		"fact":      review.FactScore,
		"coverage":  review.CoverageScore,
		"citation":  review.CitationScore,
		"verdict":   review.Verdict,
		"issues":    len(review.Issues),
		"iteration": r.Iteration,
	})
	publish(d, st, "reviewer", "done", nil)
	return nil
}

// ReviewerRetry 是 Coordinator 用的"如果分数低就 Writer 拿 critique 重写一遍"的逻辑.
// 实现为一个特殊的 Agent：检查 st.Review，分数低时 spawn 内部 RewriteWriter；高时直接 done。
type ReviewerRetry struct {
	MinScore float64 // 低于此分触发重写
}

func (ReviewerRetry) Name() string { return "coordinator" }

func (rr ReviewerRetry) Run(ctx context.Context, st *State, d Deps) error {
	if st.Review == nil {
		// 没 review 就不做 retry
		return nil
	}
	if st.Review.OverallScore >= rr.MinScore {
		publish(d, st, "coordinator", "thought", map[string]interface{}{
			"action":    "skip_retry",
			"score":     st.Review.OverallScore,
			"threshold": rr.MinScore,
		})
		return nil
	}
	publish(d, st, "coordinator", "thought", map[string]interface{}{
		"action":    "trigger_rewrite",
		"score":     st.Review.OverallScore,
		"threshold": rr.MinScore,
		"issues":    st.Review.Issues,
	})

	// 跑 RewriteWriter（带 critique）
	rw := RewriteWriter{Critique: st.Review.Issues}
	if err := rw.Run(ctx, st, d); err != nil {
		return fmt.Errorf("rewrite: %w", err)
	}

	// 重写后再评一次（iteration=2）
	r2 := Reviewer{Iteration: 2}
	return r2.Run(ctx, st, d)
}
