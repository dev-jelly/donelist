# Donelist - Product Requirements Document (PRD)

**Version**: 1.0
**Date**: 2025-11-10
**Status**: Draft

---

## 📋 Executive Summary

### Product Vision
**Donelist**는 "할 일(Todo)" 대신 "한 일(Done)"에 집중하는 혁신적인 생산성 추적 앱입니다. 사용자가 하루 동안 실제로 수행한 작업을 시간 기반으로 기록하고, 과거의 생산성 패턴을 분석할 수 있는 회고 중심의 생산성 도구입니다.

### Core Value Proposition
- **패러다임 전환**: Todo → Done (계획 중심 → 실행 기록 중심)
- **시간 기반 추적**: 15분/30분/45분 간격의 지능적인 체크인 시스템
- **회고 중심**: 이미 완료한 일에 대한 기록과 패턴 분석
- **크로스 플랫폼**: iOS, macOS, Android, Web 모든 플랫폼 지원

---

## 🎯 Product Goals

### Primary Goals
1. **사용자가 하루 동안 실제로 한 일을 정확하게 기록**
2. **시간 간격 기반의 부담 없는 체크인 시스템 구축**
3. **캘린더 기반의 직관적인 기록 조회 및 분석**
4. **크로스 플랫폼 동기화를 통한 seamless 사용자 경험**

### Success Metrics
- **사용자 참여도**: 하루 평균 체크인 응답률 > 70%
- **리텐션**: 7일 리텐션 > 50%, 30일 리텐션 > 30%
- **데이터 완성도**: 사용자당 주간 기록 완성도 > 80%
- **구독 전환율**: Free → Premium 전환율 > 5% (6개월 내)

---

## 👥 Target Users

### Primary Persona: 자기계발 실천가 (Self-Improvement Practitioner)
- **연령**: 25-40세
- **직업**: 프리랜서, 개발자, 디자이너, 지식노동자
- **Pain Points**:
  - Todo 앱에 계획만 쌓이고 실천은 안 됨
  - 실제로 뭘 했는지 기억이 안 남
  - 생산성을 측정하고 개선하고 싶지만 방법이 없음
- **Goals**:
  - 하루 동안 실제로 무엇을 했는지 명확히 파악
  - 생산성 패턴 발견 및 개선
  - 과거 활동 기록을 통한 자기 성찰

### Secondary Persona: 프로젝트 관리자
- **연령**: 30-50세
- **직업**: PM, 팀 리더, 관리자
- **Pain Points**:
  - 팀원들의 실제 작업 시간 파악 어려움
  - 프로젝트 회고 시 정확한 데이터 부족
- **Goals**:
  - 팀 생산성 가시화
  - 데이터 기반 프로세스 개선

---

## 🔑 Core Features

### 1. 시간 기반 체크인 시스템 (Time-Based Check-in)

#### 1.1 지능적 체크인 타이밍
**작동 방식**:
- **0-2시간**: 15분, 30분, 45분 간격으로 체크인 알림
  - 15분: "지난 15분간 무엇을 하셨나요?"
  - 30분: "지난 30분간 무엇을 하셨나요?"
  - 45분: "지난 45분간 무엇을 하셨나요?"
  - 60분 (1시간): 자동으로 1시간 간격으로 전환

- **2시간 이후**: 2시간 단위로만 체크인 가능
  - "지난 2시간 동안 무엇을 하셨나요?"
  - 더 세밀한 간격은 잠김 (Premium 기능으로 과거 수정 가능)

**Technical Requirements**:
- 백그라운드 타이머 시스템
- Push notification 시스템
- 사용자별 마지막 체크인 시간 추적
- 시간대(Timezone) 처리

