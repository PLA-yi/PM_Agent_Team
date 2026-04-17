package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Scraper 抓取并转换为 markdown
type Scraper interface {
	Scrape(ctx context.Context, url string) (string, error)
	IsMock() bool
}

// Jina Reader: https://r.jina.ai/{url}
type Jina struct {
	HTTP *http.Client
}

func NewJina() *Jina {
	return &Jina{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (j *Jina) IsMock() bool { return false }

func (j *Jina) Scrape(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("empty url")
	}
	endpoint := "https://r.jina.ai/" + target
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/markdown")
	resp, err := j.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("jina http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("jina %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

// MockScraper 离线 fixture
type MockScraper struct{}

func NewMockScraper() *MockScraper { return &MockScraper{} }
func (MockScraper) IsMock() bool   { return true }

func (MockScraper) Scrape(_ context.Context, target string) (string, error) {
	return fmt.Sprintf(`# Mock Scrape: %s

这是一段 mock 抓取内容，用于离线 demo。真实运行时由 Jina Reader 返回页面 markdown。

## 主要功能
- 多端同步
- 团队协作
- 智能搜索

## 定价
- 个人版：免费
- 团队版：按席位计费

## 用户评价摘要
"产品稳定、体验流畅，但 AI 能力相对简单。"
`, target), nil
}

func NewScraperAuto(mockAlways bool) Scraper {
	if mockAlways {
		return NewMockScraper()
	}
	return NewJina()
}
