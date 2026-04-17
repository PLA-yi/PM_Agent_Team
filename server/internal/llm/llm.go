// Package llm 抽象 LLM 调用层。默认实现 OpenRouter；无 key 时自动 fallback 到 mock。
package llm

import (
	"context"
	"encoding/json"
)

// Role 消息角色
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 一条对话消息
type Message struct {
	Role       Role            `json:"role"`
	Content    string          `json:"content,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	Raw        json.RawMessage `json:"-"`
}

// ToolCall LLM 决定调用工具
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // 总是 "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolSpec 提供给 LLM 的工具描述
type ToolSpec struct {
	Type     string       `json:"type"` // "function"
	Function ToolSpecFunc `json:"function"`
}

type ToolSpecFunc struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Request 一次 LLM 请求
type Request struct {
	Model       string
	Messages    []Message
	Tools       []ToolSpec
	Temperature float64
	MaxTokens   int
	JSONMode    bool // 强制 response_format=json_object
}

// Response LLM 返回
type Response struct {
	Message Message
	Usage   Usage
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Client LLM 调用接口
type Client interface {
	Complete(ctx context.Context, req Request) (*Response, error)
	IsMock() bool
}
