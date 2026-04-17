// ModulePage 通用 — 每个 PM 板块的专属页面
// 复用：左栏(Agent 集群+输入+任务列表) / 中栏(协作流) / 右栏(报告+追问+串联CTA)

import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  AgentEvent, MODULES, ModuleDef, Project, Report, SCENARIOS, Task,
  createProject, createTaskInProject, getReport, getTask, getTraces,
  listProjects, listTasks, subscribeStream,
} from "../lib/api";
import { TaskList } from "../components/TaskList";
import { AgentTimeline } from "../components/AgentTimeline";
import { ReportPreview } from "../components/ReportPreview";
import { PostsViewer } from "../components/PostsViewer";
import { AgentRoster } from "../components/AgentRoster";

interface Props {
  module: ModuleDef;
}

// 串联次序：requirement → competitor → validation
const NEXT_MODULE: Record<string, string> = {
  requirement: "/competitor",
  competitor: "/validation",
  validation: "",
};
const NEXT_MODULE_LABEL: Record<string, string> = {
  requirement: "→ 用 Top 需求做竞品调研",
  competitor: "→ 用差异化机会做需求验证",
  validation: "",
};

export function ModulePage({ module }: Props) {
  const { taskId: routeTaskId } = useParams();
  const navigate = useNavigate();

  const [tasks, setTasks] = useState<Task[]>([]);
  const [selectedId, setSelectedId] = useState<string | undefined>(routeTaskId);
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [report, setReport] = useState<Report | null>(null);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rightTab, setRightTab] = useState<"report" | "posts">("report");
  const [projects, setProjects] = useState<Project[]>([]);
  const [activeProject, setActiveProject] = useState<string | "">("");
  const unsubRef = useRef<(() => void) | null>(null);
  const pollRef = useRef<number | null>(null);

  const scenarioMeta = SCENARIOS.find((s) => s.id === module.scenario)!;

  // 只显示当前 scenario 的 tasks
  const filteredTasks = tasks.filter((t) => t.scenario === module.scenario);

  useEffect(() => {
    listTasks().then((t) => {
      setTasks(t);
      const own = t.find((x) => x.scenario === module.scenario);
      if (own && !selectedId) setSelectedId(own.id);
    }).catch((e) => setError(String(e)));
    listProjects().then(setProjects).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [module.scenario]);

  useEffect(() => {
    pollRef.current = window.setInterval(() => {
      listTasks().then(setTasks).catch(() => {});
    }, 3000);
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current);
    };
  }, []);

  useEffect(() => {
    if (!selectedId) return;
    setEvents([]);
    setReport(null);
    setError(null);

    let cancelled = false;
    Promise.all([getTask(selectedId), getTraces(selectedId), getReport(selectedId)])
      .then(([task, traces, rep]) => {
        if (cancelled) return;
        setEvents(traces);
        setReport(rep);
        if (task.status === "running" || task.status === "queued") {
          unsubRef.current = subscribeStream(
            selectedId,
            (ev) => setEvents((prev) => prev.some(e => e.seq === ev.seq) ? prev : [...prev, ev]),
            async (ok) => {
              if (ok) {
                const r = await getReport(selectedId);
                if (!cancelled) setReport(r);
              }
              listTasks().then(setTasks);
            },
          );
        }
      })
      .catch((e) => !cancelled && setError(String(e)));

    return () => {
      cancelled = true;
      unsubRef.current?.();
      unsubRef.current = null;
    };
  }, [selectedId]);

  const onCreate = useCallback(async (input: string) => {
    if (!input.trim() || busy) return;
    setBusy(true);
    setError(null);
    try {
      const t = await createTaskInProject(module.scenario, input.trim(), activeProject || undefined);
      setDraft("");
      setSelectedId(t.id);
      setTasks((prev) => [t, ...prev.filter((x) => x.id !== t.id)]);
      navigate(`${window.location.pathname.split("/")[1] ? "/" + window.location.pathname.split("/")[1] : ""}/${t.id}`, { replace: false });
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }, [busy, module.scenario, activeProject, navigate]);

  const onCreateProject = useCallback(async () => {
    const name = window.prompt("新项目名");
    if (!name?.trim()) return;
    try {
      const p = await createProject(name.trim());
      setProjects((prev) => [p, ...prev]);
      setActiveProject(p.id);
    } catch (e) {
      setError(String(e));
    }
  }, []);

  const selectedTask = tasks.find((t) => t.id === selectedId);

  // 串联到下一板块：把当前报告关键点拼成 input prefill
  const chainToNext = useCallback(() => {
    const nextRoute = NEXT_MODULE[module.id];
    if (!nextRoute || !report) return;
    const ctx = `[来自上一阶段「${module.label}」的产出]\n${report.title}\n\n${report.markdown.slice(0, 600)}...\n\n基于以上分析，进入下一阶段。`;
    sessionStorage.setItem("chain_input", ctx);
    sessionStorage.setItem("chain_project", activeProject);
    navigate(nextRoute);
  }, [module, report, navigate, activeProject]);

  // 接收上游传过来的 input
  useEffect(() => {
    const chained = sessionStorage.getItem("chain_input");
    if (chained) {
      setDraft(chained);
      sessionStorage.removeItem("chain_input");
    }
    const chainedProj = sessionStorage.getItem("chain_project");
    if (chainedProj) {
      setActiveProject(chainedProj);
      sessionStorage.removeItem("chain_project");
    }
  }, [module.id]);

  return (
    <div className="flex-1 grid grid-cols-[340px_1fr_minmax(420px,1.2fr)] gap-4 p-4 overflow-hidden">
      {/* Left column: 模块标识 + Agent 集群 + 输入 + 任务列表 */}
      <aside className="flex flex-col gap-4 overflow-hidden">
        {/* 模块徽章 */}
        <div className="ios-card p-4" style={{ borderLeftWidth: 4, borderLeftColor: module.accent }}>
          <div className="flex items-center gap-2">
            <span className="text-2xl">{module.emoji}</span>
            <div className="flex-1">
              <div className="text-base font-semibold text-ink tracking-tight">{module.label}</div>
              <div className="text-[11px] text-muted">{module.desc}</div>
            </div>
          </div>
          <div className="flex items-center gap-1 mt-2 text-[10px] mono text-muted2 uppercase tracking-wider">
            {module.stages.map((s, i) => (
              <span key={i} className="flex items-center gap-1">
                {i > 0 && <span>›</span>}
                <span>{s}</span>
              </span>
            ))}
          </div>
        </div>

        {/* Agent Roster */}
        <AgentRoster scenario={module.scenario} events={events} />

        {/* 输入 */}
        <div className="ios-card p-4 space-y-3">
          <div className="flex items-center gap-2 text-xs">
            <span className="text-muted2 uppercase tracking-wider font-semibold">Project</span>
            <select
              value={activeProject}
              onChange={(e) => setActiveProject(e.target.value)}
              className="flex-1 text-xs px-2 py-1 rounded border border-border bg-white text-ink"
            >
              <option value="">— 未归属 —</option>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
            <button onClick={onCreateProject} className="ios-btn ios-btn-ghost text-xs px-2 py-1">+</button>
          </div>
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={scenarioMeta.placeholder}
            rows={4}
            className="ios-input resize-none"
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                onCreate(draft);
              }
            }}
          />
          <button
            disabled={busy || !draft.trim()}
            onClick={() => onCreate(draft)}
            className="ios-btn ios-btn-primary w-full py-2 text-sm disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {busy ? "Submitting…" : `Run ${module.label}`}
            <span className="ml-2 text-[11px] opacity-60 mono">⌘↵</span>
          </button>
          {scenarioMeta.suggested.length > 0 && (
            <div className="flex flex-wrap gap-1.5 pt-0.5">
              {scenarioMeta.suggested.map((s, i) => (
                <button key={i} onClick={() => setDraft(s)}
                  className="ios-chip max-w-full truncate cursor-pointer" title={s}>
                  {s.length > 18 ? s.slice(0, 18) + "…" : s}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* 任务列表（仅当前 scenario 的） */}
        <div className="ios-card flex-1 overflow-hidden flex flex-col">
          <header className="h-10 px-4 flex items-center border-b border-border shrink-0 bg-white">
            <span className="text-xs font-semibold text-ink uppercase tracking-wider">本板块任务</span>
            <span className="ml-auto text-xs text-muted2">{filteredTasks.length}</span>
          </header>
          <div className="flex-1 overflow-y-auto">
            <TaskList tasks={filteredTasks} selectedId={selectedId} onSelect={setSelectedId} />
          </div>
        </div>

        {error && (
          <div className="ios-card p-3 text-xs text-danger border border-danger/30">{error}</div>
        )}
      </aside>

      {/* Middle: Agent Trace */}
      <section className="ios-card flex flex-col overflow-hidden">
        <header className="h-10 px-4 flex items-center border-b border-border shrink-0 bg-white">
          <span className="text-xs font-semibold text-ink uppercase tracking-wider">Agent 协作流</span>
          {selectedTask && (
            <span className="ml-3 ios-chip mono text-[10px]">
              {selectedTask.stage || "—"} · {selectedTask.progress}%
            </span>
          )}
          <span className="ml-auto text-xs text-muted2 mono">{events.length} events</span>
        </header>
        <div className="flex-1 overflow-y-auto bg-bg">
          <AgentTimeline events={events} />
        </div>
      </section>

      {/* Right: Report + Posts */}
      <section className="ios-card flex flex-col overflow-hidden">
        <header className="h-10 px-4 flex items-center border-b border-border shrink-0 bg-white gap-2">
          <button
            onClick={() => setRightTab("report")}
            className={`text-xs font-semibold uppercase tracking-wider px-2 py-1 rounded transition
              ${rightTab === "report" ? "text-ink bg-bg2" : "text-muted2 hover:text-ink"}`}
          >
            Report
          </button>
          <button
            onClick={() => setRightTab("posts")}
            className={`text-xs font-semibold uppercase tracking-wider px-2 py-1 rounded transition
              ${rightTab === "posts" ? "text-ink bg-bg2" : "text-muted2 hover:text-ink"}`}
          >
            Posts <span className="mono text-muted2 font-normal">raw</span>
          </button>

          {/* 串联到下一板块 CTA */}
          {NEXT_MODULE[module.id] && selectedTask?.status === "succeeded" && report && (
            <button
              onClick={chainToNext}
              className="ml-auto ios-btn text-xs px-3 py-1"
              style={{ background: module.accent, color: "#FFFFFF" }}
              title="把本报告作为上下文，自动跳到下一板块"
            >
              {NEXT_MODULE_LABEL[module.id]}
            </button>
          )}
        </header>
        <div className="flex-1 overflow-hidden">
          {rightTab === "report" ? (
            <div className="h-full overflow-y-auto">
              <ReportPreview
                report={report}
                loading={selectedTask?.status === "running" || selectedTask?.status === "queued"}
                taskId={selectedId}
                taskStatus={selectedTask?.status}
                onFollowUp={(newId) => {
                  listTasks().then((t) => {
                    setTasks(t);
                    setSelectedId(newId);
                  });
                }}
              />
            </div>
          ) : (
            <PostsViewer taskId={selectedId} />
          )}
        </div>
      </section>
    </div>
  );
}

// 三个 module 的具体 page 包装
export const RequirementPage = () => <ModulePage module={MODULES[0]} />;
export const CompetitorPage = () => <ModulePage module={MODULES[1]} />;
export const ValidationPage = () => <ModulePage module={MODULES[2]} />;
