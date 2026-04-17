package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"pmhive/server/internal/tools/social"
)

// SocialListener 跨平台批量抓帖子。
//
// 双模式：
//   - 有 st.Candidates（嵌入 competitor_research）：每个候选竞品独立查，posts 写入 st.Posts
//   - 无 Candidates（独立 social_listening 场景）：直接查 st.Input，posts 写入 InterviewChunks 走 Clusterer
//
// 抓取策略：对支持 SearchExpanded 的平台（Reddit）走多 sort × 多时间窗扩展抓取，
// 单 query 可拿 200-400 帖；总目标可上千。
type SocialListener struct {
	K             int  // 每 query 目标帖数（默认 200）
	WriteToChunks bool // true = 写 InterviewChunks（独立场景）；false = 仅写 Posts（嵌入模式）
	Optional      bool // true = 失败不阻塞 pipeline
}

// expandedSearcher 平台如果支持批量扩展，实现这个接口
type expandedSearcher interface {
	SearchExpanded(ctx context.Context, keyword string, targetK int) ([]social.Post, error)
}

func (SocialListener) Name() string { return "social" }

// IsOptional 让 Coordinator 知道社聆失败不阻塞主任务
func (s SocialListener) IsOptional() bool { return s.Optional }

func (s SocialListener) Run(ctx context.Context, st *State, d Deps) error {
	mode := "standalone"
	if !s.WriteToChunks {
		mode = "embedded"
	}
	publish(d, st, "social", "start", map[string]interface{}{
		"input":      st.Input,
		"candidates": len(st.Candidates),
		"mode":       mode,
	})

	if d.Social == nil {
		err := fmt.Errorf("social registry not configured (Deps.Social=nil)")
		publish(d, st, "social", "error", map[string]string{"err": err.Error()})
		if s.Optional {
			return nil
		}
		return err
	}

	k := s.K
	if k <= 0 {
		k = 200
	}

	authedNames := []string{}
	for _, sc := range d.Social.All() {
		if sc.IsAuthenticated() {
			authedNames = append(authedNames, sc.Platform())
		}
	}
	publish(d, st, "social", "thought", map[string]interface{}{
		"authed_platforms": authedNames,
		"unauth_platforms": stubPlatforms(d.Social, authedNames),
	})

	if len(authedNames) == 0 {
		err := fmt.Errorf("无可用社交平台 — 配 X/Douyin/TikTok/YouTube key 或检查 Reddit")
		publish(d, st, "social", "error", map[string]string{"err": err.Error()})
		if s.Optional {
			return nil
		}
		return err
	}

	// 决定要查的 query 列表 + 每 query 对应的相关性别名集
	type queryPlan struct {
		query   string
		aliases []string // 用于结果相关性过滤
	}
	var plans []queryPlan
	if len(st.Candidates) > 0 {
		for _, c := range st.Candidates {
			plans = append(plans, queryPlan{
				query:   c.SearchAlias(), // 优先英文名
				aliases: c.AllAliases(),  // zh+en 都算相关
			})
		}
	} else {
		plans = []queryPlan{{query: st.Input, aliases: []string{st.Input}}}
	}

	var allPosts []social.Post
	seen := make(map[string]struct{}) // 跨 query 去重 key=platform:id
	for _, plan := range plans {
		for _, sc := range d.Social.All() {
			if !sc.IsAuthenticated() {
				continue
			}
			publish(d, st, "social", "tool_call", map[string]string{
				"engine":   sc.Platform(),
				"query":    plan.query,
				"target_k": fmt.Sprintf("%d", k),
				"strategy": "expanded+relevance_filter",
			})
			t0 := time.Now()
			var posts []social.Post
			var err error
			if exp, ok := sc.(expandedSearcher); ok {
				posts, err = exp.SearchExpanded(ctx, plan.query, k)
			} else {
				posts, err = sc.SearchByKeyword(ctx, plan.query, k)
			}
			if err != nil {
				publish(d, st, "social", "error", map[string]string{
					"platform": sc.Platform(),
					"query":    plan.query,
					"err":      err.Error(),
				})
				continue
			}
			// 相关性过滤 + dedup
			rawCount := len(posts)
			filtered := filterRelevant(posts, plan.aliases)
			added := 0
			for _, p := range filtered {
				key := p.Platform + ":" + p.ID
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
				allPosts = append(allPosts, p)
				added++
			}
			publish(d, st, "social", "tool_result", map[string]interface{}{
				"platform":     sc.Platform(),
				"query":        plan.query,
				"raw":          rawCount,
				"relevant":     len(filtered),
				"added":        added,
				"total_so_far": len(allPosts),
				"elapsed_ms":   time.Since(t0).Milliseconds(),
			})
		}
	}

	if len(allPosts) == 0 {
		err := fmt.Errorf("社交聆听：所有 query × 平台均无返回")
		publish(d, st, "social", "error", map[string]string{"err": err.Error()})
		if s.Optional {
			publish(d, st, "social", "done", map[string]int{"posts": 0})
			return nil
		}
		return err
	}

	st.mu.Lock()
	st.Posts = append(st.Posts, allPosts...)
	st.mu.Unlock()

	// 标准模式：转 chunks 给 Clusterer（cap 50 防 LLM 爆 context）
	if s.WriteToChunks {
		topN := allPosts
		if len(topN) > 50 {
			topN = topN[:50]
		}
		st.InterviewChunks = postsToChunks(topN)
	}

	// 写 sources cap 30：仅高代表性的进引用列表（剩余帖子从 store 查）
	srcCap := 30
	if len(allPosts) < srcCap {
		srcCap = len(allPosts)
	}
	for i := 0; i < srcCap; i++ {
		p := allPosts[i]
		st.AddSource(Source{
			URL:     p.URL,
			Title:   fmt.Sprintf("[%s] %s", p.Platform, p.Title),
			Snippet: shortPreview(p.Content, 200),
		})
	}

	publish(d, st, "social", "message", map[string]int{
		"posts_total":   len(allPosts),
		"sources_added": srcCap,
		"chunks_to_llm": map[bool]int{true: 50, false: 0}[s.WriteToChunks],
	})
	publish(d, st, "social", "done", nil)
	return nil
}

