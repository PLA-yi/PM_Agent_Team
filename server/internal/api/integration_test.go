package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pmhive/server/internal/agent"
	"pmhive/server/internal/jobs"
	"pmhive/server/internal/llm"
	"pmhive/server/internal/store"
	"pmhive/server/internal/stream"
	"pmhive/server/internal/tools"
)

// TestEndToEnd 启动一个内嵌 server，发起任务、订阅 SSE、拉报告，验证 v0.1 关键路径。
func TestEndToEnd(t *testing.T) {
	mockLLM := llm.NewMock()
	mockLLM.LatencyMs = 0
	bus := stream.NewBus()
	mem := store.NewMemory()
	deps := agent.Deps{
		LLM:    mockLLM,
		Search: tools.NewMockSearcher(),
		Scrape: tools.NewMockScraper(),
		Bus:    bus,
		Model:  "mock",
	}
	worker := jobs.NewWorker(mem, bus, deps, 2)

	srv := &Server{Store: mem, Bus: bus, Worker: worker, CORSAllowed: []string{"*"}}
	httpSrv := httptest.NewServer(srv.Routes())
	defer httpSrv.Close()

	// 1. POST /api/tasks
	body, _ := json.Marshal(map[string]string{"input": "国内 AI 笔记类产品"})
	resp, err := http.Post(httpSrv.URL+"/api/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: %d %s", resp.StatusCode, b)
	}
	var task store.Task
	_ = json.NewDecoder(resp.Body).Decode(&task)
	resp.Body.Close()
	if task.ID.String() == "" {
		t.Fatal("no task id")
	}

	// 2. SSE 订阅
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, httpSrv.URL+"/api/tasks/"+task.ID.String()+"/stream", nil)
	streamResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("sse status: %d", streamResp.StatusCode)
	}

	reader := bufio.NewReader(streamResp.Body)
	eventCount := 0
	gotSucceeded := false
	deadline := time.After(8 * time.Second)
loop:
	for {
		select {
		case <-deadline:
			break loop
		default:
		}
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "event:") {
			eventCount++
			if strings.Contains(line, "task_succeeded") {
				gotSucceeded = true
				break loop
			}
		}
	}
	if eventCount < 5 {
		t.Errorf("want >=5 SSE events, got %d", eventCount)
	}
	if !gotSucceeded {
		t.Error("did not see task_succeeded event")
	}

	// 3. GET /api/tasks/:id/report
	rptResp, err := http.Get(httpSrv.URL + "/api/tasks/" + task.ID.String() + "/report")
	if err != nil || rptResp.StatusCode != http.StatusOK {
		t.Fatalf("report: %v %d", err, rptResp.StatusCode)
	}
	var rep store.Report
	_ = json.NewDecoder(rptResp.Body).Decode(&rep)
	rptResp.Body.Close()
	if !strings.Contains(rep.Markdown, "竞品调研报告") {
		end := len(rep.Markdown)
		if end > 120 {
			end = 120
		}
		t.Errorf("report missing title: %s", rep.Markdown[:end])
	}
	if len(rep.Sources) == 0 {
		t.Error("report sources empty")
	}

	// 4. GET task → succeeded
	tResp, _ := http.Get(httpSrv.URL + "/api/tasks/" + task.ID.String())
	var got store.Task
	_ = json.NewDecoder(tResp.Body).Decode(&got)
	tResp.Body.Close()
	if got.Status != store.StatusSucceeded {
		t.Errorf("want status succeeded, got %s", got.Status)
	}

	t.Logf("E2E OK: %d sse events, report %d bytes, %d sources", eventCount, len(rep.Markdown), len(rep.Sources))
}

