# Donelist Server - 프로젝트 개요

## 프로젝트 목표
완료된 작업 기반 생산성 추적 앱의 백엔드 API 서버 구축

## 핵심 개념
- **Check-in 중심**: Todo 리스트 대신 완료된 작업을 기록
- **시간 간격**: 15/30/45/120분 단위 체크인
- **타임라인 뷰**: 일간/주간/월간 작업 기록 조회
- **프리미엄 기능**: 편집 이력, 2시간 이상 지난 체크인 수정

## 기술 스택
- **언어**: Go 1.24
- **웹 프레임워크**: Gin
- **데이터베이스**: PostgreSQL (sqlx)
- **캐시**: Redis
- **인증**: JWT (Access + Refresh Token)
- **로깅**: Zap
- **설정**: Viper

## 아키텍처
3-Layer Architecture:
- Repository Layer: 데이터베이스 접근
- Service Layer: 비즈니스 로직
- Handler Layer: HTTP 요청/응답 처리

## 완료된 API
- ✅ Authentication (Register, Login, Refresh, Logout)
- ✅ Users (GetMe, UpdateMe, DeleteMe)
- ✅ Check-ins (CRUD + Edit History)
- ✅ Timeline (Daily, Weekly, Monthly views)
- ✅ Categories (CRUD with color/icon)
- ✅ Tags (Create, List, GetByID)
