package social

import (
	"context"
	"fmt"
)

// 以下为需要 auth/cookie 的平台 stub。
// 鉴权后接入策略写在每个 NewXxx 注释里 —— 用户配 key/cookie 时，
// 把 stub 实现替换为真实 client，实现 Scraper 接口即可。

// X (Twitter) — 需要 X API v2 Bearer Token 或 Tweepy Cookie
type X struct {
	BearerToken string
	HTTP        any
}

func NewX(bearer string) *X { return &X{BearerToken: bearer} }
func (X) Platform() string  { return "x" }
func (x X) IsAuthenticated() bool {
	return x.BearerToken != ""
}

// 接入指引（实现要点）：
//   POST https://api.twitter.com/2/tweets/search/recent?query={kw}
//   Headers: Authorization: Bearer {BearerToken}
//   付费 Basic tier $200/月 起；free tier 仅 100 reads/月
func (x *X) SearchByKeyword(_ context.Context, keyword string, _ int) ([]Post, error) {
	if !x.IsAuthenticated() {
		return nil, fmt.Errorf("%w: set X_BEARER_TOKEN env to enable. Get from developer.x.com", ErrNotConfigured)
	}
	return nil, fmt.Errorf("X scraper: not yet implemented (TODO: integrate v2 Search API for keyword=%s)", keyword)
}
func (x *X) BlogPosts(_ context.Context, handle string, _ int) ([]Post, error) {
	if !x.IsAuthenticated() {
		return nil, fmt.Errorf("%w: set X_BEARER_TOKEN env", ErrNotConfigured)
	}
	return nil, fmt.Errorf("X scraper: not yet implemented (TODO: GET /2/users/by/username/%s + tweets)", handle)
}

// Douyin — 需要 cookie（无官方公开 API），通过 cookie 模拟浏览器
type Douyin struct {
	Cookie string
}

func NewDouyin(cookie string) *Douyin { return &Douyin{Cookie: cookie} }
func (Douyin) Platform() string       { return "douyin" }
func (d Douyin) IsAuthenticated() bool {
	return d.Cookie != ""
}

// 接入指引：
//   关键词搜索：https://www.douyin.com/aweme/v1/web/search/item/?keyword=...
//   博主主页：https://www.douyin.com/aweme/v1/web/aweme/post/?sec_user_id=...
//   需 cookie + msToken + X-Bogus 签名（用 NodeJS 跑签名脚本生成）
func (d *Douyin) SearchByKeyword(_ context.Context, keyword string, _ int) ([]Post, error) {
	if !d.IsAuthenticated() {
		return nil, fmt.Errorf("%w: set DOUYIN_COOKIE env (从浏览器 devtools 复制)", ErrNotConfigured)
	}
	return nil, fmt.Errorf("Douyin scraper: not yet implemented (TODO: web/search/item/ + X-Bogus 签名). keyword=%s", keyword)
}
func (d *Douyin) BlogPosts(_ context.Context, handle string, _ int) ([]Post, error) {
	if !d.IsAuthenticated() {
		return nil, fmt.Errorf("%w: set DOUYIN_COOKIE env", ErrNotConfigured)
	}
	return nil, fmt.Errorf("Douyin scraper: not yet implemented. handle=%s", handle)
}

// TikTok — 需要 sessionid cookie（境外）
type TikTok struct {
	SessionID string
}

func NewTikTok(sessionID string) *TikTok { return &TikTok{SessionID: sessionID} }
func (TikTok) Platform() string          { return "tiktok" }
func (t TikTok) IsAuthenticated() bool   { return t.SessionID != "" }

// 接入指引：
//   GET https://www.tiktok.com/api/search/general/full/?keyword=...
//   Cookie: sessionid={SessionID}
//   需 ttwid + msToken 签名
func (t *TikTok) SearchByKeyword(_ context.Context, keyword string, _ int) ([]Post, error) {
	if !t.IsAuthenticated() {
		return nil, fmt.Errorf("%w: set TIKTOK_SESSIONID env", ErrNotConfigured)
	}
	return nil, fmt.Errorf("TikTok scraper: not yet implemented. keyword=%s", keyword)
}
func (t *TikTok) BlogPosts(_ context.Context, handle string, _ int) ([]Post, error) {
	if !t.IsAuthenticated() {
		return nil, fmt.Errorf("%w: set TIKTOK_SESSIONID env", ErrNotConfigured)
	}
	return nil, fmt.Errorf("TikTok scraper: not yet implemented. handle=%s", handle)
}

// YouTube — 用 Google YouTube Data API v3（需 API Key，免费 quota 10k units/天）
type YouTube struct {
	APIKey string
}

func NewYouTube(apiKey string) *YouTube { return &YouTube{APIKey: apiKey} }
func (YouTube) Platform() string        { return "youtube" }
func (y YouTube) IsAuthenticated() bool { return y.APIKey != "" }

// 接入指引：
//   GET https://www.googleapis.com/youtube/v3/search?q=...&type=video&key={APIKey}
//   Quota: search.list 100 units/call，10k/day = 100 searches/day
//   Get key: console.cloud.google.com → APIs & Services → YouTube Data API v3
func (y *YouTube) SearchByKeyword(_ context.Context, keyword string, _ int) ([]Post, error) {
	if !y.IsAuthenticated() {
		return nil, fmt.Errorf("%w: set YOUTUBE_API_KEY env (free 10k units/day)", ErrNotConfigured)
	}
	return nil, fmt.Errorf("YouTube scraper: not yet implemented (TODO: youtube/v3/search). keyword=%s", keyword)
}
func (y *YouTube) BlogPosts(_ context.Context, handle string, _ int) ([]Post, error) {
	if !y.IsAuthenticated() {
		return nil, fmt.Errorf("%w: set YOUTUBE_API_KEY env", ErrNotConfigured)
	}
	return nil, fmt.Errorf("YouTube scraper: not yet implemented. handle=%s", handle)
}
