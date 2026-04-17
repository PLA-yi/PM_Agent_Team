package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"pmhive/server/internal/llm"
	"pmhive/server/internal/stream"
	"pmhive/server/internal/tools"
)

func TestCompetitorResearchPipeline_Mock(t *testing.T) {
	mockLLM := llm.NewMock()
	mockLLM.LatencyMs = 0

	bus := stream.NewBus()
	deps := Deps{
		LLM:    mockLLM,
		Search: tools.NewMockSearcher(),
		Scrape: tools.NewMockScraper(),
		Bus:    bus,
		Model:  "mock",
	}
	st := &State{TaskID: uuid.New(), Input: "国内 AI 笔记类产品"}
	pipe := DefaultCompetitorResearchPipeline()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pipe.Run(ctx, st, deps); err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	if len(st.Outline) == 0 {
		t.Error("planner outline empty")
	}
	if len(st.Candidates) == 0 {
		t.Error("planner candidates empty")
	}
	if len(st.SearchResults) == 0 {
		t.Error("search results empty")
	}
	if len(st.Pages) == 0 {
		t.Error("scraped pages empty")
	}
	if len(st.Competitors) == 0 {
		t.Error("extractor competitors empty")
	}
	if st.Analysis == nil {
		t.Error("analyzer analysis nil")
	}
	if !strings.Contains(st.Report, "竞品调研报告") {
		t.Errorf("writer report missing title: %s", shortPreview(st.Report, 100))
	}

	// 校验事件流：至少 7 个 agent 都有 done 事件
	hist := bus.History(st.TaskID)
	doneByAgent := map[string]bool{}
	for _, ev := range hist {
		if ev.Step == "done" {
			doneByAgent[ev.Agent] = true
		}
	}
	wantAgents := []string{"coordinator", "planner", "search", "scraper", "extractor", "analyzer", "writer"}
	for _, a := range wantAgents {
		if !doneByAgent[a] {
			t.Errorf("missing done event for agent %s", a)
		}
	}

	t.Logf("pipeline OK: %d events, report %d bytes, %d sources, %d competitors",
		len(hist), len(st.Report), len(st.Sources), len(st.Competitors))
}

func TestInterviewAnalysisPipeline_Mock(t *testing.T) {
	mockLLM := llm.NewMock()
	mockLLM.LatencyMs = 0
	bus := stream.NewBus()
	deps := Deps{LLM: mockLLM, Search: tools.NewMockSearcher(), Scrape: tools.NewMockScraper(), Bus: bus, Model: "mock"}

	transcript := strings.Repeat("用户A：我觉得搜索功能太弱了，搜不到想要的东西。\n\n用户B：导出 Excel 总要手动调格式，太烦。\n\n", 5)
	st := &State{TaskID: uuid.New(), Input: transcript}
	pipe := InterviewAnalysisPipeline()
	if err := pipe.Run(context.Background(), st, deps); err != nil {
		t.Fatalf("interview pipeline: %v", err)
	}
	if len(st.InterviewChunks) == 0 {
		t.Error("no chunks")
	}
	if len(st.Insights) == 0 {
		t.Error("no insights")
	}
	if !strings.Contains(st.Report, "用户访谈洞察") {
		t.Errorf("report missing title: %s", shortPreview(st.Report, 100))
	}
	t.Logf("interview OK: %d chunks → %d insights → %d bytes report", len(st.InterviewChunks), len(st.Insights), len(st.Report))
}

func TestPRDDraftingPipeline_Mock(t *testing.T) {
	mockLLM := llm.NewMock()
	mockLLM.LatencyMs = 0
	bus := stream.NewBus()
	deps := Deps{LLM: mockLLM, Search: tools.NewMockSearcher(), Scrape: tools.NewMockScraper(), Bus: bus, Model: "mock"}

	st := &State{TaskID: uuid.New(), Input: "希望产品内能让用户一键反馈问题并被 PM/客服快速消化"}
	pipe := PRDDraftingPipeline()
	if err := pipe.Run(context.Background(), st, deps); err != nil {
		t.Fatalf("prd pipeline: %v", err)
	}
	if st.PRDBackground == "" {
		t.Error("no background")
	}
	if len(st.UserStories) == 0 {
		t.Error("no user stories")
	}
	if !strings.Contains(st.Report, "PRD") {
		t.Errorf("report missing PRD heading: %s", shortPreview(st.Report, 100))
	}
	t.Logf("prd OK: %d stories → %d sections → %d bytes report", len(st.UserStories), len(st.PRDSections), len(st.Report))
}
