# Donelist - Database Schema Design

**Version**: 1.0
**Date**: 2025-11-10
**Database**: PostgreSQL 16

---

## 📊 Schema Overview

### Entity Relationship Diagram

```
┌──────────────┐       ┌──────────────┐       ┌──────────────┐
│    users     │──────<│   checkins   │>──────│  categories  │
│              │   1:N │              │  N:M  │              │
└──────────────┘       └──────────────┘       └──────────────┘
        │                      │
        │ 1:1                  │ N:M
        ▼                      ▼
┌──────────────┐       ┌──────────────┐
│subscriptions │       │     tags     │
│              │       │              │
└──────────────┘       └──────────────┘
        │
        │ 1:N
        ▼
┌──────────────┐
│   payments   │
│              │
└──────────────┘
```

---

## 🗄️ Core Tables

### 1. users - 사용자 정보

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Authentication
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,  -- bcrypt hash

    -- Profile
    username VARCHAR(50),
    full_name VARCHAR(100),
    avatar_url TEXT,

    -- Status
    email_verified BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,

    -- Subscription tier
    tier VARCHAR(20) NOT NULL DEFAULT 'free' CHECK (tier IN ('free', 'premium')),

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE,

    -- Soft delete
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_tier ON users(tier) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_created_at ON users(created_at);

-- Updated timestamp trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

**Sample Data**:
```sql
INSERT INTO users (email, password_hash, username, full_name, tier) VALUES
('user@example.com', '$2a$10$...', 'johndoe', 'John Doe', 'free'),
('premium@example.com', '$2a$10$...', 'janedoe', 'Jane Doe', 'premium');
```

---

### 2. checkins - 체크인 기록

```sql
CREATE TABLE checkins (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Foreign Keys
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,

    -- Check-in data
    content TEXT NOT NULL CHECK (LENGTH(content) >= 1 AND LENGTH(content) <= 2000),

    -- Timing information
    checkin_time TIMESTAMP WITH TIME ZONE NOT NULL,  -- When user actually did the activity
    interval_minutes INTEGER NOT NULL CHECK (interval_minutes IN (15, 30, 45, 60, 120)),

    -- Metadata
    metadata JSONB,  -- Flexible field for future extensions
    -- Example metadata:
    -- {
    --   "mood": "productive",
    --   "location": "home",
    --   "energy_level": 8
    -- }

    -- Edit tracking
    is_edited BOOLEAN DEFAULT FALSE,
    edited_at TIMESTAMP WITH TIME ZONE,
    edit_count INTEGER DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Soft delete
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for performance
CREATE INDEX idx_checkins_user_id ON checkins(user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_checkins_user_time ON checkins(user_id, checkin_time DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_checkins_category ON checkins(category_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_checkins_created_at ON checkins(created_at);

-- Full-text search index for content
CREATE INDEX idx_checkins_content_fts ON checkins USING gin(to_tsvector('english', content));

-- Trigger for updated_at
CREATE TRIGGER update_checkins_updated_at BEFORE UPDATE ON checkins
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

**Sample Data**:
```sql
INSERT INTO checkins (user_id, content, checkin_time, interval_minutes, category_id) VALUES
(
    '550e8400-e29b-41d4-a716-446655440000',  -- user_id
    'React 컴포넌트 리팩토링 작업',
    '2025-11-10 09:15:00+00',
    15,
    'c1234567-e29b-41d4-a716-446655440001'   -- category_id (coding)
),
(
    '550e8400-e29b-41d4-a716-446655440000',
    'API 엔드포인트 설계 및 문서화',
    '2025-11-10 09:30:00+00',
    15,
    'c1234567-e29b-41d4-a716-446655440001'
);
```

---

### 3. categories - 활동 카테고리

```sql
CREATE TABLE categories (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Foreign Key (for user-custom categories)
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,  -- NULL = system category

    -- Category information
    name VARCHAR(50) NOT NULL,
    description TEXT,
    color_hex VARCHAR(7) NOT NULL DEFAULT '#6366F1',  -- e.g., "#FF5733"
    icon VARCHAR(50),  -- Icon identifier (e.g., "💻", "work", "study")

    -- System vs Custom
    is_system BOOLEAN DEFAULT FALSE,  -- System categories cannot be deleted

    -- Display order
    display_order INTEGER DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Soft delete
    deleted_at TIMESTAMP WITH TIME ZONE,

    -- Constraints
    UNIQUE(user_id, name)  -- Unique per user (or system-wide if user_id IS NULL)
);

