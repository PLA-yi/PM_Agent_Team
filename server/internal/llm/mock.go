package llm

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Mock 是无 API key 时的兜底实现：根据消息内容返回固定 fixture。
// 用于 demo / 单元测试 / 离线开发。
type Mock struct {
	// LatencyMs 模拟网络延迟，让前端 SSE 流式 UI 看起来真实
	LatencyMs int
}

func NewMock() *Mock { return &Mock{LatencyMs: 200} }

func (m *Mock) IsMock() bool { return true }

func (m *Mock) Complete(ctx context.Context, req Request) (*Response, error) {
	if m.LatencyMs > 0 {
		select {
		case <-time.After(time.Duration(m.LatencyMs) * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	last := lastUserContent(req.Messages)
	system := firstSystemContent(req.Messages)
	hint := system + "\n" + last

	// 根据 system prompt 中的 agent 角色路由不同 fixture
	// 顺序：更具体的 system 关键词先匹配；放宽 PLANNER/EXTRACTOR/WRITER 的 system 模板复用导致的歧义
	switch {
	// ===== Reviewer + Rewrite =====
	case strings.Contains(hint, "REVIEWER_AGENT"):
		return mockReviewerJSON(), nil
	case strings.Contains(hint, "RE-WRITE PASS"):
		return mockWriterMarkdown(last), nil
	// ===== PRD 起草 =====
	case strings.Contains(hint, "for PRD drafting"):
		return mockPRDBackgroundJSON(), nil
	case strings.Contains(hint, "for PRD user stories"):
		return mockPRDStoriesJSON(), nil
	case strings.Contains(hint, "for PRD composition"):
		return mockPRDMarkdown(last), nil
	// ===== Interview 分析 =====
	case strings.Contains(hint, "CLUSTERER_AGENT"):
		return mockClustererJSON(), nil
	case strings.Contains(hint, "for interview synthesis"):
		return mockInterviewSynthMarkdown(last), nil
	// ===== 竞品调研（保留原逻辑）=====
	case strings.Contains(hint, "PLANNER_AGENT"):
		return mockPlannerJSON(), nil
	case strings.Contains(hint, "EXTRACTOR_AGENT"):
		return mockExtractorJSON(), nil
	case strings.Contains(hint, "ANALYZER_AGENT"):
		return mockAnalyzerJSON(), nil
	case strings.Contains(hint, "WRITER_AGENT"):
		return mockWriterMarkdown(last), nil
	case strings.Contains(hint, "SEARCH_AGENT"), strings.Contains(hint, "SCRAPER_AGENT"), strings.Contains(hint, "REVIEW_AGENT"):
		return textResponse("ok"), nil
	default:
		return textResponse("[mock] " + truncate(last, 80)), nil
	}
}

// ---- Interview / PRD mock fixtures ----

func mockClustererJSON() *Response {
	return jsonResponse(map[string]interface{}{
		"insights": []map[string]interface{}{
			{"theme": "导出功能不够灵活", "frequency": 7, "quotes": []string{"我经常需要导出 Excel 但格式总是要手动调", "希望能选择导出哪些列"}, "need_level": "important", "confidence": 0.85},
			{"theme": "搜索结果不准确", "frequency": 5, "quotes": []string{"搜不到我想要的，搜索框形同虚设"}, "need_level": "critical", "confidence": 0.9},
			{"theme": "团队协作流程繁琐", "frequency": 4, "quotes": []string{"权限设置太复杂，新人加入要等半天"}, "need_level": "important", "confidence": 0.75},
			{"theme": "移动端体验缺失", "frequency": 3, "quotes": []string{"差旅时只能等回到电脑前才能处理"}, "need_level": "nice-to-have", "confidence": 0.6},
		},
	})
}

func mockInterviewSynthMarkdown(input string) *Response {
	topic := truncate(input, 30)
	var parsed struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal([]byte(input), &parsed); err == nil && parsed.Input != "" {
		topic = truncate(parsed.Input, 30)
	}
	md := `# 用户访谈洞察分析：` + topic + `

> 由 PMHive Multi-Agent 自动生成 · Mock 模式

## 1. 概览
共 4 个核心主题、19 次提及。**关键发现**：搜索准确度 (critical) 是用户最大痛点，导出灵活性次之。

## 2. 主题洞察

### 搜索结果不准确（critical · 置信度 0.90）
- 提及 5 次
- > "搜不到我想要的，搜索框形同虚设"

### 导出功能不够灵活（important · 置信度 0.85）
- 提及 7 次
- > "我经常需要导出 Excel 但格式总是要手动调"
- > "希望能选择导出哪些列"

### 团队协作流程繁琐（important · 置信度 0.75）
- 提及 4 次
- > "权限设置太复杂，新人加入要等半天"

### 移动端体验缺失（nice-to-have · 置信度 0.60）
- 提及 3 次

## 3. 需求列表
**Critical**
- 重构搜索算法，引入语义检索

**Important**
- 自定义导出列 + 模板保存
- 简化权限模型，预置三类角色

**Nice-to-have**
- 移动端 MVP（先做查看 + 评论）

## 4. 后续建议
优先排期"搜索"和"导出"两条线，下一轮访谈聚焦权限流程的具体卡点。
`
	return textResponse(md)
}

func mockPRDBackgroundJSON() *Response {
	return jsonResponse(map[string]interface{}{
		"background": "当前用户在多步操作中需要频繁切换上下文，反馈渠道散落在客服/微信/邮件，导致问题解决周期长达 3-5 天，NPS 持续走低。",
		"goals":      []string{"将平均问题响应时间从 24h 降至 4h", "统一反馈入口，覆盖 90% 主路径", "为产品迭代提供结构化用户洞察"},
		"non_goals":  []string{"不做电话客服", "不做实时人工接入"},
	})
}

func mockPRDStoriesJSON() *Response {
	return jsonResponse(map[string]interface{}{
		"stories": []string{
			"作为终端用户，我希望在产品任意页面点击悬浮按钮提交反馈，以便不打断当前操作。\n验收标准：\n- 悬浮按钮在所有主要页面可见\n- 提交后 5 秒内收到确认",
			"作为客服，我希望按主题/严重度筛选反馈，以便集中处理。\n验收标准：\n- 至少支持 5 种筛选维度\n- 列表 2 秒内加载",
			"作为产品经理，我希望按周看到聚合的反馈洞察报告，以便决定下周迭代优先级。\n验收标准：\n- 每周一邮件推送\n- 含 Top 5 主题与示例引用",
			"作为开发者，我希望反馈能一键转 Jira issue，以便缩短闭环。\n验收标准：\n- 支持双向同步\n- 状态变更回写",
		},
	})
}

func mockPRDMarkdown(input string) *Response {
	topic := truncate(input, 40)
	var parsed struct {
		Requirement string `json:"requirement"`
	}
	if err := json.Unmarshal([]byte(input), &parsed); err == nil && parsed.Requirement != "" {
		topic = truncate(parsed.Requirement, 40)
	}
	md := `# PRD：` + topic + `

> 由 PMHive Multi-Agent 自动生成 · Mock 模式

## 一、背景
当前用户在多步操作中需要频繁切换上下文，反馈渠道散落在客服/微信/邮件，导致问题解决周期长达 3-5 天，NPS 持续走低。

## 二、目标 / 非目标
**目标**
- 将平均问题响应时间从 24h 降至 4h
- 统一反馈入口，覆盖 90% 主路径
- 为产品迭代提供结构化用户洞察

**非目标**
- 不做电话客服
- 不做实时人工接入

## 三、用户故事与验收标准

1. 作为**终端用户**，我希望在产品任意页面点击悬浮按钮提交反馈，以便不打断当前操作。
   - 悬浮按钮在所有主要页面可见
   - 提交后 5 秒内收到确认

2. 作为**客服**，我希望按主题/严重度筛选反馈，以便集中处理。
   - 至少支持 5 种筛选维度
   - 列表 2 秒内加载

3. 作为**产品经理**，我希望按周看到聚合的反馈洞察报告，以便决定下周迭代优先级。
   - 每周一邮件推送
   - 含 Top 5 主题与示例引用

4. 作为**开发者**，我希望反馈能一键转 Jira issue，以便缩短闭环。
   - 支持双向同步
   - 状态变更回写

## 四、功能列表
- [F1] 悬浮反馈入口组件
- [F2] 反馈管理后台（筛选 + 详情）
- [F3] 周度洞察报告聚合 + 邮件
- [F4] Jira 双向同步集成

## 五、风险与依赖
- **依赖**：Jira API 配额；客服系统的工单字段映射
- **风险**：周报聚合质量与样本量强相关，冷启动期需要人工 review

## 六、待评审问题
- 反馈是否需要支持图片附件？（影响存储成本）
- Jira 同步范围是全量还是按 severity 过滤？
- 周报推送对象是 PM 还是全员？
`
	return textResponse(md)
}

func textResponse(s string) *Response {
	return &Response{Message: Message{Role: RoleAssistant, Content: s}, Usage: Usage{TotalTokens: len(s) / 4}}
}

func jsonResponse(v interface{}) *Response {
	b, _ := json.Marshal(v)
	return textResponse(string(b))
}

func lastUserContent(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

func firstSystemContent(msgs []Message) string {
	for _, m := range msgs {
		if m.Role == RoleSystem {
			return m.Content
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---- 各 Agent 的 mock fixture ----

func mockPlannerJSON() *Response {
	return jsonResponse(map[string]interface{}{
		"outline": []string{"产品概览", "功能对比", "定价对比", "用户口碑", "差异化机会"},
		"candidates": []map[string]string{
			{"name": "印象笔记", "name_en": "Yinxiang Biji", "url": "https://www.yinxiang.com"},
			{"name": "Notion", "name_en": "Notion", "url": "https://www.notion.so"},
			{"name": "飞书文档", "name_en": "Lark Docs", "url": "https://www.feishu.cn/product/docs"},
			{"name": "Obsidian", "name_en": "Obsidian", "url": "https://obsidian.md"},
			{"name": "WPS AI", "name_en": "WPS AI", "url": "https://ai.wps.cn"},
		},
	})
}

func mockExtractorJSON() *Response {
	return jsonResponse(map[string]interface{}{
		"competitors": []map[string]interface{}{
			{
				"name": "Notion", "pricing": "Free / Plus $10 / Business $20", "ai": true,
				"strengths": []string{"block 编辑器灵活", "AI 集成成熟"},
				"weaknesses": []string{"国内访问慢", "中文生态弱"},
			},
			{
				"name": "飞书文档", "pricing": "免费 / 付费版按席位", "ai": true,
				"strengths": []string{"中文体验佳", "国内组织协作生态"},
				"weaknesses": []string{"个人场景偏轻"},
			},
			{
				"name": "Obsidian", "pricing": "个人免费 / 商业 $50/年", "ai": false,
				"strengths": []string{"本地优先", "插件生态"},
				"weaknesses": []string{"无内置 AI", "上手门槛高"},
			},
		},
	})
}

func mockReviewerJSON() *Response {
	return jsonResponse(map[string]interface{}{
		"overall_score":  8.2,
		"fact_score":     8.5,
		"coverage_score": 8.0,
		"citation_score": 8.0,
		"strengths":      []string{"覆盖核心竞品", "引用真实可访问"},
		"issues":         []string{},
		"verdict":        "accept",
	})
}

func mockAnalyzerJSON() *Response {
	return jsonResponse(map[string]interface{}{
		"swot": map[string][]string{
			"opportunities": []string{
				"国内 AI 笔记 + 知识管理一体化产品仍有空白",
				"中文场景下的 PRD/竞品调研模板未被覆盖",
			},
			"threats": []string{"飞书/Notion AI 持续投入", "通用 AI 工具下沉"},
		},
		"differentiation": "面向 PM 的 Agent 集群 + 行业知识库，而不是单纯笔记/文档工具。",
	})
}

func mockWriterMarkdown(input string) *Response {
	// writer 收到的 user content 是 JSON：{"input":"...","competitors":[...],...}
	// 从中抽 topic；解析失败就 fallback 用前 30 字
	topic := truncate(input, 30)
	var parsed struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal([]byte(input), &parsed); err == nil && parsed.Input != "" {
		topic = parsed.Input
	}
	md := `# 竞品调研报告：` + topic + `

> 由 PMHive Multi-Agent 自动生成 · Mock 模式（无 API key）

## 1. 调研范围
本次调研覆盖 5 款主流产品：印象笔记、Notion、飞书文档、Obsidian、WPS AI。

## 2. 竞品矩阵

| 产品 | 定价 | AI 能力 | 中文体验 | 适用人群 |
|---|---|---|---|---|
| Notion | Free–$20 | 强 [1] | 中 | 海外团队 |
| 飞书文档 | 按席位 | 中 [2] | 强 | 国内中大型组织 |
| Obsidian | 免费/$50年 | 无 | 弱 | 重度个人知识管理 |
| 印象笔记 | 88–148/年 | 中 [3] | 强 | 个人收藏 |
| WPS AI | 订阅 | 中 [4] | 强 | 办公替代场景 |

## 3. 关键差异化机会
- **PM 垂直场景**：当前所有产品都不针对 PM 工作流（竞品调研、PRD 草稿、用户访谈）。
- **Agent 集群**：现有产品多为单点 AI copilot，未形成多 Agent 协作。
- **行业数据管道**：将 G2 / Crunchbase / 小红书 / 知乎 集成进知识库是壁垒。

## 4. 建议切入点
做面向中国 SaaS PM 的 "竞品调研 → 用户访谈 → PRD 起草" 三件套 Agent 集群，定价 $29-79/座/月。

---

**引用：**
[1] https://www.notion.so/help/ai
[2] https://www.feishu.cn/product/docs
[3] https://www.yinxiang.com
[4] https://ai.wps.cn
`
	return textResponse(md)
}
