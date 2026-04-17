// Dashboard — PM 工作流总览，串联三大板块
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { MODULES, Project, Task, listProjects, listTasks } from "../lib/api";

export function Dashboard() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);

  useEffect(() => {
    listTasks().then(setTasks).catch(() => {});
    listProjects().then(setProjects).catch(() => {});
  }, []);

  const moduleStats = MODULES.map((m) => {
    const own = tasks.filter((t) => t.scenario === m.scenario);
    const done = own.filter((t) => t.status === "succeeded").length;
    const running = own.filter((t) => t.status === "running").length;
    const recent = own[0];
    return { module: m, total: own.length, done, running, recent };
  });

  return (
    <div className="flex-1 overflow-auto bg-bg">
      <div className="max-w-6xl mx-auto p-8">
        <header className="mb-8">
          <h1 className="text-2xl font-semibold text-ink tracking-tight">PM Agent Team</h1>
          <p className="text-sm text-muted mt-1">
            按 PM 工作流串联：<span className="text-ink2 font-medium">需求分析</span>
            <span className="text-muted2 mx-1">›</span>
            <span className="text-ink2 font-medium">竞品调研</span>
            <span className="text-muted2 mx-1">›</span>
            <span className="text-ink2 font-medium">需求验证</span>
          </p>
        </header>

        {/* 三大板块大卡 */}
        <div className="grid md:grid-cols-3 gap-4 mb-8">
          {moduleStats.map(({ module, total, done, running, recent }) => (
            <Link
              key={module.id}
              to={`/${module.id}`}
              className="ios-card p-5 hover:shadow-cardHi transition"
              style={{ borderTop: `3px solid ${module.accent}` }}
            >
              <div className="flex items-start justify-between mb-3">
                <div className="text-3xl">{module.emoji}</div>
                <div className="text-right">
                  <div className="text-2xl font-semibold text-ink tabular-nums">{total}</div>
                  <div className="text-[10px] text-muted2 uppercase tracking-wider mono">tasks</div>
                </div>
              </div>
              <div className="text-base font-semibold text-ink tracking-tight mb-1">{module.label}</div>
              <p className="text-xs text-muted leading-relaxed">{module.desc}</p>
              <div className="mt-3 pt-3 border-t border-border flex items-center gap-3 text-xs">
                <span className="flex items-center gap-1">
                  <span className="dot dot-success" />
                  <span className="mono text-muted">{done}</span>
                </span>
                {running > 0 && (
                  <span className="flex items-center gap-1">
                    <span className="dot dot-running" />
                    <span className="mono text-muted">{running}</span>
                  </span>
                )}
                <span className="ml-auto text-[10px] text-muted2 mono uppercase">
                  {module.stages.length} stages · {module.stages.length * 2}+ agents
                </span>
              </div>
              {recent && (
                <div className="mt-2 text-[11px] text-muted2 truncate">最近：{recent.input}</div>
              )}
            </Link>
          ))}
        </div>

        {/* 工作流串联示意 */}
        <div className="ios-card p-6 mb-8">
          <h2 className="text-sm font-semibold text-ink uppercase tracking-wider mb-4">PM 工作流串联</h2>
          <div className="flex items-center gap-2 overflow-x-auto">
            {MODULES.map((m, i) => (
              <div key={m.id} className="flex items-center gap-2">
                {i > 0 && <span className="text-2xl text-placeholder">→</span>}
                <Link
                  to={`/${m.id}`}
                  className="rounded-lg border border-border px-4 py-3 hover:bg-bg2 transition min-w-[160px]"
                  style={{ borderLeftWidth: 4, borderLeftColor: m.accent }}
                >
                  <div className="text-xs text-muted2 mono uppercase tracking-wider">阶段 {i+1}</div>
                  <div className="text-sm font-semibold text-ink mt-0.5">{m.emoji} {m.label}</div>
                </Link>
              </div>
            ))}
          </div>
          <p className="text-xs text-muted2 mt-4 leading-relaxed">
            每个板块完成后右上角会出现「→ 进入下一板块」按钮，自动把上阶段产出作为下阶段的上下文。
            可在同一 Project 下完整跑完三阶段，跨任务知识库自动召回。
          </p>
        </div>

        {/* Projects 概览 */}
        {projects.length > 0 && (
          <div className="ios-card p-6">
            <h2 className="text-sm font-semibold text-ink uppercase tracking-wider mb-4">Projects · {projects.length}</h2>
            <div className="space-y-2">
              {projects.map((p) => {
                const projTasks = tasks.filter((t) => (t as any).project_id === p.id);
                return (
                  <div key={p.id} className="flex items-center gap-3 py-2 border-b border-border last:border-b-0">
                    <div className="flex-1">
                      <div className="text-sm font-medium text-ink">{p.name}</div>
                      {p.description && <div className="text-[11px] text-muted2">{p.description}</div>}
                    </div>
                    <div className="text-xs text-muted mono">{projTasks.length} tasks</div>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* 最近 5 任务 */}
        <div className="ios-card p-6 mt-6">
          <h2 className="text-sm font-semibold text-ink uppercase tracking-wider mb-4">最近 5 任务</h2>
          {tasks.length === 0 ? (
            <p className="text-xs text-muted2 mono">no tasks yet</p>
          ) : (
            <div className="space-y-2">
              {tasks.slice(0, 5).map((t) => {
                const m = MODULES.find((m) => m.scenario === t.scenario);
                const moduleId = m?.id ?? "";
                return (
                  <Link
                    key={t.id}
                    to={moduleId ? `/${moduleId}/${t.id}` : "#"}
                    className="flex items-center gap-3 py-2 hover:bg-bg2 px-2 -mx-2 rounded transition"
                  >
                    <span className="text-base">{m?.emoji ?? "📄"}</span>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-ink truncate">{t.input}</div>
                      <div className="text-[11px] text-muted2 mono">{m?.label} · {t.status} · {t.progress}%</div>
                    </div>
                    <div className={`dot ${
                      t.status === "succeeded" ? "dot-success" :
                      t.status === "running" ? "dot-running" :
                      t.status === "failed" ? "dot-failed" : "dot-queued"
                    }`} />
                  </Link>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
