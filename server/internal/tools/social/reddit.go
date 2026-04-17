package social

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Reddit 通过 public Atom RSS feed 抓帖子（无需 key）。
//
// 2024+ 反爬升级后，Reddit 把 .json endpoint 加了 Cloudflare WAF
// （datacenter IP + 无 cookie 直接 403）。RSS feed 仍宽松。
//
// 关键词：https://www.reddit.com/search.rss?q=...&limit=N
// 用户主页：https://www.reddit.com/user/{handle}/submitted.rss?limit=N
//
// Atom 格式只暴露 title/author/link/published，互动数据（upvotes/comments）拿不到。
// 那是 trade-off — 要互动数据需要 OAuth 走 official API（v0.6 接入）。
type Reddit struct {
	UserAgent string
	HTTP      *http.Client
}

func NewReddit() *Reddit {
	return &Reddit{
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		HTTP:      &http.Client{Timeout: 20 * time.Second},
	}
}

func (Reddit) Platform() string      { return "reddit" }
func (Reddit) IsAuthenticated() bool { return true }

// Atom feed schema（最小化）
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
	Author    atomAuthor `xml:"author"`
	Link      atomLink   `xml:"link"`
	Content   atomText   `xml:"content"`
}

type atomAuthor struct {
	Name string `xml:"name"`
	URI  string `xml:"uri"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

func (r *Reddit) SearchByKeyword(ctx context.Context, keyword string, k int) ([]Post, error) {
	if k <= 0 {
		k = 10
	}
	if k > 100 {
		k = 100
	}
	u := fmt.Sprintf("https://www.reddit.com/search.rss?q=%s&limit=%d&sort=relevance",
		url.QueryEscape(keyword), k)
	return r.fetchFeed(ctx, u)
}

// SearchExpanded 多 sort × 多时间窗扩展抓取，返回 dedup 后的 posts。
// targetK ≈ 期望条数；实际返回受 Reddit RSS 100/call 上限和 dedup 影响。
//
// 策略：5 sort (relevance/top/new/hot/comments) × 3 时间窗 (year/month/all)
// = 15 calls, 上限 1500 (dedup 后通常 200-400 unique)
func (r *Reddit) SearchExpanded(ctx context.Context, keyword string, targetK int) ([]Post, error) {
	if targetK <= 0 {
		targetK = 200
	}
	sorts := []string{"relevance", "top", "new", "hot", "comments"}
	tWindows := []string{"year", "month", "all"}

	seen := make(map[string]struct{}) // dedup by post ID
	var allPosts []Post
	var errs []string

OUTER:
	for _, sort := range sorts {
		for _, tw := range tWindows {
			if len(allPosts) >= targetK {
				break OUTER
			}
			u := fmt.Sprintf("https://www.reddit.com/search.rss?q=%s&limit=100&sort=%s&t=%s",
				url.QueryEscape(keyword), sort, tw)
			posts, err := r.fetchFeed(ctx, u)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: %v", sort, tw, err))
				// 限流时主动 sleep
				if isReddiRateLimit(err) {
					select {
					case <-time.After(2 * time.Second):
					case <-ctx.Done():
						return allPosts, ctx.Err()
					}
				}
				continue
			}
			for _, p := range posts {
				if _, dup := seen[p.ID]; dup {
					continue
				}
				seen[p.ID] = struct{}{}
				allPosts = append(allPosts, p)
				if len(allPosts) >= targetK {
					break OUTER
				}
			}
			// 礼貌间隔，降低被限流概率
			select {
			case <-time.After(250 * time.Millisecond):
			case <-ctx.Done():
				return allPosts, ctx.Err()
			}
		}
	}

	if len(allPosts) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("reddit expanded search 全失败: %s", strings.Join(errs, "; "))
	}
	return allPosts, nil
}

func isReddiRateLimit(err error) bool {
	s := err.Error()
	return strings.Contains(s, "429") || strings.Contains(s, "rate-limited")
}

func (r *Reddit) BlogPosts(ctx context.Context, handle string, k int) ([]Post, error) {
	if k <= 0 {
		k = 10
	}
	handle = strings.TrimPrefix(handle, "u/")
	handle = strings.TrimPrefix(handle, "/u/")
	u := fmt.Sprintf("https://www.reddit.com/user/%s/submitted.rss?limit=%d&sort=new",
		url.PathEscape(handle), k)
	return r.fetchFeed(ctx, u)
}

func (r *Reddit) fetchFeed(ctx context.Context, endpoint string) ([]Post, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", r.UserAgent)
	req.Header.Set("Accept", "application/atom+xml,application/rss+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := r.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("reddit rate-limited (429)")
	}
	if resp.StatusCode >= 400 {
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, fmt.Errorf("reddit %d: %s", resp.StatusCode, preview)
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("reddit xml decode: %w", err)
	}

	posts := make([]Post, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		published, _ := time.Parse(time.RFC3339, e.Published)
		posts = append(posts, Post{
			Platform:  "reddit",
			ID:        atomEntryID(e.ID),
			Author:    strings.TrimPrefix(e.Author.Name, "/u/"),
			URL:       e.Link.Href,
			Title:     strings.TrimSpace(e.Title),
			Content:   stripHTMLBasic(e.Content.Body),
			CreatedAt: published.UTC(),
			Lang:      "en",
			// Atom feed 不暴露互动数据 —— 留空，需 OAuth 走 official API 才有
			Engagement: Engage{},
		})
	}
	return posts, nil
}

// atomEntryID Reddit Atom id 格式: t3_xxxxx
func atomEntryID(id string) string {
	if i := strings.LastIndexByte(id, '/'); i >= 0 {
		return id[i+1:]
	}
	return id
}

var (
	htmlTagRe   = regexp.MustCompile(`<[^>]+>`)
	htmlSpaceRe = regexp.MustCompile(`\s+`)
)

func stripHTMLBasic(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	s = htmlSpaceRe.ReplaceAllString(s, " ")
	// 简化常见 entity
	s = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`,
		"&#x27;", "'", "&#39;", "'", "&nbsp;", " ",
	).Replace(s)
	return strings.TrimSpace(s)
}
