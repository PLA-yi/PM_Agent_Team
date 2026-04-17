package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pmhive/server/internal/llm"
)

// ============================================================================
// 需求验证模块（3 阶段）
//   1. HypothesisGenerator — 把需求拆成可验证的假设（Problem/Solution/Value）
//   2. ValidationExecutor  — 执行验证：搜数据 + 推荐访谈方法 + 模拟反驳
//   3. RiskWriter          — 列出验证盲点 + 写最终报告
// ============================================================================

// ----- 1. HypothesisGenerator ------

type HypothesisGenerator struct{}

func (HypothesisGenerator) Name() string { return "planner" }

const hypoSystem = `You are PLANNER_AGENT (HYPOTHESIS_GENERATOR).
PM 给出"待验证的需求或想法"，你要把它拆成 3-5 条**结构化假设**。

每条假设按 Lean Startup 的三类区分：
- problem: 用户是否真有这个痛点？
- solution: 我们的方案能否解决这个痛点？
- value: 用户是否愿意为此付费/改变行为？

输出 JSON：
{
  "hypotheses": [
    {
      "id": "H001",
      "statement": "我们认为 [X 用户群] 在 [Y 场景] 下需要 [Z]，因为 [...]",
      "type": "problem|solution|value",
      "confidence": 0-1（PM 当前主观自信度）
    }
  ]
}

至少包含 1 条 problem + 1 条 solution + 1 条 value。Output ONLY JSON.`

func (HypothesisGenerator) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "planner", "start", map[string]string{"input": st.Input})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: hypoSystem},
			{Role: llm.RoleUser, Content: st.Input},
		},
		Temperature: 0.4,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "planner", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed struct {
		Hypotheses []Hypothesis `json:"hypotheses"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "planner", "error", map[string]string{"err": "parse: " + err.Error()})
		return fmt.Errorf("hypo parse: %w", err)
	}
	st.Hypotheses = parsed.Hypotheses
	publish(d, st, "planner", "message", map[string]int{"hypotheses": len(parsed.Hypotheses)})
	publish(d, st, "planner", "done", nil)
	return nil
}

// ----- 2. ValidationExecutor (扮演验证执行者) ------

type ValidationExecutor struct{}

func (ValidationExecutor) Name() string { return "extractor" } // 复用 extractor timeline 颜色

const valExecSystem = `You are EXTRACTOR_AGENT (VALIDATION_EXECUTOR).
对每条假设，找到至少 1 种验证方法 + 模拟该方法可能得到的证据。

验证方法：
- user_interview: 推荐访谈对象 + 关键问题
- market_data: 找市场报告/数据点支撑或反驳
- desk_research: 调研竞品/行业惯例
- a_b_test: 设计 AB 实验

蓝军视角：主动找反驳证据（不能全找支持的）。

输出 JSON：
{
  "validations": [
    {
      "hypothesis_id": "H001",
      "method": "user_interview|market_data|desk_research|a_b_test",
      "evidence": "具体证据描述（包含数据/引用）",
      "verdict": "confirmed|refuted|inconclusive",
      "sources": ["参考资料/访谈对象描述"]
    }
  ]
}

每条假设至少 1 条 validation。鼓励同一假设多种方法交叉验证。Output ONLY JSON.`

func (ValidationExecutor) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "extractor", "start", map[string]int{"hypotheses": len(st.Hypotheses)})
	if len(st.Hypotheses) == 0 {
		return fmt.Errorf("validation: no hypotheses")
	}

	// 把抓到的 social posts / web pages 作为 grounding
	context := ""
	if len(st.Posts) > 0 {
		context += "\n## 社交平台真实声量（可作为 user_interview 模拟数据）\n"
		topN := st.Posts
		if len(topN) > 20 {
			topN = topN[:20]
		}
		for _, p := range topN {
			context += fmt.Sprintf("- [%s] %s — %s\n", p.Platform, p.Title, shortPreview(p.Content, 200))
		}
	}

	pb, _ := json.Marshal(map[string]interface{}{
		"hypotheses":     st.Hypotheses,
		"input":          st.Input,
		"social_context": context,
	})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: valExecSystem},
			{Role: llm.RoleUser, Content: string(pb)},
		},
		Temperature: 0.3,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "extractor", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed struct {
		Validations []ValidationCheck `json:"validations"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "extractor", "error", map[string]string{"err": "parse: " + err.Error()})
		return fmt.Errorf("validation parse: %w", err)
	}
	st.Validations = parsed.Validations
	publish(d, st, "extractor", "message", map[string]int{"validations": len(parsed.Validations)})
	publish(d, st, "extractor", "done", nil)
	return nil
}

// ----- 3. RiskWriter (盲点 + 报告) ------

type ValidationRiskWriter struct{}

func (ValidationRiskWriter) Name() string { return "writer" }

const valRiskSystem = `You are WRITER_AGENT (VALIDATION_RISK_WRITER).
基于已执行的 validation，识别**仍未被验证的盲点 / 风险**，然后写报告。

报告结构：
# 标题
## 一、待验证假设 (Hypotheses)
   - 列出每条假设 + 类型 + 当前可信度
## 二、验证执行结果 (Validation Results)
   - 每条假设下的 validations 表（method / verdict / evidence）
## 三、验证盲点与风险 (Risks)
   - 哪些假设证据不足 / 哪些方法没覆盖
   - 每条 risk 给 severity (high/medium/low) + mitigation
## 四、下一步建议
   - 哪些假设需要立刻找用户访谈
   - 哪些可以做 AB 测试
   - 哪些可以暂时搁置

总长度 ≤1200 字。 在报告 JSON metadata 末尾追加：

<!-- RISKS_JSON_START
{"risks": [{"risk":"...","severity":"high|medium|low","mitigation":"..."}]}
RISKS_JSON_END -->

Output ONLY Markdown (含上述注释块)。`

func (ValidationRiskWriter) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "writer", "start", nil)
	pb, _ := json.Marshal(map[string]interface{}{
		"input":       st.Input,
		"hypotheses":  st.Hypotheses,
		"validations": st.Validations,
	})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: valRiskSystem},
			{Role: llm.RoleUser, Content: string(pb)},
		},
		Temperature: 0.4,
	})
	if err != nil {
		publish(d, st, "writer", "error", map[string]string{"err": err.Error()})
		return err
	}
	md := resp.Message.Content
	if strings.TrimSpace(md) == "" {
		return fmt.Errorf("risk writer empty")
	}
	st.Report = md

	// 解出 risks JSON 注释块
	if i := strings.Index(md, "RISKS_JSON_START"); i >= 0 {
		rest := md[i+len("RISKS_JSON_START"):]
		if j := strings.Index(rest, "RISKS_JSON_END"); j >= 0 {
			rawJSON := strings.TrimSpace(rest[:j])
			var rp struct {
				Risks []ValidationRisk `json:"risks"`
			}
			if err := json.Unmarshal([]byte(rawJSON), &rp); err == nil {
				st.Risks = rp.Risks
			}
		}
	}

	publish(d, st, "writer", "message", map[string]interface{}{
		"length": len(md),
		"risks":  len(st.Risks),
	})
	publish(d, st, "writer", "done", nil)
	return nil
}
