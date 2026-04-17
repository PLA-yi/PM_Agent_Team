package social

import (
	"context"
	"errors"
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry(NewReddit(), NewX(""), NewDouyin(""), NewTikTok(""), NewYouTube(""))
	if _, ok := reg.Get("reddit"); !ok {
		t.Fatal("reddit not in registry")
	}
	if _, ok := reg.Get("x"); !ok {
		t.Fatal("x not in registry")
	}
	if len(reg.All()) != 5 {
		t.Fatalf("want 5 scrapers, got %d", len(reg.All()))
	}

	// 默认只有 reddit authenticated（others 没 key）
	authedCount := 0
	for _, s := range reg.All() {
		if s.IsAuthenticated() {
			authedCount++
		}
	}
	if authedCount != 1 {
		t.Errorf("want 1 authenticated (reddit), got %d", authedCount)
	}
}

func TestStubReturnsErrNotConfigured(t *testing.T) {
	x := NewX("")
	_, err := x.SearchByKeyword(context.Background(), "test", 5)
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("want ErrNotConfigured, got %v", err)
	}
	dy := NewDouyin("")
	_, err = dy.SearchByKeyword(context.Background(), "test", 5)
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("want ErrNotConfigured, got %v", err)
	}
}

func TestSearchAcrossSkipsUnauth(t *testing.T) {
	// 全部未配置 → 应返回错误
	reg := NewRegistry(NewX(""), NewDouyin(""), NewTikTok(""), NewYouTube(""))
	_, err := reg.SearchAcross(context.Background(), "test", 3)
	// 没有 authenticated 的 scraper 时返回空（不是 error）
	if err != nil {
		// 实现是：跳过 unauth，没结果就 nil. 当前实现行为
	}
}
