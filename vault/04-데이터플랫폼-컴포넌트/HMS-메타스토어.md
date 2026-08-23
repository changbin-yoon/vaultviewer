---
title: HMS (Hive Metastore)
type: component
tags:
  - 컴포넌트
  - hms
  - metastore
depends_on:
  - "[[04-데이터플랫폼-컴포넌트/Storage-MinIO-AIStor]]"
  - "[[04-데이터플랫폼-컴포넌트/Kubernetes-인프라]]"
---

# HMS (Hive Metastore)

## 개요

- Oracle DB 기반 구축, 운영
- Hive-Metastore Hook 적용, AIStor 연동
- 감사(Audit)용 Grafana 대시보드: Opensearch 로그 기반 DB/EventType별 CRUD 이벤트 모니터링, 위험 이벤트 하이라이트
- Iceberg 메타데이터 정리 스크립트 운영: `rewrite_data_files`, `rewrite_manifests`, `expire_snapshots`

## 장애 이력

- HMS Oracle DB 장애 대응(2026-05) — 상세: [[06-이슈-장애-히스토리/HMS-Oracle-DB-장애-2026-05]]

## K8s 배포 정보 (인수인계용)

- 배포 방식/차트 위치:
- Oracle DB 접속 정보 위치(비밀번호는 [[01-인수인계/계정-전달-방법]] 절차로):
- Iceberg 메타데이터 정리 스크립트/스케줄 위치:
- 감사 대시보드 URL:

## 관련
- [[05-프로젝트-성과/3-4-HMS-관리-모니터링]]
