---
title: "3-2. Query Engine 운영 및 성능 검증"
기간: 2024~2026
tags:
  - 프로젝트성과
  - trino
---

# Query Engine 운영 및 성능 검증 (2024~2026)

- Trino 클러스터 구축, 운영: 서비스별 Trino Cluster 배포, Hive/Iceberg/PostgreSQL 카탈로그 연동, LDAP 인증 + OPA 인가 + Group-Provider, Kafka listener, JMX exporter, 서비스 모니터, Cilium Ingress 적용
- Trino 버전 고도화(408→442→475→482), 이슈 대응(지연 원인 분석, 카탈로그/리소스 튜닝)
- Starrocks 도입 검토, 성능 평가(kube-starrocks operator 테스트), Apache Ignite(V2/V3) 클러스터 PoC(K8s discovery, cluster-init, StatefulSet)
- Trino 성능 테스트: JMeter Query 성능테스트(21개), Locust 부하테스트(Trino/Spark Operator), HDFS vs MinIO vs Ceph 비교테스트

## 상세 (채워갈 항목)

- 배경/목표:
- 나의 역할:
- 어려웠던 점 / 해결 방법:
- 정량적 성과(지표):

## 관련
- [[04-데이터플랫폼-컴포넌트/Trino]]
- [[04-데이터플랫폼-컴포넌트/StarRocks-Ignite]]
- [[06-이슈-장애-히스토리/Trino-지연-이슈]]
