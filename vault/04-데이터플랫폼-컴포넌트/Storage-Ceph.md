---
title: Storage - Ceph
type: component
tags:
  - 컴포넌트
  - storage
  - ceph
depends_on:
  - "[[04-데이터플랫폼-컴포넌트/Kubernetes-인프라]]"
---

# Ceph

## 개요

- Rook-Ceph 설치, 테스트
- External Ceph 연동
- CSI-RBD / CSI-CephFS 드라이버, StorageClass 생성
- Ceph OSD 교체(하드웨어 노후화 대응)

## 성능 테스트

- fio 기반 RBD/CephFS를 RWO/RWX 2가지 모드로 순차/랜덤/IOPS 측정 → [[Storage-Isilon]]과 비교, 운영 스토리지는 Isilon으로 최종 선정

## K8s 배포 정보 (인수인계용)

- StorageClass 목록 및 용도:
- Rook-Ceph 배포/설정 위치:
- 용량/사용률 모니터링:

## 관련
- [[05-프로젝트-성과/3-3-스토리지-도입-평가-이관]]
