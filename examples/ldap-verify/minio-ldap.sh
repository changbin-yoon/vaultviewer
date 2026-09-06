#!/bin/sh
# MinIO의 LDAP IAM을 위 OpenLDAP 서버로 향하게 한다.
#
# 주의 — 주소를 고르는 방식이 중요하다. 과거 이 클러스터의 MinIO는
# 노드 NodePort(10.10.105.172:30518)를 가리키고 있었는데, 그 노드가 교체되면서
# LDAP 인증이 통째로 죽었고 아무도 몰랐다. 노드 IP 대신 안정적인 주소
# (클러스터 밖 서버의 고정 IP, 또는 클러스터 내부라면 Service DNS)를 쓸 것.
#
# 실행 예:
#   LDAP_ADDR=10.10.105.4:389 LDAP_BIND_PASSWORD=... sh minio-ldap.sh
set -e

: "${LDAP_ADDR:?LDAP_ADDR을 넘기세요 (예: 10.10.105.4:389)}"
: "${LDAP_BIND_PASSWORD:?LDAP_BIND_PASSWORD를 넘기세요}"
NS="${NS:-minio-verify}"
POD="${POD:-minio-0}"
BIND_DN="${BIND_DN:-cn=admin,dc=example,dc=com}"

kubectl -n "$NS" exec "$POD" -- sh -c "
mc alias set l http://localhost:9000 \"\$MINIO_ROOT_USER\" \"\$MINIO_ROOT_PASSWORD\" >/dev/null
mc admin config set l identity_ldap \
  server_addr='$LDAP_ADDR' \
  lookup_bind_dn='$BIND_DN' \
  lookup_bind_password='$LDAP_BIND_PASSWORD' \
  user_dn_search_base_dn='ou=users,dc=example,dc=com' \
  user_dn_search_filter='(uid=%s)' \
  group_search_base_dn='ou=groups,dc=example,dc=com' \
  group_search_filter='(&(objectClass=groupOfNames)(member=%d))' \
  server_insecure=on
"
# identity_ldap 변경은 재시작해야 적용된다. mc admin service restart는 TTY를
# 요구하므로 statefulset을 롤링 재시작한다.
kubectl -n "$NS" rollout restart sts minio
kubectl -n "$NS" rollout status sts minio --timeout=5m

echo "확인: kubectl -n $NS exec $POD -- sh -c 'mc alias set l http://localhost:9000 \"\$MINIO_ROOT_USER\" \"\$MINIO_ROOT_PASSWORD\" >/dev/null; mc idp ldap policy entities l'"
