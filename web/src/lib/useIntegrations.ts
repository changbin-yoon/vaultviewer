import { useEffect, useState } from "react";
import * as api from "./api";
import type { OpaIntegration, S3IamIntegration, TrinoIntegration } from "./api";

// Trino/OPA/S3 IAM 연동 상태를 가져오는 공용 훅 — 대시보드와 구성도 페이지가
// 같은 로직으로 같은 API를 조회해 항상 같은 "연결됨/연결 안 됨/연동 예정"
// 판단을 내리도록 함. 실패하면 "설정 안 됨"과 같은 기본값(enabled:false)에
// 머무름 — 각 화면은 그 기본값으로 알아서 플레이스홀더를 보여줌.
export function useIntegrations() {
  const [trino, setTrino] = useState<TrinoIntegration>({ enabled: false });
  const [opa, setOpa] = useState<OpaIntegration>({ enabled: false });
  const [s3iam, setS3iam] = useState<S3IamIntegration>({ enabled: false });

  useEffect(() => {
    let cancelled = false;
    api
      .getTrinoIntegration()
      .then((data) => {
        if (!cancelled) setTrino(data);
      })
      .catch(() => {});
    api
      .getOpaIntegration()
      .then((data) => {
        if (!cancelled) setOpa(data);
      })
      .catch(() => {});
    api
      .getS3IamIntegration()
      .then((data) => {
        if (!cancelled) setS3iam(data);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  return { trino, opa, s3iam };
}

// enabled/connected 조합을 "실선으로 그릴지"(live)와 상태 라벨 하나로 정리.
// DashboardPage의 위성 노드 판정과 동일한 3단계 규칙을 공유.
export function integrationState(enabled: boolean, connected?: boolean): { live: boolean; sub: string } {
  if (!enabled) return { live: false, sub: "연동 예정" };
  return connected ? { live: true, sub: "연결됨" } : { live: false, sub: "연결 안 됨" };
}
