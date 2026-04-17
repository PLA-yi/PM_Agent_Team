package agent

import (
	"context"
	"fmt"
	"sync"

	"pmhive/server/internal/tools"
)

// Search 用搜索引擎拉取每个候选产品的相关结果
type Search struct{}

func (Search) Name() string { return "search" }

func (Search) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "search", "start", map[string]int{"queries": len(st.Candidates)})

	if len(st.Candidates) == 0 {
		// fallback: 直接用 input 搜
		results, err := d.Search.Search(ctx, st.Input, 5)
		if err != nil {
			publish(d, st, "search", "error", map[string]string{"err": err.Error()})
			return err
		}
		st.SearchResults = append(st.SearchResults, results...)
		publish(d, st, "search", "done", map[string]int{"results": len(results)})
		return nil
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for _, c := range st.Candidates {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := c.Name + " 产品介绍 定价 用户评价"
			engine := searcherName(d.Search)
			publish(d, st, "search", "tool_call", map[string]string{"engine": engine, "query": query})
			results, err := d.Search.Search(ctx, query, 3)
			if err != nil {
				publish(d, st, "search", "error", map[string]string{"err": err.Error(), "query": query})
				return
			}
			publish(d, st, "search", "tool_result", map[string]interface{}{
				"query":   query,
				"count":   len(results),
				"top_url": firstURL(results),
			})
			mu.Lock()
			st.SearchResults = append(st.SearchResults, results...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(st.SearchResults) == 0 {
		err := fmt.Errorf("search agent: no results")
		publish(d, st, "search", "error", map[string]string{"err": err.Error()})
		return err
	}
	publish(d, st, "search", "done", map[string]int{"total_results": len(st.SearchResults)})
	return nil
}

// searcherName 反射出 searcher 类型名（穿过 FallbackSearcher）
func searcherName(s tools.Searcher) string {
	if fb, ok := s.(*tools.FallbackSearcher); ok {
		return searcherName(fb.Primary)
	}
	switch s.(type) {
	case *tools.Tavily:
		return "tavily"
	case *tools.JinaSearch:
		return "jina"
	case *tools.DDGSearch:
		return "duckduckgo"
	case *tools.MockSearcher:
		return "mock"
	}
	return "search"
}

func firstURL(rs []tools.SearchResult) string {
	if len(rs) == 0 {
		return ""
	}
	return rs[0].URL
}
