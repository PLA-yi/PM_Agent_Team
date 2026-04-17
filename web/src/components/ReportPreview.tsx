import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { Report, ReviewResult, TaskUsage } from "../lib/api";
import { followUp, getUsage } from "../lib/api";

interface Props {
  report: Report | null;
  loading?: boolean;
  taskId?: string;
  taskStatus?: string;
  onFollowUp?: (newTaskId: string) => void;
}

export function ReportPreview({ report, loading, taskId, taskStatus, onFollowUp }: Props) {
  if (loading && !report) {
    return (
      <div className="p-10 text-muted2 text-xs flex flex-col items-center gap-2">
        <div className="dot dot-running" />
        <span className="mono">generating report</span>
      </div>
    );
  }
  if (!report) {
    return (
      <div className="p-10 text-muted2 text-xs text-center mono">
        report.empty
      </div>
    );
  }

  const review = report.metadata?.review as ReviewResult | undefined;

  return (
    <div className="px-8 py-6">
      <div className="flex items-center justify-between mb-4 pb-4 border-b border-border">
        <h1 className="text-base font-semibold text-ink tracking-tight truncate pr-4">{report.title}</h1>
        <button
          onClick={() => downloadMarkdown(report)}
          className="ios-btn ios-btn-ghost text-xs px-3 py-1 shrink-0"
        >
          Export .md
        </button>
      </div>

      {review && <ReviewCard review={review} />}

      {/* v0.6 LLM 用量摘要 */}
      {taskId && <UsageStrip taskId={taskId} />}

      {/* 按 scenario 渲染结构化面板（在 markdown 前）*/}
      <ScenarioPanel report={report} />

      <div className="prose-pmhive">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{report.markdown}</ReactMarkdown>
      </div>

      {report.sources && report.sources.length > 0 && (
        <div className="mt-8 pt-5 border-t border-border">
          <h3 className="text-xs text-muted2 uppercase tracking-wider mb-3 font-semibold">Sources</h3>
          <ol className="space-y-1.5 text-sm">
            {report.sources.map((s, i) => (
              <li key={i} className="flex gap-2 items-start">
                <span className="text-muted2 mono tabular-nums shrink-0">[{i + 1}]</span>
                <a
                  href={s.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-link hover:underline truncate"
                  title={s.snippet}
                >
                  {s.title || s.url}
                </a>
              </li>
            ))}
          </ol>
        </div>
      )}

      {/* 追问框 — 仅当任务完成时显示 */}
      {taskId && taskStatus === "succeeded" && (
        <FollowUpBox taskId={taskId} onSubmit={onFollowUp} />
      )}
    </div>
  );
}

// ScenarioPanel 按 scenario 类型渲染专属结构化面板（在 markdown 报告之前）
function ScenarioPanel({ report }: { report: Report }) {
  const meta = (report.metadata ?? {}) as any;
  const scenario = meta.scenario as string | undefined;

  if (scenario === "requirement_analysis" && Array.isArray(meta.requirements) && meta.requirements.length > 0) {
    return <RequirementTable requirements={meta.requirements} />;
  }
  if (scenario === "requirement_validation") {
    return <ValidationPanel hypotheses={meta.hypotheses ?? []} validations={meta.validations ?? []} risks={meta.risks ?? []} />;
  }
  return null;
}

function RequirementTable({ requirements }: { requirements: any[] }) {
  // 按 RICE 排序
  const sorted = [...requirements].sort((a, b) => (b.rice_score ?? 0) - (a.rice_score ?? 0));
  const kanoColor: Record<string, string> = {
    basic: "bg-rose-100 text-rose-700",
    performance: "bg-blue-100 text-blue-700",
    excitement: "bg-amber-100 text-amber-700",
    indifferent: "bg-zinc-100 text-zinc-600",
  };
  return (
    <div className="mb-6">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-xs font-semibold text-ink uppercase tracking-wider">需求清单 · 按 RICE 排序</h3>
        <span className="text-xs text-muted2 mono">{sorted.length} 条</span>
      </div>
      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-xs">
          <thead className="bg-bg2 text-muted2 uppercase tracking-wider">
            <tr>
              <th className="text-left px-3 py-2 font-semibold w-10">ID</th>
              <th className="text-left px-3 py-2 font-semibold">需求</th>
              <th className="text-right px-3 py-2 font-semibold w-16">RICE</th>
              <th className="text-center px-3 py-2 font-semibold w-20">Kano</th>
              <th className="text-center px-3 py-2 font-semibold w-12">来源</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((r, i) => {
              const score = (r.rice_score ?? 0).toFixed(1);
              const isP0 = (r.rice_score ?? 0) >= 60;
              const isP1 = (r.rice_score ?? 0) >= 30 && (r.rice_score ?? 0) < 60;
              return (
                <tr key={r.id ?? i} className="border-t border-border hover:bg-bg2/40">
                  <td className="px-3 py-2 mono text-muted2">{r.id ?? `R${i+1}`}</td>
                  <td className="px-3 py-2">
                    <div className="text-ink leading-snug">{r.title}</div>
                    {r.jtbd && <div className="text-[10px] text-muted2 mt-0.5 italic">{r.jtbd}</div>}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    <span className={`mono font-semibold ${isP0 ? "text-success" : isP1 ? "text-warn" : "text-muted2"}`}>
                      {score}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-center">
                    {r.kano_type && (
                      <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-medium ${kanoColor[r.kano_type] ?? "bg-zinc-100"}`}>
                        {r.kano_type}
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-2 text-center text-[10px] text-muted2 mono">
                    {r.source === "user_voice" ? "🎙" : r.source === "market_gap" ? "📊" : "💡"}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ValidationPanel({ hypotheses, validations, risks }: { hypotheses: any[]; validations: any[]; risks: any[] }) {
  const verdictColor: Record<string, string> = {
    confirmed: "bg-success/10 text-success",
    refuted: "bg-danger/10 text-danger",
    inconclusive: "bg-warn/10 text-warn",
  };
  const sevColor: Record<string, string> = {
    high: "text-danger",
    medium: "text-warn",
    low: "text-muted2",
  };
  // 按 hypothesis 聚合 validations
  const validationsByHypo: Record<string, any[]> = {};
  for (const v of validations) {
    if (!validationsByHypo[v.hypothesis_id]) validationsByHypo[v.hypothesis_id] = [];
    validationsByHypo[v.hypothesis_id].push(v);
  }

  return (
    <div className="mb-6 space-y-4">
      {/* Hypotheses */}
      {hypotheses.length > 0 && (
        <div>
          <h3 className="text-xs font-semibold text-ink uppercase tracking-wider mb-2">假设清单 · {hypotheses.length} 条</h3>
          <div className="space-y-2">
            {hypotheses.map((h: any) => (
              <div key={h.id} className="rounded-lg border border-border p-3 bg-white">
                <div className="flex items-center gap-2 mb-1">
                  <span className="mono text-[10px] text-muted2">{h.id}</span>
                  <span className="ios-chip text-[10px]">{h.type}</span>
                  <span className="ml-auto text-[10px] text-muted2">可信度 <span className="mono text-ink">{(h.confidence ?? 0).toFixed(2)}</span></span>
                </div>
                <p className="text-xs text-ink leading-relaxed">{h.statement}</p>
                {/* validations under this hypothesis */}
                {validationsByHypo[h.id] && (
                  <div className="mt-2 pt-2 border-t border-border space-y-1.5">
                    {validationsByHypo[h.id].map((v: any, vi: number) => (
                      <div key={vi} className="flex items-start gap-2 text-[11px]">
                        <span className={`px-1.5 py-0.5 rounded font-semibold uppercase tracking-wider text-[9px] ${verdictColor[v.verdict] ?? "bg-zinc-100"}`}>
                          {v.verdict}
                        </span>
                        <span className="mono text-muted2 shrink-0">{v.method}</span>
                        <span className="text-muted">{v.evidence}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Risks */}
      {risks.length > 0 && (
        <div>
          <h3 className="text-xs font-semibold text-ink uppercase tracking-wider mb-2">验证盲点 / 风险 · {risks.length} 条</h3>
          <div className="rounded-lg border border-border overflow-hidden">
            <table className="w-full text-xs">
              <thead className="bg-bg2 text-muted2 uppercase tracking-wider">
                <tr>
                  <th className="text-left px-3 py-2 font-semibold w-16">严重度</th>
                  <th className="text-left px-3 py-2 font-semibold">风险</th>
                  <th className="text-left px-3 py-2 font-semibold">缓解</th>
                </tr>
              </thead>
              <tbody>
                {risks.map((r: any, i: number) => (
                  <tr key={i} className="border-t border-border">
                    <td className={`px-3 py-2 font-semibold uppercase tracking-wider text-[10px] ${sevColor[r.severity] ?? "text-muted"}`}>
                      {r.severity}
                    </td>
                    <td className="px-3 py-2 text-ink">{r.risk}</td>
                    <td className="px-3 py-2 text-muted">{r.mitigation}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

function UsageStrip({ taskId }: { taskId: string }) {
  const [usage, setUsage] = useState<TaskUsage | null>(null);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    let live = true;
    const load = () => {
      getUsage(taskId).then((u) => { if (live) setUsage(u); }).catch(() => {});
    };
    load();
    // 任务进行中时轮询一次（约 5s）
    const id = window.setInterval(load, 5000);
    return () => { live = false; window.clearInterval(id); };
  }, [taskId]);

  if (!usage) return null;

  const cost = usage.cost_usd ?? 0;
  const costStr = cost < 0.01 ? `$${cost.toFixed(4)}` : `$${cost.toFixed(2)}`;
  const tokensStr = usage.total_tokens >= 1000
    ? `${(usage.total_tokens / 1000).toFixed(1)}k`
    : `${usage.total_tokens}`;
  const exceeded = !!usage.budget_exceeded;
  const agentRows = Object.entries(usage.by_agent ?? {})
    .sort((a, b) => b[1].cost_usd - a[1].cost_usd);

  return (
    <div className={`mb-4 rounded-lg border ${exceeded ? "border-danger/40 bg-danger/5" : "border-border bg-bg2/40"}`}>
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center gap-3 px-3 py-2 text-xs hover:bg-bg2/60 transition rounded-lg"
      >
        <span className="text-[10px] text-muted2 uppercase tracking-wider font-semibold">LLM 用量</span>
        <span className="mono font-semibold text-ink">{costStr}</span>
        <span className="text-muted2 mono">·</span>
        <span className="mono text-muted">{usage.calls} 次调用</span>
        <span className="text-muted2 mono">·</span>
        <span className="mono text-muted">{tokensStr} tokens</span>
        {usage.budget_usd && usage.budget_usd > 0 && (
          <>
            <span className="text-muted2 mono">·</span>
            <span className={`mono ${exceeded ? "text-danger font-semibold" : "text-muted2"}`}>
              budget ${usage.budget_usd.toFixed(2)}{exceeded ? " ⚠" : ""}
            </span>
          </>
        )}
        <span className="ml-auto text-placeholder text-[14px] leading-none">{open ? "−" : "+"}</span>
      </button>
      {open && agentRows.length > 0 && (
        <div className="px-3 pb-2 pt-1 border-t border-border">
          <table className="w-full text-[11px] mono">
            <thead className="text-muted2 uppercase tracking-wider">
              <tr>
                <th className="text-left py-1.5 font-semibold">Agent</th>
                <th className="text-right py-1.5 font-semibold">Calls</th>
                <th className="text-right py-1.5 font-semibold">Tokens</th>
                <th className="text-right py-1.5 font-semibold">Cost</th>
              </tr>
            </thead>
            <tbody>
              {agentRows.map(([name, u]) => (
                <tr key={name} className="border-t border-border">
                  <td className="py-1 text-ink">{name}</td>
                  <td className="py-1 text-right text-muted">{u.calls}</td>
                  <td className="py-1 text-right text-muted tabular-nums">{u.total_tokens}</td>
                  <td className="py-1 text-right text-ink tabular-nums">${u.cost_usd.toFixed(4)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function ReviewCard({ review }: { review: ReviewResult }) {
  const verdictColor: Record<string, string> = {
    accept: "text-success",
    revise: "text-warn",
    reject: "text-danger",
  };
  const verdictBg: Record<string, string> = {
    accept: "bg-success/10",
    revise: "bg-warn/10",
    reject: "bg-danger/10",
  };
  const score = review.overall_score;
  const scoreColor =
    score >= 8 ? "text-success" : score >= 6 ? "text-warn" : "text-danger";

  return (
    <div className="mb-6 p-4 rounded-lg border border-border bg-bg2/50">
      <div className="flex items-baseline gap-3">
        <span className="text-xs text-muted2 uppercase tracking-wider font-semibold">Reviewer</span>
        <span className={`text-2xl font-semibold tabular-nums ${scoreColor}`}>
          {score.toFixed(1)}
        </span>
        <span className="text-xs text-muted2 mono">/ 10</span>
        <span
          className={`ml-auto px-2 py-0.5 text-xs font-semibold uppercase tracking-wider rounded ${verdictBg[review.verdict] ?? "bg-bg2"} ${verdictColor[review.verdict] ?? "text-muted"}`}
        >
          {review.verdict}
        </span>
        {review.iteration > 1 && (
          <span className="ios-chip mono">iter {review.iteration}</span>
        )}
      </div>

      <div className="mt-3 grid grid-cols-3 gap-3 text-xs">
        <ScoreBar label="Fact" value={review.fact_score} />
        <ScoreBar label="Coverage" value={review.coverage_score} />
        <ScoreBar label="Citation" value={review.citation_score} />
      </div>

      {(review.strengths?.length > 0 || review.issues?.length > 0) && (
        <div className="mt-3 grid grid-cols-2 gap-3 text-xs">
          {review.strengths?.length > 0 && (
            <div>
              <div className="text-muted2 mb-1 uppercase tracking-wider text-[10px] font-semibold">Strengths</div>
              <ul className="space-y-0.5 text-ink2">
                {review.strengths.map((s, i) => (
                  <li key={i}>+ {s}</li>
                ))}
              </ul>
            </div>
          )}
          {review.issues?.length > 0 && (
            <div>
              <div className="text-muted2 mb-1 uppercase tracking-wider text-[10px] font-semibold">Issues</div>
              <ul className="space-y-0.5 text-ink2">
                {review.issues.map((s, i) => (
                  <li key={i}>− {s}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ScoreBar({ label, value }: { label: string; value: number }) {
  const pct = Math.max(0, Math.min(100, (value / 10) * 100));
  const color =
    value >= 8 ? "bg-success" : value >= 6 ? "bg-warn" : "bg-danger";
  return (
    <div>
      <div className="flex items-baseline justify-between mb-1">
        <span className="text-muted2 mono">{label}</span>
        <span className="mono tabular-nums text-ink">{value.toFixed(1)}</span>
      </div>
      <div className="h-1.5 bg-bg2 rounded-full overflow-hidden">
        <div className={`h-full ${color} rounded-full transition-all`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

function FollowUpBox({ taskId, onSubmit }: { taskId: string; onSubmit?: (id: string) => void }) {
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    if (!input.trim() || busy) return;
    setBusy(true);
    setErr(null);
    try {
      const t = await followUp(taskId, input.trim());
      setInput("");
      onSubmit?.(t.id);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-8 pt-5 border-t border-border">
      <h3 className="text-xs text-muted2 uppercase tracking-wider mb-3 font-semibold">追问 / Follow-up</h3>
      <div className="flex gap-2">
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="例如：再加一家国内竞品 Lark；或聚焦定价对比"
          className="ios-input flex-1 text-xs"
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit();
          }}
        />
        <button
          disabled={busy || !input.trim()}
          onClick={submit}
          className="ios-btn ios-btn-primary px-4 text-xs disabled:opacity-40"
        >
          {busy ? "submitting…" : "Ask"}
        </button>
      </div>
      <p className="text-[11px] text-muted2 mt-1.5 mono">⌘+Enter · 创建子任务，复用同 scenario</p>
      {err && <div className="text-xs text-danger mt-2">{err}</div>}
    </div>
  );
}

function downloadMarkdown(r: Report) {
  const blob = new Blob([r.markdown], { type: "text/markdown" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = (r.title || "report").replace(/[/\\?%*:|"<>]/g, "_") + ".md";
  a.click();
  URL.revokeObjectURL(url);
}
