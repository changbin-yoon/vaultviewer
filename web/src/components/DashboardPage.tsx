import type { CSSProperties } from "react";
import type { Config, OpaIntegration, Role, S3IamIntegration, TeamGrant, TrinoIntegration } from "../lib/api";
import { useIntegrations } from "../lib/useIntegrations";
import { RoleTag } from "./RoleTag";

type View = "dashboard" | "vault" | "graph" | "tags" | "search" | "audit" | "guide" | "settings" | "arch";

// SVG entry-animation delay, staggered per node — a plain CSS custom
// property, so it needs the escape hatch React's CSSProperties doesn't type.
type DelayStyle = CSSProperties & { "--al-delay"?: string };

const ROLE_DESC: Record<Role, string> = {
  adm: "읽기 · 생성 · 수정 · 삭제 (전체 권한)",
  dev: "읽기 · 생성 · 수정 (삭제 불가)",
  view: "읽기 전용",
};

// 위성 노드 목록 — 좌표 없이 이름/기본 상태만. 새 LDAP 연동 서비스가 생기면
// 이 배열에 한 줄 추가하고 ConnectionDiagram의 live/sub 오버라이드 분기만
// 더하면 됨 — 좌표는 layoutSatellites가 항상 자동으로 다시 계산함.
const SATELLITE_DEFS: { key: string; label: string; live: boolean; sub: string }[] = [
  { key: "trino", label: "Trino", live: false, sub: "연동 예정" },
  { key: "opa", label: "OPA", live: false, sub: "연동 예정" },
  { key: "s3", label: "S3 IAM", live: false, sub: "연동 예정" },
  { key: "vault", label: "Vault", live: true, sub: "" }, // sub filled in at render time
];

const CENTER = { x: 320, y: 160 };
const SATELLITE_RADIUS = { x: 210, y: 100 };

// 위성 N개를 중심 둘레 타원 위에 고르게 배치. N=4일 때는 기존 다이아몬드
// 배치(좌상/우상/우하/좌하)와 동일한 좌표가 나오도록 시작각을 잡음 — 노드가
// 늘어나도 뷰박스(640x320) 안에 들어오는 동일한 타원 위에서 각도만 나뉨.
function layoutSatellites<T extends { key: string }>(defs: T[]): (T & { x: number; y: number })[] {
  const n = defs.length;
  const angleStep = 360 / n;
  const startAngle = 180 + angleStep / 2;
  return defs.map((d, i) => {
    const angle = ((startAngle + i * angleStep) * Math.PI) / 180;
    return { ...d, x: CENTER.x + SATELLITE_RADIUS.x * Math.cos(angle), y: CENTER.y + SATELLITE_RADIUS.y * Math.sin(angle) };
  });
}

const SATELLITES = layoutSatellites(SATELLITE_DEFS);

function opaSummary(opa: OpaIntegration): string {
  const grants = opa.grants ?? [];
  if (grants.length === 0) return "grants 없음";
  return grants.map((g) => `${g.team}(${g.role})`).join(", ");
}

