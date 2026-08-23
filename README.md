# VaultViewer

LDAP 기반 RBAC를 지원하는 웹 시크릿/볼트 뷰어. 로컬 파일(마운트된 디렉토리) 모드와
쿠버네티스 클러스터(Kubernetes Secrets) 모드를 모두 지원합니다.

- **저장소**: https://github.com/changbin-yoon/vaultviewer
- **백엔드**: Go (`cmd/server`, `internal/`) — LDAP 인증/RBAC, 감사 로그, 그룹별
  팀 매핑(adm 전용 설정 화면), 로컬/K8s 스토리지 엔진, S3/MinIO 델타 백업
- **프론트엔드**: React + Tailwind (`web/`) — 옵시디언 스타일 마크다운(위키링크, 콜아웃,
  Mermaid), 그래프 뷰, 태그
- **배포**: Helm 차트 (`charts/vaultviewer/`) — 설치 방법과 값 설명은 해당 디렉토리의
  [README](charts/vaultviewer/README.md) 참고
- **변경 이력**: [`CHANGELOG.md`](CHANGELOG.md) 참고

## 로컬 개발

```bash
go run ./cmd/server        # 백엔드
cd web && npm install && npm run dev   # 프론트엔드
```

## 배포

```bash
docker buildx build --platform linux/amd64 -t <repo>/vaultviewer:<tag> --push .
helm install vaultviewer charts/vaultviewer -f my-values.yaml
```

자세한 내용은 [`charts/vaultviewer/README.md`](charts/vaultviewer/README.md) 참고.

## AI agent 연동 (MCP 서버)

`cmd/mcp-server`는 이미 떠 있는 VaultViewer 서버(`cmd/server`)의 REST API를
[MCP](https://modelcontextprotocol.io)로 감싼 별도 바이너리입니다. Claude Code
같은 MCP 클라이언트가 이 서버를 실행하면 볼트를 읽기 전용 툴 5개
(`search_vault`, `read_note`, `list_tree`, `get_note_history`,
`get_ontology_graph`)로 조회할 수 있습니다 — REST API를 직접 호출하거나
프론트매터를 스스로 파싱할 필요가 없습니다.

storage/auth 로직을 새로 구현하지 않고, VAULTVIEWER_URL의 `/api/*`를 단일
서비스 계정으로 호출만 하는 얇은 클라이언트입니다. RBAC은 그 계정의 role을
그대로 상속하므로(agent 권한의 상한), 읽기 전용 툴만 쓸 거라면 `view` role
LDAP 계정 하나로 충분합니다.

```bash
go build -o vv-mcp ./cmd/mcp-server
VAULTVIEWER_URL=http://localhost:8080 \
VAULTVIEWER_USERNAME=<service-account> \
VAULTVIEWER_PASSWORD=<password> \
  ./vv-mcp --transport stdio
```

Claude Code에 등록하려면 `.mcp.json`에 다음과 같이 추가합니다:

```json
{
  "mcpServers": {
    "vaultviewer": {
      "command": "/path/to/vv-mcp",
      "args": ["--transport", "stdio"],
      "env": {
        "VAULTVIEWER_URL": "http://localhost:8080",
        "VAULTVIEWER_USERNAME": "<service-account>",
        "VAULTVIEWER_PASSWORD": "<password>"
      }
    }
  }
}
```

`--transport http`(공식 Go SDK의 Streamable HTTP 핸들러, `-addr`로 리슨 주소
지정)도 코드로 지원하지만, 여러 agent가 공유 접속하는 배포(Docker 이미지에
포함하거나 Helm으로 띄우는 것)는 아직 만들지 않았습니다 — 필요해지면
`Dockerfile`에 `cmd/mcp-server` 빌드 스테이지를 추가하고 Helm 차트에
Deployment/Service를 추가하면 됩니다.

**쓰기 툴은 없습니다** — v1은 조회만 지원합니다. 여러 agent가 같은 MCP 서버
프로세스를 공유하면 감사 로그의 `user`가 전부 이 서비스 계정 이름으로
남는다는 한계가 있습니다(쓰기 툴이 없으므로 현재는 영향이 제한적).
