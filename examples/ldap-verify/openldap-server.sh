#!/bin/sh
# cluster-mesh2가 인증에 쓰는 OpenLDAP을 쿠버네티스 밖의 서버에 올린다.
#
# 왜 클러스터 밖인가: LDAP은 AccessLens / MinIO / Trino가 공유하는 단일
# 신원 소스라, 클러스터를 재구축할 때마다 같이 날아가면 곤란하다.
#
# 사전 조건: 대상 서버에 docker, 그리고 389 포트가 비어 있을 것.
# 실행: LDAP_ADMIN_PASSWORD=... sh openldap-server.sh
set -e

: "${LDAP_ADMIN_PASSWORD:?LDAP_ADMIN_PASSWORD를 환경변수로 넘기세요 (레포에 비밀번호를 넣지 않습니다)}"
: "${LDAP_USER_PASSWORD:?LDAP_USER_PASSWORD를 넘기세요 — seed.ldif의 테스트 계정 비밀번호로 치환됩니다}"
LDAP_DOMAIN="${LDAP_DOMAIN:-example.com}"
LDAP_ORGANISATION="${LDAP_ORGANISATION:-dataplane-console}"
BASE_DIR="${BASE_DIR:-/opt/openldap}"

# seed.ldif는 이 스크립트와 같은 디렉토리에 있어야 한다. 부트스트랩 경로에
# 마운트되면 osixia가 "빈 DB로 첫 기동"할 때 자동으로 적용한다.
mkdir -p "$BASE_DIR/data" "$BASE_DIR/config" "$BASE_DIR/seed"
# seed.ldif는 비밀번호를 플레이스홀더로 들고 있다(레포에 자격 증명을 두지
# 않는다는 프로젝트 규칙). 여기서 주입한다.
sed "s|__USER_PASSWORD__|$LDAP_USER_PASSWORD|g" \
  "$(dirname "$0")/seed.ldif" > "$BASE_DIR/seed/seed.ldif"
chmod 600 "$BASE_DIR/seed/seed.ldif"

docker rm -f openldap >/dev/null 2>&1 || true
docker run -d --name openldap \
  --restart unless-stopped \
  -p 389:389 \
  -e LDAP_ORGANISATION="$LDAP_ORGANISATION" \
  -e LDAP_DOMAIN="$LDAP_DOMAIN" \
  -e LDAP_ADMIN_PASSWORD="$LDAP_ADMIN_PASSWORD" \
  -e LDAP_TLS=false \
  -v "$BASE_DIR/data":/var/lib/ldap \
  -v "$BASE_DIR/config":/etc/ldap/slapd.d \
  -v "$BASE_DIR/seed":/container/service/slapd/assets/config/bootstrap/ldif/custom \
  osixia/openldap:1.5.0 --copy-service
#              ^^^^^^^^^^^^^^ 이 인자가 없으면 osixia는 마운트한 custom LDIF를
#                             조용히 무시한다. seed가 적용되지 않는 가장 흔한 원인.

echo "기동 완료. 확인:"
echo "  docker exec openldap ldapsearch -x -D cn=admin,dc=example,dc=com -w \"\$LDAP_ADMIN_PASSWORD\" -b dc=example,dc=com -LLL dn"
