---
title: Storage - MinIO / AIStor
type: component
tags:
  - 컴포넌트
  - storage
  - minio
  - aistor
depends_on:
  - "[[04-데이터플랫폼-컴포넌트/Kubernetes-인프라]]"
---

# MinIO / AIStor

## 개요

- 객체 스토리지 구축, 버전 업그레이드
- DirectPV 디스크 구성
- AirGapped 설치 런북/가이드 작성
- 시스템별(Dev/Prd) 권한 정책(MinIO Policy) 구성 및 운영

## 데이터 이관

- HDFS → MinIO 객체 스토리지 이관 (S3A, jceks Credential 암호화)
- 샘플 데이터 이관, PoC, 성능 테스트

## K8s 배포 정보 (인수인계용)

- DirectPV 디스크 구성 위치:
- 정책(Policy) 관리 방법:
- Airgapped 설치 가이드 위치:

## 관련
- [[05-프로젝트-성과/3-3-스토리지-도입-평가-이관]]
- [[05-프로젝트-성과/3-7-신기술조사-PoC-문서화]]
