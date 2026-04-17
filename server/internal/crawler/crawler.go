// Package crawler 简易 BFS web 爬虫。
//
// 设计原则：
//   1. 端到端可独立运行 —— 不依赖 PMHive 业务层
//   2. 礼貌（polite）爬取 —— 尊重 robots.txt + per-domain rate limit + 真实 UA
//   3. 可观测 —— 每步通过 Hook 暴露事件，调用方可监听
//   4. 边界明确 —— MaxDepth / MaxPages / MaxDomains 三层硬上限，防止失控
//
// 使用：见 example_test.go
package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Page 抓回的页面快照
type Page struct {
	URL        string    `json:"url"`
	StatusCode int       `json:"status_code"`
	Body       string    `json:"body"`        // 仅当 Config.StoreBody=true 时填充
	Title      string    `json:"title"`
	Links      []string  `json:"links"`       // 提取出来的链接（已规范化）
	FetchedAt  time.Time `json:"fetched_at"`
	BytesSize  int       `json:"bytes_size"`
	Depth      int       `json:"depth"`
	Err        string    `json:"err,omitempty"`
}

// Config 爬虫配置
type Config struct {
	Seeds          []string      // 起点 URL 列表
	MaxDepth       int           // BFS 最大深度（seed=0）
	MaxPages       int           // 全局抓取上限
	MaxConcurrency int           // 并发数（per-domain 受 rate-limit 约束更严）
	PerDomainQPS   float64       // 每域名每秒最大请求数（如 0.5 = 2 秒一次）
	UserAgent      string
	Timeout        time.Duration
	AllowDomains   []string      // 白名单域名后缀（如 "example.com"）；为空 = 同 seed 域名
	StoreBody      bool          // 是否保留 body（调研型 true，海量爬 false 省内存）
	RespectRobots  bool          // 默认 true
	OnPage         func(p Page)  // 每页回调（可选，用于流式消费）
}

// DefaultConfig 礼貌默认值
func DefaultConfig() Config {
	return Config{
		MaxDepth:       2,
		MaxPages:       50,
		MaxConcurrency: 4,
		PerDomainQPS:   0.5, // 2s/req per domain — 默认很保守
		UserAgent:      "PMHiveCrawler/0.1 (+https://github.com/pmhive)",
		Timeout:        15 * time.Second,
		StoreBody:      true,
		RespectRobots:  true,
	}
}

// Crawler BFS 爬虫
type Crawler struct {
	cfg      Config
	http     *http.Client
	robots   *robotsCache
	domainTB *domainBuckets // 每域名令牌桶
	visited  sync.Map       // url string → struct{}
	allowSet map[string]struct{}
}

// New 创建 crawler
func New(cfg Config) *Crawler {
	if cfg.MaxDepth == 0 {
		cfg.MaxDepth = 2
	}
	if cfg.MaxPages == 0 {
		cfg.MaxPages = 50
	}
	if cfg.MaxConcurrency == 0 {
		cfg.MaxConcurrency = 4
	}
	if cfg.PerDomainQPS == 0 {
		cfg.PerDomainQPS = 0.5
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "PMHiveCrawler/0.1"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	allow := make(map[string]struct{}, len(cfg.AllowDomains))
	for _, d := range cfg.AllowDomains {
		allow[strings.ToLower(d)] = struct{}{}
	}
	if len(allow) == 0 {
		// 默认 allow 所有 seed 的 host
		for _, seed := range cfg.Seeds {
			if u, err := url.Parse(seed); err == nil {
				allow[strings.ToLower(u.Host)] = struct{}{}
			}
		}
	}
	return &Crawler{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout,
		},
		robots:   newRobotsCache(cfg.UserAgent),
		domainTB: newDomainBuckets(cfg.PerDomainQPS),
		allowSet: allow,
	}
}

