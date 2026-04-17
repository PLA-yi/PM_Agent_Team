import { useCallback, useEffect, useRef, useState } from "react";
import {
  AgentEvent,
  IntegrationStatus,
  Project,
  Report,
  SCENARIOS,
  Scenario,
  Task,
  createProject,
  createTaskInProject,
  getIntegrationStatus,
  getReport,
  getTask,
  getTraces,
  listProjects,
  listTasks,
  subscribeStream,
} from "./lib/api";
import { TaskList } from "./components/TaskList";
import { AgentTimeline } from "./components/AgentTimeline";
import { ReportPreview } from "./components/ReportPreview";
import { PostsViewer } from "./components/PostsViewer";

export function App() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [selectedId, setSelectedId] = useState<string | undefined>(undefined);
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [report, setReport] = useState<Report | null>(null);
  const [scenario, setScenario] = useState<Scenario>("competitor_research");
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [rightTab, setRightTab] = useState<"report" | "posts">("report");
  const [projects, setProjects] = useState<Project[]>([]);
  const [activeProject, setActiveProject] = useState<string | "">("");
  const [integrations, setIntegrations] = useState<IntegrationStatus>({ slack: false, jira: false });
  const unsubRef = useRef<(() => void) | null>(null);
  const pollRef = useRef<number | null>(null);

  const scenarioMeta = SCENARIOS.find((s) => s.id === scenario)!;

  useEffect(() => {
    listTasks().then((t) => {
      setTasks(t);
      if (t.length > 0 && !selectedId) setSelectedId(t[0].id);
    }).catch((e) => setError(String(e)));
    listProjects().then(setProjects).catch(() => {});
    getIntegrationStatus().then(setIntegrations).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
            (ev) => setEvents((prev) => mergeEvent(prev, ev)),
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
      const t = await createTaskInProject(scenario, input.trim(), activeProject || undefined);
      setDraft("");
      setSelectedId(t.id);
      setTasks((prev) => [t, ...prev.filter((x) => x.id !== t.id)]);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }, [busy, scenario, activeProject]);

  const onCreateProject = useCallback(async () => {
    const name = window.prompt("项目名（如：AI 笔记 SaaS 立项）");
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

  return (
    <div className="h-full flex flex-col bg-bg">
      {/* Top bar — minimal, Linear-style */}
      <header className="h-12 px-5 flex items-center bg-white border-b border-border shrink-0 gap-3">
        <span className="font-semibold text-ink text-sm tracking-tight">PMHive</span>
        <span className="ios-chip mono">v0.7</span>

        {/* Project 选择器 */}
        <div className="flex items-center gap-1.5 ml-3">
          <span className="text-[11px] text-muted2 uppercase tracking-wider font-semibold">Project</span>
          <select
            value={activeProject}
            onChange={(e) => setActiveProject(e.target.value)}
            className="text-xs px-2 py-1 rounded border border-border bg-white text-ink"
          >
            <option value="">— 未归属 —</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
          <button
            onClick={onCreateProject}
            className="ios-btn ios-btn-ghost text-xs px-2 py-1"
            title="新建项目空间"
          >
            +
          </button>
        </div>

        <span className="ml-auto flex items-center gap-2 text-xs text-muted2">
          <span className={`ios-chip ${integrations.slack ? "text-success" : ""}`} title={integrations.slack ? "Slack 已配置" : "Slack 未配置 (设 SLACK_WEBHOOK_URL)"}>
            {integrations.slack ? "● Slack" : "○ Slack"}
          </span>
          <span className={`ios-chip ${integrations.jira ? "text-success" : ""}`} title={integrations.jira ? "Jira 已配置" : "Jira 未配置 (设 JIRA_*)"}>
            {integrations.jira ? "● Jira" : "○ Jira"}
          </span>
          <span>AI Product Manager Agent Cluster</span>
        </span>
      </header>

      {/* Main grid */}
      <div className="flex-1 grid grid-cols-[320px_1fr_minmax(420px,1.2fr)] gap-4 p-4 overflow-hidden">
        {/* Left column */}
        <aside className="flex flex-col gap-4 overflow-hidden">
          {/* Composer */}
          <div className="ios-card p-4 space-y-3">
            <div className="grid grid-cols-2 border border-border rounded-md overflow-hidden">
              {SCENARIOS.map((s, i) => (
                <button
                  key={s.id}
                  onClick={() => { setScenario(s.id); setDraft(""); }}
                  className={`text-xs py-1.5 transition tracking-tight font-medium
                    ${i % 2 === 1 ? "border-l border-border" : ""}
                    ${i >= 2 ? "border-t border-border" : ""}
                    ${scenario === s.id ? "bg-ink text-white" : "bg-white text-muted hover:bg-bg2 hover:text-ink"}`}
                >
                  {s.label}
                </button>
              ))}
            </div>

            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={scenarioMeta.placeholder}
              rows={scenario === "interview_analysis" ? 6 : 3}
              className="ios-input resize-none"
              style={{ fontFamily: scenario === "interview_analysis" ? "JetBrains Mono, SF Mono, Menlo, monospace" : undefined }}
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
              {busy ? "Submitting…" : `Run ${scenarioMeta.label}`}
              <span className="ml-2 text-[11px] opacity-60 mono">⌘↵</span>
            </button>
            {scenarioMeta.suggested.length > 0 && (
              <div className="flex flex-wrap gap-1.5 pt-0.5">
                {scenarioMeta.suggested.map((s, i) => (
                  <button
                    key={i}
                    onClick={() => setDraft(s)}
                    className="ios-chip max-w-full truncate cursor-pointer"
                    title={s}
                  >
                    {s.length > 20 ? s.slice(0, 20) + "…" : s}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Task list */}
          <div className="ios-card flex-1 overflow-hidden flex flex-col">
            <PanelHeader title="Tasks" right={`${tasks.length}`} />
            <div className="flex-1 overflow-y-auto">
              <TaskList tasks={tasks} selectedId={selectedId} onSelect={setSelectedId} />
            </div>
          </div>

          {error && (
            <div className="ios-card p-3 text-xs text-danger border border-danger/30">
              {error}
            </div>
          )}
        </aside>

        {/* Middle column */}
        <section className="ios-card flex flex-col overflow-hidden">
          <PanelHeader
            title="Agent Trace"
            right={
              <span className="flex items-center gap-2 text-xs text-muted2">
                {selectedTask && (
                  <>
                    <span className="mono">{selectedTask.stage || "—"}</span>
                    <span className="text-placeholder">·</span>
                    <span className="mono tabular-nums">{selectedTask.progress}%</span>
                    <span className="text-placeholder">·</span>
                  </>
                )}
                <span className="mono">{events.length} events</span>
              </span>
            }
          />
          <div className="flex-1 overflow-y-auto bg-bg">
            <AgentTimeline events={events} />
          </div>
        </section>

        {/* Right column */}
        <section className="ios-card overflow-hidden flex flex-col">
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
    </div>
  );
}

function PanelHeader({ title, right }: { title: string; right?: React.ReactNode }) {
  return (
    <header className="h-10 px-4 flex items-center border-b border-border shrink-0 bg-white">
      <span className="text-xs font-semibold text-ink uppercase tracking-wider">{title}</span>
      <span className="ml-auto text-xs text-muted2">
        {right}
      </span>
    </header>
  );
}

function mergeEvent(prev: AgentEvent[], ev: AgentEvent): AgentEvent[] {
  if (prev.some((e) => e.seq === ev.seq)) return prev;
  return [...prev, ev];
}
