import { useMemo, useState } from "react";
import type { AgentEvent } from "../lib/api";

interface Props {
  events: AgentEvent[];
}

// Agent 角色：name 显示中文，weight 控制色阶
const AGENT_LABEL: Record<string, { name: string; weight: string }> = {
  coordinator: { name: "调度员",     weight: "text-ink"   },
  planner:     { name: "规划员",     weight: "text-ink2"  },
  search:      { name: "搜索员",     weight: "text-ink2"  },
  scraper:     { name: "抓取员",     weight: "text-ink2"  },
  social:      { name: "社聆员",     weight: "text-ink2"  },
  extractor:   { name: "结构化员",   weight: "text-ink2"  },
  analyzer:    { name: "分析员",     weight: "text-ink2"  },
  writer:      { name: "撰写员",     weight: "text-ink2"  },
  reviewer:    { name: "复审员",     weight: "text-ink2"  },
  chunker:     { name: "切段员",     weight: "text-ink2"  },
  clusterer:   { name: "聚类员",     weight: "text-ink2"  },
  system:      { name: "系统",       weight: "text-muted" },
};

const STEP_LABEL: Record<string, string> = {
  start:           "开始执行",
  stage_start:     "阶段启动",
  stage_done:      "阶段完成",
  tool_call:       "调用工具",
  tool_result:     "工具返回",
  thought:         "思考",
  message:         "产出",
  done:            "完成",
  error:           "错误",
  task_succeeded:  "任务成功",
  task_failed:     "任务失败",
  warn:            "警告",
  merged_followup: "合并追问",
  kb_injected:     "注入项目记忆",
};

const STAGE_LABEL: Record<string, string> = {
  planning:        "规划阶段",
  researching:     "研究阶段",
  extracting:      "结构化阶段",
  analyzing:       "分析阶段",
  writing:         "撰写阶段",
  reviewing:       "复审阶段",
  self_correction: "自校正阶段",
  chunking:        "分段阶段",
  clustering:      "聚类阶段",
  synthesizing:    "综合阶段",
  background:      "背景阶段",
  stories:         "用户故事阶段",
  composing:       "编排阶段",
  scraping:        "抓取阶段",
};

const ENGINE_LABEL: Record<string, string> = {
  duckduckgo:  "DuckDuckGo 搜索",
  tavily:      "Tavily 搜索",
  jina:        "Jina 搜索",
  jina_reader: "Jina 抓取",
  reddit:      "Reddit",
  x:           "X (Twitter)",
  douyin:      "抖音",
  tiktok:      "TikTok",
  youtube:     "YouTube",
  mock:        "模拟",
};

function localizeStage(s: string): string {
  return STAGE_LABEL[s] ?? s;
}
function localizeEngine(e: string): string {
  return ENGINE_LABEL[e] ?? e;
}

export function AgentTimeline({ events }: Props) {
  const ordered = useMemo(() => events.slice().sort((a, b) => a.seq - b.seq), [events]);

  return (
    <div className="flex flex-col p-0">
      {ordered.length === 0 && (
        <div className="text-muted2 text-xs text-center py-12">
          等待 Agent 集群启动…
        </div>
      )}
      {ordered.map((ev, i) => (
        <EventRow key={ev.seq} ev={ev} isLast={i === ordered.length - 1} />
      ))}
    </div>
  );
}

function EventRow({ ev, isLast }: { ev: AgentEvent; isLast: boolean }) {
  const meta = AGENT_LABEL[ev.agent] ?? { name: ev.agent, weight: "text-muted" };
  const [open, setOpen] = useState(false);
  const isError = ev.step === "error" || ev.step === "task_failed";
  const isMilestone = ev.step === "stage_done" || ev.step === "done" || ev.step === "task_succeeded";
  const isWarn = ev.step === "warn";

  return (
    <div
      className={`group px-4 py-2 text-sm bg-white hover:bg-bg2/40 transition
        ${isLast ? "" : "border-b border-border"}
        ${isError ? "bg-red-50/40" : isWarn ? "bg-warn/5" : isMilestone ? "bg-green-50/40" : ""}`}
    >
      <div className="flex items-baseline gap-3 text-xs">
        <span className="text-placeholder tabular-nums w-7 shrink-0 mono">{String(ev.seq).padStart(2, '0')}</span>
        <span className={`font-medium ${meta.weight} w-20 shrink-0 truncate`}>{meta.name}</span>
        <span className="text-muted2 w-20 shrink-0 truncate">{STEP_LABEL[ev.step] ?? ev.step}</span>
        <span className="flex-1 truncate text-ink2 leading-relaxed">
          <NarrativePreview ev={ev} />
        </span>
        {ev.payload != null && (
          <button
            onClick={() => setOpen((o) => !o)}
            className="text-placeholder hover:text-ink transition shrink-0 text-[14px] leading-none"
            title={open ? "收起原始 payload" : "查看原始 payload"}
          >
            {open ? "−" : "+"}
          </button>
        )}
      </div>
      {open && (
        <pre className="mt-2 ml-10 text-xs text-muted bg-bg border border-border rounded p-3 overflow-x-auto leading-relaxed mono">
{JSON.stringify(ev.payload, null, 2)}
        </pre>
      )}
    </div>
  );
}

