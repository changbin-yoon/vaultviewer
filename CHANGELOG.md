# Changelog

VaultViewer의 주요 변경 사항을 최신순으로 기록합니다. 버전 번호는 Docker 이미지 태그
(`yoochabi/vaultviewer:<version>`)이자 Helm 차트의 `appVersion`입니다.

## 저장소 관리

- GitHub Actions CI 추가 — push/PR마다 Go `build`/`vet`/`test`/`gofmt` 검사와
  프론트엔드 `tsc`/`vite build`를 자동 실행 (`.github/workflows/ci.yml`)
- GitHub에 공개 (github.com/changbin-yoon/vaultviewer) — kubeconfig, 개인 볼트
  데이터, 실제 클러스터 도메인이 담긴 배포 값 파일은 히스토리에서 완전히 제외
- 루트 `README.md` 추가

## 0.1.15

- Go 서버 graceful shutdown: SIGTERM/SIGINT 수신 시 `http.Server.Shutdown`으로
  진행 중인 요청을 드레인(10초 타임아웃)한 뒤 종료 — Recreate 전략 롤아웃 때
  이전 파드가 요청을 처리하다 갑자기 끊기지 않도록
- Helm 차트 기본 리소스 requests/limits 설정 (기존 `resources: {}` → 무제한
  상태였음). CPU limit은 의도적으로 비움 — Go 프로세스는 CFS 스로틀링이
  지연 스파이크로 나타나서, request만으로 스케줄링을 맡김

## 0.1.14

- 감사 로그 실시간 스트리밍 — `/ws/audit` WebSocket 연결로 8초 폴링 대체.
  `MemoryRecorder`가 새 기록마다 구독자에게 non-blocking으로 전달(느린
  구독자가 Save/Delete를 막지 못하게), ping/pong keepalive로 idle 프록시
  타임아웃 방지, 프론트엔드는 연결 끊기면 3초 후 자동 재연결

## 0.1.13

- 볼트 전체 텍스트 검색 추가 — `VaultStorageEngine.Search`를 `List`+`Read`만
  사용하는 공용 `WalkAndSearch` 헬퍼로 구현해 로컬/K8s 백엔드가 한 줄로 위임,
  이미지 등 바이너리 확장자는 검색 대상에서 제외, 매치 스니펫 반환. 프론트에
  "검색" 탭 추가 (디바운스, 하이라이트, 클릭 시 노트 이동)

## 0.1.12

- 설정 화면의 "배포 환경" 표시를 설명 문구 없이 태그 한 줄로 단순화

## 0.1.11

- S3/MinIO 백업을 델타 동기화로 전환 — 매 주기 전체 재업로드 대신 로컬 파일
  MD5와 S3 객체 ETag를 비교해 변경/신규 파일만 업로드. 삭제(DELETE)는 절대
  하지 않아 실패한 동기화가 이전 백업을 파괴할 수 없도록 설계

## 0.1.10

- 설정 화면에 "배포 환경"(LOCAL/CLUSTER, `VAULTVIEWER_DEPLOYMENT_LABEL`)과
  "스토리지 모드"(`mode: local|cluster`)를 별개 개념으로 분리 표시 — 이전엔
  스토리지 모드만 보여줘서 K8s에 떠 있어도 "LOCAL"로 잘못 보였음
- Helm `service.nodePort`로 NodePort 고정 옵션 추가 (재배포마다 랜덤 포트가
  바뀌는 문제 해결)

## 0.1.9

- LDAP 그룹 → 역할 매핑에 그룹 여러 개 지원 — `VAULTVIEWER_LDAP_GROUP_ADM`
  등에 콤마로 구분된 여러 그룹명(또는 Helm values에서 리스트) 입력 가능,
  사용자가 그중 하나에만 속해도 해당 역할 부여

## 0.1.8

- 사이드바 접기/펼치기 기능 추가, 상태는 `localStorage`에 저장돼 새로고침
  후에도 유지

## 0.1.7

- 그래프 뷰에 노트 타입별 색상 표시 — frontmatter의 `type:` 값을 해시 기반
  팔레트로 매핑, 범례 표시

## 0.1.0 – 0.1.6

- 초기 구현: LDAP 기반 RBAC(검색-후-바인드, 그룹→역할 매핑, 서명된 세션
  토큰), 로컬 파일시스템 / Kubernetes Secrets 스토리지 엔진, Obsidian 스타일
  마크다운 렌더링(위키링크·콜아웃·Mermaid·백링크·이미지 임베드), 그래프 뷰,
  태그 패널, 감사 로그, env 변수 기반 설정, Helm 차트, Docker 멀티스테이지
  빌드, cluster-mesh1 실배포
