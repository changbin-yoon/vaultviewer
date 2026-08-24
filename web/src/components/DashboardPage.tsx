import { useEffect, useState, type CSSProperties } from "react";
import * as api from "../lib/api";
import type { Config, Role, TrinoIntegration } from "../lib/api";

type View = "dashboard" | "vault" | "graph" | "tags" | "search" | "audit" | "guide" | "settings" | "arch";

// SVG entry-animation delay, staggered per node — a plain CSS custom
// property, so it needs the escape hatch React's CSSProperties doesn't type.
type DelayStyle = CSSProperties & { "--al-delay"?: string };

const ROLE_DESC: Record<Role, string> = {
  adm: "읽기 · 생성 · 수정 · 삭제 (전체 권한)",
  dev: "읽기 · 생성 · 수정 (삭제 불가)",
  view: "읽기 전용",
};

// 위성 노드 배치 — 항상 4개, 시계 방향 top-left/top-right/bottom-right/bottom-left.
const SATELLITES: { key: string; label: string; x: number; y: number; live: boolean; sub: string }[] = [
  { key: "trino", label: "Trino", x: 150, y: 78, live: false, sub: "연동 예정" },
  { key: "opa", label: "OPA", x: 490, y: 78, live: false, sub: "연동 예정" },
  { key: "s3", label: "S3 IAM", x: 490, y: 242, live: false, sub: "연동 예정" },
  { key: "vault", label: "Vault", x: 150, y: 242, live: true, sub: "" }, // sub filled in at render time
];

const CENTER = { x: 320, y: 160 };

function ConnectionDiagram({
  username,
  vaultSub,
  trino,
}: {
  username: string;
  vaultSub: string;
  trino: TrinoIntegration;
}) {
  const satellites = SATELLITES.map((s) => {
    if (s.key === "vault") return { ...s, sub: vaultSub };
    if (s.key === "trino" && trino.enabled) {
      return { ...s, live: !!trino.connected, sub: trino.connected ? (trino.role ?? "") : "연결 안 됨" };
    }
    return s;
  });

  return (
    <svg
      className="al-diagram al-diagram-entry"
      viewBox="0 0 640 320"
      role="img"
      aria-label={`LDAP 계정 ${username}가 Vault${
        trino.enabled ? (trino.connected ? "와 Trino에는" : "에는") : "에는"
      } 실제로 연결되어 있고, ${trino.enabled && trino.connected ? "OPA/S3 IAM은" : "Trino/OPA/S3 IAM은"} 아직 연동 예정임을 보여주는 구조도`}
    >
      {satellites.map((s, i) => (
        <path
          key={`line-${s.key}`}
          className={s.live ? "al-trunk-live" : "al-trunk-planned"}
          d={`M ${CENTER.x} ${CENTER.y} L ${s.x} ${s.y}`}
          style={{ "--al-delay": `${0.1 + i * 0.12}s` } as DelayStyle}
        />
      ))}

      {satellites.map((s, i) => (
        <g key={`sat-${s.key}`}>
          <circle
            className={`al-sat-ring al-scale ${s.live ? "live" : ""}`}
            cx={s.x}
            cy={s.y}
            r={30}
            style={{ "--al-delay": `${0.35 + i * 0.12}s` } as DelayStyle}
          />
          <circle
            className={`al-scale ${s.live ? "al-sat-status-live" : "al-sat-status-planned"}`}
            cx={s.x + 19}
            cy={s.y - (s.y > CENTER.y ? -18 : 18)}
            r={5}
            style={{ "--al-delay": `${0.55 + i * 0.12}s` } as DelayStyle}
          />
          <text
            className="al-node-label"
            x={s.x}
            y={s.y + 4}
            textAnchor="middle"
            style={{ "--al-delay": `${0.45 + i * 0.12}s` } as DelayStyle}
          >
            {s.label}
          </text>
          <text
            className="al-node-sub"
            x={s.x}
            y={s.y + (s.y > CENTER.y ? -36 : 40)}
            textAnchor="middle"
            style={{ "--al-delay": `${0.45 + i * 0.12}s` } as DelayStyle}
          >
            {s.sub}
          </text>
        </g>
      ))}

      <circle className="al-center-ring al-scale" cx={CENTER.x} cy={CENTER.y} r={38} style={{ "--al-delay": "0s" } as DelayStyle} />
      <text
        className="al-center-label"
        x={CENTER.x}
        y={CENTER.y - 5}
        textAnchor="middle"
        fontSize="12"
        style={{ "--al-delay": "0.25s", fill: "var(--al-surface)" } as DelayStyle}
      >
        {username}
      </text>
      <text
        className="al-node-sub"
        x={CENTER.x}
        y={CENTER.y + 11}
        textAnchor="middle"
        style={{ "--al-delay": "0.25s", fill: "var(--al-surface)", opacity: 0.85 } as DelayStyle}
      >
        LDAP
      </text>
    </svg>
  );
}

