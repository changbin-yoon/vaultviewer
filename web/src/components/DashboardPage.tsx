import type { CSSProperties } from "react";
import { useState } from "react";
import type {
  Config,
  DriftReport,
  Role,
  S3Access,
  S3Capability,
  S3IamIntegration,
  TeamGrant,
  TrinoIntegration,
} from "../lib/api";
import { getS3IamDrift } from "../lib/api";
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
// 중심은 LDAP 계정, 위성은 그 신원으로 권한이 결정되는 시스템들이다 —
// 대시보드의 권한 카드 넷(LDAP + 시스템 셋)과 정확히 1:1로 대응한다.
// OPA는 카드와 함께 뺐다(추후 추가); 그때 여기 한 줄을 되살리면 된다.
const SATELLITE_DEFS: { key: string; label: string; live: boolean; sub: string }[] = [
  { key: "trino", label: "Trino", live: false, sub: "연동 예정" },
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

function ConnectionDiagram({
  username,
  vaultSub,
  trino,
  s3iam,
}: {
  username: string;
  vaultSub: string;
  trino: TrinoIntegration;
  s3iam: S3IamIntegration;
}) {
  const satellites = SATELLITES.map((s) => {
    if (s.key === "vault") return { ...s, sub: vaultSub };
    if (s.key === "trino" && trino.enabled) {
      return { ...s, live: !!trino.connected, sub: trino.connected ? (trino.role ?? "") : "연결 안 됨" };
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
    s3iam.enabled && s3iam.connected ? "S3 IAM" : null,
  ].filter((n): n is string => !!n);
  const plannedNames = ["Trino", "S3 IAM"].filter((n) => !connectedNames.includes(n));

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
// LDAP은 나머지 세 시스템의 뿌리다. 역할도, 팀도, S3 정책 attach 대상도
// 전부 여기서 나온 그룹 소속으로 결정된다. 그래서 첫 카드로 두고, 다른
// 카드의 값이 왜 그런지를 여기서 설명한다.
function LdapCard({
  role,
  department,
  teams,
  groups,
}: {
  role: Role;
  department: string;
  teams: TeamGrant[];
  groups: string[];
}) {
  // 팀 규칙(<팀>-<역할>)에 해당하는 그룹은 그 의미를 함께 보여주고,
  // 나머지는 이름만 보여준다 — 규칙에 안 맞는 그룹도 감추지 않는다.
  const teamByGroup = new Map(teams.map((t) => [`${t.team}-${t.role}`, t]));

  return (
    <div className="al-panel al-perm-card">
      <div className="al-top">
        <div className="al-sys">
          <div className="al-sys-icon">ID</div>
          <div>
            <h3>LDAP</h3>
            <div className="al-role-line">RBAC: {role}</div>
          </div>
        </div>
        <span className="al-status-dot">인증됨</span>
      </div>
      <dl>
        <div className="al-row">
          <dt>역할</dt>
          <dd>{role}</dd>
        </div>
        {department && (
          <div className="al-row">
            <dt>소속</dt>
            <dd>{department}</dd>
          </div>
        )}
        <div className="al-row">
          <dt>그룹</dt>
          <dd>{groups.length}개</dd>
        </div>
      </dl>
      {groups.length > 0 && (
        <div className="al-access">
          <div className="al-access-head">
            <span>소속 그룹</span>
            <span className="al-access-stamp">아래 권한의 출처</span>
          </div>
          {groups.map((g) => {
            const grant = teamByGroup.get(g);
            return (
              <div className="al-access-row" key={g}>
                <div className="al-access-bucket">{g}</div>
                <div className="al-access-via">
                  {grant ? `팀 ${grant.team} · 역할 ${grant.role}` : "팀 규칙에 맞지 않는 그룹"}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// Vault 권한은 계정의 역할 하나로 결정된다 — S3처럼 정책 문서가 있는 게
// 아니라 이 앱이 직접 판정하는 값이라, 능력 칩만 보여주고 출처는 역할이다.
const VAULT_CAPABILITIES: { label: string; roles: Role[]; destructive?: boolean }[] = [
  { label: "읽기", roles: ["adm", "dev", "view"] },
  { label: "생성", roles: ["adm", "dev"] },
  { label: "수정", roles: ["adm", "dev"] },
  { label: "삭제", roles: ["adm"], destructive: true },
];

function VaultCard({
  role,
  config,
  onOpen,
}: {
  role: Role;
  config: Config | null;
  onOpen: () => void;
}) {
  const granted = VAULT_CAPABILITIES.filter((c) => c.roles.includes(role));

  return (
    <div className="al-panel al-perm-card">
      <div className="al-top">
        <div className="al-sys">
          <div className="al-sys-icon">V</div>
          <div>
            <h3>Vault</h3>
            <div className="al-role-line">RBAC: {role}</div>
          </div>
        </div>
        <span className="al-status-dot">연결됨</span>
      </div>
      <dl>
        {config && (
          <div className="al-row">
            <dt>백엔드</dt>
            <dd>
              {config.mode} / {config.backend}
            </dd>
          </div>
        )}
      </dl>
      <div className="al-access">
        <div className="al-access-head">
          <span>접근 권한</span>
          <span className="al-access-stamp">역할 {role} 기준</span>
        </div>
        <div className="al-access-row">
          <div className="al-caps">
            {granted.map((c) => (
              <span key={c.label} className={`al-cap${c.destructive ? " al-cap-destructive" : ""}`}>
                {c.label}
              </span>
            ))}
            {VAULT_CAPABILITIES.filter((c) => !c.roles.includes(role)).map((c) => (
              <span key={c.label} className="al-cap al-cap-absent">
                {c.label} 없음
              </span>
            ))}
          </div>
          <div className="al-access-via">via LDAP 역할</div>
        </div>
      </div>
      <button className="al-btn" type="button" onClick={onOpen}>
        Vault 열기 →
      </button>
    </div>
  );
}

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
// 액션 이름 대신 사람이 읽는 능력으로 보여준다. 순서는 백엔드의
// capabilityOrder(읽기 -> 쓰기, 약한 권한 -> 강한 권한)와 같으므로
// 여기서 다시 정렬하지 않는다.
const CAPABILITY_LABELS: Record<S3Capability, string> = {
  list: "목록",
  read: "읽기",
  lifecycleRead: "수명주기 조회",
  write: "쓰기",
  lifecycleWrite: "수명주기 설정",
  delete: "삭제",
  bucketPolicy: "버킷 정책",
  serviceAccount: "서비스 계정",
};

// 데이터를 없앨 수 있는 능력. lifecycleWrite가 여기 있는 이유는
// "Expiration: Days 1" ILM 규칙이 DeleteObject와 결과가 같기 때문이다 —
// 경로가 다를 뿐이라 화면에서도 같은 무게로 보여야 한다.
const DESTRUCTIVE: S3Capability[] = ["delete", "lifecycleWrite"];

// 버킷별 권한 내역. 이 값은 MinIO에 질의한 결과가 아니라 AccessLens가 들고
// 있는 정책 사본에서 계산한 것이라, 정책 개수와 로드 시각을 항상 함께
// 보여줘서 "실시간"으로 오해하지 않게 한다.
// 선언과 백엔드 실물의 대조. 관리자만 볼 수 있고, 라이브 admin 호출이라
// 대시보드가 뜰 때 자동으로 부르지 않고 버튼으로 실행한다.
function S3DriftPanel() {
  const [report, setReport] = useState<DriftReport | null>(null);
  const [loading, setLoading] = useState(false);

  const run = async () => {
    setLoading(true);
    try {
      setReport(await getS3IamDrift());
    } catch (e) {
      setReport({ enabled: true, error: e instanceof Error ? e.message : String(e) });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="al-access">
      <div className="al-access-head">
        <span>선언 대조</span>
        <button className="al-btn al-btn-sm" type="button" onClick={run} disabled={loading}>
          {loading ? "확인 중…" : "확인"}
        </button>
      </div>
      {report?.enabled === false && (
        <div className="al-access-via">
          admin 자격증명이 설정되지 않아 대조할 수 없습니다 (s3iam.drift.existingSecret).
        </div>
      )}
      {/* 검사 실패를 "일치"로 접지 않는다 — 이 화면이 막으려는 바로 그 거짓말이다. */}
      {report?.error && <div className="al-access-warn">대조 실패: {report.error}</div>}
      {report?.enabled && !report.error && (
        <>
          <div className="al-access-via">
            주체 {report.subjects}개 대조 ·{" "}
            {report.checkedAt && new Date(report.checkedAt).toLocaleTimeString()} 기준
          </div>
          {report.inSync ? (
            <div className="al-access-via">선언과 실물이 일치합니다.</div>
          ) : (
            (report.items ?? []).map((item) => (
              <div className="al-access-row" key={item.dn}>
                <div className="al-access-bucket">
                  <span className={`al-cap ${item.kind === "undeclared" ? "al-cap-destructive" : ""}`}>
                    {item.kind === "undeclared" ? "선언에 없음" : "미적용"}
                  </span>{" "}
                  {item.dn}
                </div>
                <div className="al-access-via">
                  선언 {item.declared.length ? item.declared.join(", ") : "-"} / 실물{" "}
                  {item.actual.length ? item.actual.join(", ") : "-"}
                </div>
              </div>
            ))
          )}
        </>
      )}
    </div>
  );
}

function S3AccessBreakdown({ access }: { access: S3Access }) {
  if (access.buckets.length === 0) {
    return (
      <div className="al-access">
        <div className="al-access-head">
          <span>접근 권한</span>
          <span className="al-access-stamp">
            정책 {access.policyCount}개 · attach {access.attachmentCount}건 ·{" "}
            {new Date(access.loadedAt).toLocaleTimeString()} 기준
          </span>
        </div>
        <div className="al-access-via">이 계정에 붙어 있는 정책이 없습니다.</div>
      </div>
    );
  }

  return (
    <div className="al-access">
      <div className="al-access-head">
        <span>접근 권한</span>
        <span className="al-access-stamp">
          정책 {access.policyCount}개 · attach {access.attachmentCount}건 ·{" "}
          {new Date(access.loadedAt).toLocaleTimeString()} 기준
        </span>
      </div>
      {access.buckets.map((b) => (
        <div className="al-access-row" key={b.bucket}>
          <div className="al-access-bucket">{b.bucket === "*" ? "계정 전체" : b.bucket}</div>
          <div className="al-caps">
            {b.capabilities.map((c) => (
              <span
                key={c}
                className={`al-cap${DESTRUCTIVE.includes(c) ? " al-cap-destructive" : ""}`}
              >
                {CAPABILITY_LABELS[c] ?? c}
              </span>
            ))}
          </div>
        </div>
      ))}
      {access.warnings && access.warnings.length > 0 && (
        <div className="al-access-warn">
          {access.warnings.map((w, i) => (
            <span key={i}>{w}</span>
          ))}
        </div>
      )}
    </div>
  );
}

function S3IamCard({ s3iam, role }: { s3iam: S3IamIntegration; role: Role }) {
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
      {s3iam.access && <S3AccessBreakdown access={s3iam.access} />}
      {/* 역할이 못 쓰는 기능은 회색 처리가 아니라 아예 렌더하지 않는다. */}
      {s3iam.access && role === "adm" && <S3DriftPanel />}
    </div>
  );
}

export function DashboardPage({
  config,
  session,
  onNavigateView,
}: {
  config: Config | null;
  session: { username: string; role: Role; department: string; teams: TeamGrant[]; groups: string[] };
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
  const { trino, s3iam } = useIntegrations();

  // 대시보드가 보여주는 네 축: LDAP(신원) + 그 신원으로 결정되는 세 시스템.
  // OPA는 백엔드(/api/opa, internal/opa)는 그대로 살아 있고 카드만 아직
  // 없다 — 연동 예정 목록에 남겨 그 사실이 화면에 드러나게 한다.
  const integratedNames = [
    "LDAP",
    trino.enabled ? "Trino" : null,
    s3iam.enabled ? "S3 IAM" : null,
    "Vault",
  ].filter((n): n is string => !!n);
  const integratedLabel = `${integratedNames.join(" · ")} 연동됨`;
  // 신원(LDAP)을 뺀, 그 신원으로 권한이 결정되는 시스템들.
  const derivedNames = integratedNames.filter((n) => n !== "LDAP");
  const remainingNames = ["Trino", "S3 IAM"]
    .filter((n) => !integratedNames.includes(n))
    .concat("OPA");
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
            {/* LDAP은 역할을 '주는' 쪽이므로 이 문장의 대상에서 뺀다 —
                LDAP 그룹 소속 → 역할 → 나머지 시스템 권한 순서다. */}
            LDAP 그룹 소속이 {derivedNames.join("·")} 권한을 결정합니다.
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
          <ConnectionDiagram username={session.username} vaultSub={vaultSub} trino={trino} s3iam={s3iam} />
        </div>
      </section>

      <div className="al-section-head">
        <h2>시스템별 권한</h2>
        <span className="al-note">
          {integratedLabel} · {remainingCount}개 시스템 연동 예정
        </span>
      </div>

      <div className="al-perm-grid">
        {/* LDAP이 먼저다 — 나머지 세 카드의 권한이 전부 여기서 나온 그룹
            소속으로 결정된다. OPA 카드는 뺐다(추후 추가). */}
        <LdapCard
          role={session.role}
          department={session.department}
          teams={session.teams}
          groups={session.groups}
        />
        <TrinoCard trino={trino} />
        <S3IamCard s3iam={s3iam} role={session.role} />
        <VaultCard role={session.role} config={config} onOpen={() => onNavigateView("vault")} />
      </div>
    </div>
  );
}
