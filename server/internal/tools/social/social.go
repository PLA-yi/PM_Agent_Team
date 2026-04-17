// Package social 抽象社交平台数据抓取。
// 每个 platform 一个实现：reddit 真实可用（public JSON 无 key），
// X/Douyin/TikTok/YouTube 提供 stub + 接入指引。
package social

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Post 平台无关的帖子/视频/微博单元
type Post struct {
	Platform   string    `json:"platform"`            // reddit / x / douyin / tiktok / youtube
	ID         string    `json:"id"`
	Author     string    `json:"author"`
	URL        string    `json:"url"`
	Title      string    `json:"title,omitempty"`
	Content    string    `json:"content"`             // 正文 / 视频描述
	CreatedAt  time.Time `json:"created_at,omitempty"`
	Engagement Engage    `json:"engagement"`
	Lang       string    `json:"lang,omitempty"`
	Raw        any       `json:"-"`                   // 平台原始 payload（debug 用）
}

// Engage 互动数据
type Engage struct {
	Likes    int64 `json:"likes"`
	Comments int64 `json:"comments"`
	Shares   int64 `json:"shares"`
	Views    int64 `json:"views,omitempty"`
}

// Scraper 平台抓取接口
type Scraper interface {
	Platform() string
	IsAuthenticated() bool                                          // 是否配了 key/cookie，false 时一般 stub
	SearchByKeyword(ctx context.Context, keyword string, k int) ([]Post, error)
	BlogPosts(ctx context.Context, handle string, k int) ([]Post, error)
}

// ErrNotConfigured stub 实现返回此错误，提示用户配置 key/cookie
var ErrNotConfigured = errors.New("scraper not configured (need API key or cookie)")

// Registry 多平台路由
type Registry struct {
	scrapers map[string]Scraper
}

func NewRegistry(scrapers ...Scraper) *Registry {
	r := &Registry{scrapers: make(map[string]Scraper, len(scrapers))}
	for _, s := range scrapers {
		r.scrapers[s.Platform()] = s
	}
	return r
}

// Get 按 platform 名取
func (r *Registry) Get(platform string) (Scraper, bool) {
	s, ok := r.scrapers[strings.ToLower(platform)]
	return s, ok
}

// All 列出所有平台 + 鉴权状态
func (r *Registry) All() []Scraper {
	out := make([]Scraper, 0, len(r.scrapers))
	for _, s := range r.scrapers {
		out = append(out, s)
	}
	return out
}

// SearchAcross 跨平台搜索：依次调每个 authenticated 的 scraper
func (r *Registry) SearchAcross(ctx context.Context, keyword string, kPerPlatform int) ([]Post, error) {
	var all []Post
	var errs []string
	for _, s := range r.scrapers {
		if !s.IsAuthenticated() {
			continue
		}
		posts, err := s.SearchByKeyword(ctx, keyword, kPerPlatform)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", s.Platform(), err))
			continue
		}
		all = append(all, posts...)
	}
	if len(all) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all platforms failed: %s", strings.Join(errs, "; "))
	}
	return all, nil
}
