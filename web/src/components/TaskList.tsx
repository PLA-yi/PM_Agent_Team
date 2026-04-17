import type { Task } from "../lib/api";

interface Props {
  tasks: Task[];
  selectedId?: string;
  onSelect: (id: string) => void;
}

const STATUS_DOT: Record<string, string> = {
  queued: "dot-queued",
  running: "dot-running",
  succeeded: "dot-success",
  failed: "dot-failed",
  cancelled: "dot-queued",
};

const STATUS_LABEL: Record<string, string> = {
  queued: "Queued",
  running: "Running",
  succeeded: "Done",
  failed: "Failed",
  cancelled: "Cancelled",
};

const SCENARIO_TAG: Record<string, string> = {
  competitor_research: "竞品",
  interview_analysis: "访谈",
  prd_drafting: "PRD",
  social_listening: "社聆",
};

export function TaskList({ tasks, selectedId, onSelect }: Props) {
  if (tasks.length === 0) {
    return (
      <div className="text-muted2 text-xs p-6 text-center leading-6">
        No tasks yet
      </div>
    );
  }
  return (
    <ul>
      {tasks.map((t, idx) => {
        const active = t.id === selectedId;
        return (
          <li key={t.id}>
            <button
              onClick={() => onSelect(t.id)}
              className={`w-full text-left px-4 py-3 transition flex flex-col gap-1.5 border-l-2
                ${active ? "bg-bg2 border-l-ink" : "border-l-transparent hover:bg-bg2/60"}`}
            >
              <div className="flex items-center gap-2 text-xs">
                <span className="ios-chip">{SCENARIO_TAG[t.scenario] ?? "—"}</span>
                {t.parent_task_id && (
                  <span className="ios-chip mono text-[10px]">追问</span>
                )}
                <span className={`dot ${STATUS_DOT[t.status] ?? "dot-queued"}`} />
                <span className="text-muted2">{STATUS_LABEL[t.status] ?? t.status}</span>
                <span className="ml-auto text-muted2 mono tabular-nums">{t.progress}%</span>
              </div>
              <div className="text-sm text-ink line-clamp-2 leading-snug">
                {t.parent_task_id && <span className="text-muted2">↳ </span>}
                {t.input}
              </div>
              <div className="text-xs text-muted2 mono">
                {t.stage || "—"} · {new Date(t.created_at).toLocaleTimeString("en-US", { hour12: false, hour: '2-digit', minute: '2-digit' })}
              </div>
            </button>
            {idx < tasks.length - 1 && <div className="ios-divider" />}
          </li>
        );
      })}
    </ul>
  );
}
