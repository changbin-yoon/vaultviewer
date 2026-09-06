# 드리프트 검증용 고정물

cluster-mesh2의 MinIO에는 **의도적으로 선언에 없는 attach가 1건** 남아 있다.

```
cn=view,ou=groups,dc=example,dc=com  ->  readonly
```

`policy/attachments.yaml`에는 없다. 선언과 실물을 대조하는 기능이 이걸
잡아내야 정상이다 — 모든 것이 일치하는 환경에서는 그 기능이 동작하는지
확인할 방법이 없다.

## 왜 하필 이걸 남겼나

2026-09-06 첫 대조에서 선언에 없는 attach가 3건 발견됐다:

```
cn=adm   -> readwrite    {"Action":["s3:*"],"Resource":["arn:aws:s3:::*"]}
cn=dev   -> readwrite    (같음)
cn=view  -> readonly
```

앞의 두 개는 **제거했다**. `readwrite`는 전 버킷 전권인데, 하필 이 차트의
`ldap.groupRoleMap` 기본값이 `adm`/`dev`/`view`다. 기본값대로 배포하는
사람이 자연스럽게 그 이름의 그룹을 만드는 순간 구성원 전원이 모든 버킷에
대한 전권을 얻는다. 대시보드는 훨씬 좁은 팀 정책 기준 권한을 보여주면서.
발견 시점에 해당 그룹들이 LDAP에 존재하지 않아 실제 노출은 없었다.

`cn=view -> readonly`만 남긴 것은 읽기 전용이라 위험도가 낮기 때문이다.
그래도 함정의 모양은 같으므로(그룹 이름이 차트 기본값과 겹친다), 이
디렉토리를 운영 환경 구성의 본으로 삼지 말 것.

## 되돌리려면

```sh
mc idp ldap policy detach <alias> readonly --group 'cn=view,ou=groups,dc=example,dc=com'
```

## 이 발견이 말해주는 것

이전 구조(그룹 이름에서 정책을 추론)로는 이 3건을 **원리적으로 찾을 수
없었다**. "선언에 없는 것"이라는 개념 자체가 성립하지 않기 때문이다.
attach를 선언으로 옮긴 첫 대조에서 바로 나왔다.
