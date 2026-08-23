import { useEffect, useState } from "react";
import { AuthProvider, useAuth } from "./lib/auth";
import * as api from "./lib/api";
import type { Config } from "./lib/api";
import { LoginPage } from "./components/LoginPage";
import { TopBar } from "./components/TopBar";
import { Tree } from "./components/Tree";
import { SecretPanel } from "./components/SecretPanel";
import { AuditLogPage } from "./components/AuditLogPage";
import { SettingsPage } from "./components/SettingsPage";
import { GraphView } from "./components/GraphView";
import { TagsPage } from "./components/TagsPage";
import { SearchPage } from "./components/SearchPage";

type View = "vault" | "graph" | "tags" | "search" | "audit" | "settings";

function Shell() {
  const [view, setView] = useState<View>("vault");
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [config, setConfig] = useState<Config | null>(null);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try {
      return localStorage.getItem("vv_sidebar_collapsed") === "1";
    } catch {
      return false;
    }
  });

  useEffect(() => {
    api.getConfig().then(setConfig).catch(() => setConfig(null));
  }, []);

  useEffect(() => {
    try {
      localStorage.setItem("vv_sidebar_collapsed", sidebarCollapsed ? "1" : "0");
    } catch {
      // localStorage unavailable (private mode, etc.) — collapse state just won't persist.
    }
  }, [sidebarCollapsed]);

  return (
    <div className="min-h-screen flex flex-col">
      <TopBar config={config} view={view} onNavigateView={setView} />
      {view === "vault" && (
        <div className="grid grow" style={{ gridTemplateColumns: sidebarCollapsed ? "36px 1fr" : "264px 1fr" }}>
          {sidebarCollapsed ? (
            <div className="border-r border-[var(--color-divider)] flex flex-col items-center pt-4">
              <button
                className="btn btn-ghost"
                style={{ fontSize: 14, lineHeight: 1, padding: "4px 6px" }}
                title="사이드바 펼치기"
                onClick={() => setSidebarCollapsed(false)}
              >
                ▸
              </button>
            </div>
          ) : (
            <Tree
              selectedPath={selectedPath}
              onSelect={setSelectedPath}
              onCollapse={() => setSidebarCollapsed(true)}
            />
          )}
          <SecretPanel selectedPath={selectedPath} onNavigate={setSelectedPath} />
        </div>
      )}
      {view === "graph" && (
        <GraphView
          onNavigate={(path) => {
            setSelectedPath(path);
            setView("vault");
          }}
        />
      )}
      {view === "tags" && (
        <TagsPage
          onNavigate={(path) => {
            setSelectedPath(path);
            setView("vault");
          }}
        />
      )}
      {view === "search" && (
        <SearchPage
          onNavigate={(path) => {
            setSelectedPath(path);
            setView("vault");
          }}
        />
      )}
      {view === "audit" && <AuditLogPage />}
      {view === "settings" && <SettingsPage config={config} />}
    </div>
  );
}

function Root() {
  const { session, loading } = useAuth();
  if (loading) return null;
  if (!session) return <LoginPage />;
  return <Shell />;
}

export default function App() {
  return (
    <AuthProvider>
      <Root />
    </AuthProvider>
  );
}
