package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// HackerNews 用 Algolia HN Search API（公开免费、无 key）
//
// Endpoint: https://hn.algolia.com/api/v1/search?query={kw}&tags=story
// 文档: https://hn.algolia.com/api
//
// 海外创业者/开发者真实声量，比 Reddit 更聚焦"早期产品 / 创业讨论"。
type HackerNews struct {
	HTTP *http.Client
}

func NewHackerNews() *HackerNews {
	return &HackerNews{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (HackerNews) Platform() string      { return "hackernews" }
func (HackerNews) IsAuthenticated() bool { return true } // 公开 API 不要 key

type hnSearchResp struct {
	Hits []hnHit `json:"hits"`
}

type hnHit struct {
	ObjectID    string `json:"objectID"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Author      string `json:"author"`
	Points      int64  `json:"points"`
	NumComments int64  `json:"num_comments"`
	StoryText   string `json:"story_text"`
	CreatedAtI  int64  `json:"created_at_i"`
	Tags        []string `json:"_tags"`
}

func (h *HackerNews) SearchByKeyword(ctx context.Context, keyword string, k int) ([]Post, error) {
	if k <= 0 {
		k = 20
	}
	if k > 100 {
		k = 100 // Algolia HN max hitsPerPage = 1000，但太多没必要
	}
	u := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&tags=story&hitsPerPage=%d",
		url.QueryEscape(keyword), k)
	return h.fetchHN(ctx, u)
}

// SearchExpanded 多策略 (relevance/recent + 不同 tag) 扩展抓取
func (h *HackerNews) SearchExpanded(ctx context.Context, keyword string, targetK int) ([]Post, error) {
	if targetK <= 0 {
		targetK = 100
	}
	strategies := []string{
		"https://hn.algolia.com/api/v1/search?query=%s&tags=story&hitsPerPage=100",
		"https://hn.algolia.com/api/v1/search_by_date?query=%s&tags=story&hitsPerPage=100",
		"https://hn.algolia.com/api/v1/search?query=%s&tags=ask_hn&hitsPerPage=100",
		"https://hn.algolia.com/api/v1/search?query=%s&tags=show_hn&hitsPerPage=100",
		"https://hn.algolia.com/api/v1/search?query=%s&tags=comment&hitsPerPage=100",
	}
	seen := map[string]struct{}{}
	out := []Post{}
	var errs []string
OUTER:
	for _, tmpl := range strategies {
		if len(out) >= targetK {
			break
		}
		u := fmt.Sprintf(tmpl, url.QueryEscape(keyword))
		posts, err := h.fetchHN(ctx, u)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		for _, p := range posts {
			if _, dup := seen[p.ID]; dup {
				continue
			}
			seen[p.ID] = struct{}{}
			out = append(out, p)
			if len(out) >= targetK {
				break OUTER
			}
		}
		// HN Algolia 限速 ~10k req/h，礼貌间隔
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return out, ctx.Err()
		}
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("hn expanded all failed: %v", errs)
	}
	return out, nil
}

// BlogPosts HN 不严格意义上有"用户主页帖子" — 用 author tag 过滤
func (h *HackerNews) BlogPosts(ctx context.Context, handle string, k int) ([]Post, error) {
	if k <= 0 {
		k = 20
	}
	u := fmt.Sprintf("https://hn.algolia.com/api/v1/search?tags=author_%s,story&hitsPerPage=%d",
		url.QueryEscape(handle), k)
	return h.fetchHN(ctx, u)
}

func (h *HackerNews) fetchHN(ctx context.Context, endpoint string) ([]Post, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PMHive/0.5 (https://github.com/PLA-yi/PM_Agent_Team)")

	resp, err := h.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hn http: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, fmt.Errorf("hn %d: %s", resp.StatusCode, preview)
	}

	var parsed hnSearchResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("hn decode: %w", err)
	}

	out := make([]Post, 0, len(parsed.Hits))
	for _, hit := range parsed.Hits {
		permalink := "https://news.ycombinator.com/item?id=" + hit.ObjectID
		if hit.URL != "" {
			permalink = hit.URL
		}
		content := hit.StoryText
		if content == "" {
			content = hit.Title
		}
		out = append(out, Post{
			Platform:  "hackernews",
			ID:        hit.ObjectID,
			Author:    hit.Author,
			URL:       permalink,
			Title:     hit.Title,
			Content:   content,
			CreatedAt: time.Unix(hit.CreatedAtI, 0).UTC(),
			Engagement: Engage{
				Likes:    hit.Points,
				Comments: hit.NumComments,
			},
			Lang: "en",
			Raw:  hit,
		})
	}
	return out, nil
}

// Helper: HN ID 转纯字符串便于稳定 ID
func hnHitIDStr(id int) string { return strconv.Itoa(id) }
