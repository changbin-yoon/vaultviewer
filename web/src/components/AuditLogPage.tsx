import { useEffect, useState } from "react";
import * as api from "../lib/api";
import type { AuditLog } from "../lib/api";
import { EmptyState } from "./EmptyState";
import { stripMdExtension } from "../lib/markdown";

const ACTION_CLASS: Record<string, string> = {
  create: "tag tag-neutral",
  update: "tag tag-accent",
  delete: "tag tag-neutral",
  rename: "tag tag-accent",
};

const RECONNECT_MS = 3000;

export function AuditLogPage() {
  const [entries, setEntries] = useState<AuditLog[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [live, setLive] = useState(false);

  async function refresh() {
    try {
      const data = await api.getAudit();
      setEntries(data ?? []);
      setError(null);
    } catch {
      setError("감사 로그를 불러오지 못했습니다.");
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  // Live updates over WebSocket rather than polling: the initial list above
  // comes from REST, and every entry recorded after that arrives here as
  // it happens. Reconnects on drop (idle-timeout proxies, network blips)
  // with a fixed retry delay — simple, and fine at this app's scale.
  useEffect(() => {
    let cancelled = false;
    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    function connect() {
      if (cancelled) return;
      ws = new WebSocket(api.auditStreamUrl());
      ws.onopen = () => setLive(true);
      ws.onmessage = (event) => {
        try {
          const entry = JSON.parse(event.data) as AuditLog;
          setEntries((prev) => [entry, ...(prev ?? [])]);
        } catch {
          // Ignore malformed frames rather than tearing down the connection.
        }
      };
      ws.onclose = () => {
        setLive(false);
        if (!cancelled) reconnectTimer = setTimeout(connect, RECONNECT_MS);
      };
      ws.onerror = () => ws?.close();
    }
    connect();

    return () => {
      cancelled = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      ws?.close();
    };
  }, []);

  return (
    <main className="p-6">
      <div className="flex items-center gap-4 mb-1">
        <h4>감사 로그</h4>
        <span className="flex items-center gap-1.5 text-xs text-muted">
          <span
            className="inline-block rounded-full"
            style={{ width: 6, height: 6, background: live ? "var(--color-good)" : "var(--color-divider)" }}
          />
          {live ? "실시간 연결됨" : "연결 중…"}
        </span>
        <button className="btn btn-secondary ml-auto" onClick={refresh}>
          새로고침
        </button>
      </div>
      <p className="text-xs text-muted mb-5">이벤트 {entries?.length ?? 0}건</p>

      {error && (
        <div className="mb-4">
          <EmptyState title={error} />
        </div>
      )}

      {entries && entries.length === 0 && !error && (
        <EmptyState title="기록된 변경 이력이 없습니다" description="시크릿을 생성, 수정, 삭제하면 여기에 표시됩니다." />
      )}

      {entries && entries.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th style={{ width: "16%" }}>시각</th>
              <th style={{ width: "14%" }}>사용자</th>
              <th style={{ width: "10%" }}>작업</th>
              <th style={{ width: "36%" }}>대상</th>
              <th style={{ width: "24%" }}>사유</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e, i) => (
              <tr key={`${e.path}-${e.timestamp}-${i}`}>
                <td className="mono text-muted">{new Date(e.timestamp).toLocaleString()}</td>
                <td className="mono">{e.user}</td>
                <td>
                  <span className={ACTION_CLASS[e.action] ?? "tag tag-neutral"}>{e.action.toUpperCase()}</span>
                </td>
                <td className="mono">
                  {e.previousPath && <span className="text-muted">{stripMdExtension(e.previousPath)} → </span>}
                  /{stripMdExtension(e.path)}
                </td>
                <td className="text-muted">{e.reason || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
