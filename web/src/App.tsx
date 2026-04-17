import { useEffect, useState } from "react";
import { BrowserRouter, Link, Navigate, NavLink, Route, Routes } from "react-router-dom";
import { IntegrationStatus, MODULES, getIntegrationStatus } from "./lib/api";
import { Dashboard } from "./pages/Dashboard";
import { CompetitorPage, RequirementPage, ValidationPage } from "./pages/ModulePage";

export function App() {
  const [integrations, setIntegrations] = useState<IntegrationStatus>({ slack: false, jira: false });

  useEffect(() => {
    getIntegrationStatus().then(setIntegrations).catch(() => {});
  }, []);

  return (
    <BrowserRouter>
      <div className="h-full flex flex-col bg-bg">
        {/* 顶 nav */}
        <header className="h-12 px-5 flex items-center bg-white border-b border-border shrink-0 gap-4">
          <Link to="/" className="font-semibold text-ink text-sm tracking-tight hover:opacity-70">
            PM Agent Team
          </Link>
          <span className="ios-chip mono">v0.5</span>

          <nav className="flex items-center gap-1 ml-4">
            <NavTab to="/" exact label="Dashboard" />
            {MODULES.map((m) => (
              <NavTab key={m.id} to={`/${m.id}`} label={`${m.emoji} ${m.label}`} accent={m.accent} />
            ))}
          </nav>

          <span className="ml-auto flex items-center gap-2 text-xs text-muted2">
            <span className={`ios-chip ${integrations.slack ? "text-success" : ""}`}>
              {integrations.slack ? "● Slack" : "○ Slack"}
            </span>
            <span className={`ios-chip ${integrations.jira ? "text-success" : ""}`}>
              {integrations.jira ? "● Jira" : "○ Jira"}
            </span>
          </span>
        </header>

        {/* Routes */}
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/requirement" element={<RequirementPage />} />
          <Route path="/requirement/:taskId" element={<RequirementPage />} />
          <Route path="/competitor" element={<CompetitorPage />} />
          <Route path="/competitor/:taskId" element={<CompetitorPage />} />
          <Route path="/validation" element={<ValidationPage />} />
          <Route path="/validation/:taskId" element={<ValidationPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </div>
    </BrowserRouter>
  );
}

function NavTab({ to, label, accent, exact }: { to: string; label: string; accent?: string; exact?: boolean }) {
  return (
    <NavLink
      to={to}
      end={exact}
      className={({ isActive }) =>
        `text-xs font-medium px-2.5 py-1 rounded transition tracking-tight
         ${isActive ? "bg-ink text-white" : "text-muted hover:text-ink hover:bg-bg2"}`
      }
      style={({ isActive }) =>
        isActive && accent ? { borderBottom: `2px solid ${accent}` } : undefined
      }
    >
      {label}
    </NavLink>
  );
}
