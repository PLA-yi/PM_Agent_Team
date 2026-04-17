package tools

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
)

// FallbackSearcher 包装一个真实 searcher：失败/无授权时降级到 mock，单次任务不全垮。
// 也用于在 dev 时显示明确告警，提示用户配置 key。
type FallbackSearcher struct {
	Primary  Searcher
	Fallback Searcher
	warned   atomic.Bool
}

func NewFallback(primary, fallback Searcher) *FallbackSearcher {
	return &FallbackSearcher{Primary: primary, Fallback: fallback}
}

func (f *FallbackSearcher) IsMock() bool {
	// 对外报告非 mock —— 如果 primary 永久失败用户看得到 warning
	return f.Primary.IsMock()
}

func (f *FallbackSearcher) Search(ctx context.Context, query string, k int) ([]SearchResult, error) {
	out, err := f.Primary.Search(ctx, query, k)
	if err == nil {
		return out, nil
	}
	if errors.Is(err, ErrSearchAuthRequired) || isAuthError(err) {
		if f.warned.CompareAndSwap(false, true) {
			log.Printf("⚠️  Search primary 鉴权失败，降级 mock。补 TAVILY_API_KEY 或 JINA_API_KEY 即可走真实搜索。 err=%v", err)
		}
		return f.Fallback.Search(ctx, query, k)
	}
	log.Printf("⚠️  Search primary 其他失败，降级 mock。 err=%v", err)
	return f.Fallback.Search(ctx, query, k)
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, k := range []string{"401", "402", "Authentication", "Unauthorized", "credit"} {
		if contains(s, k) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
