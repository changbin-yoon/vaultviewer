# VaultViewer Helm Chart

LDAP 기반 RBAC를 지원하는 시크릿/볼트 뷰어를 쿠버네티스에 배포하는 차트입니다.
`local`(마운트된 디렉토리를 노트/시크릿 뷰어로 서빙)과 `cluster`(해당 네임스페이스의
Kubernetes Secrets를 직접 관리) 두 모드를 지원합니다.

## 사전 준비

1. **LDAP 디렉토리** — `adm`/`dev`/`view` 역할에 매핑할 그룹이 있는 LDAP/AD 서버.
   그룹 CN이 정확히 `adm`/`dev`/`view`가 아니어도 됩니다 (`ldap.groupRoleMap`으로 매핑).
   한 역할에 서로 다른 LDAP 그룹 여러 개를 매핑하고 싶다면(예: 조직 개편으로 그룹명이
   바뀌었거나, 여러 팀 그룹을 모두 adm으로 인정해야 하는 경우) 값을 리스트로 쓰면 됩니다:

   ```yaml
   ldap:
     groupRoleMap:
       adm:
         - "dt-bi-adm"          # 기존 그룹
         - "platform-admins"    # 새로 추가된 그룹 — 둘 다 adm 역할 부여
       dev: "dev"                # 그룹이 하나뿐이면 문자열로 써도 됨
       view: "view"
   ```

   사용자가 나열된 그룹 중 하나에라도 속해 있으면 해당 역할이 부여되고, 여러 역할에
   걸쳐 있으면(adm과 view 둘 다) 더 높은 권한(adm)이 적용됩니다.
2. **이미지** — `docker buildx build --platform linux/amd64 -t <repo>/vaultviewer:<tag> --push .`
   로 빌드한 뒤 `image.repository`/`image.tag`에 지정하세요. 클러스터 노드 아키텍처와
   빌드 머신 아키텍처가 다르면(예: Apple Silicon Mac → amd64 노드) 반드시
   `--platform`을 지정해야 `ImagePullBackOff` 없이 뜹니다.
3. **LDAP bind 비밀번호 / 세션 서명 키** — 값을 values 파일에 직접 넣지 말고 미리
   Secret으로 만들어 `existingSecret`으로 참조하세요:

   ```bash
   kubectl create secret generic vaultviewer-ldap \
     --from-literal=bind-password='<LDAP 서비스 계정 비밀번호>'
   kubectl create secret generic vaultviewer-session \
     --from-literal=session-secret="$(openssl rand -hex 32)"
   ```

   세션 시크릿을 안 만들면 파드 재시작마다 새로 발급되어 모든 사용자가 다시
   로그인해야 합니다 (그 외엔 기능상 문제 없음).

## 설치

```bash
cp charts/vaultviewer/values-example.yaml my-values.yaml
# my-values.yaml 편집: ldap.host / baseDN / bindDN, image 등

helm install vaultviewer charts/vaultviewer -f my-values.yaml
```

## 예제 데이터로 시작하기 (local 모드)

빈 볼트로 시작하면 아무것도 안 보입니다. `examples/vault-seed/`에 콜아웃·표·
Mermaid·위키링크·백링크를 보여주는 최소 예제 노트가 있습니다:

```bash
POD=$(kubectl get pod -l app.kubernetes.io/instance=vaultviewer -o jsonpath='{.items[0].metadata.name}')
kubectl cp examples/vault-seed/. "$POD:/data"
```

## 접속

```bash
# Ingress를 안 쓰거나 DNS가 아직 없다면 NodePort로:
helm upgrade vaultviewer charts/vaultviewer -f my-values.yaml --set service.type=NodePort
kubectl get svc vaultviewer   # 할당된 nodePort 확인 → http://<노드IP>:<nodePort>

# 또는 즉석 확인용 포트포워드:
kubectl port-forward svc/vaultviewer 8080:8080
```

Ingress를 쓰려면 `values-example.yaml`의 `ingress.*` 섹션 주석을 풀고 클러스터에
이미 Ingress 컨트롤러(nginx 등)가 떠 있어야 합니다.

## UI 배지 (LOCAL / CLUSTER)

