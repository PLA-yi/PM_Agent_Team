package tools

import (
	"context"
	"testing"
)

func TestMockSearcher(t *testing.T) {
	s := NewMockSearcher()
	got, err := s.Search(context.Background(), "AI 笔记", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 results, got %d", len(got))
	}
	if got[0].URL == "" || got[0].Title == "" {
		t.Fatalf("missing fields: %+v", got[0])
	}
}

func TestMockScraper(t *testing.T) {
	s := NewMockScraper()
	out, err := s.Scrape(context.Background(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 50 {
		t.Fatalf("scrape too short: %s", out)
	}
}

func TestNewSearcherFactory(t *testing.T) {
	// always → mock
	if !NewSearcher(SearchConfig{MockMode: "always"}).IsMock() {
		t.Fatal("always should mock")
	}
	// auto + Tavily key → Tavily 包 Fallback
	s := NewSearcher(SearchConfig{TavilyKey: "k", MockMode: "auto"})
	fb, ok := s.(*FallbackSearcher)
	if !ok {
		t.Fatalf("want *FallbackSearcher, got %T", s)
	}
	if _, ok := fb.Primary.(*Tavily); !ok {
		t.Fatalf("want primary *Tavily, got %T", fb.Primary)
	}
	// auto + Jina key → Jina 包 Fallback
	s2 := NewSearcher(SearchConfig{JinaKey: "k", MockMode: "auto"})
	fb2, ok := s2.(*FallbackSearcher)
	if !ok {
		t.Fatalf("want *FallbackSearcher, got %T", s2)
	}
	if _, ok := fb2.Primary.(*JinaSearch); !ok {
		t.Fatalf("want primary *JinaSearch, got %T", fb2.Primary)
	}
	// auto 无 key → DDG（真免费默认）
	s3 := NewSearcher(SearchConfig{MockMode: "auto"})
	fb3, ok := s3.(*FallbackSearcher)
	if !ok {
		t.Fatalf("want *FallbackSearcher, got %T", s3)
	}
	if _, ok := fb3.Primary.(*DDGSearch); !ok {
		t.Fatalf("want primary *DDGSearch, got %T", fb3.Primary)
	}
	// 显式 mock
	if !NewSearcher(SearchConfig{Provider: "mock"}).IsMock() {
		t.Fatal("explicit mock should mock")
	}
}
