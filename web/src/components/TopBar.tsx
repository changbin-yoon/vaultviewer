import { useAuth } from "../lib/auth";
import { RoleTag } from "./RoleTag";
import type { Config } from "../lib/api";

function initials(username: string) {
  return username.slice(0, 2).toUpperCase();
}

export function TopBar({
  config,
  view,
  onNavigateView,
}: {
  config: Config | null;
  view: "vault" | "graph" | "tags" | "audit" | "settings";
  onNavigateView: (v: "vault" | "graph" | "tags" | "audit" | "settings") => void;
}) {
  const { session, logout } = useAuth();
  if (!session) return null;

  return (
    <div className="flex items-center gap-4.5 px-5.5 py-3 border-b border-[var(--color-divider)]">
      <div className="font-[var(--font-heading)] font-semibold text-[19px] tracking-[.02em]">
        VAULT<span className="text-[var(--color-accent)]">VIEWER</span>
      </div>
      {config && <span className="tag tag-outline mono tracking-[.08em]">{config.deployment}</span>}

      <nav className="flex gap-1 text-sm">
        {(
          [
            ["vault", "볼트"],
            ["graph", "그래프"],
            ["tags", "태그"],
            ["audit", "감사 로그"],
            ["settings", "설정"],
          ] as const
        ).map(([key, label]) => (
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
        <span className="mono text-sm">{session.username}</span>
        <button
          className="w-7 h-7 border border-[var(--color-divider)] grid place-items-center text-[11px]"
          title="로그아웃"
          onClick={logout}
        >
          {initials(session.username)}
        </button>
      </div>
    </div>
  );
}
