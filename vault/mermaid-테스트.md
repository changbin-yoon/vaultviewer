---
title: Mermaid 테스트
tags:
  - 테스트
---

# Mermaid 테스트

VaultViewer에서 마크다운 코드 블록에 `mermaid` 언어 태그를 붙이면 코드 대신 다이어그램으로 렌더링됩니다. 이미지 업로드 테스트도 겸해서 아래에 붙여봅니다.

## 표 테스트

| 항목 | 설명 | 비고 |
|---|---|---|
| 역할 | adm/dev/view | LDAP 그룹 매핑 |
| 모드 | LOCAL/CLUSTER | 배포 시 결정 |
| 감사 로그 | 생성/수정/삭제 기록 | 사유 포함 |

![[image_c61872d3_549b10.png]]

![[testimg.png]]

## 플로우차트

```mermaid
flowchart LR
    A[사용자 로그인] --> B{LDAP 인증}
    B -- 성공 --> C[역할 확인]
    B -- 실패 --> D[401 반환]
    C -- adm/dev --> E[읽기·쓰기 가능]
    C -- view --> F[읽기 전용]
```

## 시퀀스 다이어그램

```mermaid
sequenceDiagram
    participant U as 사용자
    participant V as VaultViewer
    participant L as LDAP

    U->>V: 로그인 요청
    V->>L: bind + 그룹 조회
    L-->>V: 그룹 멤버십
    V-->>U: 세션 토큰 + 역할
```
