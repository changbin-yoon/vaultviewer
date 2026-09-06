#!/bin/sh
# policy/generated/*.json 을 MinIO에 정책으로 적용한다.
#
# AccessLens의 S3 IAM 화면은 이 파일들을 "MinIO에 붙어 있는 정책의 사본"으로
# 취급해 사용자의 권한을 계산한다(미러 방식). 따라서 policy/gen_policies.py를
# 고쳐 재생성했다면 반드시 이 스크립트로 MinIO에도 반영해야 한다 —
# 반영하지 않으면 화면과 실제 권한이 조용히 어긋난다.
#
# 정책 이름은 그대로 두고 내용만 갱신되므로 그룹 attach는 유지된다.
set -e
NS="${NS:-minio-verify}"
kubectl -n "$NS" create configmap accesslens-policies \
  --from-file="$(dirname "$0")/../../policy/generated/" \
  --dry-run=client -o yaml | kubectl -n "$NS" apply -f -
kubectl -n "$NS" delete job accesslens-policy-apply --ignore-not-found
kubectl -n "$NS" apply -f "$(dirname "$0")/policy-apply-job.yaml"
kubectl -n "$NS" wait --for=condition=complete job/accesslens-policy-apply --timeout=3m
kubectl -n "$NS" logs job/accesslens-policy-apply
