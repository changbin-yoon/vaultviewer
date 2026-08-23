import { useAuth } from "../lib/auth";
import { RoleTag } from "./RoleTag";
import type { Config } from "../lib/api";

type View = "vault" | "graph" | "tags" | "search" | "audit" | "guide" | "settings";

const NAV: { key: View; label: string; adminOnly?: boolean }[] = [
  { key: "vault", label: "문서" },
  { key: "graph", label: "그래프" },
  { key: "guide", label: "작성가이드" },
  { key: "tags", label: "태그" },
  { key: "search", label: "검색" },
  { key: "audit", label: "감사 로그", adminOnly: true },
  { key: "settings", label: "설정", adminOnly: true },
];

export function TopBar({
  config,
  view,
  onNavigateView,
}: {
  config: Config | null;
  view: View;
  onNavigateView: (v: View) => void;
}) {
  const { session, logout } = useAuth();
  if (!session) return null;
  const isAdmin = session.role === "adm";

  return (
    <div className="flex items-center gap-4.5 px-5.5 py-3 border-b border-[var(--color-divider)] overflow-x-auto whitespace-nowrap">
      <button
        className="font-[var(--font-heading)] font-semibold text-[19px] tracking-[.02em]"
        onClick={() => onNavigateView("vault")}
      >
        VAULT<span className="text-[var(--color-accent)]">VIEWER</span>
      </button>
      {config && <span className="tag tag-outline mono tracking-[.08em]">{config.deployment}</span>}

      <nav className="flex gap-1 text-sm">
        {NAV.filter((item) => !item.adminOnly || isAdmin).map(({ key, label }) => (
          <button
            key={key}
            onClick={() => onNavigateView(key)}
            className="px-3 py-1.5"
            style={
              view === key
                ? { background: "var(--color-accent)", color: "var(--color-bg)" }
                : { color: "var(--color-text)" }
            }
          >
            {label}
          </button>
        ))}
      </nav>

      <div className="ml-auto flex items-center gap-2.5">
        <span className="text-[11px] text-muted">역할</span>
        <RoleTag role={session.role} />
        {session.department && <span className="text-sm text-muted">{session.department}</span>}
        <span className="mono text-sm">{session.username}</span>
        <button className="btn btn-secondary" onClick={logout}>
          로그아웃
        </button>
      </div>
    </div>
  );
}