// Run 执行 BFS 爬取，返回所有抓到的 Page（含失败的，Err 字段标记）。
// 实现：递归 spawn + 信号量限并发 + WaitGroup 等终止。
// 这是 dynamic-spawn pattern 的标准 Go 写法 —— wg.Add 必须在 goroutine 启动前同步调用。
func (c *Crawler) Run(ctx context.Context) ([]Page, error) {
	if len(c.cfg.Seeds) == 0 {
		return nil, fmt.Errorf("no seeds")
	}

	pages := make([]Page, 0, c.cfg.MaxPages)
	var pagesMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.cfg.MaxConcurrency)

	// process 递归处理一个 URL：fetch → collect → spawn children
	var process func(rawURL string, depth int)
	process = func(rawURL string, depth int) {
		defer wg.Done()

		// 全局上限早退
		pagesMu.Lock()
		if len(pages) >= c.cfg.MaxPages {
			pagesMu.Unlock()
			return
		}
		pagesMu.Unlock()

		// 信号量限并发（受 ctx 取消保护）
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-sem }()

		page := c.fetchOne(ctx, rawURL, depth)

		pagesMu.Lock()
		pages = append(pages, page)
		pagesMu.Unlock()

		if c.cfg.OnPage != nil {
			c.cfg.OnPage(page)
		}

		// 入队子链接
		if page.Err == "" && depth < c.cfg.MaxDepth {
			for _, link := range page.Links {
				if _, loaded := c.visited.LoadOrStore(link, struct{}{}); loaded {
					continue
				}
				if !c.allowedHost(link) {
					continue
				}
				wg.Add(1)
				go process(link, depth+1)
			}
		}
	}

	// 入队 seeds
	for _, seed := range c.cfg.Seeds {
		c.visited.Store(seed, struct{}{})
		wg.Add(1)
		go process(seed, 0)
	}

	wg.Wait()
	return pages, nil
}

// fetchOne 抓单页
func (c *Crawler) fetchOne(ctx context.Context, rawURL string, depth int) Page {
	page := Page{URL: rawURL, Depth: depth, FetchedAt: time.Now()}

	u, err := url.Parse(rawURL)
	if err != nil {
		page.Err = "parse: " + err.Error()
		return page
	}

	// robots.txt 检查
	if c.cfg.RespectRobots {
		allowed, _ := c.robots.Allowed(ctx, u, c.http)
		if !allowed {
			page.Err = "blocked by robots.txt"
			return page
		}
	}

	// per-domain rate limit
	c.domainTB.Wait(ctx, u.Host)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		page.Err = err.Error()
		return page
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := c.http.Do(req)
	if err != nil {
		page.Err = "http: " + err.Error()
		return page
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	page.StatusCode = resp.StatusCode
	page.BytesSize = len(body)

	if resp.StatusCode >= 400 {
		page.Err = fmt.Sprintf("status %d", resp.StatusCode)
		return page
	}

	html := string(body)
	if c.cfg.StoreBody {
		page.Body = html
	}
	page.Title = extractTitle(html)
	page.Links = extractLinks(rawURL, html)
	return page
}

// allowedHost 判断 URL 是否在白名单
func (c *Crawler) allowedHost(rawURL string) bool {
	if len(c.allowSet) == 0 {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	for allow := range c.allowSet {
		if host == allow || strings.HasSuffix(host, "."+allow) {
			return true
		}
	}
	return false
}

// ===== HTML 解析（最小化：title + a[href]） =====

var (
	titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	hrefRe  = regexp.MustCompile(`(?i)<a[^>]+href=["']([^"']+)["']`)
	wsRe    = regexp.MustCompile(`\s+`)
)

func extractTitle(html string) string {
	m := titleRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(wsRe.ReplaceAllString(m[1], " "))
}

func extractLinks(baseURL, html string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	matches := hrefRe.FindAllStringSubmatch(html, -1)
	seen := make(map[string]struct{})
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		// 跳过 anchor-only / mailto / javascript:
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		ref, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if ref.Scheme != "" && ref.Scheme != "http" && ref.Scheme != "https" {
			continue
		}
		abs := base.ResolveReference(ref)
		abs.Fragment = ""
		s := abs.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
