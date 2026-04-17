package agent

// Pipeline 一个场景的完整 Agent 编排
type Pipeline = Coordinator // 类型别名，对外语义更清晰

// PipelineFor 根据 scenario 名返回对应 pipeline。未知 scenario 返回 nil。
func PipelineFor(scenario string) *Pipeline {
	switch scenario {
	case "competitor_research":
		p := DefaultCompetitorResearchPipeline()
		return &p
	case "interview_analysis":
		p := InterviewAnalysisPipeline()
		return &p
	case "prd_drafting":
		p := PRDDraftingPipeline()
		return &p
	case "social_listening":
		p := SocialListeningPipeline()
		return &p
	default:
		return nil
	}
}

// SocialListeningPipeline 独立社聆场景：直接对 Input 拉社交帖，跑 Clusterer 出洞察
// （和 competitor_research 嵌入式区别：standalone 模式 + 失败要报错）
func SocialListeningPipeline() Coordinator {
	return Coordinator{
		Stages: []Stage{
			{Name: "scraping", Agents: []Agent{SocialListener{K: 10, WriteToChunks: true, Optional: false}}},
			{Name: "clustering", Agents: []Agent{Clusterer{}}},
			{Name: "synthesizing", Agents: []Agent{InsightSynthesizer{}}},
		},
	}
}

// InterviewAnalysisPipeline 用户访谈分析：
//  Coordinator → Chunker（拆段）→ Clusterer（主题聚类）→ Synthesizer（产出洞察 + 需求列表）
func InterviewAnalysisPipeline() Coordinator {
	return Coordinator{
		Stages: []Stage{
			{Name: "chunking", Agents: []Agent{Chunker{}}},
			{Name: "clustering", Agents: []Agent{Clusterer{}}},
			{Name: "synthesizing", Agents: []Agent{InsightSynthesizer{}}},
		},
	}
}

// PRDDraftingPipeline PRD 自动起草：
//  Coordinator → BackgroundExpander → UserStoryAuthor → PRDComposer
func PRDDraftingPipeline() Coordinator {
	return Coordinator{
		Stages: []Stage{
			{Name: "background", Agents: []Agent{BackgroundExpander{}}},
			{Name: "stories", Agents: []Agent{UserStoryAuthor{}}},
			{Name: "composing", Agents: []Agent{PRDComposer{}}},
		},
	}
}
