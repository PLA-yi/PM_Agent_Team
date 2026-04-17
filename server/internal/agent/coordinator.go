package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pmhive/server/internal/llm"
)

// OptionalAgent 接口：声明 IsOptional() 的 agent 失败不阻塞 stage
type OptionalAgent interface {
	IsOptional() bool
}

func isOptional(a Agent) bool {
	if oa, ok := a.(OptionalAgent); ok {
		return oa.IsOptional()
	}
	return false
}

// Coordinator 是顶层编排器：决定 Agent 执行顺序与并行度，发布阶段事件。
type Coordinator struct {
	Stages []Stage
}

// Stage 一个执行阶段；同 stage 内的 agents 并行；不同 stage 串行
type Stage struct {
	Name   string  // planning / researching / extracting / analyzing / writing
	Agents []Agent // 并行
}

// DefaultCompetitorResearchPipeline 竞品调研场景默认 DAG
//
// researching 阶段三路并行：Search + Scraper + Social
// 写完后 Reviewer 评分；分数 < 7 触发一次 Writer 重写（self-correction）
func DefaultCompetitorResearchPipeline() Coordinator {
	return Coordinator{
		Stages: []Stage{
			{Name: "planning", Agents: []Agent{Planner{}}},
			{Name: "researching", Agents: []Agent{
				Search{},
				Scraper{MaxPages: 5},
				SocialListener{K: 200, WriteToChunks: false, Optional: true},
			}},
			{Name: "extracting", Agents: []Agent{Extractor{}}},
			{Name: "analyzing", Agents: []Agent{Analyzer{}}},
			{Name: "writing", Agents: []Agent{Writer{}}},
			{Name: "reviewing", Agents: []Agent{Reviewer{Iteration: 1}}},
			{Name: "self_correction", Agents: []Agent{ReviewerRetry{MinScore: 7.0}}},
		},
	}
}

// Run 执行整个 pipeline
func (c Coordinator) Run(ctx context.Context, st *State, d Deps) error {
	// #8 注入 task_id 给 LLM client 用于 cost attribute
	ctx = llm.WithTask(ctx, st.TaskID.String())

	publish(d, st, "coordinator", "start", map[string]interface{}{
		"input":  st.Input,
		"stages": stageNames(c.Stages),
	})
	start := time.Now()

	for i, stage := range c.Stages {
		publish(d, st, "coordinator", "stage_start", map[string]interface{}{
			"stage":    stage.Name,
			"index":    i + 1,
			"of":       len(c.Stages),
			"agents":   agentNames(stage.Agents),
			"parallel": len(stage.Agents) > 1,
		})

		if len(stage.Agents) == 1 {
			a := stage.Agents[0]
			actx := llm.WithAgent(ctx, a.Name())
			if err := a.Run(actx, st, d); err != nil {
				// #4 Optional agent 失败不阻塞 stage（已检查接口）
				if isOptional(a) {
					publish(d, st, "coordinator", "warn", map[string]string{
						"stage": stage.Name, "agent": a.Name(),
						"err": err.Error(), "note": "optional agent failed, continuing",
					})
				} else {
					publish(d, st, "coordinator", "error", map[string]string{
						"stage": stage.Name, "agent": a.Name(), "err": err.Error(),
					})
					return fmt.Errorf("stage %s agent %s: %w", stage.Name, a.Name(), err)
				}
			}
		} else {
			// 并行
			var wg sync.WaitGroup
			errs := make([]error, len(stage.Agents))
			for idx, a := range stage.Agents {
				wg.Add(1)
				go func(i int, a Agent) {
					defer wg.Done()
					actx := llm.WithAgent(ctx, a.Name())
					errs[i] = a.Run(actx, st, d)
				}(idx, a)
			}
			wg.Wait()
			// 统计：optional 失败仅 warn，required 失败立刻返回
			var hardErr error
			for i, e := range errs {
				if e == nil {
					continue
				}
				a := stage.Agents[i]
				if isOptional(a) {
					publish(d, st, "coordinator", "warn", map[string]string{
						"stage": stage.Name, "agent": a.Name(),
						"err": e.Error(), "note": "optional agent failed, continuing",
					})
					continue
				}
				publish(d, st, "coordinator", "error", map[string]string{
					"stage": stage.Name, "agent": a.Name(), "err": e.Error(),
				})
				if hardErr == nil {
					hardErr = fmt.Errorf("stage %s agent %s: %w", stage.Name, a.Name(), e)
				}
			}
			if hardErr != nil {
				return hardErr
			}
		}
		publish(d, st, "coordinator", "stage_done", map[string]string{"stage": stage.Name})
	}

	publish(d, st, "coordinator", "done", map[string]interface{}{
		"elapsed_ms":   time.Since(start).Milliseconds(),
		"sources":      len(st.Sources),
		"competitors":  len(st.Competitors),
		"report_bytes": len(st.Report),
	})
	return nil
}

func agentNames(as []Agent) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name()
	}
	return out
}

func stageNames(ss []Stage) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Name
	}
	return out
}
