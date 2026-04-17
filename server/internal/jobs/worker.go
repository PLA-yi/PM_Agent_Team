// Package jobs 提供轻量级任务执行器：内存 worker pool 跑 Agent pipeline。
// 后续可替换为 River（postgres queue）而不影响 API 层。
package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"pmhive/server/internal/agent"
	"pmhive/server/internal/store"
	"pmhive/server/internal/stream"
)

// Worker 异步任务执行器
type Worker struct {
	Store store.Store
	Bus   *stream.Bus
	Deps  agent.Deps
	queue chan uuid.UUID
}

func NewWorker(s store.Store, bus *stream.Bus, deps agent.Deps, concurrency int) *Worker {
	if concurrency <= 0 {
		concurrency = 2
	}
	w := &Worker{
		Store: s,
		Bus:   bus,
		Deps:  deps,
		queue: make(chan uuid.UUID, 64),
	}
	for i := 0; i < concurrency; i++ {
		go w.loop()
	}
	return w
}

// Enqueue 把任务加入执行队列
func (w *Worker) Enqueue(taskID uuid.UUID) {
	w.queue <- taskID
}

func (w *Worker) loop() {
	for taskID := range w.queue {
		w.run(taskID)
	}
}

func (w *Worker) run(taskID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	t, err := w.Store.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	_ = w.Store.UpdateTaskStatus(ctx, taskID, store.StatusRunning, "starting", 5, "")

	// 阶段进度桥接：单独 ctx，pipeline 结束后立即关闭，防止 stale 事件覆盖终态
	bridgeCtx, stopBridge := context.WithCancel(ctx)
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		w.bridgeProgress(bridgeCtx, taskID)
	}()

	st := &agent.State{TaskID: taskID, Input: t.Input}
	// v0.7+: 注入 Project 历史知识
	if t.ProjectID != nil {
		entries, err := w.Store.QueryKnowledge(ctx, *t.ProjectID, splitKeywords(t.Input), 30)
		if err == nil && len(entries) > 0 {
			var sb strings.Builder
			for _, e := range entries {
				fmt.Fprintf(&sb, "- [%s] %s — %s\n", e.Kind, e.Subject, shortText(e.Content, 200))
			}
			st.PriorContext = sb.String()
			w.Bus.Publish(taskID, "system", "kb_injected", map[string]int{"entries": len(entries)})
		}
	}
	pipe := agent.PipelineFor(string(t.Scenario))
	if pipe == nil {
		err := fmt.Errorf("unknown scenario: %s", t.Scenario)
		_ = w.Store.UpdateTaskStatus(ctx, taskID, store.StatusFailed, "failed", 100, err.Error())
		w.Bus.Publish(taskID, "system", "task_failed", map[string]string{"err": err.Error()})
		stopBridge()
		<-bridgeDone
		_ = w.persistTraces(ctx, taskID)
		return
	}
	pipeErr := pipe.Run(ctx, st, w.Deps)

	// 关闭桥接，等其 drain 完已收到的事件，再写终态
	stopBridge()
	<-bridgeDone

	if pipeErr != nil {
		_ = w.Store.UpdateTaskStatus(ctx, taskID, store.StatusFailed, "failed", 100, pipeErr.Error())
		w.Bus.Publish(taskID, "system", "task_failed", map[string]string{"err": pipeErr.Error()})
		_ = w.persistTraces(ctx, taskID)
		return
	}

	// 写报告
	rep := &store.Report{
		TaskID:   taskID,
		Title:    titleFor(string(t.Scenario), t.Input),
		Markdown: st.Report,
		Sources:  st.Sources,
		Metadata: map[string]any{
			"scenario":     string(t.Scenario),
			"competitors":  st.Competitors,
			"analysis":     st.Analysis,
			"insights":     st.Insights,
			"prd_sections": st.PRDSections,
			"user_stories": st.UserStories,
			"review":       st.Review,
			"requirements": st.Requirements,
			"hypotheses":   st.Hypotheses,
			"validations":  st.Validations,
			"risks":        st.Risks,
		},
	}
	if err := w.Store.SaveReport(ctx, rep); err != nil {
		_ = w.Store.UpdateTaskStatus(ctx, taskID, store.StatusFailed, "failed", 100, "save report: "+err.Error())
		_ = w.persistTraces(ctx, taskID)
		return
	}

	// 持久化 social posts（可能上千）
	if len(st.Posts) > 0 {
		if err := w.Store.SavePosts(ctx, taskID, st.Posts); err != nil {
			w.Bus.Publish(taskID, "system", "warn", map[string]string{"err": "save posts: " + err.Error()})
		}
	}

	_ = w.Store.UpdateTaskStatus(ctx, taskID, store.StatusSucceeded, "done", 100, "")
	w.Bus.Publish(taskID, "system", "task_succeeded", map[string]int{"report_bytes": len(st.Report), "sources": len(st.Sources)})
	_ = w.persistTraces(ctx, taskID)

	// v0.7+ Project 知识沉淀：抽 entities 入 KB
	if t.ProjectID != nil {
		w.harvestKnowledge(ctx, *t.ProjectID, taskID, st)
	}

	// v0.6+ 追问 merge：child task 完成 → 把 sources/posts/report 增量合并到 parent
	if t.ParentTaskID != nil {
		w.mergeIntoParent(ctx, *t.ParentTaskID, t, st)
	}
}