function PlannedCard({ icon, name }: { icon: string; name: string }) {
  return (
    <div className="al-panel al-perm-card">
      <div className="al-top">
        <div className="al-sys">
          <div className="al-sys-icon al-planned">{icon}</div>
          <div>
            <h3>{name}</h3>
            <div className="al-role-line">아직 연동되지 않았습니다</div>
          </div>
        </div>
        <span className="al-status-dot al-planned">연동 예정</span>
      </div>
      <p className="al-planned-note">
        백엔드에 {name} 조회 API가 추가되면 이 카드에 실제 권한 정보가 표시됩니다. 지금은 자리만
        잡아둔 상태입니다.
      </p>
    </div>
  );
}

// role/catalogs are config-driven (operator-set role/catalog labels), not a
// live Trino GRANT lookup — only "connected" reflects a real check.
function TrinoCard({ trino }: { trino: TrinoIntegration }) {
  if (!trino.enabled) return <PlannedCard icon="T" name="Trino" />;

  return (
    <div className="al-panel al-perm-card">
      <div className="al-top">
        <div className="al-sys">
          <div className={`al-sys-icon${trino.connected ? "" : " al-planned"}`}>T</div>
          <div>
            <h3>Trino</h3>
            <div className="al-role-line">RBAC: {trino.role}</div>
          </div>
        </div>
        <span className={`al-status-dot${trino.connected ? "" : " al-planned"}`}>
          {trino.connected ? "연결됨" : "연결 안 됨"}
        </span>
      </div>
      <dl>
        <div className="al-row">
          <dt>역할</dt>
          <dd>{trino.role}</dd>
        </div>
        {trino.catalogs && trino.catalogs.length > 0 && (
          <div className="al-row">
            <dt>카탈로그</dt>
            <dd>{trino.catalogs.join(", ")}</dd>
          </div>
        )}
      </dl>
    </div>
  );
}

export function DashboardPage({
  config,
  session,
  onNavigateView,
}: {
  config: Config | null;
  session: { username: string; role: Role; department: string };
  onNavigateView: (v: View) => void;
}) {
  const vaultSub = config ? `${config.mode} 모드` : "";
  const [trino, setTrino] = useState<TrinoIntegration>({ enabled: false });

  useEffect(() => {
    let cancelled = false;
    api
      .getTrinoIntegration()
      .then((data) => {
        if (!cancelled) setTrino(data);
      })
      .catch(() => {
        // Leave the disabled default — matches "not configured" so the
        // Trino card falls back to the same placeholder as OPA/S3 IAM.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const integratedLabel = trino.enabled ? "Vault · Trino 연동됨" : "Vault 연동됨";
  const remainingCount = trino.enabled ? 2 : 3;

  return (
    <div className="al-scope al-page">
      <section className="al-hero">
        <div className="al-panel al-id-card">
          <div className="al-who">
            <div className="al-avatar-lg">{session.username.slice(0, 2)}</div>
            <div>
              <div className="al-name">{session.username}</div>
              {session.department && <div className="al-dept">{session.department}</div>}
            </div>
          </div>
          <div>
            <div className="al-id-row">
              <span className="al-k">역할</span>
              <span className="al-v">
                <span className="al-role-pill">{session.role}</span>
              </span>
            </div>
            <div className="al-id-row">
              <span className="al-k">권한 범위</span>
              <span className="al-v">{ROLE_DESC[session.role]}</span>
            </div>
            {config && (
              <>
                <div className="al-id-row">
                  <span className="al-k">배포 환경</span>
                  <span className="al-v">{config.deployment}</span>
                </div>
                <div className="al-id-row">
                  <span className="al-k">스토리지</span>
                  <span className="al-v">
                    {config.mode} / {config.backend}
                  </span>
                </div>
              </>
            )}
          </div>
          <p className="al-caption">
            이 계정의 역할이 Vault{trino.enabled ? "·Trino" : ""} 권한을 결정합니다.{" "}
            {trino.enabled ? "OPA/S3 IAM" : "Trino/OPA/S3 IAM"} 연동이 추가되면 같은 계정 하나로 그
            권한도 함께 보이게 됩니다.
          </p>
        </div>

        <div className="al-panel al-diagram-panel">
          <h2>계정 연결 구조</h2>
          <ConnectionDiagram username={session.username} vaultSub={vaultSub} trino={trino} />
        </div>
      </section>

      <div className="al-section-head">
        <h2>시스템별 권한</h2>
        <span className="al-note">
          {integratedLabel} · {remainingCount}개 시스템 연동 예정
        </span>
      </div>

      <div className="al-perm-grid">
        <TrinoCard trino={trino} />
        <PlannedCard icon="O" name="OPA" />
        <PlannedCard icon="S3" name="S3 IAM" />

        <div className="al-panel al-perm-card">
          <div className="al-top">
            <div className="al-sys">
              <div className="al-sys-icon">V</div>
              <div>
                <h3>Vault</h3>
                <div className="al-role-line">RBAC: {session.role}</div>
              </div>
            </div>
            <span className="al-status-dot">연결됨</span>
          </div>
          <dl>
            <div className="al-row">
              <dt>권한 범위</dt>
              <dd>{ROLE_DESC[session.role]}</dd>
            </div>
            {config && (
              <div className="al-row">
                <dt>백엔드</dt>
                <dd>
                  {config.mode} / {config.backend}
                </dd>
              </div>
            )}
          </dl>
          <button className="al-btn" type="button" onClick={() => onNavigateView("vault")}>
            Vault 열기 →
          </button>
        </div>
      </div>
    </div>
  );
}
