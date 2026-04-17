package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// JinaSearch 用 s.jina.ai 做免费 web 搜索（无需 key，低速率限制）。
// 文档：https://jina.ai/reader/  GET https://s.jina.ai/{encoded_query}
// Accept: application/json 返回结构化结果。
type JinaSearch struct {
	APIKey string // 可选；有 key 则速率限制更宽松
	HTTP   *http.Client
}

func NewJinaSearch(apiKey string) *JinaSearch {
	return &JinaSearch{
		APIKey: apiKey,
		HTTP:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (j *JinaSearch) IsMock() bool { return false }

// Jina Search JSON response
type jinaSearchResp struct {
	Code   int                   `json:"code"`
	Status int                   `json:"status"`
	Data   []jinaSearchResultRaw `json:"data"`
	Meta   map[string]any        `json:"meta,omitempty"`
}

type jinaSearchResultRaw struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// ErrSearchAuthRequired Jina/Tavily 等需要 key 的明确错误（factory 据此判断是否降级）
var ErrSearchAuthRequired = fmt.Errorf("search auth required")

func (j *JinaSearch) Search(ctx context.Context, query string, k int) ([]SearchResult, error) {
	if k <= 0 {
		k = 5
	}
	endpoint := "https://s.jina.ai/" + url.PathEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if j.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+j.APIKey)
	}

	resp, err := j.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jina search http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 || resp.StatusCode == 402 {
		return nil, fmt.Errorf("%w: %s", ErrSearchAuthRequired, truncateBody(string(body), 200))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jina search %d: %s", resp.StatusCode, truncateBody(string(body), 300))
	}
	var parsed jinaSearchResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("jina search decode: %w; body=%s", err, truncateBody(string(body), 300))
	}
	out := make([]SearchResult, 0, len(parsed.Data))
	for i, r := range parsed.Data {
		if i >= k {
			break
		}
		content := r.Description
		if content == "" {
			content = r.Content
		}
		out = append(out, SearchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Content: strings.TrimSpace(content),
			Score:   1.0 - float64(i)*0.05,
		})
	}
	return out, nil
}

func truncateBody(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
