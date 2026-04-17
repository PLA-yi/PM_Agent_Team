package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Anthropic 直连 Anthropic Messages API。
// 文档：https://docs.anthropic.com/en/api/messages
type Anthropic struct {
	APIKey       string
	DefaultModel string
	HTTP         *http.Client
}

func NewAnthropic(apiKey, model string) *Anthropic {
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	return &Anthropic{
		APIKey:       apiKey,
		DefaultModel: model,
		HTTP:         &http.Client{Timeout: 180 * time.Second},
	}
}

func (a *Anthropic) IsMock() bool { return false }

// Anthropic Messages API 的请求 schema（system 是顶层字段，messages 不含 system role）
type anthMessage struct {
	Role    string         `json:"role"`
	Content []anthContent  `json:"content"`
}

type anthContent struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
}

type anthRequest struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	System      string        `json:"system,omitempty"`
	Messages    []anthMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

type anthResponse struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Role       string        `json:"role"`
	Content    []anthContent `json:"content"`
	Model      string        `json:"model"`
	StopReason string        `json:"stop_reason"`
	Usage      anthUsage     `json:"usage"`
	Error      *anthError    `json:"error,omitempty"`
}

type anthUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (a *Anthropic) Complete(ctx context.Context, req Request) (*Response, error) {
	model := req.Model
	if model == "" {
		model = a.DefaultModel
	}

	// 拆出 system，转换 messages 格式
	var system strings.Builder
	var messages []anthMessage
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			if system.Len() > 0 {
				system.WriteString("\n\n")
			}
			system.WriteString(m.Content)
		case RoleUser, RoleAssistant:
			content := m.Content
			// 如果要求 JSON 输出，给 user 消息加约束（Anthropic 没有 response_format）
			if req.JSONMode && m.Role == RoleUser {
				content += "\n\nIMPORTANT: Respond with valid JSON only. No commentary, no markdown code fences."
			}
			messages = append(messages, anthMessage{
				Role:    string(m.Role),
				Content: []anthContent{{Type: "text", Text: content}},
			})
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := anthRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		System:      system.String(),
		Messages:    messages,
		Temperature: req.Temperature,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed anthResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode: %w; body=%s", err, string(respBody))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("anthropic %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if len(parsed.Content) == 0 {
		return nil, fmt.Errorf("anthropic: empty content; raw=%s", string(respBody))
	}

	// 拼接所有 text block（通常只有一个）
	var sb strings.Builder
	for _, c := range parsed.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	out := sb.String()
	// JSONMode: 剥围栏 + 提取首个完整 JSON 对象
	if req.JSONMode {
		out = ExtractJSONObject(out)
	}

	return &Response{
		Message: Message{Role: RoleAssistant, Content: out},
		Usage: Usage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
			TotalTokens:      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	}, nil
}

// stripJSONFence 已迁移至 jsonutil.go