**User Stories**:
```
US-1.1: 15분 체크인
As a user,
I want to receive a notification every 15 minutes during the first hour,
So that I can record my activities in real-time with minimal memory burden.

Acceptance Criteria:
- ✅ 앱 실행 후 15분마다 알림 수신
- ✅ 알림 클릭 시 간단한 입력 폼 표시
- ✅ 입력 후 다음 15분 타이머 시작
- ✅ 백그라운드에서도 알림 정상 작동
```

#### 1.2 체크인 입력 인터페이스
**Features**:
- **빠른 입력**: 텍스트 입력 (최소 1자 이상)
- **시간 표시**: "15분 전 ~ 방금" 시간 범위 표시
- **선택적 카테고리**: Work, Study, Exercise, Rest, etc.
- **선택적 태그**: #coding, #meeting, #reading 등
- **음성 입력**: 말로 빠르게 기록 (optional)

**Technical Requirements**:
- 텍스트 입력 검증 (최소 길이, 최대 길이)
- 카테고리/태그 자동완성
- Speech-to-text 연동 (iOS/Android native)

#### 1.3 놓친 체크인 처리
**2시간 이내**:
- 세밀한 시간대 기록 가능
- "45분 전에 뭐했어요?" 식으로 물어볼 수 있음

**2시간 이후**:
- 2시간 단위로만 기록 가능
- 더 세밀한 시간은 "과거 수정" Premium 기능 필요

---

### 2. 캘린더 뷰 (Calendar View)

#### 2.1 일간 뷰 (Daily View)
**Features**:
- 타임라인 형태로 하루 기록 표시
- 15분/30분/45분/2시간 단위 블록
- 각 블록에 기록된 활동 표시
- 빈 블록 (기록 안 한 시간) 시각적으로 구분

**UI Components**:
```
┌─────────────────────────────────┐
│  2025년 11월 10일 일요일         │
├─────────────────────────────────┤
│ 09:00 - 09:15  [코딩 - React]   │
│ 09:15 - 09:30  [코딩 - API]     │
│ 09:30 - 10:00  [회의]           │
│ 10:00 - 12:00  [미기록]  ⚠️     │
│ 12:00 - 13:00  [점심]           │
│ 13:00 - 15:00  [프로젝트 작업]  │
└─────────────────────────────────┘
```

#### 2.2 주간 뷰 (Weekly View)
**Features**:
- 7일간의 기록을 한눈에
- 일별 완성도 표시 (기록률 %)
- 주간 통계 요약
  - 총 기록 시간
  - 가장 많이 한 활동 Top 3
  - 생산적인 시간대 분석

**UI Components**:
```
┌──────────────────────────────────────┐
│  2025년 11월 4주차                   │
├──────────────────────────────────────┤
│ 월 ████████░░  80% (10시간 기록)    │
│ 화 ██████████  95% (12시간 기록)    │
│ 수 ██████░░░░  65% (8시간 기록)     │
│ 목 ████████░░  78% (9시간 기록)     │
│ 금 ██████████  92% (11시간 기록)    │
│ 토 ████░░░░░░  45% (5시간 기록)     │
│ 일 ██░░░░░░░░  25% (3시간 기록)     │
├──────────────────────────────────────┤
│ 📊 주간 통계                          │
│ • 총 기록: 58시간                     │
│ • Top 활동: 코딩(30h), 회의(12h)    │
│ • 생산적 시간: 14:00-17:00          │
└──────────────────────────────────────┘
```

#### 2.3 월간 뷰 (Monthly View)
**Features**:
- 캘린더 형태로 한 달 전체 보기
- 일별 기록 완성도를 색상으로 표시
- 날짜 클릭 시 일간 뷰로 이동

---

### 3. 과거 기록 수정 (Past Edit) - Premium Feature

#### 3.1 수정 제한 정책
**Free Tier**:
- 2시간 이내: 무제한 수정 가능
- 2시간 이후: 수정 불가 (Premium 필요)

**Premium Tier**:
- 무제한 과거 수정 가능
- 수정 횟수 제한 없음
- 수정 이력 추적 (audit log)

