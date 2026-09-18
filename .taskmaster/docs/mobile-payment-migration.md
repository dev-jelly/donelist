# 모바일 앱 스토어 결제 시스템 마이그레이션 PRD

## 개요
Stripe 웹 결제에서 애플 앱스토어(In-App Purchase)와 구글 플레이 결제(Google Play Billing)로 완전히 전환합니다.

## 목표
- Stripe 의존성 제거 및 모바일 네이티브 결제로 100% 전환
- 애플/구글 앱스토어 정책 100% 준수
- 서버 사이드 영수증 검증을 통한 안전한 구독 관리
- 크로스 플랫폼 구독 상태 동기화

## 범위

### Phase 1: 모바일 결제 시스템 기반 구축
1. 애플 앱스토어 통합
   - App Store Connect API 연동
   - App Store Server API (영수증 검증)
   - StoreKit 2 서버 알림 처리
   - 환불 및 구독 취소 처리

2. 구글 플레이 통합
   - Google Play Developer API 연동
   - Play Store 영수증 검증
   - Real-time Developer Notifications (RTDN) 처리
   - 환불 및 구독 취소 처리

3. 통합 구독 관리 시스템
   - 플랫폼 독립적인 구독 모델
   - 영수증 저장 및 검증
   - 구독 상태 동기화
   - 구독 갱신 자동 처리

### Phase 2: Stripe 제거 및 마이그레이션
1. Stripe 의존성 제거
   - Stripe 코드 제거
   - 데이터베이스 스키마 업데이트
   - 환경 변수 정리

2. 기존 구독자 마이그레이션
   - Stripe 구독자 데이터 백업
   - 마이그레이션 스크립트 작성
   - 무료 전환 또는 수동 재구독 안내

3. API 엔드포인트 업데이트
   - 모바일 전용 결제 API
   - 영수증 검증 API
   - 구독 상태 조회 API

### Phase 3: 모니터링 및 최적화
1. 영수증 검증 실패 처리
2. 결제 실패 재시도 로직
3. 구독 만료 알림
4. 결제 관련 메트릭 및 대시보드

## 기술 스택

### 애플 앱스토어
- App Store Server API (REST API)
- App Store Server Notifications v2
- JWT 토큰 기반 인증

### 구글 플레이
- Google Play Developer API v3
- Real-time Developer Notifications (RTDN)
- Google Cloud Pub/Sub
- OAuth 2.0 인증

### 서버
- Go 1.24+
- PostgreSQL (구독 정보 저장)
- Redis (영수증 캐싱)

## 데이터 모델

### subscriptions 테이블 업데이트
```sql
ALTER TABLE subscriptions
  DROP COLUMN stripe_customer_id,
  DROP COLUMN stripe_subscription_id,
  ADD COLUMN platform VARCHAR(20) NOT NULL, -- 'ios' or 'android'
  ADD COLUMN store_transaction_id VARCHAR(255) UNIQUE,
  ADD COLUMN store_original_transaction_id VARCHAR(255),
  ADD COLUMN store_product_id VARCHAR(255) NOT NULL,
  ADD COLUMN receipt_data TEXT,
  ADD COLUMN last_verified_at TIMESTAMP WITH TIME ZONE;
```

### 새 테이블: receipt_validations
```sql
CREATE TABLE receipt_validations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  platform VARCHAR(20) NOT NULL,
  transaction_id VARCHAR(255) NOT NULL,
  receipt_data TEXT NOT NULL,
  validation_status VARCHAR(50) NOT NULL,
  validation_response JSONB,
  validated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_receipt_user_platform (user_id, platform),
  INDEX idx_receipt_transaction (transaction_id)
);
```

## API 엔드포인트

### iOS 결제
- POST /api/v1/payments/ios/validate - 영수증 검증 및 구독 활성화
- POST /api/v1/payments/ios/webhook - App Store Server Notifications

### Android 결제
- POST /api/v1/payments/android/validate - 영수증 검증 및 구독 활성화
- POST /api/v1/payments/android/webhook - Real-time Developer Notifications

### 공통
- GET /api/v1/subscription/status - 현재 구독 상태 조회
- POST /api/v1/subscription/restore - 구독 복원 (새 기기에서)

## 보안 고려사항
1. 모든 영수증 검증은 서버 사이드에서만 수행
2. Apple/Google API 키는 환경 변수로 관리
3. 영수증 데이터 암호화 저장
4. Rate limiting 적용 (영수증 검증 남용 방지)
5. 영수증 재사용 방지 (transaction_id 중복 체크)

## 구독 상품 정의
```yaml
products:
  - id: premium_monthly
    ios_product_id: com.donelist.premium.monthly
    android_product_id: premium_monthly
    price_tier: $9.99/month
    features:
      - 과거 체크인 무제한 수정
      - 고급 통계 및 분석
      - 데이터 내보내기

  - id: premium_yearly
    ios_product_id: com.donelist.premium.yearly
    android_product_id: premium_yearly
    price_tier: $99.99/year
    features:
      - 과거 체크인 무제한 수정
      - 고급 통계 및 분석
      - 데이터 내보내기
      - 17% 할인 (연간 결제)
```

## 마일스톤

### M1: iOS 결제 시스템 (2-3주)
- App Store Connect 설정
- 영수증 검증 API 구현
- Server Notifications 처리
- iOS 앱 결제 통합 테스트

### M2: Android 결제 시스템 (2-3주)
- Google Play Console 설정
- 영수증 검증 API 구현
- RTDN 처리
- Android 앱 결제 통합 테스트

### M3: Stripe 제거 및 마이그레이션 (1주)
- Stripe 코드 제거
- 데이터베이스 마이그레이션
- 기존 구독자 처리
- 문서 업데이트

### M4: 모니터링 및 최적화 (1주)
- 결제 메트릭 대시보드
- 알림 시스템
- 성능 최적화
- 프로덕션 배포

## 성공 지표
- 영수증 검증 성공률 > 99.5%
- 구독 활성화 응답 시간 < 2초
- 영수증 검증 실패율 < 0.5%
- 구독 동기화 지연 < 5분

## 위험 요소
1. 앱스토어 심사 지연 가능성
2. 영수증 검증 API 장애
3. 기존 Stripe 구독자 마이그레이션 복잡도
4. 플랫폼별 정책 변경

## 참고 자료
- [Apple: Validating receipts with the App Store](https://developer.apple.com/documentation/appstoreserverapi)
- [Google: Google Play Billing Library](https://developer.android.com/google/play/billing)
- [Apple: App Store Server Notifications](https://developer.apple.com/documentation/appstoreservernotifications)
- [Google: Real-time developer notifications](https://developer.android.com/google/play/billing/rtdn-reference)
