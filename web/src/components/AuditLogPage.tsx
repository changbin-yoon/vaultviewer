import { useEffect, useState } from "react";
import * as api from "../lib/api";
import type { AuditLog } from "../lib/api";
import { EmptyState } from "./EmptyState";
import { stripMdExtension } from "../lib/markdown";

const ACTION_CLASS: Record<string, string> = {
  create: "tag tag-neutral",
  update: "tag tag-accent",
  delete: "tag tag-neutral",
};

const POLL_MS = 8000;

export function AuditLogPage() {
  const [entries, setEntries] = useState<AuditLog[] | null>(null);
  const [error, setError] = useState<string | null>(null);

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
    const id = setInterval(refresh, POLL_MS);
    return () => clearInterval(id);
  }, []);

  return (
    <main className="p-6">
      <div className="flex items-center gap-4 mb-1">
        <h4>감사 로그</h4>
        <span className="text-xs text-muted">{POLL_MS / 1000}초마다 자동 새로고침</span>
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
                <td className="mono">/{stripMdExtension(e.path)}</td>
                <td className="text-muted">{e.reason || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
