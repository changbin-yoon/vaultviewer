---
title: Trino
type: component
tags:
  - 컴포넌트
  - trino
depends_on:
  - "[[04-데이터플랫폼-컴포넌트/데이터카탈로그-포맷]]"
  - "[[04-데이터플랫폼-컴포넌트/HMS-메타스토어]]"
  - "[[04-데이터플랫폼-컴포넌트/보안-인가]]"
  - "[[04-데이터플랫폼-컴포넌트/Kubernetes-인프라]]"
---

# Trino

## 개요

- 서비스별 Trino Cluster 배포
- 카탈로그 연동: Hive / Iceberg / PostgreSQL
- 인증/인가: LDAP 인증 + OPA 인가 + Group-Provider
- 연동: Kafka listener, JMX exporter, 서비스 모니터, Cilium Ingress

## 버전 이력

- 버전 고도화: 408 → 442 → 475 → 482
- 이슈 대응: 지연 원인 분석, 카탈로그/리소스 튜닝

## 성능 테스트

- JMeter Query 성능테스트(21개 쿼리)
- Locust 부하테스트(Trino/Spark Operator)
- HDFS vs MinIO vs Ceph 스토리지 비교테스트

## K8s 배포 정보 (인수인계용)

- Helm 차트/이미지 위치:
- values.yaml 저장소 위치:
- 스케일링 기준 및 리소스 쿼터:
- 자주 발생하는 이슈와 대처:

## 관련
- [[05-프로젝트-성과/3-2-QueryEngine-운영-성능검증]]
- [[06-이슈-장애-히스토리/Trino-지연-이슈]]
- [[03-클러스터-운영/런북/런북-Trino-쿼리지연]]