// filterRelevant 过滤掉与候选名无关的 posts。
// Reddit 全文搜索拆词后会带回大量 false positive（如 "妙记AI" → AITAH/AirPods 这种）。
// 规则：title + content + author 必须包含 aliases 中任一关键词（不区分大小写）。
// 注意：对中英文 alias 都做完整子串匹配。
func filterRelevant(posts []social.Post, aliases []string) []social.Post {
	if len(aliases) == 0 {
		return posts
	}
	// normalize aliases：小写 + 去空白
	keys := make([]string, 0, len(aliases))
	for _, a := range aliases {
		a = strings.TrimSpace(strings.ToLower(a))
		if a == "" || len(a) < 2 {
			continue
		}
		keys = append(keys, a)
	}
	if len(keys) == 0 {
		return posts
	}
	out := make([]social.Post, 0, len(posts))
	for _, p := range posts {
		hay := strings.ToLower(p.Title + " " + p.Content + " " + p.Author + " " + p.URL)
		for _, k := range keys {
			if strings.Contains(hay, k) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func postsToChunks(posts []social.Post) []string {
	chunks := make([]string, 0, len(posts))
	for _, p := range posts {
		var sb strings.Builder
		fmt.Fprintf(&sb, "[%s · u/%s · %d↑ %d💬] %s\n",
			p.Platform, p.Author, p.Engagement.Likes, p.Engagement.Comments, p.Title)
		if p.Content != "" {
			sb.WriteString(shortPreview(p.Content, 600))
		}
		chunks = append(chunks, sb.String())
	}
	return chunks
}

func stubPlatforms(reg *social.Registry, authed []string) []string {
	authedSet := make(map[string]bool, len(authed))
	for _, p := range authed {
		authedSet[p] = true
	}
	var out []string
	for _, sc := range reg.All() {
		if !authedSet[sc.Platform()] {
			out = append(out, sc.Platform())
		}
	}
	return out
}
