# Donelist - UI/UX Overview

**Version**: 1.0
**Date**: 2025-11-10
**Platforms**: iOS, macOS, Android, Web

---

## 🎨 Design Philosophy

### Core Principles
1. **최소 마찰 (Minimal Friction)**: 체크인은 3초 이내 완료
2. **명확한 피드백 (Clear Feedback)**: 모든 액션에 즉각 반응
3. **아름다운 데이터 (Beautiful Data)**: 기록을 시각적으로 매력적으로 표현
4. **부담 없는 추적 (Unobtrusive Tracking)**: 스마트한 알림으로 방해 최소화

---

## 📱 Core Screens

### 1. Check-in Input Screen

**Trigger**: 15/30/45분 알림 → 탭 → 즉시 입력

```
┌─────────────────────────────────────┐
│  지난 15분간 무엇을 하셨나요?       │
├─────────────────────────────────────┤
│  [                               ]  │
│  텍스트 입력 (최대 2000자)          │
│                                     │
│  📂 Work ▼   #coding #react         │
│                                     │
│  [ 취소 ]              [ 저장 ✓ ]  │
└─────────────────────────────────────┘
```

**Features**:
- 자동 포커스 on 텍스트 입력
- 카테고리 quick select (최근 사용 기준)
- 태그 자동완성
- 음성 입력 버튼 (optional)

---

### 2. Timeline View (Daily)

**Main Screen**: 하루 전체 타임라인

```
┌─────────────────────────────────────┐
│  ◀  2025년 11월 10일 일요일   ▶     │
├─────────────────────────────────────┤
│  09:00 - 09:15  💼 Work             │
│  React 컴포넌트 리팩토링            │
│  #coding #react                     │
│                                     │
│  09:15 - 09:30  💼 Work             │
│  API 설계 문서 작성                 │
│  #api #documentation                │
│                                     │
│  09:30 - 10:00  ⚠️ 미기록           │
│                                     │
│  10:00 - 12:00  😴 Rest             │
│  점심 식사 및 휴식                  │
│                                     │
│  📊 오늘 완성도: 75.5%              │
│  총 8시간 기록됨                    │
└─────────────────────────────────────┘
```

**Interactions**:
- 탭: 체크인 상세 보기
- 롱프레스: Quick edit/delete menu
- 스와이프: 이전/다음 날짜
- Pull-to-refresh: 최신 데이터 동기화

---

### 3. Calendar View (Weekly)

```
┌─────────────────────────────────────┐
│  2025년 11월 2주차                  │
├─────────────────────────────────────┤
│ 월 ████████░░  80%  (10h)          │
│ 화 ██████████  95%  (12h)          │
│ 수 ██████░░░░  65%  (8h)           │
│ 목 ████████░░  78%  (9h)           │
│ 금 ██████████  92%  (11h)          │
│ 토 ████░░░░░░  45%  (5h)           │
│ 일 ██░░░░░░░░  25%  (3h)           │
├─────────────────────────────────────┤
│ 📊 주간 통계                        │
│ • 총 58시간 기록                    │
│ • 가장 많이: 코딩 (30h)             │
│ • 생산 시간: 14:00-17:00            │
└─────────────────────────────────────┘
```

---

### 4. Settings & Profile

```
┌─────────────────────────────────────┐
│  John Doe                           │
│  johndoe@example.com                │
│  🎖️ Premium Member                  │
├─────────────────────────────────────┤
│  ⚙️ 알림 설정                        │
│  🎨 테마 (다크/라이트)              │
│  📂 카테고리 관리                   │
│  🏷️ 태그 관리                       │
│  💳 구독 관리                       │
│  📤 데이터 내보내기 (Premium)       │
│  🔐 개인정보 & 보안                 │
│  ℹ️ 앱 정보                         │
└─────────────────────────────────────┘
```

---

## 🎨 Visual Design System

### Color Palette

**Primary Colors**:
```css
--primary: #6366F1;      /* Indigo */
--primary-dark: #4F46E5;
--primary-light: #818CF8;
```

