// AgentRoster — 角色花名册：当前 scenario 下出场的所有专家卡
import { useEffect, useState } from "react";
import { AgentEvent, RoleMeta, listRolesByScenario } from "../lib/api";

interface Props {
  scenario: string;
  events: AgentEvent[]; // 用 trace 推断每个角色当前状态
}

type RoleStatus = "idle" | "running" | "done" | "error";

function statusOf(roleKey: string, events: AgentEvent[]): RoleStatus {
  let status: RoleStatus = "idle";
  for (const ev of events) {
    if (ev.agent !== roleKey) continue;
    if (ev.step === "start") status = "running";
    if (ev.step === "done") status = "done";
    if (ev.step === "error") status = "error";
  }
  return status;
}

export function AgentRoster({ scenario, events }: Props) {
  const [roles, setRoles] = useState<RoleMeta[]>([]);

  useEffect(() => {
    if (!scenario) return;
    listRolesByScenario(scenario).then(setRoles).catch(() => {});
  }, [scenario]);

  if (roles.length === 0) {
    return null;
  }

  return (
    <div className="ios-card p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-xs uppercase tracking-wider font-semibold text-ink">Agent 集群 · 专家阵容</h3>
        <span className="text-[11px] text-muted2 mono">{roles.length} 角色</span>
      </div>
      <div className="grid grid-cols-2 gap-2">
        {roles.map((r) => {
          const st = statusOf(r.key, events);
          const dotColor =
            st === "running" ? "dot-running" :
            st === "done" ? "dot-success" :
            st === "error" ? "dot-failed" : "dot-queued";
          return (
            <div
              key={r.key}
              className={`rounded-lg p-2.5 border transition
                ${st === "running" ? "border-warn bg-warn/5"
                  : st === "done" ? "border-success/30"
                  : st === "error" ? "border-danger/40 bg-danger/5"
                  : "border-border bg-white"}`}
              title={r.specialty}
            >
              <div className="flex items-start gap-2">
                <div className="text-2xl leading-none">{r.avatar}</div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-1.5">
                    <span className={`dot ${dotColor}`} />
                    <span className="text-xs font-semibold text-ink truncate">{r.title}</span>
                  </div>
                  <div className="text-[10px] text-muted2 mono uppercase tracking-wider truncate">{r.title_en}</div>
                  <div className="text-[11px] text-muted leading-snug mt-1 line-clamp-2">{r.specialty}</div>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