#### 3.2 수정 UI
**Features**:
- 기존 기록 위에 "수정" 버튼 표시
- 수정 시 원본 기록 보존 (이력 관리)
- 수정 사유 입력 (optional)

**Technical Requirements**:
- 수정 이력 데이터베이스 스키마
- 사용자 구독 상태 확인
- 수정 권한 검증 로직

---

### 4. 동기화 및 멀티 플랫폼

#### 4.1 실시간 동기화
**Features**:
- 모든 플랫폼 간 실시간 동기화
- 오프라인 모드 지원
- 충돌 해결 전략 (last-write-wins)

**Technical Requirements**:
- WebSocket 또는 Server-Sent Events
- 로컬 캐싱 및 오프라인 큐
- 동기화 상태 UI 표시

#### 4.2 플랫폼별 최적화
**iOS/macOS**:
- Native notification
- Apple Watch 연동 (Phase 2)
- Siri Shortcuts 지원

**Android**:
- Native notification
- Widget 지원
- Wear OS 연동 (Phase 2)

**Web**:
- Progressive Web App (PWA)
- Desktop notification
- Keyboard shortcuts

---

## 💰 Monetization Strategy

### Freemium Model

#### Free Tier
**Features**:
- ✅ 무제한 체크인 및 기록
- ✅ 일간/주간/월간 뷰
- ✅ 기본 통계
- ✅ 2시간 이내 수정 가능
- ❌ 2시간 이후 과거 수정 불가
- ❌ 고급 분석 기능 없음
- ❌ 데이터 export 제한

#### Premium Tier ($4.99/month or $49.99/year)
**Features**:
- ✅ 무제한 과거 기록 수정
- ✅ 고급 분석 및 인사이트
  - 생산성 트렌드 분석
  - 활동 패턴 AI 분석
  - 목표 달성률 추적
- ✅ 데이터 export (CSV, JSON, PDF)
- ✅ 커스텀 카테고리 및 태그
- ✅ 테마 커스터마이징
- ✅ 우선 고객 지원

### Revenue Projections (1년차)
```
Month 1-3:   Free users only (launch phase)
Month 4-6:   5% conversion → ~$500/month
Month 7-9:   10% conversion → ~$2,000/month
Month 10-12: 15% conversion → ~$5,000/month

Target: 10,000 active users → 1,500 premium → $7,500 MRR
```

---

## 🛠️ Technical Architecture (High-Level)

### Technology Stack Recommendations

#### Backend
- **Language**: Node.js (TypeScript) or Go
- **Framework**: Express.js / Nest.js / Gin
- **Database**: PostgreSQL (main) + Redis (cache)
- **Real-time**: WebSocket (Socket.io) or SSE
- **Storage**: S3-compatible (for future attachments)

#### Frontend - Web
- **Framework**: React 18+ or Next.js 14+
- **State Management**: Zustand or Jotai
- **UI Library**: Tailwind CSS + Shadcn/ui
- **Real-time**: Socket.io-client or SSE

#### Mobile - iOS
- **Language**: Swift
- **Framework**: SwiftUI
- **Architecture**: MVVM + Combine
- **Local DB**: SQLite + CoreData

#### Mobile - Android
- **Language**: Kotlin
- **Framework**: Jetpack Compose
- **Architecture**: MVVM + Coroutines
- **Local DB**: Room

#### Mobile - macOS
- **Shared codebase with iOS**: SwiftUI (Mac Catalyst)

---

## 📅 Development Roadmap

### Phase 1: MVP (3-4 months)
**Goal**: 핵심 기능 구현 및 iOS 출시

**Milestones**:
1. **Month 1: Backend + API**
   - 사용자 인증 (JWT)
   - 체크인 CRUD API
   - 타임라인 조회 API
   - PostgreSQL 스키마 설계

2. **Month 2: iOS App**
   - 체크인 알림 시스템
   - 타임라인 UI
   - 일간/주간 캘린더 뷰
   - 로컬 오프라인 지원