// mergeIntoParent 把 child task 的产物 append 到 parent
func (w *Worker) mergeIntoParent(ctx context.Context, parentID uuid.UUID, child *store.Task, st *agent.State) {
	// 1. posts 追加
	if len(st.Posts) > 0 {
		_ = w.Store.SavePosts(ctx, parentID, st.Posts)
	}
	// 2. report 追加 section
	appendix := fmt.Sprintf(`

---

## 追问补充：%s

%s
`, child.Input, st.Report)
	_ = w.Store.UpdateReportMarkdown(ctx, parentID, func(md string) string {
		return md + appendix
	})
	w.Bus.Publish(parentID, "system", "merged_followup", map[string]interface{}{
		"child_task": child.ID.String(),
		"new_posts":  len(st.Posts),
		"new_chars":  len(appendix),
	})
}

// harvestKnowledge 从完成 task 抽 entities 入 Project KB
func (w *Worker) harvestKnowledge(ctx context.Context, projectID uuid.UUID, taskID uuid.UUID, st *agent.State) {
	entries := []store.KnowledgeEntry{}
	// 把 competitors 转成 entity 条目
	for _, c := range st.Competitors {
		entries = append(entries, store.KnowledgeEntry{
			ProjectID:  projectID,
			SourceTask: taskID,
			Kind:       "entity",
			Subject:    c.Name,
			Content:    fmt.Sprintf("Pricing: %s | AI: %v | Strengths: %s | Weaknesses: %s",
				c.Pricing, c.AI, joinStr(c.Strengths), joinStr(c.Weaknesses)),
			URL: c.URL,
		})
	}
	// 把社交原声 top 10 当 quote
	postCap := 10
	if len(st.Posts) < postCap {
		postCap = len(st.Posts)
	}
	for i := 0; i < postCap; i++ {
		p := st.Posts[i]
		entries = append(entries, store.KnowledgeEntry{
			ProjectID:  projectID,
			SourceTask: taskID,
			Kind:       "quote",
			Subject:    p.Title,
			Content:    p.Content,
			URL:        p.URL,
		})
	}
	if len(entries) > 0 {
		_ = w.Store.AddKnowledge(ctx, entries)
	}
}

func joinStr(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += "; "
		}
		out += s
	}
	return out
}

// bridgeProgress 订阅 bus，把 coordinator 的 stage_done 事件落到 task 进度
func (w *Worker) bridgeProgress(ctx context.Context, taskID uuid.UUID) {
	ch, unsub := w.Bus.Subscribe(ctx, taskID, 0)
	defer unsub()

	stageProgress := map[string]int{
		// 竞品调研
		"planning":     20,
		"researching":  50,
		"extracting":   70,
		"analyzing":    85,
		"writing":      95,
		// 访谈分析
		"chunking":     25,
		"clustering":   65,
		"synthesizing": 95,
		// PRD 起草
		"background":   30,
		"stories":      65,
		"composing":    95,
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Agent == "coordinator" && (ev.Step == "stage_start" || ev.Step == "stage_done") {
				stage := extractStage(ev.Payload)
				if stage == "" {
					continue
				}
				progress := stageProgress[stage]
				if ev.Step == "stage_start" && progress > 5 {
					progress -= 10
				}
				if progress > 0 {
					_ = w.Store.UpdateTaskStatus(ctx, taskID, store.StatusRunning, stage, progress, "")
				}
			}
		}
	}
}

func extractStage(payload []byte) string {
	// payload 是 {"stage":"planning",...}，简单字符串扫描免反序列化
	s := string(payload)
	idx := strings.Index(s, `"stage":"`)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(`"stage":"`):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func (w *Worker) persistTraces(ctx context.Context, taskID uuid.UUID) error {
	hist := w.Bus.History(taskID)
	return w.Store.SaveTraces(ctx, taskID, hist)
}

func splitKeywords(s string) []string {
	// 简陋 tokenizer：空格 + 中英标点切。LLM context-aware 检索 v0.8 上 embedding。
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '，' || r == '。' || r == ',' || r == '.' || r == '/' || r == '|' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func shortText(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	rs := []rune(s)
	return string(rs[:n]) + "..."
}

func titleFor(scenario, input string) string {
	if len(input) > 40 {
		input = input[:40] + "..."
	}
	prefix := map[string]string{
		"competitor_research":    "竞品调研",
		"interview_analysis":     "访谈分析",
		"prd_drafting":           "PRD 草稿",
		"social_listening":       "社交聆听",
		"requirement_analysis":   "需求分析",
		"requirement_validation": "需求验证",
	}[scenario]
	if prefix == "" {
		prefix = "任务"
	}
	return fmt.Sprintf("%s：%s", prefix, input)
}
