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
	case "requirement_analysis":
		p := RequirementAnalysisPipeline()
		return &p
	case "requirement_validation":
		p := RequirementValidationPipeline()
		return &p
	default:
		return nil
	}
}

// RequirementAnalysisPipeline 需求分析：发现 → 评分 → 排序
//   Coordinator → Discoverer → SocialListener (optional, Reddit 拉真实声量) →
//   Analyzer (RICE+Kano 打分) → Prioritizer (排序+写报告) → Reviewer
func RequirementAnalysisPipeline() Coordinator {
	return Coordinator{
		Stages: []Stage{
			{Name: "discovering", Agents: []Agent{RequirementDiscoverer{}}},
			{Name: "user_voice", Agents: []Agent{
				SocialListener{K: 100, WriteToChunks: false, Optional: true},
			}},
			{Name: "scoring", Agents: []Agent{RequirementAnalyzer{}}},
			{Name: "writing", Agents: []Agent{RequirementPrioritizer{}}},
			{Name: "reviewing", Agents: []Agent{Reviewer{Iteration: 1}}},
		},
	}
}

// RequirementValidationPipeline 需求验证：假设 → 执行 → 风险
//   Coordinator → HypothesisGenerator → SocialListener (拉用户原声做证据) →
//   ValidationExecutor → RiskWriter → Reviewer
func RequirementValidationPipeline() Coordinator {
	return Coordinator{
		Stages: []Stage{
			{Name: "hypothesizing", Agents: []Agent{HypothesisGenerator{}}},
			{Name: "user_voice", Agents: []Agent{
				SocialListener{K: 100, WriteToChunks: false, Optional: true},
			}},
			{Name: "validating", Agents: []Agent{ValidationExecutor{}}},
			{Name: "writing", Agents: []Agent{ValidationRiskWriter{}}},
			{Name: "reviewing", Agents: []Agent{Reviewer{Iteration: 1}}},
		},
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
