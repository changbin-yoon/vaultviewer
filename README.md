# AccessLens

LDAP 기반 RBAC를 지원하는 웹 시크릿/권한 뷰어. 로컬 파일(마운트된 디렉토리) 모드와
쿠버네티스 클러스터(Kubernetes Secrets) 모드를 모두 지원합니다. (Helm 차트/이미지의
내부 명칭은 이전 이름인 `vaultviewer`를 그대로 씁니다 — 이유는
[`charts/vaultviewer/README.md`](charts/vaultviewer/README.md) 참고.)

- **저장소**: https://github.com/changbin-yoon/vaultviewer
- **백엔드**: Go (`cmd/server`, `internal/`) — LDAP 인증/RBAC(그룹 검색 필터·역할·팀
  매핑까지 설정 가능), 감사 로그, 로컬/K8s 스토리지 엔진, S3/MinIO 델타 백업
- **프론트엔드**: React + Tailwind (`web/`) — 옵시디언 스타일 마크다운(위키링크, 콜아웃,
  Mermaid), 그래프 뷰, 태그, 역할/팀별 권한을 보여주는 대시보드
- **AI agent 연동**: `cmd/mcp-server` — [아래](#ai-agent-연동-mcp-서버) 참고
- **배포**: Helm 차트 (`charts/vaultviewer/`) — 값 하나하나에 대한 자세한 설명은
  [charts/vaultviewer/README.md](charts/vaultviewer/README.md) 참고. 이 문서는
  "처음 배포하는 사람"이 순서대로 따라올 수 있게 하는 최소 경로만 다룹니다.
- **변경 이력**: [`CHANGELOG.md`](CHANGELOG.md) 참고

## 로컬 개발

```bash
go run ./cmd/server        # 백엔드 (기본 :8080)
cd web && npm install && npm run dev   # 프론트엔드 (기본 :5173, 백엔드로 프록시)
```

로컬 개발 시에도 LDAP은 필요합니다 (`ACCESSLENS_LDAP_*` 환경변수) — 테스트용
OpenLDAP을 띄우거나 사내 LDAP/AD를 가리키세요. 필수 환경변수는
`internal/auth/config.go`의 `LoadConfigFromEnv` 주석에 전부 나열되어 있습니다.

## 쿠버네티스에 처음 배포하기

아래는 처음부터 끝까지 순서대로 따라 하면 되는 최소 경로입니다. 각 단계에서
더 깊은 옵션(airgapped 레지스트리, S3 백업, git 백엔드, 네이티브 K8s 리소스 등)은
[`charts/vaultviewer/README.md`](charts/vaultviewer/README.md)를 링크해뒀습니다.

### 0. 준비물

- LDAP/AD 서버 — `adm`/`dev`/`view`로 매핑할 그룹이 있는 디렉토리 (그룹 CN이 정확히
  `adm`/`dev`/`view`가 아니어도 됩니다)
- 이미지를 pull할 수 있는 쿠버네티스 클러스터 + `kubectl`/`helm` CLI
- 이미지를 빌드해 올릴 컨테이너 레지스트리 (기본은 Docker Hub, 프라이빗 레지스트리도 가능)

### 1. 이미지 빌드 & 푸시

```bash
# 클러스터 노드 아키텍처를 반드시 확인하세요 (Apple Silicon Mac에서 빌드하면
# 기본이 arm64라, amd64 클러스터라면 --platform을 꼭 지정해야 합니다)
docker build --platform linux/amd64 -t <repo>/vaultviewer:<tag> .
docker push <repo>/vaultviewer:<tag>
```

### 2. LDAP bind 비밀번호 / 세션 서명 키 Secret 생성

```bash
kubectl create secret generic vaultviewer-ldap \
  --from-literal=bind-password='<LDAP 서비스 계정 비밀번호>'
kubectl create secret generic vaultviewer-session \
  --from-literal=session-secret="$(openssl rand -hex 32)"
```

### 3. values 파일 준비

[`charts/vaultviewer/values-example.yaml`](charts/vaultviewer/values-example.yaml)을
복사해서 `image`/`ldap.host`/`ldap.baseDN`/`ldap.bindDN`을 채우세요:

```bash
cp charts/vaultviewer/values-example.yaml my-values.yaml
# my-values.yaml 편집
```

그룹 CN이 `adm`/`dev`/`view`가 아니면 `ldap.groupRoleMap`을 매핑하고, 그룹 검색
자체의 LDAP 필터(예: AD/posixGroup 등 다른 스키마)를 바꿔야 하면
`ldap.groupSearchFilter`를 설정하세요 — 둘 다 값 파일에 예시 주석이 있습니다. 그룹
CN이 `<팀>-<역할>` 패턴(예: `bi-adm`)이면 대시보드가 자동으로 "소속 팀 및 권한"을
보여줍니다 — 별도 설정 불필요, 그룹 CN 명명 규칙만 맞으면 됩니다.

### 4. 설치

`helm template`/`helm lint`는 `values.schema.json`으로 값 형식을 미리 검증하므로,
필수 필드(`mode`, `image.*`, `ldap.host`/`baseDN`/`bindDN`)가 비어 있으면 설치 전에
바로 에러로 알려줍니다.

```bash
helm install vaultviewer charts/vaultviewer -f my-values.yaml
```

### 5. 접속 확인

```bash
# Ingress가 아직 없다면 포트포워드로 빠르게 확인:
kubectl port-forward svc/vaultviewer 8080:8080
curl http://localhost:8080/healthz   # {"status":"ok"} 가 나오면 정상
```

브라우저로 `http://localhost:8080` 접속 후 LDAP 계정으로 로그인해보세요. Ingress
설정, NodePort 고정, 예제 데이터로 시작하기 등은
[charts/vaultviewer/README.md](charts/vaultviewer/README.md#접속) 참고.

### 다음 단계

- **업그레이드**: 이미지/차트를 새로 빌드했으면 `helm upgrade vaultviewer charts/vaultviewer -f my-values.yaml`
- **RBAC 확인**: `values.schema.json`은 `helm lint`/`helm template` 단계에서 값 형식을
  검증하지만, 실제로 어느 그룹이 어떤 권한을 받는지는 로그인해서 확인하는 게
  제일 확실합니다.
- **네이티브 K8s 리소스** (NetworkPolicy/PodDisruptionBudget/HPA/startupProbe)는
  전부 기본 비활성 — 필요하면 [charts/vaultviewer/README.md](charts/vaultviewer/README.md)의
  해당 섹션 참고.
- **Airgapped 클러스터**, **S3/MinIO 백업**, **git 백엔드**는 모두
  [charts/vaultviewer/README.md](charts/vaultviewer/README.md)에 별도 섹션으로
  정리되어 있습니다.

## AI agent 연동 (MCP 서버)

`cmd/mcp-server`는 이미 떠 있는 AccessLens 서버(`cmd/server`)의 REST API를
[MCP](https://modelcontextprotocol.io)로 감싼 별도 바이너리입니다. storage/auth
로직을 새로 구현하지 않고, `ACCESSLENS_URL`의 `/api/*`를 단일 계정으로 호출만
하는 얇은 클라이언트입니다 — **RBAC은 전부 REST API의 기존 미들웨어가 그대로
적용**되므로, 이 서버 자체는 인가 로직을 갖지 않습니다. 즉 어떤 도구가 성공/실패
하는지는 접속에 쓴 LDAP 계정의 role에 그대로 달려 있습니다.

읽기 도구 5개:

- `get_ontology_graph`, `search_vault`, `read_note`, `list_tree`, `get_note_history`

쓰기 도구 3개 (연결된 계정이 `view`면 403으로 실패):

- `save_note(path, content, reason?)` — 해당 경로에 노트가 없으면 생성, 있으면
  덮어씀 (REST의 PUT이 원래 create/update를 구분하지 않아서 이 도구도 하나로
  통합했습니다). `dev` 이상 필요.
- `delete_note(path)` — `adm` 전용.
- `rename_note(from, to, reason?)` — 같은 디렉토리 내에서만 가능. `dev` 이상 필요.

```bash
go build -o vv-mcp ./cmd/mcp-server
ACCESSLENS_URL=http://localhost:8080 \
ACCESSLENS_USERNAME=<ldap-username> \
ACCESSLENS_PASSWORD=<password> \
  ./vv-mcp --transport stdio
```

기동하면 어떤 계정으로 인증됐는지 로그에 남습니다:

```
authenticated to accesslens (http://localhost:8080) as "ycb_dev"
```

**여러 agent/사람이 쓴다면, MCP 서버 프로세스 하나당 계정 하나를 쓰세요** — 감사
로그(`/api/audit`)는 세션에 실제로 로그인한 계정 이름을 기록하므로, 프로세스별로
계정을 분리하기만 하면 "누가 무엇을 바꿨는지"가 코드 변경 없이 그대로 구분됩니다.
반대로 여러 agent가 계정 하나를 공유하면 감사 로그에서 서로 구분이 안 됩니다 —
기동 로그의 계정명이 예상과 다르면 바로 알아챌 수 있습니다.

Claude Code에 등록하려면 `.mcp.json`에 다음과 같이 추가합니다:

```json
{
  "mcpServers": {
    "vaultviewer": {
      "command": "/path/to/vv-mcp",
      "args": ["--transport", "stdio"],
      "env": {
        "ACCESSLENS_URL": "http://localhost:8080",
        "ACCESSLENS_USERNAME": "<ldap-username>",
        "ACCESSLENS_PASSWORD": "<password>"
      }
    }
  }
}
```

`--transport http`(공식 Go SDK의 Streamable HTTP 핸들러, `-addr`로 리슨 주소
지정)도 코드로 지원하지만, 여러 agent가 공유 접속하는 배포(Docker 이미지에
포함하거나 Helm으로 띄우는 것)는 아직 만들지 않았습니다 — 위에서 설명한 대로
프로세스당 계정 하나가 기본 운영 방식이라, 공유 서비스로 배포하려면 이 가정부터
다시 검토해야 합니다.