**Category Colors**:
```css
--work: #3B82F6;         /* Blue */
--study: #8B5CF6;        /* Purple */
--exercise: #10B981;     /* Green */
--rest: #F59E0B;         /* Orange */
--social: #EC4899;       /* Pink */
--personal: #6366F1;     /* Indigo */
```

**System Colors**:
```css
--success: #10B981;
--warning: #F59E0B;
--error: #EF4444;
--info: #3B82F6;
```

**Neutral Colors**:
```css
--gray-50: #F9FAFB;
--gray-100: #F3F4F6;
--gray-900: #111827;
```

### Typography

**iOS/macOS (San Francisco)**:
```
Title: 28pt, Bold
Heading: 20pt, Semibold
Body: 16pt, Regular
Caption: 12pt, Regular
```

**Android (Roboto)**:
```
Title: 28sp, Bold
Heading: 20sp, Medium
Body: 16sp, Regular
Caption: 12sp, Regular
```

**Web (Inter)**:
```
Title: 1.75rem, 700
Heading: 1.25rem, 600
Body: 1rem, 400
Caption: 0.75rem, 400
```

---

## 🌓 Dark Mode

**Full dark mode support** across all platforms:

```css
/* Light Mode */
--background: #FFFFFF;
--text: #111827;
--card: #F9FAFB;

/* Dark Mode */
--background: #111827;
--text: #F9FAFB;
--card: #1F2937;
```

---

## 📐 Layout Specifications

### Spacing Scale
```
xs: 4px
sm: 8px
md: 16px
lg: 24px
xl: 32px
2xl: 48px
```

### Border Radius
```
sm: 4px
md: 8px
lg: 12px
full: 9999px
```

### Shadows
```
sm: 0 1px 2px rgba(0,0,0,0.05)
md: 0 4px 6px rgba(0,0,0,0.1)
lg: 0 10px 15px rgba(0,0,0,0.1)
```

---

## 🔔 Notification Design

### Push Notification

```
┌─────────────────────────────────────┐
│  ⏰ Donelist                        │
│  지난 15분간 무엇을 하셨나요?       │
│  탭하여 기록하기                    │
└─────────────────────────────────────┘
```

### In-App Notification

```
┌─────────────────────────────────────┐
│  ✅ 체크인 저장 완료!               │
└─────────────────────────────────────┘
(2초 후 자동 사라짐)
```

---

## ♿ Accessibility

### Requirements
- [x] **VoiceOver (iOS) / TalkBack (Android) 완벽 지원**
- [x] **Dynamic Type / Font Scaling**
- [x] **High Contrast Mode**
- [x] **Keyboard Navigation (Web/macOS)**
- [x] **WCAG 2.1 AA 준수**

### Color Contrast Ratios
- Body text: 7:1 (AAA)
- UI elements: 4.5:1 (AA)
- Large text: 3:1 (AA)

---

## 📱 Platform-Specific Guidelines

### iOS/macOS
- **Human Interface Guidelines** 준수
- Native SF Symbols 아이콘 사용
- Haptic feedback on actions
- Swipe gestures for navigation

### Android
- **Material Design 3** 준수
- Adaptive icons
- Snackbar for feedback
- FAB for quick check-in

### Web
- **Responsive design** (mobile-first)
- Progressive Web App (PWA)
- Keyboard shortcuts
- Browser notification API

---

## 🎬 Animation & Transitions

### Micro-interactions
```
Button press: scale(0.95) + haptic
List item delete: slide-out + fade
Card flip: 3D rotation
Loading: pulse skeleton
```

### Page Transitions
```
iOS: Push/Pop (native)
Android: Slide (Material)
Web: Fade (300ms ease-in-out)
```

---

## 🧪 Wireframe Links

**Coming Soon**: Figma wireframes for all screens

---

**Document Status**: Draft v1.0
**Next Steps**: Create high-fidelity Figma designs
**Owner**: Design Team
