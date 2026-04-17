import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { Report, ReviewResult } from "../lib/api";
import { followUp } from "../lib/api";

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
