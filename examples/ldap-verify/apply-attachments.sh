#!/bin/sh
# policy/attachments.yaml 의 선언을 MinIO에 실제로 적용한다.
#
# 선언 파일만 고치면 AccessLens 화면은 바뀌지만 MinIO는 그대로다 — 그 상태가
# 바로 대시보드의 "선언 대조"가 잡아내는 드리프트다. 선언을 진실로 삼으려면
# 이 스크립트로 백엔드에 밀어넣어야 한다.
#
# 이 스크립트는 선언에 있는 것을 attach 하기만 한다. 선언에 없는데 백엔드에
# 붙어 있는 attach(대조 결과의 "선언에 없음")는 건드리지 않는다 — 권한을
# 떼는 것은 되돌리기 어렵고, 무엇이 왜 붙어 있는지 사람이 보고 판단할
# 문제다. 떼려면 대조 결과를 보고 직접:
#
#   mc idp ldap policy detach <alias> <policy> --group '<DN>'
#
# 실행: ALIAS=myminio sh apply-attachments.sh
set -e
: "${ALIAS:?ALIAS를 넘기세요 (mc alias 이름)}"
FILE="${FILE:-$(dirname "$0")/../../policy/attachments.yaml}"

# 선언 형식이 고정(policy: 한 줄 + groups/users 아래 DN 목록)이라 awk로 충분하다.
awk '
  /^  - policy:/ { policy=$3; kind=""; next }
  /^    groups:/ { kind="--group"; next }
  /^    users:/  { kind="--user";  next }
  /^      - / {
    dn=$0
    sub(/^      - "?/, "", dn); sub(/"?$/, "", dn)
    if (policy != "" && kind != "") print policy "\t" kind "\t" dn
  }
' "$FILE" | while IFS="$(printf '\t')" read -r policy kind dn; do
  echo "attach $policy -> $dn"
  mc idp ldap policy attach "$ALIAS" "$policy" "$kind" "$dn" || \
    echo "  (이미 붙어 있거나 실패 — 위 메시지 확인)"
done

echo
echo "대조:"
echo "  mc idp ldap policy entities $ALIAS"
echo "  또는 대시보드 S3 IAM 카드의 '선언 대조' (관리자)"