앱 이름 옆 배지는 로컬 바이너리·Docker로 띄우면 항상 `LOCAL`이고, Helm으로 배포하면
기본값 `deploymentLabel: "CLUSTER"`가 표시됩니다. 스토리지 모드(`mode`)와는 별개
값이라 `mode: local`(PVC 마운트)이어도 배포 위치는 `CLUSTER`로 뜹니다. 여러 클러스터에
나눠 배포한다면 클러스터별 values 파일에서 이름을 구분해서 지정하세요:

```yaml
deploymentLabel: "CLUSTER-PROD"   # 다른 클러스터용 values 파일엔 "CLUSTER-STAGING" 등
```

## S3/MinIO 백업 (local 모드)

`mode: local`은 PVC 하나에 모든 데이터가 있어서, PVC가 잘못되면(StorageClass가
`reclaimPolicy: Delete`인 경우 `helm uninstall` 한 번으로도) 복구할 방법이 없습니다.
`backup.enabled: true`로 켜면 마운트된 디렉토리 전체를 주기적으로 S3 호환
스토리지(MinIO 등)에 동기화합니다.

```bash
kubectl create secret generic vaultviewer-s3 \
  --from-literal=access-key='<MinIO access key>' \
  --from-literal=secret-key='<MinIO secret key>'
```

```yaml
backup:
  enabled: true
  endpoint: "minio.minio-system.svc.cluster.local:9000"
  bucket: "vaultviewer-backup"     # 미리 만들어둔 버킷이어야 함 (자동 생성 안 함)
  useSSL: false
  intervalMinutes: 30
  existingSecret: vaultviewer-s3
```

동작 방식:
- 매 `intervalMinutes`마다 로컬 디렉토리를 훑으면서 `<prefix>/<YYYY-MM-DD>/...`
  경로에 이미 있는 객체와 비교해, **내용이 바뀌었거나 아직 없는 파일만** 업로드합니다
  (로컬 파일의 MD5와 S3에 저장된 객체의 ETag를 비교 — 메모리 상태가 아니라 S3
  자체를 기준으로 비교하므로 파드가 재시작돼도 정확합니다). 안 바뀐 파일은 매번
  건너뛰어서 불필요한 업로드가 없습니다. 날짜가 바뀌면 새 프리픽스로 넘어가서
  **이전 날짜의 백업은 절대 덮어쓰지 않습니다** — 하루 단위로 독립된 복구 지점이
  생기는 셈이고, 그날 첫 동기화는 그 프리픽스에 아직 아무 것도 없으니 전체가 한 번
  더 올라갑니다(=날짜별 스냅샷은 항상 완전한 사본).
- 실패해도(버킷이 아직 없거나, 네트워크 문제 등) 업로드했던 객체를 지우지 않습니다 —
  한 번의 실패한 동기화가 이전의 정상 백업을 파괴할 수 없도록, 이 기능은 오직
  업로드(PUT)만 하고 삭제(DELETE)는 절대 하지 않습니다.
- `prefix`를 비워두면 릴리스 이름을 자동으로 씁니다 — 버킷 하나를 여러 릴리스
  (`vaultviewer`, `vaultviewer-demo` 등)가 같이 써도 경로가 겹치지 않습니다.
- `mode: cluster`에서 `backup.enabled: true`를 켜면 배포 자체가 실패합니다 — 이
  기능은 마운트된 디렉토리를 대상으로 하며, cluster 모드의 Kubernetes Secret은
  대상이 아닙니다.

## 모드별 참고사항

- **local**: `local.persistence`로 PVC를 만들거나(`existingClaim`) 기존 PVC를 재사용합니다.
  ReadWriteOnce PVC이므로 롤아웃 시 새 파드가 볼륨을 못 붙는 문제를 피하려고
  Deployment 전략을 `Recreate`로 고정해뒀습니다.
- **cluster**: `cluster.namespace`(비우면 릴리스 네임스페이스)의 Secret을 관리합니다.
  파드의 ServiceAccount에 해당 네임스페이스 Secret에 대한 get/list/watch/create/
  update/patch/delete 권한을 주는 Role/RoleBinding이 `cluster.rbac.create=true`일 때
  자동 생성됩니다. Kubernetes Secret은 "빈 네임스페이스"라는 개념이 없어서, 첫 시크릿
  키를 추가하기 전까지는 그 네임스페이스 자체가 트리에 나타나지 않습니다.

전체 값 목록과 설명은 [`values.yaml`](./values.yaml)의 주석을 참고하세요.
