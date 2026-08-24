# AccessLens 가이드라인

## 프로젝트 개요
AccessLens(이전 이름: VaultViewer)는 LDAP 기반 RBAC를 지원하고 두 가지 실행 모드(로컬 파일/컨테이너 매핑 모드 & 쿠버네티스 클러스터 모드)를 지원하는 웹 기반 시크릿/권한 뷰어입니다. Go 모듈 경로는 `github.com/accesslens/accesslens`, 서버 env var 접두사는 `ACCESSLENS_`입니다. Helm 차트 디렉토리명(`charts/vaultviewer/`)과 리소스 이름, Docker 이미지 저장소(`yoochabi/vaultviewer`)는 이미 배포된 클러스터(cluster-mesh1)와의 호환을 위해 의도적으로 이전 이름을 유지합니다 — 바꾸기 전에 PVC/Secret 마이그레이션 계획부터 세우세요.

## 실행 모드
1. **로컬 모드**:
   - 로컬 디렉토리 경로를 컨테이너 볼륨 경로에 매핑합니다.
   - 마운트된 경로에서 시크릿/볼트 파일을 직접 읽습니다.
2. **클러스터 모드**:
   - 쿠버네티스 내부에 배포됩니다.
   - Kubernetes Secrets API 또는 In-Cluster Vault 연동을 통해 시크릿에 접근합니다.

## LDAP 기반 역할 기반 접근 제어(RBAC)
사용자는 세 가지 역할에 매핑되는 LDAP 그룹에 속합니다:
- **`adm`**: 읽기, 생성, 수정, 삭제 (전체 권한)
- **`dev`**: 읽기, 생성, 수정 (삭제 불가)
- **`view`**: 읽기 전용

## 기능 요구사항
- **핵심 기능**: 파일/볼트 트리 뷰, 시크릿 조회, RBAC에 따른 편집.
- **감사(Audit)**: 모든 변경 이력(생성, 수정, 삭제)을 사용자 신원 및 타임스탬프와 함께 로깅.
- **확장성**: 향후 단계에서 Git 백엔드(예: 편집 시 자동 커밋)를 원활하게 통합할 수 있도록 스토리지 인터페이스 추상화를 설계.

## 기술 스택 및 규칙
- **백엔드**: Go / FastAPI (REST API + WebSocket을 통한 감사 로그 스트림)
- **프론트엔드**: React + Tailwind CSS
- **코드 스타일**: `storage`(로컬/k8s/git), `auth`(LDAP/RBAC), `audit` 간의 기능적 분리.

## Claude Code 지침
- API 엔드포인트에서는 항상 RBAC 권한을 준수할 것.
- Git 및 K8s 백엔드를 컨트롤러 로직 수정 없이 추가할 수 있도록 스토리지 프로바이더가 통합 인터페이스(예: `VaultStorageEngine`)를 구현하도록 할 것.
- LDAP 자격 증명이나 암호화 키를 하드코딩하지 말 것.
