# VaultViewer

LDAP 기반 RBAC를 지원하는 웹 시크릿/볼트 뷰어. 로컬 파일(마운트된 디렉토리) 모드와
쿠버네티스 클러스터(Kubernetes Secrets) 모드를 모두 지원합니다.

- **저장소**: https://github.com/changbin-yoon/vaultviewer
- **백엔드**: Go (`cmd/server`, `internal/`) — LDAP 인증/RBAC, 감사 로그, 로컬/K8s
  스토리지 엔진, S3/MinIO 델타 백업
- **프론트엔드**: React + Tailwind (`web/`) — 옵시디언 스타일 마크다운(위키링크, 콜아웃,
  Mermaid), 그래프 뷰, 태그
- **배포**: Helm 차트 (`charts/vaultviewer/`) — 설치 방법과 값 설명은 해당 디렉토리의
  [README](charts/vaultviewer/README.md) 참고

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
