---
title: Airflow
type: component
tags:
  - 컴포넌트
  - airflow
depends_on:
  - "[[04-데이터플랫폼-컴포넌트/Spark]]"
  - "[[04-데이터플랫폼-컴포넌트/Kafka]]"
  - "[[04-데이터플랫폼-컴포넌트/Kubernetes-인프라]]"
---

# Airflow

## 개요

- Airflow 오케스트레이션 체계 구축
- 커스텀 이미지 / Helm 차트
- DataOps 공용 DAG 파이프라인: Kafka→Iceberg, Spark Operator 연동, 메트릭 컨슈머, DB/logfile 클린업 등

## K8s 배포 정보 (인수인계용)

- Helm 차트/커스텀 이미지 위치:
- DAG 저장소 위치 및 배포 방식:
- 스케줄러/Executor 구성:
- 자주 발생하는 이슈와 대처:

## 관련
- [[05-프로젝트-성과/3-5-배포자동화-커스텀개발]]
- [[06-이슈-장애-히스토리/Airflow-지연-이슈]]
