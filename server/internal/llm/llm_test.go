package llm

import (
	"context"
	"strings"
	"testing"
)

func TestMockRouting(t *testing.T) {
	m := NewMock()
	m.LatencyMs = 0
	cases := []struct {
		role    string
		want    string
	}{
		{"PLANNER_AGENT", "outline"},
		{"EXTRACTOR_AGENT", "competitors"},
		{"ANALYZER_AGENT", "swot"},
		{"WRITER_AGENT", "竞品调研报告"},
	}
	for _, c := range cases {
		t.Run(c.role, func(t *testing.T) {
			req := Request{Messages: []Message{{Role: RoleSystem, Content: "You are " + c.role}, {Role: RoleUser, Content: "test"}}}
			resp, err := m.Complete(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(resp.Message.Content, c.want) {
				t.Fatalf("want %q in mock response, got: %s", c.want, resp.Message.Content)
			}
		})
	}
}

func TestFactoryMockFallback(t *testing.T) {
	// 无 key auto → mock
	if !New(Config{Model: "x", MockMode: "auto"}).IsMock() {
		t.Fatal("empty key auto should mock")
	}
	// always 强制 mock
	if !New(Config{AnthropicKey: "sk-xxx", MockMode: "always"}).IsMock() {
		t.Fatal("always should mock even with key")
	}
	// auto + Anthropic key → 直连 Anthropic
	if c := New(Config{AnthropicKey: "sk-xxx", MockMode: "auto"}); c.IsMock() {
		t.Fatal("anthropic key should use real")
	}
	// auto + OpenRouter key → OpenRouter
	if c := New(Config{OpenRouterKey: "sk-or-xxx", MockMode: "auto"}); c.IsMock() {
		t.Fatal("openrouter key should use real")
	}
	// 显式指定 provider=anthropic 但 key 在 APIKey 字段
	if c := New(Config{Provider: "anthropic", APIKey: "sk-yyy", MockMode: "auto"}); c.IsMock() {
		t.Fatal("explicit anthropic provider should use real")
	}
}
