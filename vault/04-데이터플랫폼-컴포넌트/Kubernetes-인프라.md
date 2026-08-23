---
title: Kubernetes 인프라
type: component
tags:
  - 컴포넌트
  - k8s
---

# Kubernetes 인프라

## 개요

- 클러스터 구축 도구: Kubespray, RKE2
- CNI: Cilium
- 배포 도구: Helm, Kustomize
- GitOps: ArgoCD (Platform/Services 레포 분리, ApplicationSet + Kustomize overlay)
- Operator: Spark Operator, Strimzi(Kafka), CNPG, Rook-Ceph, Starrocks Operator
- 보안: Sealed Secrets

## 구축/운영 이력

- dev/stg/prd/storage 4개 환경 K8s 클러스터 구축 — 설치/워커 추가/업그레이드/해제 스크립트 자동화
- Cilium CNI, Keycloak, MinIO 등 플랫폼 공통 컴포넌트 구성
- 수백 대 Worker Node 확보 및 리소스 할당 정책 수립
- 상세: [[05-프로젝트-성과/3-1-K8s-신규플랫폼-구축]]

## K8s 배포 정보 (인수인계용)

- 설치/업그레이드/워커 추가 스크립트 위치:
- ArgoCD Application 구조 및 레포 경로:
- 리소스 할당 정책(쿼터/limitrange):

## 관련
- [[03-클러스터-운영/클러스터-개요]]
- [[03-클러스터-운영/배포-변경관리]]
