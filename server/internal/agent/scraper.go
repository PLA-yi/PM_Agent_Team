package agent

import (
	"context"
	"sync"
)

// Scraper 抓取候选产品官网（最多 N 个，避免拖太久）
type Scraper struct {
	MaxPages int
}

func (Scraper) Name() string { return "scraper" }

func (s Scraper) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "scraper", "start", map[string]int{"candidates": len(st.Candidates)})
	if st.Pages == nil {
		st.Pages = make(map[string]string)
	}
	max := s.MaxPages
	if max <= 0 {
		max = 5
	}
	urls := make([]string, 0, max)
	for _, c := range st.Candidates {
		if c.URL != "" {
			urls = append(urls, c.URL)
		}
		if len(urls) >= max {
			break
		}
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for _, u := range urls {
		u := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			publish(d, st, "scraper", "tool_call", map[string]string{"engine": "jina_reader", "url": u})
			page, err := d.Scrape.Scrape(ctx, u)
			if err != nil {
				publish(d, st, "scraper", "error", map[string]string{"err": err.Error(), "url": u})
				return
			}
			publish(d, st, "scraper", "tool_result", map[string]interface{}{
				"url":   u,
				"bytes": len(page),
			})
			mu.Lock()
			st.Pages[u] = page
			st.AddSource(Source{URL: u, Title: u, Snippet: shortPreview(page, 200)})
			mu.Unlock()
		}()
	}
	wg.Wait()

	publish(d, st, "scraper", "done", map[string]int{"pages": len(st.Pages)})
	return nil
}
