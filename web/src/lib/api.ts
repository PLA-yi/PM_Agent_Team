// 简单 API 客户端 — 走 vite proxy /api -> :8080

export type TaskStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled";

export type Scenario =
  | "requirement_analysis"
  | "competitor_research"
  | "requirement_validation"
  | "interview_analysis"
  | "prd_drafting"
  | "social_listening";

// 三大 PM 板块
export type PMModule = "requirement" | "competitor" | "validation";

export interface ModuleDef {
  id: PMModule;
  label: string;
  scenario: Scenario;
  desc: string;
  emoji: string;
  accent: string; // hex
  stages: string[]; // 中文阶段名
}

export const MODULES: ModuleDef[] = [
  {
    id: "requirement",
    label: "需求分析",
    scenario: "requirement_analysis",
    desc: "找到需求 · 深度分析 · RICE+Kano 排序",
    emoji: "📋",
    accent: "#3B82F6",
    stages: ["需求发现", "用户原声", "RICE 评分", "排序与建议"],
  },
  {
    id: "competitor",
    label: "竞品调研",
    scenario: "competitor_research",
    desc: "横向矩阵 · 用户原声 · SWOT 差异化",
    emoji: "🔍",
    accent: "#FF9500",
    stages: ["规划候选", "搜索+抓取+社聆", "结构化", "SWOT 分析", "撰写", "复审"],
  },
  {
    id: "validation",
    label: "需求验证",
    scenario: "requirement_validation",
    desc: "假设拆解 · 多方法验证 · 盲点识别",
    emoji: "✅",
    accent: "#16A34A",
    stages: ["假设生成", "用户原声", "验证执行", "风险盲点"],
  },
];

export interface Task {
  id: string;
  parent_task_id?: string;
  scenario: Scenario;
  input: string;
  status: TaskStatus;
  stage: string;
  progress: number;
  error?: string;
  created_at: string;
  updated_at: string;
  finished_at?: string;
}

export interface ReviewResult {
  overall_score: number;
  fact_score: number;
  coverage_score: number;
  citation_score: number;
  strengths: string[];
  issues: string[];
  verdict: "accept" | "revise" | "reject";
  iteration: number;
}

export interface AgentEvent {
  task_id: string;
  seq: number;
  agent: string;
  step: string;
  payload: any;
  created_at: string;
}

export interface Source {
  url: string;
  title: string;
  snippet: string;
}

export interface Report {
  task_id: string;
  title: string;
  markdown: string;
  sources: Source[];
  metadata?: Record<string, any> & { review?: ReviewResult };
  updated_at: string;
}

export async function followUp(parentId: string, input: string): Promise<Task> {
  const r = await fetch(`/api/tasks/${parentId}/followup`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ input }),
  });
  if (!r.ok) {
    const err = await r.text();
    throw new Error("followUp: " + err);
  }
  return r.json();
}

// ===== Projects =====

export interface Project {
  id: string;
  name: string;
  description?: string;
  created_at: string;
  updated_at: string;
}

export async function listProjects(): Promise<Project[]> {
  const r = await fetch("/api/projects");
  if (!r.ok) throw new Error("listProjects: " + r.status);
  return (await r.json()).projects ?? [];
}

export async function createProject(name: string, description = ""): Promise<Project> {
  const r = await fetch("/api/projects", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, description }),
  });
  if (!r.ok) throw new Error("createProject: " + (await r.text()));
  return r.json();
}

// 修改 createTask 支持 project_id
export async function createTaskInProject(
  scenario: Scenario,
  input: string,
  projectId?: string,
): Promise<Task> {
  const body: any = { scenario, input };
  if (projectId) body.project_id = projectId;
  const r = await fetch("/api/tasks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error("createTask: " + (await r.text()));
  return r.json();
}

// ===== Integrations =====

export interface IntegrationStatus {
  slack: boolean;
  jira: boolean;
}

// v0.5: 专业角色画像
export interface RoleMeta {
  key: string;
  title: string;
  title_en: string;
  avatar: string;
  specialty: string;
  used_in: string[];
}

export async function listRolesByScenario(scenario: string): Promise<RoleMeta[]> {
  const r = await fetch(`/api/agents/roles/${scenario}`);
  if (!r.ok) return [];
  return (await r.json()).roles ?? [];
}

export async function getIntegrationStatus(): Promise<IntegrationStatus> {
  const r = await fetch("/api/integrations/status");
  if (!r.ok) throw new Error("integrationStatus: " + r.status);
  return r.json();
}

export async function notifySlack(taskId: string): Promise<void> {
  const r = await fetch("/api/integrations/slack/notify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ task_id: taskId }),
  });
  if (!r.ok) throw new Error("slack notify: " + (await r.text()));
}

export async function createJiraIssue(taskId: string, projectKey: string): Promise<{ key: string }> {
  const r = await fetch("/api/integrations/jira/issue", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ task_id: taskId, project_key: projectKey }),
  });
  if (!r.ok) throw new Error("jira issue: " + (await r.text()));
  return r.json();
}

