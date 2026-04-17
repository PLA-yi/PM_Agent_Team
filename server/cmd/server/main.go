// PMHive 后端入口
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pmhive/server/internal/agent"
	"pmhive/server/internal/api"
	"pmhive/server/internal/integrations/jira"
	"pmhive/server/internal/integrations/slack"
	"pmhive/server/internal/jobs"
	"pmhive/server/internal/llm"
	"pmhive/server/internal/store"
	"pmhive/server/internal/stream"
	"pmhive/server/internal/tools"
	"pmhive/server/internal/tools/social"
)

func main() {
	addr := envOr("HTTP_ADDR", ":8080")
	provider := envOr("LLM_PROVIDER", "")
	openRouterKey := os.Getenv("OPENROUTER_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	aihubmixKey := os.Getenv("AIHUBMIX_API_KEY")
	llmBaseURL := os.Getenv("LLM_BASE_URL")
	llmModel := envOr("LLM_MODEL", "claude-sonnet-4-5")
	tavilyKey := os.Getenv("TAVILY_API_KEY")
	jinaKey := os.Getenv("JINA_API_KEY")
	searchProvider := envOr("SEARCH_PROVIDER", "")
	mockMode := envOr("MOCK_MODE", "auto")
	corsRaw := envOr("CORS_ORIGINS", "http://localhost:5173")

	llmClient := llm.New(llm.Config{
		Provider:      provider,
		OpenRouterKey: openRouterKey,
		AnthropicKey:  anthropicKey,
		AIhubmixKey:   aihubmixKey,
		BaseURL:       llmBaseURL,
		Model:         llmModel,
		MockMode:      mockMode,
	})
	searcher := tools.NewSearcher(tools.SearchConfig{
		Provider:  searchProvider,
		TavilyKey: tavilyKey,
		JinaKey:   jinaKey,
		MockMode:  mockMode,
	})

	scrapeMockOnly := strings.EqualFold(mockMode, "always")
	scraper := tools.NewScraperAuto(scrapeMockOnly)

	// Social registry — Reddit 默认开（无需 key），其他平台读 env key
	socialReg := social.NewRegistry(
		social.NewReddit(),
		social.NewX(os.Getenv("X_BEARER_TOKEN")),
		social.NewDouyin(os.Getenv("DOUYIN_COOKIE")),
		social.NewTikTok(os.Getenv("TIKTOK_SESSIONID")),
		social.NewYouTube(os.Getenv("YOUTUBE_API_KEY")),
	)

	bus := stream.NewBus()
	mem := store.NewMemory()
	deps := agent.Deps{
		LLM:    llmClient,
		Search: searcher,
		Scrape: scraper,
		Social: socialReg,
		Bus:    bus,
		Model:  llmModel,
	}
	worker := jobs.NewWorker(mem, bus, deps, 4)

	slackCli := slack.New(os.Getenv("SLACK_WEBHOOK_URL"))
	jiraCli := jira.New(os.Getenv("JIRA_BASE_URL"), os.Getenv("JIRA_EMAIL"), os.Getenv("JIRA_API_TOKEN"))

	srv := &api.Server{
		Store:       mem,
		Bus:         bus,
		Worker:      worker,
		Slack:       slackCli,
		Jira:        jiraCli,
		CORSAllowed: splitCSV(corsRaw),
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("PMHive listening on %s", addr)
	log.Printf("  LLM:    mock=%v   model=%s", llmClient.IsMock(), llmModel)
	log.Printf("  Search: mock=%v   provider=%s", searcher.IsMock(), searchProviderName(searcher))
	log.Printf("  Scrape: mock=%v   provider=jina_reader", scraper.IsMock())
	authedSocial := []string{}
	for _, sc := range socialReg.All() {
		if sc.IsAuthenticated() {
			authedSocial = append(authedSocial, sc.Platform())
		}
	}
	log.Printf("  Social: authed=%v", authedSocial)
	log.Printf("  Slack:  configured=%v", slackCli.IsConfigured())
	log.Printf("  Jira:   configured=%v", jiraCli.IsConfigured())
	log.Printf("CORS allowed: %v", srv.CORSAllowed)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http: %v", err)
		}
	}()

	// graceful shutdown
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	<-sigc
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func envOr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func searchProviderName(s tools.Searcher) string {
	if fb, ok := s.(*tools.FallbackSearcher); ok {
		return searchProviderName(fb.Primary) + "→mock(fallback)"
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
	return "unknown"
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
