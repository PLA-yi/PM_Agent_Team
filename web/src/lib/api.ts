// 简单 API 客户端 — 走 vite proxy /api -> :8080

export type TaskStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled";

export type Scenario = "competitor_research" | "interview_analysis" | "prd_drafting" | "social_listening";

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
    id: "competitor_research",
    label: "竞品调研",
    short: "竞品",
    placeholder: "一句话描述要调研的赛道或产品（如：国内 AI 笔记类产品）",
    suggested: ["国内 AI 笔记类产品", "海外低代码工作流平台", "AI 编码 IDE 赛道"],
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
