// Package agent — 专业角色画像（v0.5 Agent 集群人格化）
package agent

// RoleMeta 一个 agent 的专业角色元数据
type RoleMeta struct {
	Key        string `json:"key"`         // agent.Name() 返回值（timeline 颜色锚点）
	Title      string `json:"title"`       // 中文职位
	TitleEn    string `json:"title_en"`    // 英文职位
	Avatar     string `json:"avatar"`      // 单字符 emoji 头像
	Specialty  string `json:"specialty"`   // 一句话专长
	UsedIn     []string `json:"used_in"`   // 在哪些 scenario 出现
}

// RoleRegistry 全部已知角色 — 前端展示 agent cluster 卡片用
var RoleRegistry = []RoleMeta{
	// 跨板块复用
	{
		Key: "coordinator", Title: "协调员", TitleEn: "Coordinator",
		Avatar: "🎬", Specialty: "总调度，决定 Agent 运行顺序与并行度",
		UsedIn: []string{"requirement_analysis", "competitor_research", "requirement_validation", "interview_analysis", "prd_drafting", "social_listening"},
	},
	{
		Key: "social", Title: "社聆员", TitleEn: "Social Listener",
		Avatar: "📡", Specialty: "Reddit/HN/X 跨平台抓真实用户声量",
		UsedIn: []string{"requirement_analysis", "competitor_research", "requirement_validation", "social_listening"},
	},
	{
		Key: "reviewer", Title: "复审员", TitleEn: "Reviewer",
		Avatar: "🧐", Specialty: "三维评分（事实/覆盖/引用），不达标触发自校正",
		UsedIn: []string{"requirement_analysis", "competitor_research", "requirement_validation"},
	},

	// 需求分析板块专家
	{
		Key: "planner", Title: "用户研究员 / 市场情报员 / 精益教练", TitleEn: "Planner Family",
		Avatar: "🗺️", Specialty: "需求场景=找信号；竞品=扫赛道；验证=拆假设",
		UsedIn: []string{"requirement_analysis", "competitor_research", "requirement_validation", "prd_drafting"},
	},
	{
		Key: "analyzer", Title: "数据分析师 / 行业分析师", TitleEn: "Analyst",
		Avatar: "📊", Specialty: "需求=RICE+Kano 评分；竞品=SWOT+差异化",
		UsedIn: []string{"requirement_analysis", "competitor_research"},
	},

	// 竞品调研专家
	{
		Key: "search", Title: "市场情报员", TitleEn: "Market Intelligence",
		Avatar: "🔭", Specialty: "DDG/Tavily/Jina 多源搜索",
		UsedIn: []string{"competitor_research"},
	},
	{
		Key: "scraper", Title: "行业研究员", TitleEn: "Industry Researcher",
		Avatar: "🔍", Specialty: "Jina Reader 抓官网/定价页",
		UsedIn: []string{"competitor_research"},
	},
	{
		Key: "extractor", Title: "投行商业分析师 / 验证执行人", TitleEn: "Business Analyst",
		Avatar: "💼", Specialty: "结构化竞品矩阵 / 验证证据收集（蓝军视角）",
		UsedIn: []string{"competitor_research", "requirement_validation", "prd_drafting"},
	},

	// 验证板块专家
	{
		Key: "writer", Title: "AI 产品经理 / 撰写员", TitleEn: "AI PM Writer",
		Avatar: "✍️", Specialty: "排序建议 / 报告撰写 / 风险盲点合成",
		UsedIn: []string{"requirement_analysis", "competitor_research", "requirement_validation", "interview_analysis", "prd_drafting"},
	},

	// 访谈分析专家
	{
		Key: "chunker", Title: "访谈整理员", TitleEn: "Interview Chunker",
		Avatar: "✂️", Specialty: "把长访谈切成可消化片段",
		UsedIn: []string{"interview_analysis"},
	},
	{
		Key: "clusterer", Title: "洞察聚类员", TitleEn: "Insight Clusterer",
		Avatar: "🧩", Specialty: "把片段按主题聚类，统计频次",
		UsedIn: []string{"interview_analysis", "social_listening"},
	},

	{
		Key: "system", Title: "系统", TitleEn: "System",
		Avatar: "⚙️", Specialty: "任务终态、数据合并等系统级事件",
		UsedIn: []string{"requirement_analysis", "competitor_research", "requirement_validation", "interview_analysis", "prd_drafting", "social_listening"},
	},
}

// RolesForScenario 返回某个 scenario 用到的所有专家角色
func RolesForScenario(scenario string) []RoleMeta {
	out := make([]RoleMeta, 0)
	for _, r := range RoleRegistry {
		for _, sc := range r.UsedIn {
			if sc == scenario {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
