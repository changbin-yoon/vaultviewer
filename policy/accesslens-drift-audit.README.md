# accesslens-drift-audit 정책

AccessLens가 **선언(attachments.yaml)과 S3 백엔드 실물을 대조**하기 위해 쓰는
계정의 정책이다.

```json
{"Effect":"Allow","Action":["admin:ListUsers","admin:GetPolicy"],
 "Resource":["arn:aws:s3:::*"]}
```

## 왜 이 두 개뿐인가

2026-09-06 실측으로 좁혔다:

| 액션 조합 | policy entities | policy info | 버킷 데이터 | 서버 정보 |
|---|---|---|---|---|
| `admin:ListUsers` | OK | 거부 | 거부 | 거부 |
| `+ admin:GetPolicy` | OK | OK | **거부** | **거부** |

`admin:ListUsers`는 이름과 달리 "LDAP 주체↔정책 매핑 조회" 권한이고,
attach 대조에 필요한 전부다. `admin:GetPolicy`는 정책 문서까지 비교할 때
추가한다. **이 조합으로는 객체 데이터를 한 바이트도 읽을 수 없다.**
`admin:*`을 주지 말 것.

## 왜 사용자 자격증명이 필요 없는가

"이 사용자가 실제로 무엇을 할 수 있나"를 그 사람의 세션으로 호출해 확인하는
방식(프로브)을 먼저 시도했다가 철회했다. AccessLens는 로그인 후 사용자
비밀번호를 보관하지 않으므로 그 사람의 STS 세션을 만들 수 없고, 고정
서비스 계정으로 대신 호출하면 **엉뚱한 신원의 권한을 재게 된다**
(2026-09-06에 실제로 그렇게 배포됐다가 되돌렸다 — 세 계정이 전부 같은 답을
받았다).

대조 방식은 같은 질문("화면이 실물과 어긋나는가")에 사용자 자격증명 없이
답한다. 백엔드가 자기 상태를 직접 말해주기 때문이다.

## 적용

```sh
mc admin policy create <alias> accesslens-drift-audit policy/accesslens-drift-audit.json
mc admin user svcacct add <alias> <root-user> \
  --access-key <ak> --secret-key <sk> --policy policy/accesslens-drift-audit.json

kubectl -n accesslens create secret generic vaultviewer-s3iam-admin \
  --from-literal=accessKey='<ak>' --from-literal=secretKey='<sk>'
```
