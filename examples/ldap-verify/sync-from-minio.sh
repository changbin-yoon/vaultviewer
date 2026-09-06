#!/bin/sh
# MinIO/AIStor에 실제로 등록·attach 되어 있는 상태를 AccessLens가 읽는
# 파일들로 내려받는다.
#
# 방향이 중요하다. **MinIO가 진실이고 이 레포는 사본을 받는 쪽**이다.
# AccessLens는 권한을 바꾸지 않는다 — 사용자에게 "당신은 지금 무엇을 할 수
# 있는가"를 설명할 뿐이고, 그러려면 MinIO가 실제로 들고 있는 것과 같은
# 정책·attach 정보가 필요하다. 이 스크립트가 그 사본을 갱신한다.
#
# 내려받는 것:
#   policy/generated/<policy>.json   각 정책이 무엇을 허용하는가
#   policy/attachments.yaml          누가 그 정책을 들고 있는가 (LDAP DN 기준)
#
# 실행: ALIAS=myminio sh sync-from-minio.sh
#
# 필요한 권한은 읽기 전용 두 개뿐이다 — admin:ListUsers, admin:GetPolicy.
# 대시보드의 "선언 대조"가 쓰는 계정을 그대로 쓰면 된다.
set -e
: "${ALIAS:?ALIAS를 넘기세요 (mc alias 이름)}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_POLICIES="$ROOT/policy/generated"
OUT_ATTACHMENTS="$ROOT/policy/attachments.yaml"
HELPER="$(dirname "$0")/sync_from_minio.py"

# 정책 목록은 attach 응답에서 얻는다. mc admin policy ls 는 admin:ListPolicies가
# 필요한데 이 계정에는 없고(실측 확인), 애초에 필요하지도 않다 — AccessLens가
# 설명해야 할 것은 누군가에게 실제로 붙어 있는 정책뿐이다.
#
# MinIO 빌트인 정책(readwrite/readonly 등)은 사본에 넣지 않는다. 이 조직이
# 정의한 정책이 아니고, 누군가 갖고 있다면 대조에서 "선언에 없음"으로 잡히는
# 편이 낫다 — 그게 곧 알아야 할 사실이다.
mkdir -p "$OUT_POLICIES"

echo "attach 상태 내려받는 중…"
ENTITIES="$(mc idp ldap policy entities "$ALIAS" --json)"
NAMES="$(printf '%s' "$ENTITIES" | python3 "$HELPER" names)"
printf '%s' "$ENTITIES" | python3 "$HELPER" attachments "$OUT_ATTACHMENTS"

echo "정책 문서 내려받는 중…"
count=0
for name in $NAMES; do
  mc admin policy info "$ALIAS" "$name" --json \
    | python3 "$HELPER" policy > "$OUT_POLICIES/$name.json"
  echo "  $name"
  count=$((count + 1))
done
echo "정책 $count개."

echo
echo "다음: ConfigMap을 갱신해야 AccessLens가 새 사본을 읽는다."
echo "  kubectl -n <ns> create configmap accesslens-policies \\"
echo "    --from-file=policy/generated/ --from-file=policy/attachments.yaml \\"
echo "    --dry-run=client -o yaml | kubectl -n <ns> apply -f -"
echo "  kubectl -n <ns> rollout restart deploy/accesslens-vaultviewer"
