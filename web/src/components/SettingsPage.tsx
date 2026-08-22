import type { Config } from "../lib/api";
import { useAuth } from "../lib/auth";
import { RoleTag } from "./RoleTag";

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
      </div>
    </main>
  );
}
