import { useEffect, useState } from "react";
import * as api from "../lib/api";
import type { Config } from "../lib/api";
import { useAuth } from "../lib/auth";
import { RoleTag } from "./RoleTag";

interface Row {
  key: string;
  value: string;
}

function GroupTeamMapping({ config }: { config: Config | null }) {
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    api
      .getGroupTeams()
      .then((m) => setRows(Object.entries(m).map(([key, value]) => ({ key, value }))))
      .catch(() => setError("불러오지 못했습니다."))
      .finally(() => setLoading(false));
  }, []);

  function updateRow(i: number, field: keyof Row, v: string) {
    setRows((rs) => rs.map((r, idx) => (idx === i ? { ...r, [field]: v } : r)));
    setSaved(false);
  }

  function removeRow(i: number) {
    setRows((rs) => rs.filter((_, idx) => idx !== i));
    setSaved(false);
  }

  function addRow() {
    setRows((rs) => [...rs, { key: "", value: "" }]);
  }

  async function save() {
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      const mapping: Record<string, string> = {};
      for (const r of rows) {
        const key = r.key.trim();
        const value = r.value.trim();
        if (key && value) mapping[key] = value;
      }
      await api.saveGroupTeams(mapping);
      setRows(Object.entries(mapping).map(([key, value]) => ({ key, value })));
      setSaved(true);
    } catch {
      setError("저장에 실패했습니다.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div>
      <h5 className="text-[12px] tracking-[.08em] uppercase text-muted mb-2.5">그룹별 팀 매핑</h5>
      <p className="text-sm text-muted mb-3">
        LDAP 그룹 CN을 화면에 보여줄 팀 이름으로 매핑합니다. 로그인 시 사용자가 속한 그룹 중 매핑이 있는
        첫 번째 그룹의 이름이 상단 "소속"으로 표시되고, 매핑이 없으면 LDAP의 o 속성 값을 그대로 씁니다.
        {config?.mode === "cluster" && (
          <> 클러스터 모드에는 파일로 저장할 영구 볼륨이 없어 <strong>파드가 재시작되면 초기화</strong>됩니다.</>
        )}
      </p>
      {loading ? (
        <p className="text-sm text-muted">불러오는 중…</p>
      ) : (
        <>
          <div className="flex flex-col gap-2 mb-3">
            {rows.map((r, i) => (
              <div key={i} className="flex gap-2 items-center">
                <input
                  className="input mono"
                  style={{ flex: 1 }}
                  placeholder="LDAP 그룹 CN (예: dt-bi-adm)"
                  value={r.key}
                  onChange={(e) => updateRow(i, "key", e.target.value)}
                />
                <input
                  className="input"
                  style={{ flex: 1 }}
                  placeholder="팀 이름"
                  value={r.value}
                  onChange={(e) => updateRow(i, "value", e.target.value)}
                />
                <button className="btn btn-secondary" onClick={() => removeRow(i)}>
                  삭제
                </button>
              </div>
            ))}
            {rows.length === 0 && <p className="text-sm text-muted">등록된 매핑이 없습니다.</p>}
          </div>
          {error && (
            <p className="text-sm mb-3" style={{ color: "var(--color-danger)" }}>
              {error}
            </p>
          )}
          <div className="flex gap-2 items-center">
            <button className="btn btn-secondary" onClick={addRow}>
              행 추가
            </button>
            <button className="btn btn-primary" disabled={saving} onClick={save}>
              저장
            </button>
            {saved && <span className="text-sm text-muted">저장됨</span>}
          </div>
        </>
      )}
    </div>
  );
}

export function SettingsPage({ config }: { config: Config | null }) {
  const { session } = useAuth();

  return (
    <main className="p-6 max-w-2xl">
      <h4 className="mb-1">설정</h4>
      <p className="text-sm text-muted mb-8">
        실행 모드와 스토리지 백엔드는 서버 배포 시 환경 변수로 결정되며, 이 화면에서 즉시 전환할 수 없습니다.
      </p>

      <div className="flex flex-col gap-6">
        <div>
          <h5 className="text-[12px] tracking-[.08em] uppercase text-muted mb-2.5">배포 환경</h5>
          <span className="tag tag-outline mono tracking-[.08em]">{config?.deployment ?? "—"}</span>
        </div>

        <div>
          <h5 className="text-[12px] tracking-[.08em] uppercase text-muted mb-2.5">스토리지 모드</h5>
          <div className="flex items-center gap-2">
            <span className="tag tag-outline mono tracking-[.08em]">{config?.mode ?? "—"}</span>
            <span className="text-sm text-muted">백엔드 · {config?.backend ?? "—"}</span>
          </div>
          {config?.root && <div className="mono text-xs text-muted mt-2">경로 · {config.root}</div>}
        </div>

        <div>
          <h5 className="text-[12px] tracking-[.08em] uppercase text-muted mb-2.5">내 계정</h5>
          <div className="flex items-center gap-2">
            <span className="mono text-sm">{session?.username}</span>
            {session && <RoleTag role={session.role} />}
          </div>
        </div>

        <div>
          <h5 className="text-[12px] tracking-[.08em] uppercase text-muted mb-2.5">권한별 가능한 작업</h5>
          <table className="table">
            <thead>
              <tr>
                <th>역할</th>
                <th>읽기</th>
                <th>생성 · 수정</th>
                <th>삭제</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td className="mono">adm</td>
                <td>✓</td>
                <td>✓</td>
                <td>✓</td>
              </tr>
              <tr>
                <td className="mono">dev</td>
                <td>✓</td>
                <td>✓</td>
                <td>—</td>
              </tr>
              <tr>
                <td className="mono">view</td>
                <td>✓</td>
                <td>—</td>
                <td>—</td>
              </tr>
            </tbody>
          </table>
        </div>

        {session?.role === "adm" && <GroupTeamMapping config={config} />}
      </div>
    </main>
  );
}