3. **Month 3: Web App**
   - React 기반 웹 앱
   - 캘린더 뷰 구현
   - 실시간 동기화

4. **Month 4: Testing & Launch**
   - Beta 테스트
   - 버그 수정
   - App Store 출시

### Phase 2: Android + Premium (2-3 months)
**Goal**: Android 출시 및 수익화 시작

**Milestones**:
1. **Month 5: Android App**
   - Kotlin + Jetpack Compose
   - iOS 기능 패리티

2. **Month 6: Premium Features**
   - 과거 수정 기능
   - 구독 결제 시스템 (Stripe + In-App Purchase)
   - 고급 통계 및 분석

### Phase 3: macOS + Advanced Features (2 months)
**Goal**: macOS 앱 및 고급 기능

**Milestones**:
1. **Month 7: macOS App**
   - SwiftUI Mac Catalyst
   - 메뉴바 앱 지원

2. **Month 8: AI Insights**
   - 활동 패턴 AI 분석
   - 생산성 추천 시스템

---

## 🎨 Design Principles

### User Experience
1. **마찰 최소화**: 체크인은 3초 이내에 완료 가능해야 함
2. **명확한 피드백**: 모든 액션에 즉각적인 피드백 제공
3. **아름다운 데이터**: 기록을 시각적으로 아름답게 표현
4. **부담 없는 추적**: 알림이 성가시지 않도록 스마트하게 조절

### Visual Design
- **Modern Minimalism**: 깔끔하고 현대적인 디자인
- **Color Coding**: 카테고리별 색상 구분
- **Data Visualization**: 차트와 그래프로 직관적 표현
- **Dark Mode**: 다크 모드 완벽 지원

---

## 🔒 Privacy & Security

### Data Privacy
- **User Data Ownership**: 사용자 데이터는 사용자 소유
- **Data Encryption**: 전송 및 저장 시 암호화
- **Minimal Data Collection**: 필수 데이터만 수집
- **GDPR Compliance**: EU 개인정보보호 규정 준수

### Security Measures
- JWT 기반 인증
- HTTPS only
- Rate limiting
- Input sanitization
- SQL injection 방어
- XSS 방어

---

## 📊 Success Criteria

### Launch Success (Month 1-3)
- [ ] 1,000 active users
- [ ] App Store rating > 4.5
- [ ] Daily check-in rate > 60%
- [ ] Crash-free rate > 99.5%

### Growth Success (Month 4-6)
- [ ] 5,000 active users
- [ ] 5% premium conversion
- [ ] 7-day retention > 50%
- [ ] NPS score > 40

### Scale Success (Month 7-12)
- [ ] 10,000 active users
- [ ] 15% premium conversion
- [ ] 30-day retention > 30%
- [ ] $5,000+ MRR

---

## 🚀 Next Steps

1. **Architecture Design**: 상세 시스템 아키텍처 문서 작성
2. **Database Schema**: PostgreSQL 스키마 설계 및 마이그레이션
3. **API Specification**: RESTful API 명세서 작성 (OpenAPI)
4. **UI/UX Wireframes**: Figma로 모든 화면 와이어프레임 제작
5. **Development Setup**: 개발 환경 구축 및 CI/CD 파이프라인

---

## 📝 Appendix

### Glossary
- **Check-in**: 사용자가 특정 시간에 한 일을 기록하는 행위
- **Timeline**: 시간 순서대로 정렬된 체크인 기록 목록
- **Past Edit**: 2시간 이후의 기록을 수정하는 Premium 기능

### References
- [Similar Apps Analysis]: RescueTime, Toggl Track, Timeular
- [Design Inspiration]: Linear, Notion, Things 3
- [Technical References]: PostgreSQL Docs, Swift Documentation

---

**Document Status**: Draft v1.0
**Next Review Date**: 2025-11-15
**Owner**: Product Team
