// scrape-demo: 跑通 Tier1 Reddit + Tier2 Crawler，输出真实数据证据。
// 用法：
//   go run ./cmd/scrape-demo reddit "AI product manager"
//   go run ./cmd/scrape-demo crawler https://b3log.org/siyuan
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"pmhive/server/internal/crawler"
	"pmhive/server/internal/tools/social"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: scrape-demo <reddit|crawler> <query|url>")
		os.Exit(2)
	}
	mode := os.Args[1]
	arg := os.Args[2]

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch mode {
	case "reddit":
		demoReddit(ctx, arg)
	case "hn", "hackernews":
		demoHN(ctx, arg)
	case "crawler":
		demoCrawler(ctx, arg)
	default:
		fmt.Println("unknown mode:", mode)
		os.Exit(2)
	}
}

func demoHN(ctx context.Context, query string) {
	hn := social.NewHackerNews()
	posts, err := hn.SearchExpanded(ctx, query, 30)
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	fmt.Printf("=== HN expanded search: %q → %d posts ===\n\n", query, len(posts))
	limit := 8
	if len(posts) < limit {
		limit = len(posts)
	}
	for i := 0; i < limit; i++ {
		p := posts[i]
		fmt.Printf("[%d] %s\n  by %s · %d↑ · %d 💬\n  %s\n\n",
			i+1, p.Title, p.Author, p.Engagement.Likes, p.Engagement.Comments, p.URL)
	}
}

func demoReddit(ctx context.Context, query string) {
	r := social.NewReddit()
	posts, err := r.SearchByKeyword(ctx, query, 5)
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	fmt.Printf("=== Reddit search: %q → %d posts ===\n\n", query, len(posts))
	for i, p := range posts {
		fmt.Printf("[%d] %s\n  by u/%s · %d↑ · %d 💬\n  %s\n  preview: %s\n\n",
			i+1, p.Title, p.Author, p.Engagement.Likes, p.Engagement.Comments,
			p.URL, truncate(p.Content, 120))
	}
	// 也输出 JSON 给程序消费
	out, _ := json.MarshalIndent(posts, "", "  ")
	_ = os.WriteFile("/tmp/reddit_demo.json", out, 0644)
	fmt.Println("→ raw JSON saved to /tmp/reddit_demo.json")
}

func demoCrawler(ctx context.Context, seed string) {
	cfg := crawler.DefaultConfig()
	cfg.Seeds = []string{seed}
	cfg.MaxDepth = 1
	cfg.MaxPages = 8
	cfg.PerDomainQPS = 1.0 // 1 req/s 礼貌
	cfg.OnPage = func(p crawler.Page) {
		status := "OK"
		if p.Err != "" {
			status = "ERR: " + p.Err
		}
		fmt.Printf("  d=%d  %s  [%s]  title=%q  links=%d\n",
			p.Depth, p.URL, status, truncate(p.Title, 50), len(p.Links))
	}

	fmt.Printf("=== Crawler BFS from %s (depth=1, max=8) ===\n", seed)
	c := crawler.New(cfg)
	pages, err := c.Run(ctx)
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	ok := 0
	for _, p := range pages {
		if p.Err == "" {
			ok++
		}
	}
	fmt.Printf("\n=== Done: %d pages fetched, %d successful ===\n", len(pages), ok)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