function ConnectionDiagram({
  username,
  vaultSub,
  trino,
  opa,
  s3iam,
}: {
  username: string;
  vaultSub: string;
  trino: TrinoIntegration;
  opa: OpaIntegration;
  s3iam: S3IamIntegration;
}) {
  const satellites = SATELLITES.map((s) => {
    if (s.key === "vault") return { ...s, sub: vaultSub };
    if (s.key === "trino" && trino.enabled) {
      return { ...s, live: !!trino.connected, sub: trino.connected ? (trino.role ?? "") : "연결 안 됨" };
    }
    if (s.key === "opa" && opa.enabled) {
      return { ...s, live: !!opa.connected, sub: opa.connected ? opaSummary(opa) : "연결 안 됨" };
    }
    if (s.key === "s3" && s3iam.enabled) {
      return { ...s, live: !!s3iam.connected, sub: s3iam.connected ? (s3iam.role ?? "") : "연결 안 됨" };
    }
    return s;
  });

  // 연결 상태에 따라 aria-label을 동적으로 구성 — "연결된 시스템" / "아직
  // 연동 예정인 시스템" 목록을 나눠서 문장으로 조립.
  const connectedNames = [
    "Vault",
    trino.enabled && trino.connected ? "Trino" : null,
    opa.enabled && opa.connected ? "OPA" : null,
    s3iam.enabled && s3iam.connected ? "S3 IAM" : null,
  ].filter((n): n is string => !!n);
  const plannedNames = ["Trino", "OPA", "S3 IAM"].filter((n) => !connectedNames.includes(n));

  return (
    <svg
      className="al-diagram al-diagram-entry"
      viewBox="0 0 640 320"
      role="img"
      aria-label={`LDAP 계정 ${username}가 ${connectedNames.join("/")}에는 실제로 연결되어 있고${
        plannedNames.length > 0 ? `, ${plannedNames.join("/")}은 아직 연동 예정임` : ""
      }을 보여주는 구조도`}
    >
      {satellites.map((s, i) => (
        <path
          key={`line-${s.key}`}
          className={s.live ? "al-trunk-live" : "al-trunk-planned"}
          d={`M ${CENTER.x} ${CENTER.y} L ${s.x} ${s.y}`}
          style={{ "--al-delay": `${0.1 + i * 0.12}s` } as DelayStyle}
        />
      ))}

      {satellites.map(
        (s, i) =>
          s.live && (
            <path
              key={`flow-${s.key}`}
              className="al-flow"
              pathLength="1"
              d={`M ${CENTER.x} ${CENTER.y} L ${s.x} ${s.y}`}
              style={{ "--al-delay": `${0.1 + i * 0.12}s` } as DelayStyle}
            />
          ),
      )}

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

// role is the account's single overall resolved role (highest-precedence
// across every LDAP group it's in — see auth.ResolveRole), never per-team.
// catalogs are the deduplicated union across every team in `teams` (via
// OPA's live teams map) when the account has team-scoped groups, otherwise
// the operator-configured flat list — see internal/api's /api/trino
// handler. Only "connected" reflects a real Trino check either way.
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
        {trino.teams && trino.teams.length > 0 && (
          <div className="al-row">
            <dt>소속 팀</dt>
            <dd>{trino.teams.join(", ")}</dd>
          </div>
        )}
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

// team/catalogs/operations are read live from OPA's grants document for
// the caller's mapped LDAP group — not AccessLens config. See internal/opa.
function OpaCard({ opa }: { opa: OpaIntegration }) {
  if (!opa.enabled) return <PlannedCard icon="O" name="OPA" />;

  const grants = opa.grants ?? [];
  const catalogs = [...new Set(grants.flatMap((g) => g.catalogs))];
  const operations = [...new Set(grants.flatMap((g) => g.operations))];

  return (
    <div className="al-panel al-perm-card">
      <div className="al-top">
        <div className="al-sys">
          <div className={`al-sys-icon${opa.connected ? "" : " al-planned"}`}>O</div>
          <div>
            <h3>OPA</h3>
            <div className="al-role-line">
              {grants.length > 0 ? grants.map((g) => `${g.team}(${g.role})`).join(", ") : "grants 없음"}
            </div>
          </div>
        </div>
        <span className={`al-status-dot${opa.connected ? "" : " al-planned"}`}>
          {opa.connected ? "연결됨" : "연결 안 됨"}
        </span>
      </div>
      <dl>
        {catalogs.length > 0 && (
          <div className="al-row">
            <dt>카탈로그</dt>
            <dd>{catalogs.join(", ")}</dd>
          </div>
        )}
        {operations.length > 0 && (
          <div className="al-row">
            <dt>허용 작업</dt>
            <dd>{operations.join(", ")}</dd>
          </div>
        )}
      </dl>
    </div>
  );
}

// role is the account's single overall resolved role, never per-team (same
// as Trino's card). buckets are the deduplicated union across every team in
// `teams` (via s3iam.bucketMap) when the account has team-scoped groups,
// otherwise the operator-configured flat list — see internal/api's
// /api/s3iam handler. Only "connected"/accessKeyId/expiresAt reflect a real
// AssumeRoleWithLDAPIdentity check against the S3 endpoint. See internal/s3iam.
function S3IamCard({ s3iam }: { s3iam: S3IamIntegration }) {
  if (!s3iam.enabled) return <PlannedCard icon="S3" name="S3 IAM" />;

  return (
    <div className="al-panel al-perm-card">
      <div className="al-top">
        <div className="al-sys">
          <div className={`al-sys-icon${s3iam.connected ? "" : " al-planned"}`}>S3</div>
          <div>
            <h3>S3 IAM</h3>
            <div className="al-role-line">RBAC: {s3iam.role}</div>
          </div>
        </div>
        <span className={`al-status-dot${s3iam.connected ? "" : " al-planned"}`}>
          {s3iam.connected ? "연결됨" : "연결 안 됨"}
        </span>
      </div>
      <dl>
        <div className="al-row">
          <dt>역할</dt>
          <dd>{s3iam.role}</dd>
        </div>
        {s3iam.teams && s3iam.teams.length > 0 && (
          <div className="al-row">
            <dt>소속 팀</dt>
            <dd>{s3iam.teams.join(", ")}</dd>
          </div>
        )}
        {s3iam.buckets && s3iam.buckets.length > 0 && (
          <div className="al-row">
            <dt>버킷</dt>
            <dd>{s3iam.buckets.join(", ")}</dd>
          </div>
        )}
        {s3iam.accessKeyId && (
          <div className="al-row">
            <dt>Access Key</dt>
            <dd>{s3iam.accessKeyId}</dd>
          </div>
        )}
        {s3iam.expiresAt && (
          <div className="al-row">
            <dt>만료</dt>
            <dd>{new Date(s3iam.expiresAt).toLocaleTimeString()}</dd>
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
  session: { username: string; role: Role; department: string; teams: TeamGrant[] };
  onNavigateView: (v: View) => void;
}) {
  const vaultSub = config ? `${config.mode} 모드` : "";
  // The avatar shows an org-level mark rather than username initials: the
  // primary team's name when the account's LDAP groups resolve one (e.g.
  // "bi-adm" -> "BI"), otherwise the department/소속 (e.g. "플랫폼운영팀" ->
  // "플랫") for accounts whose groups don't follow that convention. Only
  // falls back to username initials if neither is available.
  const primaryTeam = session.teams[0]?.team;
  const avatarLabel = primaryTeam
    ? primaryTeam.toUpperCase()
    : session.department
      ? session.department.slice(0, 2)
      : session.username.slice(0, 2);
  const { trino, opa, s3iam } = useIntegrations();

  const integratedNames = [
    "Vault",
    trino.enabled ? "Trino" : null,
    opa.enabled ? "OPA" : null,
    s3iam.enabled ? "S3 IAM" : null,
  ].filter((n): n is string => !!n);
  const integratedLabel = `${integratedNames.join(" · ")} 연동됨`;
  const remainingNames = ["Trino", "OPA", "S3 IAM"].filter((n) => !integratedNames.includes(n));
  const remainingCount = remainingNames.length;

  return (
    <div className="al-scope al-page">
      <section className="al-hero">
        <div className="al-panel al-id-card">
          <div className="al-who">
            <div className="al-avatar-lg">{avatarLabel}</div>
            <div>
              <div className="al-name">{session.username}</div>
              {session.department && <div className="al-dept">{session.department}</div>}
            </div>
          </div>
          <div>
            <div className="al-id-row">
              <span className="al-k">역할</span>
              <span className="al-v">
                <RoleTag role={session.role} />
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
          {session.teams.length > 0 && (
            <div className="al-team-grants">
              <div className="al-team-grants-label">소속 팀 및 권한</div>
              {session.teams.map((t) => (
                <div key={t.team} className="al-team-grant-row">
                  <div className="al-team-top">
                    <span className="al-team-badge">{t.team}</span>
                    <RoleTag role={t.role} />
                  </div>
                  <span className="al-team-desc">{ROLE_DESC[t.role]}</span>
                </div>
              ))}
            </div>
          )}
          <p className="al-caption">
            이 계정의 역할이 {integratedNames.join("·")} 권한을 결정합니다.
            {remainingNames.length > 0 && (
              <>
                {" "}
                {remainingNames.join("/")} 연동이 추가되면 같은 계정 하나로 그 권한도 함께 보이게
                됩니다.
              </>
            )}
          </p>
        </div>

        <div className="al-panel al-diagram-panel">
          <h2>계정 연결 구조</h2>
          <ConnectionDiagram username={session.username} vaultSub={vaultSub} trino={trino} opa={opa} s3iam={s3iam} />
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
        <OpaCard opa={opa} />
        <S3IamCard s3iam={s3iam} />

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
