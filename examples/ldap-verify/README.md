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

그룹 CN과 정책명이 같은 것은 이 배포의 편의일 뿐, 어느 시스템도 거기에
의존하지 않는다. MinIO는 그룹 **DN**에 정책을 붙이고, AccessLens도
`policy/attachments.yaml`의 선언을 DN으로 대조한다 — 이름이 달라도 되고,
한 그룹이 여러 정책을 들어도 되고, 사용자 DN에 직접 붙어도 된다.

`auth.ResolveTeams`가 파싱하는 `<팀>-<역할>` 규칙은 AccessLens 자체의 역할
(adm/dev/view)과 대시보드 팀 표시에만 쓰인다. S3 권한 계산과는 무관하다.

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

# 3) MinIO의 현재 정책·attach 상태를 레포로 내려받기
ALIAS=<mc alias> sh sync-from-minio.sh

# 4) 그 사본을 ConfigMap으로 AccessLens에 전달
#    (정책 문서 = 무엇을 허용하는가, attach 선언 = 누가 들고 있는가)
kubectl -n accesslens create configmap accesslens-policies \
  --from-file=../../policy/generated/ \
  --from-file=../../policy/attachments.yaml

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

## 방향 — MinIO가 진실이다

AccessLens는 **권한을 바꾸지 않는다.** 사용자에게 "당신은 지금 무엇을 할 수
있는가"를 설명할 뿐이고, 그러려면 MinIO가 실제로 들고 있는 것과 같은
정책·attach 정보를 갖고 있어야 한다.

```
MinIO/AIStor  ──(sync-from-minio.sh)──▶  policy/generated/*.json
   (진실)                                 policy/attachments.yaml
                                                  │
                                            (ConfigMap)
                                                  ▼
                                            AccessLens 화면
```

정책을 바꾸는 것은 MinIO 쪽에서 하고(`mc admin policy` / `mc idp ldap policy
attach`), 그 다음 이 스크립트로 사본을 갱신한다. 레포에서 MinIO로 밀어넣는
경로는 의도적으로 두지 않았다 — 권한 변경 도구와 권한 조회 도구는 같은 것이
아니어야 한다.

`policy/gen_policies.py` 는 이 정책 셋을 **최초에 저작할 때** 쓴 도구다.
평소 운영 경로가 아니며, 실수로 사본을 덮어쓰지 않도록 `FORCE=1` 없이는
실행되지 않는다.

## 선언 대조 (드리프트 검사)

대시보드의 S3 IAM 카드에서 관리자만 볼 수 있는 "선언 대조" 버튼이
`policy/attachments.yaml`(사본)과 MinIO 실물을 비교한다. 사본이 낡았는지
알려주는 장치다. 두 방향을 구분한다:

- **선언에 없음** — 백엔드가 아무도 적어두지 않은 attach를 들고 있다.
  선언만 읽어서는 영원히 보이지 않는 쪽이라 더 위험하다.
- **미적용** — 선언에는 있는데 백엔드에 없다. 화면이 실제로는 없는 권한을
  약속하고 있다는 뜻이다.

필요한 자격증명은 `policy/accesslens-drift-audit.json` 정책을 가진 서비스
계정뿐이다(`admin:ListUsers` + `admin:GetPolicy`, 객체 데이터 접근 불가).
`sync-from-minio.sh` 도 같은 계정을 쓴다. 사용자 자격증명은 필요 없다 —
이유는 그 정책의 README 참고.

차이가 나오면 **사본을 다시 받으면 된다**:

```sh
ALIAS=<alias> sh sync-from-minio.sh
```

## 드리프트 검증용 고정물

이 환경에는 선언에 없는 attach가 1건 의도적으로 남아 있다.
[DRIFT-FIXTURE.md](DRIFT-FIXTURE.md) 참고 — 모든 것이 일치하는 환경에서는
대조 기능이 동작하는지 확인할 방법이 없기 때문이다.

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
