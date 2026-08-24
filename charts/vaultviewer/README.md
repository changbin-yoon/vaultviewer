# AccessLens Helm Chart

LDAP 기반 RBAC를 지원하는 AccessLens 시크릿/권한 뷰어를 쿠버네티스에 배포하는
차트입니다. 차트 디렉토리명·릴리스 리소스 이름(`vaultviewer.fullname` 등)과
`image.repository`는 제품명을 AccessLens로 바꾼 뒤에도 의도적으로 `vaultviewer`를
그대로 씁니다 — 바꾸면 다음 `helm upgrade` 때 Deployment/PVC/ServiceAccount 이름이
전부 새로 계산돼(`vaultviewer` → `vaultviewer-accesslens` 식) 기존 PVC(실제 볼트
데이터)가 새 이름의 빈 PVC로 대체될 위험이 있기 때문입니다. 마이그레이션을
의도적으로 하고 싶으면 PVC/Secret 이전 계획부터 세우세요.
`local`(마운트된 디렉토리를 노트/시크릿 뷰어로 서빙)과 `cluster`(해당 네임스페이스의
Kubernetes Secrets를 직접 관리) 두 모드를 지원합니다.

## 사전 준비

1. **LDAP 디렉토리** — `adm`/`dev`/`view` 역할에 매핑할 그룹이 있는 LDAP/AD 서버.
   그룹 CN이 정확히 `adm`/`dev`/`view`가 아니어도 됩니다 (`ldap.groupRoleMap`으로 매핑).
   사용자 엔트리에 `o`(organizationName) 속성을 설정해두면 상단바에 "소속"으로
   표시됩니다 — 단, 설정 화면(adm 전용)의 "그룹별 팀 매핑"에 사용자가 속한 그룹이
   등록돼 있으면 그 이름이 우선합니다 (아래 [감사 로그 · 그룹별 팀 매핑
   영속화](#감사-로그--그룹별-팀-매핑-영속화) 참고).
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

## Airgapped 클러스터에 배포하기

프론트엔드가 Google Fonts를 원격에서 받아오던 부분은 빌드 시점에 내려받아
`web/public/fonts/`에 넣어두는 방식으로 바꿔서, 배포된 앱 자체는 런타임에
외부로 나가는 요청이 없습니다(백엔드도 LDAP/Trino/OPA/S3 IAM 등 설정된 내부
서비스만 호출). 남은 건 이미지 하나뿐 — Docker Hub 접근이 되는 머신에서
받아서 airgapped 클러스터가 실제로 pull할 수 있는 곳(내부 프라이빗
레지스트리)에 올려주면 됩니다.

```bash
# 1) 인터넷이 되는 머신에서 이미지를 받아 tar로 저장
docker pull yoochabi/vaultviewer:<tag>
docker save yoochabi/vaultviewer:<tag> -o vaultviewer-<tag>.tar

# 2) tar를 airgapped 환경으로 옮긴 뒤, 그 안에서 불러와 내부 레지스트리로 push
docker load -i vaultviewer-<tag>.tar
docker tag yoochabi/vaultviewer:<tag> <내부-레지스트리>/yoochabi/vaultviewer:<tag>
docker push <내부-레지스트리>/yoochabi/vaultviewer:<tag>
```

그다음 `image.registry`만 내부 레지스트리로 지정하면 됩니다(레지스트리가
인증을 요구하면 `imagePullSecrets`도 함께):

```yaml
image:
  registry: "<내부-레지스트리>"   # 예: harbor.internal:5000
  repository: yoochabi/vaultviewer
  tag: "<tag>"

imagePullSecrets:
  - name: my-registry-secret
```

`helm package charts/vaultviewer`로 차트 자체도 `.tgz`로 묶어서 함께 옮길 수
있습니다 — 이 차트는 하위 차트(`Chart.yaml` dependencies)가 없어서 별도
`helm dependency build` 없이 그대로 옮겨 쓰면 됩니다.

## 예제 데이터로 시작하기 (local 모드)

빈 볼트로 시작하면 아무것도 안 보입니다. `examples/vault-seed/`에 콜아웃·표·
Mermaid·위키링크·백링크를 보여주는 최소 예제 노트가 있습니다. 별도 데모
릴리스로 띄우고 싶다면 [`values-demo.yaml`](./values-demo.yaml)을 참고하세요
(NodePort 고정, 기존 LDAP/세션 Secret 재사용):

```bash
helm install vaultviewer-demo charts/vaultviewer -f charts/vaultviewer/values-demo.yaml

POD=$(kubectl get pod -l app.kubernetes.io/instance=vaultviewer-demo -o jsonpath='{.items[0].metadata.name}')
kubectl cp examples/vault-seed/. "$POD:/data"
```

## 접속

```bash
# Ingress를 안 쓰거나 DNS가 아직 없다면 NodePort로:
helm upgrade vaultviewer charts/vaultviewer -f my-values.yaml --set service.type=NodePort
kubectl get svc vaultviewer   # 할당된 nodePort 확인 → http://<노드IP>:<nodePort>

# nodePort를 지정 안 하면 재배포마다 랜덤 포트로 바뀐다 — 고정하려면
# service.nodePort도 같이 지정 (values-demo.yaml 참고):
#   service:
#     type: NodePort
#     nodePort: 31455

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

## 감사 로그 · 그룹별 팀 매핑 영속화

DB가 없어서 둘 다 파일(local 모드) 또는 메모리(cluster 모드)로만 관리합니다.

- **감사 로그** — 모든 생성/수정/삭제 이력(누가/언제/왜). `local` 모드는 마운트된
  디렉토리에 `.vaultviewer-audit.jsonl`(JSON Lines)로 append돼 파드 재시작에도
  남습니다. `cluster` 모드는 영구 저장 위치가 없어 메모리에만 있고, 재시작하면
  초기화됩니다.
- **그룹별 팀 매핑** — LDAP 그룹 CN을 화면에 보여줄 팀 이름으로 매핑(설정 화면,
  adm 전용에서 직접 관리). `local` 모드는 `.vaultviewer-group-teams.json`으로
  영속화, `cluster` 모드는 마찬가지로 메모리 전용(재시작 시 초기화 — 설정 화면에
  안내 문구가 표시됩니다).

두 파일 모두 마운트된 디렉토리 루트의 점 파일이라 볼트 트리 UI에는 자동으로
숨겨지고, 위 S3 백업을 켜두면 동기화 대상에도 포함됩니다.

## Git 백엔드 (local 모드, 실험적)

`local.git.enabled: true`로 켜면 마운트된 디렉토리가 진짜 git 저장소가 되고,
노트를 저장·삭제할 때마다 자동으로 커밋됩니다 — 커밋 author는 실제 작업한
사용자, committer는 고정된 "AccessLens" 서비스 계정입니다.

```yaml
local:
  git:
    enabled: true
```

- **로컬 저장소만** — 원격(GitHub/GitLab 등)으로 push하지 않습니다. 필요하면
  파드 안에서 직접 `git remote add` + `git push`를 하거나, 향후 단계로 미룸.
- **기존 감사 로그와는 별개** — `/api/audit`·설정 화면의 "노트 이력"은 지금처럼
  `.vaultviewer-audit.jsonl` 기반으로 그대로 동작합니다. git 저장소는 병행하는
  추가 레이어로, 실제 diff·git 도구가 필요할 때 파드 안에서 직접 확인하세요:
  ```bash
  kubectl exec deploy/vaultviewer -- git -C /data log --oneline
  kubectl exec deploy/vaultviewer -- git -C /data show <commit>
  ```
- 감사 로그·그룹별 팀 매핑 파일(위 참고)은 `.gitignore`로 제외돼 있어 커밋에
  섞이지 않습니다.
- 기존에 데이터가 있던 볼트에서 처음 켜면, 있던 내용을 담아 초기 커밋 한 번을
  만듭니다. 이후 재시작에서는 다시 초기화하지 않습니다(멱등).
- 빈 디렉토리(네임스페이스만 만들고 파일이 없는 경우)는 git이 표현할 수 없어
  커밋되지 않습니다 — 그 안에 첫 파일이 저장돼야 git에 나타납니다.

## `/api/graph` — 온톨로지를 외부에서 쓰기 (AI agent 연동 등)

볼트 전체의 노드/타입 있는 관계를 JSON으로 반환하는 읽기 전용
엔드포인트입니다(로그인한 사용자라면 역할 무관, view 포함 모두 접근
가능). 프론트엔드 그래프 뷰와 별개로, AI agent나 MCP 서버처럼 AccessLens
바깥의 소비자가 "이 컴포넌트가 죽으면 뭐가 영향받나" 같은 질문에 답하려고
할 때 모든 노트를 직접 읽고 프론트매터를 다시 파싱하지 않아도 되도록
만든 API입니다. 로컬 mode/cluster mode 모두 동작(백엔드가 이미 읽는
`VaultStorageEngine`을 그대로 사용).

```bash
TOKEN=$(curl -s -X POST http://<host>/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"...","password":"..."}' | jq -r .token)

curl -s http://<host>/api/graph -H "Authorization: Bearer $TOKEN" | jq
```

```json
{
  "nodes": [{"id": "01-예제/Trino.md", "name": "Trino.md", "resolved": true, "type": "component"}],
  "edges": [{"source": "01-예제/Trino.md", "target": "01-예제/HMS-메타스토어.md", "relation": "depends_on"}]
}
```

`relation`이 없는 엣지는 노트 본문의 평범한 `[[위키링크]]`, 있으면
프론트매터로 선언한 타입 있는 관계입니다. 관계 문법 자체는 볼트 안
"작성가이드" 탭 또는 `examples/vault-seed/01-예제/온톨로지-사전.md`
참고.

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
