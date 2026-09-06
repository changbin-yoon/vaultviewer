#!/usr/bin/env python3
"""sync-from-minio.sh 의 JSON 처리 부분.

셸 heredoc 안에 파이썬을 끼워 넣는 대신 파일로 분리했다 — 인용 규칙이
겹치면 조용히 깨지는 종류의 코드이고, 이 파일은 그 자체로 실행해 볼 수 있다.
"""
import json
import sys

# 이 조직이 정의한 정책이 아니므로 사본에 넣지 않는다.
BUILTIN = {"consoleAdmin", "diagnostics", "readonly", "readwrite", "writeonly"}

HEADER = [
    "# MinIO에 실제로 attach 되어 있는 상태의 사본.",
    "# sync-from-minio.sh 가 생성한다 — 손으로 고치지 말 것.",
    "#",
    "# MinIO가 보고하는 모양 그대로다. 그룹 DN 기준이고, 한 그룹이 여러 정책을,",
    "# 한 정책이 여러 그룹을 가질 수 있으며, 사용자 DN 직접 attach도 표현된다.",
    "#",
    "# 갱신:  ALIAS=<alias> sh examples/ldap-verify/sync-from-minio.sh",
    "# 대조:  대시보드 S3 IAM 카드의 '선언 대조' (관리자)",
    "attachments:",
]


def mappings(payload):
    """빌트인을 제외한 policy -> subjects 매핑."""
    result = payload["result"]
    return [m for m in (result.get("policyMappings") or []) if m["policy"] not in BUILTIN]


def main():
    mode = sys.argv[1]

    if mode == "names":
        payload = json.load(sys.stdin)
        print(" ".join(sorted({m["policy"] for m in mappings(payload)})))
        return

    if mode == "attachments":
        payload = json.load(sys.stdin)
        lines = list(HEADER)
        kept = 0
        for m in mappings(payload):
            lines.append("  - policy: %s" % m["policy"])
            for field in ("groups", "users"):
                values = m.get(field) or []
                if values:
                    lines.append("    %s:" % field)
                    for dn in values:
                        lines.append('      - "%s"' % dn)
            kept += 1
        with open(sys.argv[2], "w") as f:
            f.write("\n".join(lines) + "\n")
        print("  attach %d건 (빌트인 정책 제외)." % kept)
        return

    if mode == "policy":
        # mc 는 정책 문서를 policyInfo.Policy 안에 감싸서 준다.
        payload = json.load(sys.stdin)
        print(json.dumps(payload["policyInfo"]["Policy"], indent=2, ensure_ascii=False))
        return

    sys.exit("unknown mode: %s" % mode)


if __name__ == "__main__":
    main()
