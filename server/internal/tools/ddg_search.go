package tools

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 真实浏览器 UA 池（轮换，降低被识别为爬虫的概率）
var ddgUserAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7; rv:122.0) Gecko/20100101 Firefox/122.0",
}

// DDGSearch 通过 html.duckduckgo.com/html/ 抓取搜索结果。
// 真免费、无需 key；解析 HTML pattern 即可。
// 限制：DDG 偶尔限流（返回空结果），fallback 兜底处理。
type DDGSearch struct {
	HTTP *http.Client
}

func NewDDGSearch() *DDGSearch {
	return &DDGSearch{HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (DDGSearch) IsMock() bool { return false }

var (
	ddgResultRe = regexp.MustCompile(
		`(?s)<a class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>.*?` +
			`<a class="result__snippet"[^>]*>(.*?)</a>`,
	)
	ddgTagStrip = regexp.MustCompile(`<[^>]+>`)
	ddgWS       = regexp.MustCompile(`\s+`)
)

func (d *DDGSearch) Search(ctx context.Context, query string, k int) ([]SearchResult, error) {
	if k <= 0 {
		k = 5
	}
	// 重试 3 次，指数 backoff + 抖动；每次换 UA
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			// 退避：500ms / 1.5s / 3.5s + 随机抖动
			backoff := time.Duration(500*(1<<attempt)) * time.Millisecond
			jitter := time.Duration(rand.Intn(500)) * time.Millisecond
			select {
			case <-time.After(backoff + jitter):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		out, err := d.searchOnce(ctx, query, k)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (d *DDGSearch) searchOnce(ctx context.Context, query string, k int) ([]SearchResult, error) {
	form := url.Values{}
	form.Set("q", query)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://html.duckduckgo.com/html/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", ddgUserAgents[rand.Intn(len(ddgUserAgents))])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", "https://duckduckgo.com/")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")

	resp, err := d.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ddg http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 429 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("ddg %d (rate-limited)", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ddg %d: %s", resp.StatusCode, truncateBody(string(body), 200))
	}

	matches := ddgResultRe.FindAllStringSubmatch(string(body), -1)
	out := make([]SearchResult, 0, len(matches))
	for i, m := range matches {
		if i >= k {
			break
		}
		rawURL := decodeDDGRedirect(m[1])
		title := cleanHTML(m[2])
		snippet := cleanHTML(m[3])
		if rawURL == "" || title == "" {
			continue
		}
		out = append(out, SearchResult{
			Title:   title,
			URL:     rawURL,
			Content: snippet,
			Score:   1.0 - float64(i)*0.05,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ddg: no results parsed (likely soft-rate-limited)")
	}
	return out, nil
}

// decodeDDGRedirect DDG 用 //duckduckgo.com/l/?uddg=<encoded_url>&... 包了一层重定向
func decodeDDGRedirect(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if uddg := u.Query().Get("uddg"); uddg != "" {
		if decoded, err := url.QueryUnescape(uddg); err == nil {
			return decoded
		}
	}
	return raw
}

func cleanHTML(s string) string {
	s = ddgTagStrip.ReplaceAllString(s, "")
	s = ddgWS.ReplaceAllString(s, " ")
	return strings.TrimSpace(htmlUnescape(s))
}

// htmlUnescape 简化版（DDG 用得到的几个 entity）
func htmlUnescape(s string) string {
	repl := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#x27;", "'",
		"&#39;", "'",
		"&nbsp;", " ",
	)
	return repl.Replace(s)
}
