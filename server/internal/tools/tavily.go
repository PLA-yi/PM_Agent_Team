// Package tools 提供 Agent 调用的外部能力：搜索、抓取等。
package tools

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

// SearchResult 一条搜索结果
type SearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score,omitempty"`
}

// Searcher 搜索接口
type Searcher interface {
	Search(ctx context.Context, query string, k int) ([]SearchResult, error)
	IsMock() bool
}

// ---- Tavily 实现 ----

type Tavily struct {
	APIKey string
	HTTP   *http.Client
}

func NewTavily(apiKey string) *Tavily {
	return &Tavily{APIKey: apiKey, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (t *Tavily) IsMock() bool { return false }

type tavilyReq struct {
	APIKey            string `json:"api_key"`
	Query             string `json:"query"`
	SearchDepth       string `json:"search_depth"`
	MaxResults        int    `json:"max_results"`
	IncludeRawContent bool   `json:"include_raw_content"`
}

type tavilyResp struct {
	Results []SearchResult `json:"results"`
	Error   string         `json:"error,omitempty"`
}

func (t *Tavily) Search(ctx context.Context, query string, k int) ([]SearchResult, error) {
	if k <= 0 {
		k = 5
	}
	body, _ := json.Marshal(tavilyReq{
		APIKey: t.APIKey, Query: query, SearchDepth: "basic", MaxResults: k, IncludeRawContent: false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tavily %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed tavilyResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("tavily decode: %w; body=%s", err, string(respBody))
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("tavily: %s", parsed.Error)
	}
	return parsed.Results, nil
}

// ---- Mock Searcher ----
// ⚠️ 重要：mock searcher 不再返回写死的产品 URL（曾经写死 Notion/飞书/Obsidian
// 导致 fallback 时污染 LLM 上下文，产生不伦不类的报告）。
// 现在返回 query-aware 的占位结果：URL 指向 query 本身的元搜索链接，content
// 明确标记 "[FALLBACK_MOCK]"，让 LLM 知道这是 placeholder，应基于其它真实
// 数据（如 scraper 抓的页面）做判断。

type MockSearcher struct{}

func NewMockSearcher() *MockSearcher { return &MockSearcher{} }
func (MockSearcher) IsMock() bool    { return true }

func (MockSearcher) Search(_ context.Context, query string, k int) ([]SearchResult, error) {
	if k <= 0 {
		k = 3
	}
	if strings.TrimSpace(query) == "" {
		return []SearchResult{}, nil
	}
	// 生成 query-aware 占位结果（指向通用搜索引擎，避免硬编码任何垂类产品）
	encoded := strings.ReplaceAll(query, " ", "+")
	results := []SearchResult{
		{
			Title:   "[FALLBACK_MOCK] " + query + " — Bing 搜索",
			URL:     "https://www.bing.com/search?q=" + encoded,
			Content: "[FALLBACK_MOCK] 真实搜索源不可用（限流/无 key），此为占位结果。LLM 应忽略此条，基于 scraper 抓取到的真实页面或自身知识做判断。query=" + query,
			Score:   0.5,
		},
	}
	if k >= 2 {
		results = append(results, SearchResult{
			Title:   "[FALLBACK_MOCK] " + query + " — 知乎相关",
			URL:     "https://www.zhihu.com/search?type=content&q=" + encoded,
			Content: "[FALLBACK_MOCK] 占位结果，请忽略。query=" + query,
			Score:   0.4,
		})
	}
	if k >= 3 {
		results = append(results, SearchResult{
			Title:   "[FALLBACK_MOCK] " + query + " — Wikipedia",
			URL:     "https://zh.wikipedia.org/wiki/Special:Search?search=" + encoded,
			Content: "[FALLBACK_MOCK] 占位结果，请忽略。query=" + query,
			Score:   0.3,
		})
	}
	return results, nil
}

// SearchConfig 搜索 provider 配置
type SearchConfig struct {
	Provider  string // "tavily" | "jina" | "ddg" | "mock" | ""(auto)
	TavilyKey string
	JinaKey   string
	MockMode  string // auto / always / never
}

// NewSearcher 工厂。
// auto 模式优先级：Tavily key > Jina key > DuckDuckGo（免费无 key）> Mock
// 所有真实 provider 都包 FallbackSearcher：单次失败自动降级 mock，任务不全垮。
func NewSearcher(cfg SearchConfig) Searcher {
	mode := strings.ToLower(cfg.MockMode)
	if mode == "always" {
		return NewMockSearcher()
	}
	mock := NewMockSearcher()

	switch strings.ToLower(cfg.Provider) {
	case "tavily":
		if cfg.TavilyKey != "" {
			return NewFallback(NewTavily(cfg.TavilyKey), mock)
		}
	case "jina":
		if cfg.JinaKey != "" {
			return NewFallback(NewJinaSearch(cfg.JinaKey), mock)
		}
	case "ddg":
		return NewFallback(NewDDGSearch(), mock)
	case "mock":
		return mock
	}
	// auto
	if cfg.TavilyKey != "" {
		return NewFallback(NewTavily(cfg.TavilyKey), mock)
	}
	if cfg.JinaKey != "" {
		return NewFallback(NewJinaSearch(cfg.JinaKey), mock)
	}
	// 默认 DDG —— 真免费、无 key、立刻可用
	return NewFallback(NewDDGSearch(), mock)
}

// NewSearcherLegacy 旧调用兼容
func NewSearcherLegacy(tavilyKey, mockMode string) Searcher {
	return NewSearcher(SearchConfig{TavilyKey: tavilyKey, MockMode: mockMode})
}
