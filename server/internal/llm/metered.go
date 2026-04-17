package llm

import (
	"context"
	"time"
)

// MeteredClient 包一个 Client，每次 Complete() 都把用量写到 Recorder
//
// agent / task_id 通过 ctx 传：调用方包 ctx.WithValue 进去，
// 这样 LLM 接口签名不用改，向后兼容。
type MeteredClient struct {
	Inner    Client
	Recorder *Recorder
	Provider string // "anthropic" / "openrouter" / "aihubmix" / "mock"
}

func NewMetered(inner Client, recorder *Recorder, provider string) *MeteredClient {
	return &MeteredClient{Inner: inner, Recorder: recorder, Provider: provider}
}

func (m *MeteredClient) IsMock() bool { return m.Inner.IsMock() }

// Context key — 调用方塞 task_id / agent name 进 ctx
type ctxKey string

const (
	ctxTaskID ctxKey = "llm.task_id"
	ctxAgent  ctxKey = "llm.agent"
)

// WithTask 把 taskID 注入 ctx，下游 LLM 调用自动 attribute
func WithTask(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, ctxTaskID, taskID)
}

// WithAgent 把 agent name 注入 ctx
func WithAgent(ctx context.Context, agent string) context.Context {
	return context.WithValue(ctx, ctxAgent, agent)
}

func taskFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxTaskID).(string); ok {
		return v
	}
	return ""
}

func agentFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxAgent).(string); ok {
		return v
	}
	return ""
}

func (m *MeteredClient) Complete(ctx context.Context, req Request) (*Response, error) {
	t0 := time.Now()
	resp, err := m.Inner.Complete(ctx, req)
	elapsed := time.Since(t0).Milliseconds()

	model := req.Model
	if model == "" && resp != nil {
		// 部分 client 会回填默认 model，但这里 resp.Message.Model 不存在；用 fallback
		model = "(default)"
	}

	rec := CallRecord{
		TaskID:   taskFrom(ctx),
		Agent:    agentFrom(ctx),
		Model:    model,
		Provider: m.Provider,
		ElapsedMs: elapsed,
		At:       t0,
	}
	if err != nil {
		rec.Err = err.Error()
	}
	if resp != nil {
		rec.PromptTokens = resp.Usage.PromptTokens
		rec.OutputTokens = resp.Usage.CompletionTokens
		rec.TotalTokens = resp.Usage.TotalTokens
		rec.CostUSD = EstimateCost(model, rec.PromptTokens, rec.OutputTokens)
	}

	if m.Recorder != nil {
		ok := m.Recorder.Record(rec)
		if !ok {
			// budget 超限：返回错误让 pipeline 优雅终止
			// 注意：本次调用的结果仍返回（不浪费）
			return resp, ErrBudgetExceeded
		}
	}
	return resp, err
}