-- Indexes
CREATE INDEX idx_categories_user_id ON categories(user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_categories_system ON categories(is_system) WHERE deleted_at IS NULL;

-- Trigger for updated_at
CREATE TRIGGER update_categories_updated_at BEFORE UPDATE ON categories
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

**System Categories (Pre-populated)**:
```sql
-- System-wide default categories
INSERT INTO categories (id, user_id, name, description, color_hex, icon, is_system) VALUES
('c1234567-e29b-41d4-a716-446655440001', NULL, 'Work', '업무 관련 활동', '#3B82F6', '💼', TRUE),
('c1234567-e29b-41d4-a716-446655440002', NULL, 'Study', '학습 및 공부', '#8B5CF6', '📚', TRUE),
('c1234567-e29b-41d4-a716-446655440003', NULL, 'Exercise', '운동 및 건강', '#10B981', '🏃', TRUE),
('c1234567-e29b-41d4-a716-446655440004', NULL, 'Rest', '휴식 및 여가', '#F59E0B', '😴', TRUE),
('c1234567-e29b-41d4-a716-446655440005', NULL, 'Social', '사교 및 모임', '#EC4899', '🤝', TRUE),
('c1234567-e29b-41d4-a716-446655440006', NULL, 'Personal', '개인 활동', '#6366F1', '👤', TRUE);
```

---

### 4. tags - 태그 시스템

```sql
CREATE TABLE tags (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Foreign Key
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Tag information
    name VARCHAR(50) NOT NULL,
    color_hex VARCHAR(7) DEFAULT '#64748B',

    -- Usage count (for suggestions)
    usage_count INTEGER DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Constraints
    UNIQUE(user_id, name)
);

-- Indexes
CREATE INDEX idx_tags_user_id ON tags(user_id);
CREATE INDEX idx_tags_name ON tags(name);
CREATE INDEX idx_tags_usage ON tags(user_id, usage_count DESC);

-- Trigger
CREATE TRIGGER update_tags_updated_at BEFORE UPDATE ON tags
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

---

### 5. checkin_tags - 체크인-태그 연결 (Many-to-Many)

```sql
CREATE TABLE checkin_tags (
    checkin_id UUID NOT NULL REFERENCES checkins(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Composite primary key
    PRIMARY KEY (checkin_id, tag_id)
);

-- Indexes
CREATE INDEX idx_checkin_tags_checkin ON checkin_tags(checkin_id);
CREATE INDEX idx_checkin_tags_tag ON checkin_tags(tag_id);

-- Trigger to increment tag usage_count
CREATE OR REPLACE FUNCTION increment_tag_usage()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tags SET usage_count = usage_count + 1 WHERE id = NEW.tag_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER increment_tag_usage_on_insert AFTER INSERT ON checkin_tags
    FOR EACH ROW EXECUTE FUNCTION increment_tag_usage();

-- Trigger to decrement tag usage_count
CREATE OR REPLACE FUNCTION decrement_tag_usage()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tags SET usage_count = usage_count - 1 WHERE id = OLD.tag_id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER decrement_tag_usage_on_delete AFTER DELETE ON checkin_tags
    FOR EACH ROW EXECUTE FUNCTION decrement_tag_usage();
```

---

### 6. subscriptions - 구독 정보

```sql
CREATE TABLE subscriptions (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Foreign Key
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,

    -- Subscription details
    tier VARCHAR(20) NOT NULL CHECK (tier IN ('free', 'premium')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'canceled', 'past_due', 'trialing')),

    -- Payment provider info
    stripe_customer_id VARCHAR(255),
    stripe_subscription_id VARCHAR(255),

    -- Billing
    current_period_start TIMESTAMP WITH TIME ZONE,
    current_period_end TIMESTAMP WITH TIME ZONE,
    cancel_at TIMESTAMP WITH TIME ZONE,
    canceled_at TIMESTAMP WITH TIME ZONE,

    -- Trial
    trial_start TIMESTAMP WITH TIME ZONE,
    trial_end TIMESTAMP WITH TIME ZONE,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_subscriptions_user ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_stripe_customer ON subscriptions(stripe_customer_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);

-- Trigger
CREATE TRIGGER update_subscriptions_updated_at BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Trigger to sync tier with users table
CREATE OR REPLACE FUNCTION sync_user_tier()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE users SET tier = NEW.tier WHERE id = NEW.user_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER sync_user_tier_on_subscription_change AFTER INSERT OR UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION sync_user_tier();
```

---

### 7. payments - 결제 이력

```sql
CREATE TABLE payments (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Foreign Keys
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL,

    -- Payment information
    stripe_payment_intent_id VARCHAR(255) UNIQUE,
    amount_cents INTEGER NOT NULL,  -- Amount in cents (e.g., 499 = $4.99)
    currency VARCHAR(3) DEFAULT 'USD',

    -- Status
    status VARCHAR(20) NOT NULL CHECK (status IN ('succeeded', 'failed', 'pending', 'canceled', 'refunded')),

    -- Metadata
    description TEXT,
    receipt_url TEXT,

    -- Timestamps
    paid_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_payments_user ON payments(user_id);
CREATE INDEX idx_payments_subscription ON payments(subscription_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_stripe_intent ON payments(stripe_payment_intent_id);

-- Trigger
CREATE TRIGGER update_payments_updated_at BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

---

### 8. refresh_tokens - JWT Refresh Token 관리

```sql
CREATE TABLE refresh_tokens (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Foreign Key
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Token information
    token_hash VARCHAR(255) NOT NULL UNIQUE,  -- SHA256 hash of refresh token

    -- Expiration
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Revocation
    is_revoked BOOLEAN DEFAULT FALSE,
    revoked_at TIMESTAMP WITH TIME ZONE,

    -- Device information
    device_info JSONB,  -- { "platform": "iOS", "version": "1.0.0", "device_id": "..." }

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Automatic expiration index
    CHECK (expires_at > created_at)
);

-- Indexes
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_token ON refresh_tokens(token_hash) WHERE NOT is_revoked;
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at);

-- Automatic cleanup of expired tokens (via background job)
CREATE OR REPLACE FUNCTION cleanup_expired_refresh_tokens()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM refresh_tokens
    WHERE expires_at < CURRENT_TIMESTAMP OR is_revoked = TRUE;

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;
```

---

### 9. edit_history - 체크인 수정 이력 (Premium Feature)

```sql
CREATE TABLE edit_history (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Foreign Keys
    checkin_id UUID NOT NULL REFERENCES checkins(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Previous values
    previous_content TEXT NOT NULL,
    previous_category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    previous_checkin_time TIMESTAMP WITH TIME ZONE NOT NULL,

    -- New values
    new_content TEXT NOT NULL,
    new_category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    new_checkin_time TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Edit metadata
    edit_reason TEXT,

    -- Timestamps
    edited_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_edit_history_checkin ON edit_history(checkin_id);
CREATE INDEX idx_edit_history_user ON edit_history(user_id);
CREATE INDEX idx_edit_history_time ON edit_history(edited_at DESC);
```

---

## 🔍 Views for Common Queries

### View: Daily Timeline Summary

```sql
CREATE OR REPLACE VIEW daily_timeline AS
SELECT
    c.user_id,
    DATE(c.checkin_time) AS date,
    COUNT(*) AS checkin_count,
    SUM(c.interval_minutes) AS total_minutes,
    ARRAY_AGG(DISTINCT cat.name) AS categories_used,
    ARRAY_AGG(DISTINCT t.name) AS tags_used,
    MIN(c.checkin_time) AS first_checkin,
    MAX(c.checkin_time) AS last_checkin
FROM checkins c
LEFT JOIN categories cat ON c.category_id = cat.id
LEFT JOIN checkin_tags ct ON c.id = ct.checkin_id
LEFT JOIN tags t ON ct.tag_id = t.id
WHERE c.deleted_at IS NULL
GROUP BY c.user_id, DATE(c.checkin_time);

-- Usage: SELECT * FROM daily_timeline WHERE user_id = ? AND date = ?;
```

### View: Weekly Statistics

```sql
CREATE OR REPLACE VIEW weekly_stats AS
SELECT
    c.user_id,
    DATE_TRUNC('week', c.checkin_time) AS week_start,
    COUNT(*) AS total_checkins,
    SUM(c.interval_minutes) AS total_minutes,
    COUNT(DISTINCT DATE(c.checkin_time)) AS active_days,
    (COUNT(*) * 100.0 / NULLIF(COUNT(DISTINCT DATE(c.checkin_time)) * 24, 0)) AS completion_rate
FROM checkins c
WHERE c.deleted_at IS NULL
GROUP BY c.user_id, DATE_TRUNC('week', c.checkin_time);
```

---

## 📈 Materialized Views for Analytics

### Top Categories (Refresh daily)

```sql
CREATE MATERIALIZED VIEW mv_top_categories AS
SELECT
    c.user_id,
    cat.id AS category_id,
    cat.name AS category_name,
    COUNT(*) AS usage_count,
    SUM(c.interval_minutes) AS total_minutes,
    RANK() OVER (PARTITION BY c.user_id ORDER BY COUNT(*) DESC) AS rank
FROM checkins c
INNER JOIN categories cat ON c.category_id = cat.id
WHERE c.deleted_at IS NULL
    AND c.checkin_time >= CURRENT_DATE - INTERVAL '30 days'
GROUP BY c.user_id, cat.id, cat.name;

-- Create unique index for concurrent refresh
CREATE UNIQUE INDEX idx_mv_top_categories ON mv_top_categories(user_id, category_id);

-- Refresh schedule (via cron job or background worker)
-- REFRESH MATERIALIZED VIEW CONCURRENTLY mv_top_categories;
```

---

## 🎯 Performance Optimization

### Partitioning Strategy for checkins (Optional for scale)

```sql
-- Convert checkins table to partitioned table (if scale > 10M rows)
-- Partition by month for time-series queries

CREATE TABLE checkins_partitioned (
    LIKE checkins INCLUDING ALL
) PARTITION BY RANGE (checkin_time);

-- Create monthly partitions
CREATE TABLE checkins_2025_11 PARTITION OF checkins_partitioned
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE checkins_2025_12 PARTITION OF checkins_partitioned
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- ... (create partitions as needed)
```

### Indexes for Common Query Patterns

```sql
-- User timeline query (most common)
CREATE INDEX idx_checkins_user_time_desc ON checkins(user_id, checkin_time DESC)
    WHERE deleted_at IS NULL;

-- Category-based analytics
CREATE INDEX idx_checkins_category_time ON checkins(category_id, checkin_time)
    WHERE deleted_at IS NULL;

-- Full-text search with user filter
CREATE INDEX idx_checkins_user_fts ON checkins(user_id, to_tsvector('english', content))
    WHERE deleted_at IS NULL;

-- Date-range queries
CREATE INDEX idx_checkins_time_brin ON checkins USING brin(checkin_time)
    WITH (pages_per_range = 128);
```

---

## 🔧 Database Functions

### Function: Get User Timeline

```sql
CREATE OR REPLACE FUNCTION get_user_timeline(
    p_user_id UUID,
    p_start_date TIMESTAMP WITH TIME ZONE,
    p_end_date TIMESTAMP WITH TIME ZONE,
    p_limit INTEGER DEFAULT 100,
    p_offset INTEGER DEFAULT 0
)
RETURNS TABLE (
    checkin_id UUID,
    content TEXT,
    checkin_time TIMESTAMP WITH TIME ZONE,
    interval_minutes INTEGER,
    category_name VARCHAR,
    category_color VARCHAR,
    tags TEXT[]
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        c.id AS checkin_id,
        c.content,
        c.checkin_time,
        c.interval_minutes,
        cat.name AS category_name,
        cat.color_hex AS category_color,
        ARRAY_AGG(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL) AS tags
    FROM checkins c
    LEFT JOIN categories cat ON c.category_id = cat.id
    LEFT JOIN checkin_tags ct ON c.id = ct.checkin_id
    LEFT JOIN tags t ON ct.tag_id = t.id
    WHERE c.user_id = p_user_id
        AND c.checkin_time >= p_start_date
        AND c.checkin_time < p_end_date
        AND c.deleted_at IS NULL
    GROUP BY c.id, c.content, c.checkin_time, c.interval_minutes, cat.name, cat.color_hex
    ORDER BY c.checkin_time DESC
    LIMIT p_limit
    OFFSET p_offset;
END;
$$ LANGUAGE plpgsql STABLE;

-- Usage:
-- SELECT * FROM get_user_timeline(
--     '550e8400-e29b-41d4-a716-446655440000',
--     '2025-11-01'::timestamptz,
--     '2025-12-01'::timestamptz,
--     50,
--     0
-- );
```

### Function: Calculate User Statistics

```sql
CREATE OR REPLACE FUNCTION calculate_user_stats(
    p_user_id UUID,
    p_start_date TIMESTAMP WITH TIME ZONE,
    p_end_date TIMESTAMP WITH TIME ZONE
)
RETURNS JSON AS $$
DECLARE
    result JSON;
BEGIN
    SELECT json_build_object(
        'total_checkins', COUNT(*),
        'total_minutes', COALESCE(SUM(interval_minutes), 0),
        'total_hours', ROUND(COALESCE(SUM(interval_minutes), 0) / 60.0, 2),
        'active_days', COUNT(DISTINCT DATE(checkin_time)),
        'avg_checkins_per_day', ROUND(COUNT(*) / NULLIF(COUNT(DISTINCT DATE(checkin_time)), 0), 2),
        'top_category', (
            SELECT json_build_object('name', cat.name, 'count', COUNT(*))
            FROM checkins c2
            INNER JOIN categories cat ON c2.category_id = cat.id
            WHERE c2.user_id = p_user_id
                AND c2.checkin_time >= p_start_date
                AND c2.checkin_time < p_end_date
                AND c2.deleted_at IS NULL
            GROUP BY cat.name
            ORDER BY COUNT(*) DESC
            LIMIT 1
        ),
        'most_productive_hour', (
            SELECT EXTRACT(HOUR FROM checkin_time)
            FROM checkins c3
            WHERE c3.user_id = p_user_id
                AND c3.checkin_time >= p_start_date
                AND c3.checkin_time < p_end_date
                AND c3.deleted_at IS NULL
            GROUP BY EXTRACT(HOUR FROM checkin_time)
            ORDER BY COUNT(*) DESC
            LIMIT 1
        )
    ) INTO result
    FROM checkins c
    WHERE c.user_id = p_user_id
        AND c.checkin_time >= p_start_date
        AND c.checkin_time < p_end_date
        AND c.deleted_at IS NULL;

    RETURN result;
END;
$$ LANGUAGE plpgsql STABLE;

-- Usage:
-- SELECT calculate_user_stats(
--     '550e8400-e29b-41d4-a716-446655440000',
--     '2025-11-01'::timestamptz,
--     '2025-12-01'::timestamptz
-- );
```

---

## 🔒 Row-Level Security (RLS)

### Enable RLS for Multi-Tenant Security

```sql
-- Enable RLS on checkins table
ALTER TABLE checkins ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only see their own checkins
CREATE POLICY checkins_isolation_policy ON checkins
    USING (user_id = current_setting('app.current_user_id')::UUID);

-- Policy: Users can only insert their own checkins
CREATE POLICY checkins_insert_policy ON checkins
    FOR INSERT
    WITH CHECK (user_id = current_setting('app.current_user_id')::UUID);

-- Similar policies for other tables
ALTER TABLE tags ENABLE ROW LEVEL SECURITY;
CREATE POLICY tags_isolation_policy ON tags
    USING (user_id = current_setting('app.current_user_id')::UUID);

ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
CREATE POLICY categories_policy ON categories
    USING (
        user_id = current_setting('app.current_user_id')::UUID
        OR user_id IS NULL  -- System categories
    );
```

**Usage in Go**:
```go
// Set current user context before queries
_, err := db.Exec("SET LOCAL app.current_user_id = $1", userID)
if err != nil {
    return err
}

// Now all queries are automatically filtered by RLS
rows, err := db.Query("SELECT * FROM checkins")
```

---

## 📊 Migration Strategy

### Migration Files Structure

```
migrations/
├── 000001_init.up.sql               # Initial schema
├── 000001_init.down.sql             # Rollback
├── 000002_add_edit_history.up.sql  # Add edit tracking
├── 000002_add_edit_history.down.sql
├── 000003_add_rls_policies.up.sql  # Add RLS
├── 000003_add_rls_policies.down.sql
└── ...
```

### Example Migration (000001_init.up.sql)

```sql
-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Create users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    -- ... (rest of schema)
);

-- Create other tables
-- ... (full schema as defined above)

-- Create indexes
-- ... (all indexes)

-- Create functions
-- ... (all functions)

-- Create triggers
-- ... (all triggers)
```

### Migration Tool: golang-migrate

```bash
# Install
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Create new migration
migrate create -ext sql -dir migrations -seq add_feature_name

# Run migrations
migrate -path migrations -database "postgres://user:pass@localhost:5432/donelist?sslmode=disable" up

# Rollback
migrate -path migrations -database "postgres://..." down 1
```

---

## 🧪 Seed Data for Development

```sql
-- Insert test user
INSERT INTO users (id, email, password_hash, username, full_name, tier) VALUES
('550e8400-e29b-41d4-a716-446655440000', 'test@donelist.com', '$2a$10$abcd...', 'testuser', 'Test User', 'premium');

-- Insert sample checkins
INSERT INTO checkins (user_id, content, checkin_time, interval_minutes, category_id) VALUES
('550e8400-e29b-41d4-a716-446655440000', 'Morning standup meeting', '2025-11-10 09:00:00+00', 15, 'c1234567-e29b-41d4-a716-446655440001'),
('550e8400-e29b-41d4-a716-446655440000', 'Code review for PR #123', '2025-11-10 09:15:00+00', 30, 'c1234567-e29b-41d4-a716-446655440001'),
('550e8400-e29b-41d4-a716-446655440000', 'Lunch break', '2025-11-10 12:00:00+00', 60, 'c1234567-e29b-41d4-a716-446655440004');

-- Insert sample tags
INSERT INTO tags (user_id, name, color_hex) VALUES
('550e8400-e29b-41d4-a716-446655440000', 'urgent', '#EF4444'),
('550e8400-e29b-41d4-a716-446655440000', 'frontend', '#3B82F6'),
('550e8400-e29b-41d4-a716-446655440000', 'backend', '#10B981');
```

---

## 📝 Next Steps

1. **API Specification**: OpenAPI 명세서에서 이 스키마 참조
2. **Go Repository Layer**: 이 스키마 기반으로 repository 패턴 구현
3. **Testing**: Unit test용 test fixtures 생성
4. **Performance Testing**: 대용량 데이터로 쿼리 성능 테스트
5. **Backup Strategy**: 백업 및 복구 전략 수립

---

**Document Status**: Draft v1.0
**Next Review Date**: 2025-11-15
**Owner**: Database Team
