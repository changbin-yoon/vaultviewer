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
import { GuidePage } from "./components/GuidePage";
import { DashboardPage } from "./components/DashboardPage";
import { ArchPage } from "./components/ArchPage";

type View = "dashboard" | "vault" | "graph" | "tags" | "search" | "audit" | "guide" | "settings" | "arch";

function Shell() {
  const { session } = useAuth();
  const [view, setView] = useState<View>("dashboard");
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [config, setConfig] = useState<Config | null>(null);
  // Bumped whenever a note/namespace is created or deleted, so the sidebar
  // tree (which caches what it's fetched) knows to refresh — see Tree's
  // refreshSignal prop.
  const [vaultVersion, setVaultVersion] = useState(0);
  const bumpVaultVersion = () => setVaultVersion((v) => v + 1);
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

  // Root only mounts Shell once a session exists, but useAuth()'s return
  // type is still nullable here — narrow it so DashboardPage below can take
  // a non-null session prop instead of re-checking everywhere.
  if (!session) return null;

  return (
    <div className="min-h-screen flex flex-col">
      <TopBar config={config} view={view} onNavigateView={setView} />
      {view === "dashboard" && <DashboardPage config={config} session={session} onNavigateView={setView} />}
      {view === "arch" && <ArchPage />}
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
              refreshSignal={vaultVersion}
            />
          )}
          <SecretPanel selectedPath={selectedPath} onNavigate={setSelectedPath} onMutate={bumpVaultVersion} />
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
      {view === "guide" && <GuidePage />}
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
