package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExtractTitle(t *testing.T) {
	cases := map[string]string{
		`<html><head><title>Hello World</title></head>`:        "Hello World",
		`<HTML><HEAD><TITLE>UPPER</TITLE>`:                     "UPPER",
		`<html><head><title>多行
带空格</title>`:                                                 "多行 带空格",
		`<html><head>no title here`:                            "",
	}
	for in, want := range cases {
		got := extractTitle(in)
		if got != want {
			t.Errorf("extractTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractLinks(t *testing.T) {
	html := `
<html><body>
  <a href="https://example.com/a">A</a>
  <a href="/relative">Rel</a>
  <a href="?q=foo">Query</a>
  <a href="mailto:x@y.com">Mail</a>
  <a href="#anchor">Anchor</a>
  <a href="javascript:void(0)">JS</a>
  <a href="https://example.com/a">Dup</a>
</body></html>`
	links := extractLinks("https://example.com/", html)
	want := []string{
		"https://example.com/a",
		"https://example.com/relative",
		"https://example.com/?q=foo",
	}
	if len(links) != len(want) {
		t.Fatalf("want %d links, got %d: %v", len(want), len(links), links)
	}
	for i, w := range want {
		if links[i] != w {
			t.Errorf("link[%d] = %q, want %q", i, links[i], w)
		}
	}
}

func TestParseRobots(t *testing.T) {
	robots := `# Comment
User-agent: BadBot
Disallow: /

User-agent: *
Disallow: /private
Disallow: /admin
Allow: /public
`
	rules := parseRobots(robots)
	if len(rules.disallowPrefixes) != 2 {
		t.Errorf("want 2 disallow, got %v", rules.disallowPrefixes)
	}
	if len(rules.allowPrefixes) != 1 {
		t.Errorf("want 1 allow, got %v", rules.allowPrefixes)
	}
	if !matchRobots(rules, "/foo") {
		t.Error("/foo should be allowed")
	}
	if matchRobots(rules, "/private/secret") {
		t.Error("/private/secret should be disallowed")
	}
	// allow 优先于 disallow
	if !matchRobots(rules, "/public/data") {
		t.Error("/public/data should be allowed")
	}
}

func TestRateLimit(t *testing.T) {
	rb := newDomainBuckets(10) // 10 qps = 100ms 间隔
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 5; i++ {
		rb.Wait(ctx, "example.com")
	}
	elapsed := time.Since(start)
	// 5 次调用，前 1 次免等，后 4 次每次等 100ms ≈ 400ms 总
	if elapsed < 350*time.Millisecond {
		t.Errorf("rate limit too fast: %v", elapsed)
	}
	if elapsed > 800*time.Millisecond {
		t.Errorf("rate limit too slow: %v", elapsed)
	}
}

// TestCrawlerEndToEnd 用 httptest 起一个有链接图的小站，验证 BFS + dedup + depth 限制
func TestCrawlerEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	var hits atomic.Int64
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nAllow: /\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		path := r.URL.Path
		switch path {
		case "/":
			fmt.Fprint(w, `<html><head><title>Index</title></head><body>
				<a href="/page1">P1</a>
				<a href="/page2">P2</a>
				<a href="/admin">Admin (will be 404 / linked but not allowed)</a>
			</body></html>`)
		case "/page1":
			fmt.Fprint(w, `<html><head><title>Page1</title></head><body>
				<a href="/page3">P3</a>
				<a href="/">Back to Index</a>
			</body></html>`)
		case "/page2":
			fmt.Fprint(w, `<html><head><title>Page2</title></head></html>`)
		case "/page3":
			fmt.Fprint(w, `<html><head><title>Page3</title></head></html>`)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.Seeds = []string{srv.URL + "/"}
	cfg.MaxDepth = 2
	cfg.MaxPages = 20
	cfg.PerDomainQPS = 50 // 测试加速
	cfg.RespectRobots = true
	cfg.OnPage = func(p Page) {
		t.Logf("fetched depth=%d url=%s status=%d", p.Depth, p.URL, p.StatusCode)
	}

	c := New(cfg)
	pages, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// 期望：4 个 200 页面 (/, /page1, /page2, /page3) + 1 个 404 (/admin)
	successCount := 0
	urls := make(map[string]bool)
	for _, p := range pages {
		urls[p.URL] = true
		if p.StatusCode == 200 {
			successCount++
		}
	}
	if successCount < 4 {
		t.Errorf("want >=4 success pages, got %d. URLs: %v", successCount, urls)
	}

	// dedup: / 不应被抓两次
	pageCount := map[string]int{}
	for _, p := range pages {
		pageCount[p.URL]++
	}
	for u, c := range pageCount {
		if c > 1 {
			t.Errorf("dedup failed: %s fetched %d times", u, c)
		}
	}

	t.Logf("crawler OK: %d pages fetched, %d unique URLs, %d server hits",
		len(pages), len(urls), hits.Load())
}

// TestRobotsBlock 验证 robots.txt 拒绝路径不被抓
func TestRobotsBlock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/private") {
			t.Errorf("robots-blocked URL was fetched: %s", r.URL.Path)
		}
		fmt.Fprint(w, `<html><body><a href="/private/secret">Secret</a><a href="/public">Public</a></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.Seeds = []string{srv.URL + "/"}
	cfg.MaxDepth = 2
	cfg.PerDomainQPS = 50

	c := New(cfg)
	pages, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if strings.Contains(p.URL, "/private") && p.Err != "blocked by robots.txt" {
			t.Errorf("expected /private to be blocked, got: %+v", p)
		}
	}
}
