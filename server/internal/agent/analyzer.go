package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"pmhive/server/internal/llm"
)

// Analyzer 横向对比 + SWOT + 差异化机会
type Analyzer struct{}

func (Analyzer) Name() string { return "analyzer" }

const analyzerSystem = `You are ANALYZER_AGENT.
Given a JSON list of competitors, output JSON:
{"swot":{"opportunities":["",""],"threats":["",""]},"differentiation":"一句话差异化建议（中文）"}
Keep each bullet under 30 字。
Output ONLY valid JSON.`

func (Analyzer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "analyzer", "start", nil)

	cb, _ := json.Marshal(map[string]interface{}{
		"input":       st.Input,
		"competitors": st.Competitors,
	})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: analyzerSystem},
			{Role: llm.RoleUser, Content: string(cb)},
		},
		Temperature: 0.4,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "analyzer", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "analyzer", "error", map[string]string{"err": "parse: " + err.Error(), "raw": shortPreview(resp.Message.Content, 200)})
		return fmt.Errorf("analyzer parse: %w", err)
	}
	st.Analysis = parsed

	publish(d, st, "analyzer", "message", parsed)
	publish(d, st, "analyzer", "done", nil)
	return nil
}
