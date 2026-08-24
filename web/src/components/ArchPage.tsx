export function ArchPage() {
  return (
    <div className="al-scope al-page">
      <div className="al-panel al-arch-panel">
        <h2 style={{ fontSize: "1.05rem", marginBottom: 6 }}>시스템 구성도</h2>
        <p className="al-caption">
          AccessLens 하나가 LDAP으로 로그인 계정을 확인하고, 감사 로그를 남기며 Vault를
          조회/수정합니다. Trino·OPA·S3 IAM은 아직 백엔드 연동이 없는 계획 단계입니다(점선).
        </p>

        <figure className="al-arch-figure">
          <svg
            className="al-arch"
            viewBox="0 0 1040 480"
            role="img"
            aria-label="사용자 브라우저가 HTTPS로 AccessLens에 로그인하면, AccessLens가 LDAP으로 인증하고 감사 로그를 기록한 뒤 Vault(VaultStorageEngine)를 조회/수정하며, VaultStorageEngine은 로컬 파일시스템, Git 백엔드, K8s Secrets 중 하나로 위임하는 구조입니다. Trino, OPA, S3 IAM은 점선으로 표시된 연동 예정 항목입니다."
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

            {/* AccessLens -> Trino/OPA/S3 IAM (연동 예정, 점선) */}
            <line x1="460" y1="195" x2="150" y2="292" stroke="var(--al-line)" strokeWidth="1.4" strokeDasharray="5 5" markerEnd="url(#alArchArrow)" />
            <line x1="510" y1="195" x2="390" y2="292" stroke="var(--al-line)" strokeWidth="1.4" strokeDasharray="5 5" markerEnd="url(#alArchArrow)" />
            <line x1="610" y1="195" x2="630" y2="292" stroke="var(--al-line)" strokeWidth="1.4" strokeDasharray="5 5" markerEnd="url(#alArchArrow)" />
            <text className="al-node-sub" x="255" y="235" textAnchor="middle">JDBC/REST (예정)</text>
            <text className="al-node-sub" x="450" y="235" textAnchor="middle">정책 조회 API (예정)</text>
            <text className="al-node-sub" x="620" y="245" textAnchor="middle">STS AssumeRole (예정)</text>

            {/* AccessLens -> VaultStorageEngine (실제 연동, 실선) */}
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

            {/* Trino / OPA / S3 IAM (연동 예정) */}
            <rect x="65" y="292" width="170" height="56" rx="10" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" strokeDasharray="5 5" />
            <text className="al-node-label" x="150" y="316" textAnchor="middle" style={{ fill: "var(--al-text-faint)" }}>Trino</text>
            <text className="al-node-sub" x="150" y="332" textAnchor="middle">연동 예정</text>

            <rect x="305" y="292" width="170" height="56" rx="10" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" strokeDasharray="5 5" />
            <text className="al-node-label" x="390" y="316" textAnchor="middle" style={{ fill: "var(--al-text-faint)" }}>OPA</text>
            <text className="al-node-sub" x="390" y="332" textAnchor="middle">연동 예정</text>

            <rect x="545" y="292" width="170" height="56" rx="10" fill="var(--al-surface)" stroke="var(--al-border)" strokeWidth="1.5" strokeDasharray="5 5" />
            <text className="al-node-label" x="630" y="316" textAnchor="middle" style={{ fill: "var(--al-text-faint)" }}>S3 IAM</text>
            <text className="al-node-sub" x="630" y="332" textAnchor="middle">연동 예정</text>

            {/* VaultStorageEngine (실제) */}
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
            실선은 실제로 동작하는 경로(LDAP 인증, 감사 로그, VaultStorageEngine → 로컬 파일/Git/K8s
            Secrets)이고, 점선은 아직 백엔드 연동이 없는 계획 단계(Trino, OPA, S3 IAM)입니다.
          </figcaption>
        </figure>
      </div>
    </div>
  );
}
