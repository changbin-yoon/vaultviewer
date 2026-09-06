# accesslens-master 정책

AccessLens가 **권한 프로브가 남긴 객체를 치우기 위해** 쓰는 계정의 정책이다.
프로브 자체는 이 계정이 아니라 로그인한 사용자의 STS 세션으로 돈다 — 마스터로
프로브하면 "마스터가 무엇을 할 수 있나"를 재는 것이라 기능이 무의미해진다.

## 왜 필요한가

get/delete 프로브는 부작용이 없다(없는 키를 대상으로 하므로 인가 검사만
통과하고 끝난다). put만은 실제로 써야 판정되는데, 여기서 문제가 생긴다:

    bi-dev 티어 = PutObject 있음, DeleteObject 없음

즉 쓰기 권한을 확인한 그 계정이 자기가 쓴 프로브 객체를 지우지 못한다.
2026-09-06 실측에서 실제로 0바이트 객체가 team-bi에 남았다. 마스터 계정은
그 정리만 담당한다.

## 범위를 좁히는 편이 낫다

지금 Resource가 `arn:aws:s3:::*/*` 라 모든 버킷의 모든 객체에 대한
put/get/delete다. 정리에 필요한 것은 프로브 키 하나뿐이므로, 다음으로
바꾸면 기능은 그대로면서 사고 반경이 프로브 접두사 안으로 한정된다:

    "Resource": ["arn:aws:s3:::*/.accesslens-probe/*"]

운영 환경으로 가져갈 때는 이쪽을 쓸 것. 넓은 범위를 그대로 두면 AccessLens가
"모든 버킷의 아무 객체나 지울 수 있는 자격증명"을 들고 있게 된다.

## 적용

    mc admin policy create <alias> accesslens-master policy/accesslens-master.json
    mc admin user svcacct add <alias> <root-user> \
      --access-key <ak> --secret-key <sk> --policy policy/accesslens-master.json

발급한 키는 Kubernetes Secret으로만 주입한다(레포에 두지 않는다):

    kubectl -n accesslens create secret generic vaultviewer-s3iam-master \
      --from-literal=accessKey='<ak>' --from-literal=secretKey='<sk>'
