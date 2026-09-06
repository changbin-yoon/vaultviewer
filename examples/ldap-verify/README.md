# cluster-mesh2 검증 환경 구성

AccessLens를 실제 LDAP + MinIO에 붙여 검증하는 스택의 구성 기록.
**여기 있는 파일이 진실이고, 살아있는 클러스터 상태가 아니다** — 이 디렉토리는
바로 그 교훈에서 나왔다(아래 "왜 이 디렉토리가 있는가" 참고).

## 구성도

```
10.10.105.4 (클러스터 밖)          cluster-mesh2
┌──────────────────────┐          ┌────────────────────────────────┐
│ openldap (docker)    │◀─────────│ accesslens/  AccessLens        │
│  :389                │◀─────────│ minio-verify/ MinIO (LDAP IAM) │
│  /opt/openldap/data  │◀─────────│ trino-verify/ Trino            │
└──────────────────────┘          └────────────────────────────────┘
```

LDAP이 세 소비자의 단일 신원 소스다. 클러스터 밖에 두는 이유는 클러스터를
재구축해도 신원이 살아남게 하기 위해서다.

## 신원 모델

`seed.ldif`가 만드는 것:

| 그룹 CN | MinIO 정책 | 구성원 |
|---|---|---|
| `bi-adm` | `bi-adm` | `ycb` |
| `bi-dev` | `bi-dev` | `ycb_dev` |
| `bi-view` | `bi-view` | `ycb_view` |
| `ml-adm` | `ml-adm` | `hsycb3650` |
| `ml-dev` | `ml-dev` | (placeholder) |
| `ml-view` | `ml-view` | `ycb_dev` |
| `ops-adm` | `ops-adm` | `hsycb3650` |
| `ops-dev` | `ops-dev` | (placeholder) |
| `ops-view` | `ops-view` | `ycb_view` |

**그룹 CN = MinIO 정책명**인 것이 이 설계의 핵심이다. AccessLens의
`auth.ResolveTeams`가 `<팀>-<역할>`을 파싱하고, `s3iam` 패키지가 같은 이름으로
정책 문서를 찾는다. 이름이 어긋나면 권한 화면이 조용히 빈다.

`ycb_dev`(bi-dev + ml-view)와 `ycb_view`(bi-view + ops-view)는 일부러 두 팀에
걸쳐 있다 — 다중 팀 합집합 경로를 실제로 태우기 위한 것이라 옮길 때 유지할 것.

`placeholder` 사용자는 `groupOfNames`의 "member 최소 1개" 제약을 채우는 더미다.
로그인용이 아니다.

## 구축 순서

```sh
# 1) LDAP 서버 (10.10.105.4에서)
LDAP_ADMIN_PASSWORD='...' LDAP_USER_PASSWORD='...' sh openldap-server.sh

# 2) MinIO를 그 LDAP으로
LDAP_ADDR=10.10.105.4:389 LDAP_BIND_PASSWORD='...' sh minio-ldap.sh

# 3) 팀 정책 적용
sh apply-policies.sh

# 4) 정책 사본을 AccessLens 네임스페이스에도 (카드의 권한 내역 계산용)
kubectl -n accesslens create configmap accesslens-policies \
  --from-file=../../policy/generated/

# 5) AccessLens (README 상단의 Secret 3개를 먼저 만들 것)
helm upgrade --install accesslens ../../charts/vaultviewer \
  -n accesslens --create-namespace -f accesslens-values.yaml
```

Trino는 자체 Helm 릴리스이며 `group-provider.properties` /
`password-authenticator.properties`의 `ldap.url`을 같은 주소로 맞춰야 한다.

## 검증

```sh
kubectl -n accesslens port-forward svc/accesslens-vaultviewer 8080:8080
# ycb / ycb_dev / ycb_view 로 로그인
```

`/api/s3iam`의 `connected=true`는 MinIO STS `AssumeRoleWithLDAPIdentity`가
실제 LDAP으로 성공했다는 뜻이다 — 엔드포인트가 살아있다는 것보다 강한 신호다.

기대값:

| 계정 | teams | buckets |
|---|---|---|
| `ycb` | `[bi]` | `[team-bi]` |
| `ycb_dev` | `[bi, ml]` | `[team-bi, team-ml]` |
| `ycb_view` | `[bi, ops]` | `[team-bi, team-ops]` |

카드의 "접근 권한" 내역은 `policy/generated/`의 사본에서 계산한 값이지
MinIO에 질의한 결과가 아니다. 그래서 정책 개수와 로드 시각을 함께 보여준다.
정책을 재생성했다면 **MinIO(`apply-policies.sh`)와 이 ConfigMap을 함께**
갱신할 것 — 한쪽만 바꾸면 화면과 실제 권한이 조용히 어긋난다.

## 왜 이 디렉토리가 있는가

2026-09-06 이전, 이 스택의 구성은 살아있는 클러스터 상태로만 존재했다.
그 결과:

- MinIO와 Trino가 **노드 NodePort**(`10.10.105.172:30518`)를 LDAP 주소로
  들고 있었는데, 그 노드가 교체되면서 두 시스템의 LDAP 인증이 통째로 죽었다.
  아무도 몰랐다 — 둘 다 "연결 실패"를 조용히 삼켰기 때문이다.
- 클러스터 내 openldap에는 **PVC가 없었고**, seed configmap이 부트스트랩 경로에
  마운트되어 있지도 않아서 디렉토리 트리가 완전히 비어 있었다. 존재하지만
  아무 사용자도 없는 LDAP이었다.

그래서 이 디렉토리의 규칙은 두 가지다:

1. **주소는 안정적인 것을 쓴다.** 노드 IP와 NodePort는 노드 수명에 묶인다.
2. **구성은 레포에 있고, 클러스터에는 적용될 뿐이다.**

## 비밀번호

이 디렉토리에는 비밀번호가 없다(프로젝트 규칙: 자격 증명 하드코딩 금지).
전부 환경변수나 Kubernetes Secret으로 주입한다 — `seed.ldif`의 계정 비밀번호도
`__USER_PASSWORD__` 플레이스홀더이며 `openldap-server.sh`가 기동 시점에
`$LDAP_USER_PASSWORD`로 치환한다.

검증용 계정 4개가 같은 비밀번호를 쓰는 것은 테스트 편의를 위한 것이다.
실제 디렉토리에서는 이 seed를 그대로 쓰지 말 것.
