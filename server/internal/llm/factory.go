package llm

import "strings"

// Config 工厂入参
type Config struct {
	Provider      string // "anthropic" | "openrouter" | "aihubmix" | "openai-compat" | ""(auto)
	APIKey        string // 通用 key，按 provider 路由
	OpenRouterKey string
	AnthropicKey  string
	AIhubmixKey   string
	BaseURL       string // 仅 provider="openai-compat" 时使用
	Model         string
	MockMode      string // "auto" | "always" | "never"
}

// New 根据 Config 创建 Client。
//
// 显式 provider 优先；auto 模式下按以下优先级回落：
//   AIhubmix > Anthropic > OpenRouter > Mock
//
// 这个顺序的底层逻辑：国内开发者优先（AIhubmix 有国内节点），
// 然后是直连 Anthropic，最后是 OpenRouter 海外多模型路由。
func New(cfg Config) Client {
	mode := strings.ToLower(cfg.MockMode)
	if mode == "always" {
		return NewMock()
	}

	switch strings.ToLower(cfg.Provider) {
	case "anthropic":
		if k := firstNonEmpty(cfg.AnthropicKey, cfg.APIKey); k != "" {
			return NewAnthropic(k, cfg.Model)
		}
	case "aihubmix":
		if k := firstNonEmpty(cfg.AIhubmixKey, cfg.APIKey); k != "" {
			return NewAIhubmix(k, cfg.Model)
		}
	case "openrouter":
		if k := firstNonEmpty(cfg.OpenRouterKey, cfg.APIKey); k != "" {
			return NewOpenRouter(k, cfg.Model)
		}
	case "openai-compat":
		if k := firstNonEmpty(cfg.APIKey); k != "" && cfg.BaseURL != "" {
			return NewOpenAICompat(cfg.BaseURL, k, cfg.Model, "openai-compat")
		}
	}

	// auto fallback
	if cfg.AIhubmixKey != "" {
		return NewAIhubmix(cfg.AIhubmixKey, cfg.Model)
	}
	if cfg.AnthropicKey != "" {
		return NewAnthropic(cfg.AnthropicKey, cfg.Model)
	}
	if cfg.OpenRouterKey != "" {
		return NewOpenRouter(cfg.OpenRouterKey, cfg.Model)
	}

	return NewMock()
}

// NewLegacy 旧调用兼容
func NewLegacy(apiKey, model, mockMode string) Client {
	return New(Config{OpenRouterKey: apiKey, Model: model, MockMode: mockMode})
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
