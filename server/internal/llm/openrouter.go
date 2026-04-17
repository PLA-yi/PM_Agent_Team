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

const (
	openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
	aiHubMixURL   = "https://aihubmix.com/v1/chat/completions"
)

// OpenAICompat 通用 OpenAI 兼容协议 client。
// 适配 OpenRouter / AIhubmix / DeepSeek / Qwen / 自建网关。
type OpenAICompat struct {
	Endpoint     string // 完整的 chat/completions URL
	APIKey       string
	DefaultModel string
	Provider     string // 仅用于日志/错误前缀
	HTTP         *http.Client
}

// 旧名兼容
type OpenRouter = OpenAICompat

func NewOpenRouter(apiKey, defaultModel string) *OpenAICompat {
	if defaultModel == "" {
		defaultModel = "anthropic/claude-sonnet-4.6"
	}
	return &OpenAICompat{
		Endpoint:     openRouterURL,
		APIKey:       apiKey,
		DefaultModel: defaultModel,
		Provider:     "openrouter",
		HTTP:         &http.Client{Timeout: 120 * time.Second},
	}
}

// NewAIhubmix AIhubmix 网关（国内 OpenAI 兼容代理，支持 Claude/GPT/Qwen 等）
func NewAIhubmix(apiKey, defaultModel string) *OpenAICompat {
	if defaultModel == "" {
		defaultModel = "claude-sonnet-4-5"
	}
	return &OpenAICompat{
		Endpoint:     aiHubMixURL,
		APIKey:       apiKey,
		DefaultModel: defaultModel,
		Provider:     "aihubmix",
		HTTP:         &http.Client{Timeout: 120 * time.Second},
	}
}

// NewOpenAICompat 任意自定义 baseURL
//   baseURL: e.g. "https://api.deepseek.com/v1"，自动追加 /chat/completions
func NewOpenAICompat(baseURL, apiKey, defaultModel, provider string) *OpenAICompat {
	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	if provider == "" {
		provider = "openai-compat"
	}
	return &OpenAICompat{
		Endpoint:     endpoint,
		APIKey:       apiKey,
		DefaultModel: defaultModel,
		Provider:     provider,
		HTTP:         &http.Client{Timeout: 180 * time.Second},
	}
}

func (o *OpenAICompat) IsMock() bool { return false }

type orRequest struct {
	Model          string                 `json:"model"`
	Messages       []Message              `json:"messages"`
	Tools          []ToolSpec             `json:"tools,omitempty"`
	Temperature    float64                `json:"temperature,omitempty"`
	MaxTokens      int                    `json:"max_tokens,omitempty"`
	ResponseFormat map[string]interface{} `json:"response_format,omitempty"`
}

type orChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type orResponse struct {
	ID      string     `json:"id"`
	Choices []orChoice `json:"choices"`
	Usage   Usage      `json:"usage"`
	Error   *orError   `json:"error,omitempty"`
}

type orError struct {
	Message string      `json:"message"`
	Type    string      `json:"type,omitempty"`
	Code    interface{} `json:"code,omitempty"` // 有些网关返回 string 有些返回 int
}

func (o *OpenAICompat) Complete(ctx context.Context, req Request) (*Response, error) {
	model := req.Model
	if model == "" {
		model = o.DefaultModel
	}
	body := orRequest{
		Model:       model,
		Messages:    req.Messages,
		Tools:       req.Tools,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if req.JSONMode {
		body.ResponseFormat = map[string]interface{}{"type": "json_object"}
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)
	// OpenRouter 推荐填这两项作为来源标识；其他网关忽略不影响
	httpReq.Header.Set("HTTP-Referer", "https://pmhive.local")
	httpReq.Header.Set("X-Title", "PMHive")

	resp, err := o.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s http: %w", o.Provider, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %d: %s", o.Provider, resp.StatusCode, string(respBody))
	}
	var parsed orResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("%s decode: %w; body=%s", o.Provider, err, string(respBody))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("%s error: %s", o.Provider, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("%s: no choices in response; body=%s", o.Provider, string(respBody))
	}
	// JSONMode 容错：剥围栏 + 提取首个完整 JSON 对象（防止 LLM 加自然语言）
	if req.JSONMode {
		parsed.Choices[0].Message.Content = ExtractJSONObject(parsed.Choices[0].Message.Content)
	}
	return &Response{Message: parsed.Choices[0].Message, Usage: parsed.Usage}, nil
}
