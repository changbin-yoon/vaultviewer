import { integrationState, useIntegrations } from "../lib/useIntegrations";

// Trino/OPA/S3 IAM 커넥터 박스 하나 — live 여부에 따라 실선/accent(연결) vs
// 점선/muted(미연동·연결 끊김)로 스타일이 갈림. 대시보드 카드와 같은 판정
// 규칙(integrationState)을 그대로 써서 두 화면이 항상 같은 상태를 보여줌.
function ArchNode({
  x,
  y,
  w,
  h,
  label,
  sub,
  live,
}: {
  x: number;
  y: number;
  w: number;
  h: number;
  label: string;
  sub: string;
  live: boolean;
}) {
  return (
    <>
      <rect
        x={x}
        y={y}
        width={w}
        height={h}
        rx={10}
        fill={live ? "var(--al-accent-soft)" : "var(--al-surface)"}
        stroke={live ? "var(--al-accent)" : "var(--al-border)"}
        strokeWidth={live ? 2 : 1.5}
        strokeDasharray={live ? undefined : "5 5"}
      />
      <text
        className="al-node-label"
        x={x + w / 2}
        y={y + 24}
        textAnchor="middle"
        style={{ fill: live ? "var(--al-accent-strong)" : "var(--al-text-faint)" }}
      >
        {label}
      </text>
      <text className="al-node-sub" x={x + w / 2} y={y + 40} textAnchor="middle">
        {sub}
      </text>
    </>
  );
}

// AccessLens에서 Trino/OPA/S3 IAM 박스로 이어지는 커넥터 — live면 실선(+흐름
// 오버레이), 아니면 점선. Dashboard의 al-trunk-live/planned/al-flow와 같은
// CSS를 재사용해 두 구성도의 시각 언어를 통일함.
function ArchConnector({ x1, y1, x2, y2, live }: { x1: number; y1: number; x2: number; y2: number; live: boolean }) {
  const d = `M ${x1} ${y1} L ${x2} ${y2}`;
  return (
    <>
      <path className={live ? "al-trunk-live" : "al-trunk-planned"} d={d} markerEnd="url(#alArchArrow)" />
      {live && <path className="al-flow" pathLength="1" d={d} />}
    </>
  );
}