// NarrativePreview 把 payload 渲染成中文自然语言
function NarrativePreview({ ev }: { ev: AgentEvent }) {
  const p = ev.payload;
  if (p == null) return null;
  if (typeof p !== "object") return <>{String(p)}</>;

  const agent = ev.agent;
  const step = ev.step;

  // ===== 工具调用 =====
  if (step === "tool_call") {
    if (p.engine && p.query) {
      return (
        <>
          调用 <span className="font-medium text-ink2">{localizeEngine(p.engine)}</span>，关键词：
          <span className="text-ink"> "{p.query}"</span>
          {p.target_k && p.target_k !== "0" && (
            <span className="text-muted2"> (目标 {p.target_k} 条)</span>
          )}
        </>
      );
    }
    if (p.engine && p.url) {
      return (
        <>
          通过 <span className="font-medium text-ink2">{localizeEngine(p.engine)}</span> 抓取：
          <span className="text-ink mono"> {p.url}</span>
        </>
      );
    }
  }

  // ===== 工具返回 =====
  if (step === "tool_result") {
    if (p.platform && p.query !== undefined) {
      const raw = p.raw ?? 0;
      const relevant = p.relevant ?? raw;
      const added = p.added ?? 0;
      return (
        <>
          <span className="text-muted2">{localizeEngine(p.platform)}</span> 返回 {raw} 条
          {raw !== relevant && (
            <span>，过滤后剩 <span className="text-ink2">{relevant}</span> 相关</span>
          )}
          ，新增 <span className="text-success">+{added}</span>
        </>
      );
    }
    if (p.url && p.bytes !== undefined) {
      return (
        <>
          抓回 <span className="text-ink mono">{p.url}</span>
          <span className="text-muted2"> ({(p.bytes / 1024).toFixed(1)} KB)</span>
        </>
      );
    }
    if (p.count !== undefined && p.query) {
      return (
        <>
          搜索 "{p.query}"，返回 <span className="text-ink2">{p.count}</span> 条
        </>
      );
    }
  }

  // ===== 阶段事件 =====
  if (step === "stage_start" || step === "stage_done") {
    const stage = localizeStage(p.stage);
    if (step === "stage_start") {
      const agents = Array.isArray(p.agents)
        ? p.agents.map((a: string) => AGENT_LABEL[a]?.name ?? a).join(" + ")
        : "";
      return (
        <>
          进入 <span className="text-accent font-medium">{stage}</span>
          {agents && <span className="text-muted2"> · {agents}{p.parallel ? " 并行" : ""}</span>}
        </>
      );
    }
    return (
      <>
        <span className="text-success font-medium">{stage}</span> 完成
      </>
    );
  }

  // ===== Coordinator 启动 =====
  if (agent === "coordinator" && step === "start") {
    return (
      <>
        启动调度，输入：<span className="text-ink">"{p.input}"</span>
        {p.stages && Array.isArray(p.stages) && (
          <span className="text-muted2"> · {p.stages.length} 个阶段</span>
        )}
      </>
    );
  }

  if (agent === "coordinator" && step === "thought") {
    if (p.action === "trigger_rewrite") {
      return (
        <>
          复审分数 <span className="text-warn">{p.score}</span> 低于阈值 {p.threshold}，
          触发<span className="font-medium text-ink2">自校正重写</span>
          {p.issues && Array.isArray(p.issues) && (
            <span className="text-muted2"> ({p.issues.length} 条改进意见)</span>
          )}
        </>
      );
    }
    if (p.action === "skip_retry") {
      return (
        <>
          复审分数 <span className="text-success">{p.score}</span> ≥ 阈值 {p.threshold}，无需重写
        </>
      );
    }
  }

  if (agent === "coordinator" && step === "done") {
    return (
      <>
        全流程完成，耗时 <span className="mono text-ink">{((p.elapsed_ms || 0) / 1000).toFixed(1)}s</span>
        {p.competitors !== undefined && <span className="text-muted2"> · {p.competitors} 个竞品</span>}
        {p.sources !== undefined && <span className="text-muted2"> · {p.sources} 条引用</span>}
      </>
    );
  }

  // ===== Planner =====
  if (agent === "planner" && step === "message") {
    if (p.outline_count !== undefined) {
      const candNames = Array.isArray(p.candidates)
        ? p.candidates.map((c: any) => c.name_en || c.name).slice(0, 3).join(" / ")
        : "";
      return (
        <>
          产出 <span className="text-ink2">{p.outline_count}</span> 个调研维度，
          <span className="text-ink2">{p.candidate_count}</span> 个候选竞品
          {candNames && <span className="text-muted2"> · {candNames}…</span>}
        </>
      );
    }
    if (p.background) {
      return (
        <>
          已规划背景：<span className="text-ink2">"{String(p.background).slice(0, 60)}…"</span>
          {p.goals && Array.isArray(p.goals) && (
            <span className="text-muted2"> · {p.goals.length} 个目标</span>
          )}
        </>
      );
    }
  }

  // ===== Search / Scraper / Social start =====
  if (agent === "search" && step === "start") {
    return <>对 <span className="text-ink2">{p.queries}</span> 个关键词并发搜索</>;
  }
  if (agent === "scraper" && step === "start") {
    return <>对 <span className="text-ink2">{p.candidates}</span> 个候选页面抓取</>;
  }
  if (agent === "social" && step === "start") {
    const cands = p.candidates ?? 0;
    return (
      <>
        对 <span className="text-ink2">{cands}</span> 个候选拉社交平台讨论
        {p.mode && <span className="text-muted2"> · {p.mode === "embedded" ? "嵌入模式" : "独立模式"}</span>}
      </>
    );
  }
  if (agent === "social" && step === "thought") {
    if (Array.isArray(p.authed_platforms)) {
      const authed = p.authed_platforms.map(localizeEngine).join(" / ");
      const unauth = Array.isArray(p.unauth_platforms) ? p.unauth_platforms.map(localizeEngine).join(" / ") : "";
      return (
        <>
          已鉴权平台：<span className="text-ink2">{authed || "无"}</span>
          {unauth && <span className="text-muted2"> · 未配 key：{unauth}</span>}
        </>
      );
    }
  }
  if (agent === "social" && step === "message") {
    return (
      <>
        共抓 <span className="text-ink">{p.posts_total}</span> 条帖子
        {p.sources_added !== undefined && <span className="text-muted2"> · 引用列表收 {p.sources_added}</span>}
      </>
    );
  }

  // ===== Extractor / Analyzer =====
  if (agent === "extractor" && step === "message") {
    if (p.competitors_count !== undefined) {
      return <>提取 <span className="text-ink2">{p.competitors_count}</span> 个结构化竞品</>;
    }
    if (p.stories !== undefined) {
      return <>生成 <span className="text-ink2">{p.stories}</span> 条用户故事</>;
    }
  }
  if (agent === "analyzer" && step === "message") {
    return <>SWOT 与差异化机会分析完成</>;
  }

  // ===== Writer / Reviewer =====
  if (agent === "writer" && step === "start") {
    if (p.mode === "rewrite") {
      const issues = Array.isArray(p.critique) ? p.critique.length : 0;
      return <>开始重写报告（基于 {issues} 条复审意见）</>;
    }
    return <>开始撰写报告…</>;
  }
  if (agent === "writer" && step === "message") {
    return (
      <>
        报告 <span className="text-ink mono">{p.length || 0}</span> 字
        {p.preview && <span className="text-muted2"> · "{String(p.preview).slice(0, 50)}…"</span>}
      </>
    );
  }
  if (agent === "reviewer" && step === "start") {
    return (
      <>
        复审第 <span className="text-ink2">{p.iteration}</span> 轮
        <span className="text-muted2"> · 报告 {p.report_len} 字 / {p.competitors} 竞品 / {p.sources} 引用</span>
      </>
    );
  }
  if (agent === "reviewer" && step === "message") {
    const verdict = p.verdict;
    const verdictCN = verdict === "accept" ? "通过" : verdict === "revise" ? "需修改" : "拒绝";
    const verdictColor = verdict === "accept" ? "text-success" : verdict === "revise" ? "text-warn" : "text-danger";
    return (
      <>
        综合分 <span className={`font-medium ${verdictColor}`}>{p.overall}/10</span>
        <span className="text-muted2"> (事实 {p.fact} · 覆盖 {p.coverage} · 引用 {p.citation})</span>
        <span className={`ml-1 ${verdictColor}`}> · {verdictCN}</span>
        {p.issues > 0 && <span className="text-muted2"> · {p.issues} 条改进意见</span>}
      </>
    );
  }

  // ===== System =====
  if (agent === "system" && step === "task_succeeded") {
    return (
      <>
        任务成功完成
        {p.report_bytes !== undefined && <span className="text-muted2"> · 报告 {p.report_bytes} 字 / {p.sources} 引用</span>}
      </>
    );
  }
  if (agent === "system" && step === "task_failed") {
    return <span className="text-danger">任务失败：{p.err}</span>;
  }
  if (agent === "system" && step === "merged_followup") {
    return (
      <>
        追问产物已合并：<span className="text-ink2">+{p.new_posts}</span> 帖子 ·
        <span className="text-ink2"> +{p.new_chars}</span> 字
      </>
    );
  }
  if (agent === "system" && step === "kb_injected") {
    return (
      <>
        从项目知识库注入 <span className="text-ink2">{p.entries}</span> 条历史记忆
      </>
    );
  }

  // ===== Error =====
  if (step === "error") {
    return <span className="text-danger">{p.err || "未知错误"}</span>;
  }

  // ===== 兜底：展示前两个字段 =====
  const keys = Object.keys(p).slice(0, 2);
  return (
    <>
      {keys.map((k, i) => (
        <span key={k}>
          {i > 0 && <span className="text-placeholder mx-1.5">·</span>}
          <span className="text-placeholder">{k}=</span>
          {typeof p[k] === "object" ? "{…}" : String(p[k]).slice(0, 40)}
        </span>
      ))}
    </>
  );
}
