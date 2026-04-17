package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pmhive/server/internal/llm"
)

// ===== BackgroundExpander =====
// 把一句话需求扩展成背景 + 目标 + 非目标

type BackgroundExpander struct{}

func (BackgroundExpander) Name() string { return "planner" } // 复用 planner timeline 颜色

const bgSystem = `You are PLANNER_AGENT for PRD drafting.
Given a one-line product requirement (Chinese), produce JSON:
{"background":"3-5 句业务背景与现状","goals":["目标1","目标2","目标3"],"non_goals":["非目标1","非目标2"]}
- 背景围绕用户痛点 / 现有痛点 / 触发该需求的业务原因
- goals: 可量化、可验收
- non_goals: 明确划出本期不做什么
Output ONLY valid JSON.`

func (BackgroundExpander) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "planner", "start", map[string]string{"input": st.Input})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: bgSystem},
			{Role: llm.RoleUser, Content: st.Input},
		},
		Temperature: 0.4,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "planner", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed struct {
		Background string   `json:"background"`
		Goals      []string `json:"goals"`
		NonGoals   []string `json:"non_goals"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "planner", "error", map[string]string{"err": "parse: " + err.Error(), "raw": shortPreview(resp.Message.Content, 200)})
		return fmt.Errorf("background parse: %w", err)
	}
	st.PRDBackground = parsed.Background
	st.Outline = parsed.Goals // 复用 outline 字段做 goals 传递
	// non_goals 进 sections
	bg := PRDSection{Heading: "背景", Body: parsed.Background}
	st.PRDSections = []PRDSection{bg}
	if len(parsed.Goals) > 0 {
		st.PRDSections = append(st.PRDSections, PRDSection{
			Heading: "目标", Body: "- " + strings.Join(parsed.Goals, "\n- "),
		})
	}
	if len(parsed.NonGoals) > 0 {
		st.PRDSections = append(st.PRDSections, PRDSection{
			Heading: "非目标", Body: "- " + strings.Join(parsed.NonGoals, "\n- "),
		})
	}
	publish(d, st, "planner", "message", parsed)
	publish(d, st, "planner", "done", nil)
	return nil
}

// ===== UserStoryAuthor =====
// 写用户故事 + 验收标准

type UserStoryAuthor struct{}

func (UserStoryAuthor) Name() string { return "extractor" } // 复用 extractor timeline 颜色

const storySystem = `You are EXTRACTOR_AGENT for PRD user stories.
Given background + goals, produce JSON:
{"stories":["作为<角色>，我希望<功能>，以便<价值>。验收标准：- ...","..."]}
- 4-6 stories
- 每个 story 内嵌验收标准（用换行）
Output ONLY valid JSON.`

func (UserStoryAuthor) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "extractor", "start", nil)
	payload, _ := json.Marshal(map[string]interface{}{
		"requirement": st.Input,
		"background":  st.PRDBackground,
		"goals":       st.Outline,
	})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: storySystem},
			{Role: llm.RoleUser, Content: string(payload)},
		},
		Temperature: 0.4,
		JSONMode:    true,
	})
	if err != nil {
		publish(d, st, "extractor", "error", map[string]string{"err": err.Error()})
		return err
	}
	var parsed struct {
		Stories []string `json:"stories"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &parsed); err != nil {
		publish(d, st, "extractor", "error", map[string]string{"err": "parse: " + err.Error(), "raw": shortPreview(resp.Message.Content, 200)})
		return fmt.Errorf("stories parse: %w", err)
	}
	st.UserStories = parsed.Stories
	st.PRDSections = append(st.PRDSections, PRDSection{
		Heading: "用户故事 & 验收标准",
		Body:    "1. " + strings.Join(parsed.Stories, "\n\n"),
	})
	publish(d, st, "extractor", "message", map[string]int{"stories": len(parsed.Stories)})
	publish(d, st, "extractor", "done", nil)
	return nil
}

// ===== PRDComposer =====
// 渲染最终 PRD Markdown

type PRDComposer struct{}

func (PRDComposer) Name() string { return "writer" }

const prdSystem = `You are WRITER_AGENT for PRD composition.
Given a structured PRD (background, goals, non-goals, user stories),
render a complete Chinese PRD in Markdown with sections:
1. # PRD 标题
2. ## 一、背景
3. ## 二、目标 / 非目标
4. ## 三、用户故事与验收标准
5. ## 四、功能列表（从 stories 反推）
6. ## 五、风险与依赖（自动识别 2-3 条）
7. ## 六、待评审问题（开放问题列表）

Keep under 1500 字。 Output ONLY Markdown.`

func (PRDComposer) Run(ctx context.Context, st *State, d Deps) error {
	publish(d, st, "writer", "start", nil)
	payload, _ := json.Marshal(map[string]interface{}{
		"requirement": st.Input,
		"background":  st.PRDBackground,
		"goals":       st.Outline,
		"sections":    st.PRDSections,
		"stories":     st.UserStories,
	})
	resp, err := d.LLM.Complete(ctx, llm.Request{
		Model: d.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: prdSystem},
			{Role: llm.RoleUser, Content: string(payload)},
		},
		Temperature: 0.5,
	})
	if err != nil {
		publish(d, st, "writer", "error", map[string]string{"err": err.Error()})
		return err
	}
	st.Report = resp.Message.Content
	publish(d, st, "writer", "message", map[string]interface{}{"length": len(st.Report), "preview": shortPreview(st.Report, 240)})
	publish(d, st, "writer", "done", nil)
	return nil
}
