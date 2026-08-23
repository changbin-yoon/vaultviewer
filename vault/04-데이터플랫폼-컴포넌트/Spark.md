---
title: Spark
type: component
tags:
  - 컴포넌트
  - spark
depends_on:
  - "[[04-데이터플랫폼-컴포넌트/데이터카탈로그-포맷]]"
  - "[[04-데이터플랫폼-컴포넌트/Kubernetes-인프라]]"
---

# Spark

## 개요

- DataLake Spark Runtime 이미지: Spark 3.5.7 + Iceberg 카탈로그 + 메트릭 + Spark Connect
- 서비스별 SparkApplication(CR) 배포 템플릿
- Spark WorkDir 정리 CronJob
- 팀별 SA 기반 Kubeconfig 생성
- Spark Operator 모니터링(Prometheus 메트릭, 알람 규칙)

## K8s 배포 정보 (인수인계용)

- Operator/이미지 저장소 위치:
- SparkApplication 템플릿 위치:
- 스케일링 기준 및 리소스 쿼터:
- 자주 발생하는 이슈와 대처:

## 관련
- [[05-프로젝트-성과/3-5-배포자동화-커스텀개발]]
- [[06-이슈-장애-히스토리/Spark-작업지연-이슈]]
- [[03-클러스터-운영/런북/런북-Spark-작업-대량실패]]