export function ArchPage() {
  const { trino, opa, s3iam } = useIntegrations();
  const trinoState = integrationState(trino.enabled, trino.connected);
  const opaState = integrationState(opa.enabled, opa.connected);
  const s3State = integrationState(s3iam.enabled, s3iam.connected);

  const liveNames = [
    trinoState.live && "Trino",
    opaState.live && "OPA",
    s3State.live && "S3 IAM",
  ].filter((n): n is string => !!n);
  const notLiveNames = ["Trino", "OPA", "S3 IAM"].filter((n) => !liveNames.includes(n));

  return (
    <div className="al-scope al-page">
      <div className="al-panel al-arch-panel">
        <h2 style={{ fontSize: "1.05rem", marginBottom: 6 }}>시스템 구성도</h2>
        <p className="al-caption">
          AccessLens 하나가 LDAP으로 로그인 계정을 확인하고, 감사 로그를 남기며 Vault를
          조회/수정합니다. Trino·OPA·S3 IAM 박스는 실제 연결 상태에 따라 아래 구성도가 실시간으로
          바뀝니다{liveNames.length > 0 ? ` — 지금은 ${liveNames.join("/")}가 연결됨` : ""}
          {notLiveNames.length > 0 ? `${liveNames.length > 0 ? "," : " —"} ${notLiveNames.join("/")}은 아직임` : ""}.
        </p>

        <figure className="al-arch-figure">
          <svg
            className="al-arch al-diagram-entry"
            viewBox="0 0 1040 480"
            role="img"
            aria-label={`사용자 브라우저가 HTTPS로 AccessLens에 로그인하면, AccessLens가 LDAP으로 인증하고 감사 로그를 기록한 뒤 Vault(VaultStorageEngine)를 조회/수정하며, VaultStorageEngine은 로컬 파일시스템, Git 백엔드, K8s Secrets 중 하나로 위임하는 구조입니다. ${
              liveNames.length > 0 ? `${liveNames.join("/")}는 실제로 연결되어 있고` : "Trino/OPA/S3 IAM은 아직 연결되어 있지 않고"
            }${notLiveNames.length > 0 && liveNames.length > 0 ? `, ${notLiveNames.join("/")}은 아직 연동되지 않은` : ""} 상태입니다.`}
          >
            <defs>
              <marker id="alArchArrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--al-line)" />
              </marker>
            </defs>

            {/* 사용자 -> AccessLens */}
            <line x1="520" y1="80" x2="520" y2="135" stroke="var(--al-line)" strokeWidth="1.6" markerEnd="url(#alArchArrow)" />
            <text className="al-node-sub" x="536" y="112" textAnchor="start">HTTPS 로그인</text>

            {/* AccessLens -> LDAP */}
            <line x1="430" y1="165" x2="270" y2="165" stroke="var(--al-line)" strokeWidth="1.6" markerEnd="url(#alArchArrow)" />
            <text className="al-node-sub" x="350" y="155" textAnchor="middle">LDAP bind · 그룹 조회</text>

            {/* AccessLens -> Audit */}
            <line x1="690" y1="165" x2="765" y2="165" stroke="var(--al-line)" strokeWidth="1.6" markerEnd="url(#alArchArrow)" />
            <text className="al-node-sub" x="727" y="155" textAnchor="middle">기록</text>

            {/* AccessLens -> Trino/OPA/S3 IAM — 연결 상태에 따라 실선/점선 전환 */}
            <ArchConnector x1={460} y1={195} x2={150} y2={292} live={trinoState.live} />
            <ArchConnector x1={510} y1={195} x2={390} y2={292} live={opaState.live} />
            <ArchConnector x1={610} y1={195} x2={630} y2={292} live={s3State.live} />
            <text className="al-node-sub" x="255" y="235" textAnchor="middle">Basic Auth · /v1/info</text>
            <text className="al-node-sub" x="450" y="235" textAnchor="middle">정책 조회 API (grants)</text>
            <text className="al-node-sub" x="620" y="245" textAnchor="middle">STS AssumeRole</text>

            {/* AccessLens -> VaultStorageEngine (항상 실제 연동, 실선) */}
            <line x1="660" y1="195" x2="860" y2="292" stroke="var(--al-accent)" strokeWidth="2" markerEnd="url(#alArchArrow)" />
            <text className="al-node-sub" x="790" y="235" textAnchor="middle">인터페이스 호출</text>

            {/* VaultStorageEngine -> 3개 백엔드 */}
            <line x1="820" y1="348" x2="765" y2="405" stroke="var(--al-line)" strokeWidth="1.6" markerEnd="url(#alArchArrow)" />
            <line x1="860" y1="348" x2="860" y2="405" stroke="var(--al-line)" strokeWidth="1.6" markerEnd="url(#alArchArrow)" />
            <line x1="900" y1="348" x2="960" y2="405" stroke="var(--al-line)" strokeWidth="1.6" markerEnd="url(#alArchArrow)" />

            {/* 사용자 브라우저 */}
            <rect x="430" y="30" width="180" height="50" rx="10" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" />
            <text className="al-node-label" x="520" y="59" textAnchor="middle">사용자 브라우저</text>

            {/* AccessLens (core) */}
            <rect x="430" y="135" width="260" height="60" rx="12" fill="var(--al-accent-soft)" stroke="var(--al-accent)" strokeWidth="2" />
            <text className="al-node-label" x="560" y="160" textAnchor="middle" style={{ fill: "var(--al-accent-strong)" }}>AccessLens</text>
            <text className="al-node-sub" x="560" y="176" textAnchor="middle">Web UI · REST API</text>

            {/* LDAP */}
            <rect x="115" y="139" width="155" height="52" rx="10" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" />
            <text className="al-node-label" x="192" y="162" textAnchor="middle">LDAP</text>
            <text className="al-node-sub" x="192" y="178" textAnchor="middle">사용자 · 그룹 디렉터리</text>

            {/* Audit */}
            <rect x="765" y="139" width="150" height="52" rx="10" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" />
            <text className="al-node-label" x="840" y="162" textAnchor="middle">감사 로그</text>
            <text className="al-node-sub" x="840" y="178" textAnchor="middle">WS 스트림 기록</text>

            {/* Trino / OPA / S3 IAM — 실제 연결 상태 반영 */}
            <ArchNode x={65} y={292} w={170} h={56} label="Trino" sub={trinoState.sub} live={trinoState.live} />
            <ArchNode x={305} y={292} w={170} h={56} label="OPA" sub={opaState.sub} live={opaState.live} />
            <ArchNode x={545} y={292} w={170} h={56} label="S3 IAM" sub={s3State.sub} live={s3State.live} />

            {/* VaultStorageEngine (항상 실제 연동) */}
            <rect x="775" y="292" width="170" height="56" rx="12" fill="var(--al-accent-soft)" stroke="var(--al-accent)" strokeWidth="1.5" />
            <text className="al-node-label" x="860" y="313" textAnchor="middle" style={{ fill: "var(--al-accent-strong)" }}>VaultStorageEngine</text>
            <text className="al-node-sub" x="860" y="329" textAnchor="middle">스토리지 추상 인터페이스</text>

            {/* 3개 백엔드 구현체 */}
            <rect x="705" y="405" width="120" height="48" rx="9" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" />
            <text className="al-node-label" x="765" y="427" textAnchor="middle" fontSize="10.5">로컬 파일</text>
            <text className="al-node-sub" x="765" y="442" textAnchor="middle">local 모드</text>

            <rect x="800" y="405" width="120" height="48" rx="9" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" />
            <text className="al-node-label" x="860" y="427" textAnchor="middle" fontSize="10.5">Git 백엔드</text>
            <text className="al-node-sub" x="860" y="442" textAnchor="middle">편집 시 자동 커밋</text>

            <rect x="900" y="405" width="120" height="48" rx="9" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" />
            <text className="al-node-label" x="960" y="427" textAnchor="middle" fontSize="10.5">K8s Secrets</text>
            <text className="al-node-sub" x="960" y="442" textAnchor="middle">클러스터 모드</text>
          </svg>
          <figcaption>
            실선은 실제로 연결이 확인된 경로(LDAP 인증, 감사 로그, VaultStorageEngine → 로컬 파일/Git/K8s
            Secrets, 그리고 지금 연결된 Trino/OPA/S3 IAM)이고, 점선은 아직 연동되지 않았거나 연결에
            실패한 경로입니다. 이 페이지를 열 때마다 Trino/OPA/S3 IAM 상태를 다시 확인합니다.
          </figcaption>
        </figure>
      </div>
    </div>
  );
}