export const SCENARIOS: { id: Scenario; label: string; short: string; placeholder: string; suggested: string[] }[] = [
  {
    id: "requirement_analysis",
    label: "需求分析",
    short: "需求",
    placeholder: "描述你的产品/市场上下文（Agent 会自动发现需求 + 评分排序）\n例：我们做一个面向 SaaS PM 的协作工具，已上线 3 个月有 200 付费用户...",
    suggested: ["AI 笔记 SaaS 待优化方向", "面向小红书博主的内容管理工具", "B2B SaaS 协作平台 v2 规划"],
  },
  {
    id: "competitor_research",
    label: "竞品调研",
    short: "竞品",
    placeholder: "一句话描述要调研的赛道或产品（如：国内 AI 笔记类产品）",
    suggested: ["国内 AI 笔记类产品", "海外低代码工作流平台", "AI 编码 IDE 赛道"],
  },
  {
    id: "requirement_validation",
    label: "需求验证",
    short: "验证",
    placeholder: "描述待验证的需求/假设（Agent 会拆 problem/solution/value 三类假设并执行验证）\n例：PM 需要 AI 自动生成竞品调研报告且愿意付 $29/月",
    suggested: [
      "PM 愿意为 AI 调研工具付 $29/月",
      "用户访谈分析自动化需求是否真存在",
      "Jira 集成是否能驱动转化",
    ],
  },
  {
    id: "interview_analysis",
    label: "访谈分析",
    short: "访谈",
    placeholder: "粘贴访谈转写文本，多场访谈用空行分隔",
    suggested: [
      "用户A：搜索功能太弱，搜不到想要的。\n\n用户B：导出 Excel 总要手动调格式。\n\n用户C：移动端基本没法用。",
    ],
  },
  {
    id: "prd_drafting",
    label: "PRD 起草",
    short: "PRD",
    placeholder: "一句话描述需求（如：希望产品内能一键反馈并快速消化）",
    suggested: [
      "希望产品内能让用户一键反馈问题并被 PM/客服快速消化",
      "增加跨部门协作的项目里程碑视图",
    ],
  },
  {
    id: "social_listening",
    label: "社交聆听",
    short: "社聆",
    placeholder: "输入要监听的产品名/赛道（Reddit 默认开，X/抖音/TikTok/YouTube 需配 key）",
    suggested: ["Notion AI", "Cursor IDE", "Manus agent"],
  },
];

export async function listTasks(): Promise<Task[]> {
  const r = await fetch("/api/tasks");
  if (!r.ok) throw new Error("listTasks: " + r.status);
  const data = await r.json();
  return data.tasks ?? [];
}

export async function createTask(scenario: Scenario, input: string): Promise<Task> {
  const r = await fetch("/api/tasks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ scenario, input }),
  });
  if (!r.ok) {
    const err = await r.text();
    throw new Error("createTask: " + err);
  }
  return r.json();
}

export async function getTask(id: string): Promise<Task> {
  const r = await fetch(`/api/tasks/${id}`);
  if (!r.ok) throw new Error("getTask: " + r.status);
  return r.json();
}

export async function getReport(id: string): Promise<Report | null> {
  const r = await fetch(`/api/tasks/${id}/report`);
  if (r.status === 404) return null;
  if (!r.ok) throw new Error("getReport: " + r.status);
  return r.json();
}

export async function getTraces(id: string): Promise<AgentEvent[]> {
  const r = await fetch(`/api/tasks/${id}/traces`);
  if (!r.ok) throw new Error("getTraces: " + r.status);
  const data = await r.json();
  return data.traces ?? [];
}

export interface SocialPost {
  platform: string;
  id: string;
  author: string;
  url: string;
  title: string;
  content: string;
  created_at?: string;
  engagement: { likes: number; comments: number; shares: number; views?: number };
  lang?: string;
}

export interface PostsResp {
  posts: SocialPost[];
  total: number;
  limit: number;
  offset: number;
}

export async function getPosts(
  id: string,
  opts: { platform?: string; q?: string; limit?: number; offset?: number } = {},
): Promise<PostsResp> {
  const params = new URLSearchParams();
  if (opts.platform) params.set("platform", opts.platform);
  if (opts.q) params.set("q", opts.q);
  if (opts.limit) params.set("limit", String(opts.limit));
  if (opts.offset) params.set("offset", String(opts.offset));
  const r = await fetch(`/api/tasks/${id}/posts?` + params);
  if (!r.ok) throw new Error("getPosts: " + r.status);
  return r.json();
}

export function subscribeStream(
  id: string,
  onEvent: (ev: AgentEvent) => void,
  onDone?: (ok: boolean) => void,
): () => void {
  const es = new EventSource(`/api/tasks/${id}/stream`);
  es.onmessage = (raw) => {
    try {
      onEvent(JSON.parse(raw.data));
    } catch (e) {
      console.error("parse SSE", e);
    }
  };
  const allSteps = ["start", "stage_start", "stage_done", "tool_call", "tool_result",
    "thought", "message", "done", "error", "task_succeeded", "task_failed"];
  for (const step of allSteps) {
    es.addEventListener(step, (raw: MessageEvent) => {
      try {
        const ev: AgentEvent = JSON.parse(raw.data);
        onEvent(ev);
        if (step === "task_succeeded" || step === "task_failed") {
          onDone?.(step === "task_succeeded");
          es.close();
        }
      } catch (e) {
        console.error("parse SSE evt", e);
      }
    });
  }
  es.onerror = (e) => console.warn("SSE error", e);
  return () => es.close();
}
