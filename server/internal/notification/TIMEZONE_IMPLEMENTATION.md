# Timezone Handling & DND Implementation

## Overview

This implementation provides comprehensive timezone handling with DST (Daylight Saving Time) support and Do Not Disturb (DND) functionality for the notification system.

## Components

### 1. TimezoneHandler (`timezone.go`)

Core timezone conversion and DST handling utilities.

**Key Features:**
- IANA timezone database support
- Automatic DST detection and handling
- Location caching for performance
- DST transition detection
- Common timezone list

**Main Methods:**
```go
// Get timezone location with caching
GetLocation(timezone string) (*time.Location, error)

// Convert UTC time to user's local timezone
ConvertToUserTime(utcTime time.Time, timezone string) (time.Time, error)

// Convert user's local time to UTC
ConvertToUTC(localTime time.Time, timezone string) (time.Time, error)

// Get detailed timezone information including DST status
GetTimezoneInfo(timezone string, at time.Time) (*TimezoneInfo, error)

// Check if DST is active
IsDST(t time.Time, loc *time.Location) bool

// Find next DST transition
GetNextDSTTransition(t time.Time, loc *time.Location) *time.Time

// Validate timezone string
ValidateTimezone(timezone string) error
```

### 2. TimezoneManager (`timezone.go`)

Manages user timezone preferences with travel detection.

**Key Features:**
- User timezone preference management
- Automatic travel detection
- Schedule adjustment recommendations

**Main Methods:**
```go
// Update user timezone and detect travel
UpdateUserTimezone(pref *UserTimezonePreference, newTimezone string) (travelDetected bool, err error)

// Get timezone for scheduling
GetTimezoneForScheduling(pref *UserTimezonePreference) string

// Determine if schedules should be adjusted
ShouldAdjustSchedules(pref *UserTimezonePreference) bool
```

### 3. DNDScheduler (`dnd_scheduler.go`)

Enhanced DND scheduling with timezone awareness.

**Key Features:**
- Timezone-aware DND period checking
- Quiet hours spanning midnight
- Day-of-week scheduling
- Next available time calculation

**Main Methods:**
```go
// Check if time falls in DND period
IsInDNDPeriod(settings *NotificationSettings, checkTime time.Time) (bool, string)

// Get next available time outside DND
GetNextAvailableTime(settings *NotificationSettings, requestedTime time.Time) (time.Time, error)

// Calculate DND windows for next N days
CalculateDNDWindows(settings *NotificationSettings, days int) ([]DNDWindow, error)

// Validate DND settings
ValidateDNDSettings(settings *NotificationSettings) error
```

### 4. DNDOverrideService (`dnd_override.go`)

Manages temporary DND overrides for urgent notifications.

**Key Features:**
- Temporary DND overrides
- Priority-based override logic
- Multiple override reasons (urgent, emergency, travel, VIP, etc.)
- Override validation and scheduling

**Override Reasons:**
- `urgent` - Time-sensitive notifications
- `emergency` - Critical alerts (24-hour default)
- `manual` - User-created overrides
- `vip` - Important contact notifications
- `critical` - System-critical alerts
- `travel` - Timezone change adjustments

**Main Methods:**
```go
// Create temporary override
CreateOverride(ctx context.Context, userID uuid.UUID, duration time.Duration, reason OverrideReason) (*DNDOverride, error)

// Create scheduled override for specific time range
CreateScheduledOverride(ctx context.Context, userID uuid.UUID, startTime, endTime time.Time, reason OverrideReason) (*DNDOverride, error)

// Check if override is active
IsOverrideActive(ctx context.Context, userID uuid.UUID, checkTime time.Time) (bool, *DNDOverride, error)

// Determine if notification should be sent during DND
ShouldSendDuringDND(ctx context.Context, notification *Notification, settings *NotificationSettings) (bool, string, error)

// Travel-specific override
CreateTravelOverride(ctx context.Context, userID uuid.UUID, newTimezone string, duration time.Duration) (*DNDOverride, error)

// Emergency override (24 hours)
CreateEmergencyOverride(ctx context.Context, userID uuid.UUID) (*DNDOverride, error)
```

### 5. SettingsEngine (`settings_engine.go`)

Integrates timezone and DND logic with notification settings.

**Main Methods:**
```go
// Check if notification should be sent
ShouldSendNotification(ctx context.Context, userID uuid.UUID, notificationType NotificationType, scheduledTime time.Time) (bool, error)

// Get next available time for notification
GetNextAvailableTime(ctx context.Context, userID uuid.UUID, requestedTime time.Time) (time.Time, error)

// Apply settings to batch of notifications
ApplyBatchSettings(ctx context.Context, notifications []*Notification) ([]*Notification, error)
```

## Database Schema

### Users Table Extensions
```sql
ALTER TABLE users
ADD COLUMN timezone VARCHAR(100) DEFAULT 'UTC',
ADD COLUMN timezone_auto_detect BOOLEAN DEFAULT true,
ADD COLUMN last_timezone VARCHAR(100),
ADD COLUMN timezone_changed_at TIMESTAMP WITH TIME ZONE;
```

### DND Overrides Table
```sql
CREATE TABLE dnd_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    reason VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT dnd_override_valid_time CHECK (end_time > start_time),
    CONSTRAINT dnd_override_valid_duration CHECK (end_time <= start_time + INTERVAL '7 days')
);
```

### Timezone History Table
```sql
CREATE TABLE user_timezone_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    old_timezone VARCHAR(100),
    new_timezone VARCHAR(100) NOT NULL,
    changed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    change_source VARCHAR(50) DEFAULT 'manual',
    ip_address INET,
    user_agent TEXT
);
```

## Usage Examples

### Example 1: Schedule Notification in User's Timezone

```go
// Schedule a notification at 9 AM in user's local time
notification, err := integrationExample.Example1_ScheduleNotificationInUserTimezone(
    ctx,
    userID,
    9,  // hour
    0,  // minute
)
```

### Example 2: Handle User Travel

```go
// User changes timezone from New York to Tokyo
err := integrationExample.Example2_HandleUserTravel(
    ctx,
    userID,
    "Asia/Tokyo",
)
// Automatically creates a 24-hour travel override
```

### Example 3: Send Urgent Notification During DND

```go
// Send urgent notification that bypasses DND
shouldSend, reason, err := integrationExample.Example3_SendUrgentNotificationDuringDND(
    ctx,
    userID,
    "Critical Alert",
    "Immediate action required",
)
```

### Example 4: Handle DST Transitions

```go
// Adjust recurring notifications for DST
err := integrationExample.Example4_HandleDSTTransition(
    ctx,
    userID,
    recurringNotifications,
)
// Ensures 9 AM stays 9 AM after DST change
```

### Example 5: Get DND Status

```go
// Get comprehensive DND and override status
status, err := integrationExample.Example5_GetUserDNDStatus(ctx, userID)
// Returns:
// - DND settings and current status
// - Active overrides
// - Upcoming DND windows
// - Next available time
```

### Example 6: Schedule Daily Reminders with DST Handling

```go
// Schedule daily reminders that maintain local time across DST
notifications, err := integrationExample.Example10_ScheduleDailyReminderWithDST(
    ctx,
    userID,
    9,  // local hour
    0,  // local minute
    30, // days ahead
)
```

## DST Handling

### Key DST Features

1. **Automatic Detection**: System automatically detects when DST is active
2. **Transition Handling**: Finds exact DST transition times (accurate to the minute)
3. **Schedule Adjustment**: Maintains local clock time across DST changes
4. **Validation**: Handles non-existent times (spring forward) and ambiguous times (fall back)

### DST Transition Detection

```go
// Get next DST transition
nextTransition := handler.GetNextDSTTransition(time.Now(), location)

// Get spring and fall transitions for a year
spring, fall, err := handler.GetDSTTransitionDates(2024, "America/New_York")
```

### Spring Forward (2 AM → 3 AM)
- Times between 2:00 AM and 3:00 AM don't exist
- Go automatically adjusts to 3:00 AM
- System detects and handles appropriately

### Fall Back (2 AM → 1 AM)
- Times between 1:00 AM and 2:00 AM occur twice
- System disambiguates using context
- First occurrence: DST active
- Second occurrence: Standard time

## DND Rules

### Quiet Hours Configuration

```go
settings := &NotificationSettings{
    DNDEnabled:    true,
    DNDStartTime:  time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC),  // 10 PM
    DNDEndTime:    time.Date(0, 1, 1, 8, 0, 0, 0, time.UTC),   // 8 AM
    DNDDays:       []int{1, 2, 3, 4, 5},  // Monday-Friday
    Timezone:      "America/New_York",
}
```

### DND Features

1. **Time-based**: Start and end times in user's local timezone
2. **Day-based**: Specific days of the week (0=Sunday, 6=Saturday)
3. **Midnight spanning**: Handles periods like 10 PM to 6 AM
4. **Timezone aware**: All times converted to user's timezone

### Override Priority

1. **Emergency overrides** - Always send (24-hour duration)
2. **Active DND overrides** - Bypass DND for specific period
3. **Urgent priority notifications** - Bypass DND
4. **High priority** - Configurable (default: respect DND)
5. **Normal/Low priority** - Always respect DND

## Performance Considerations

### Location Caching
- `time.Location` objects are cached using `sync.Map`
- Concurrent-safe access
- Prevents repeated timezone file parsing

### Database Indexes
```sql
-- User timezone lookups
CREATE INDEX idx_users_timezone ON users(timezone);

-- Active override lookups
CREATE INDEX idx_dnd_overrides_active ON dnd_overrides(user_id, start_time, end_time)
    WHERE end_time > NOW();

-- Expired override cleanup
CREATE INDEX idx_dnd_overrides_cleanup ON dnd_overrides(end_time)
    WHERE end_time < NOW();
```

### Cleanup Job

```sql
-- Function to cleanup old overrides
SELECT cleanup_expired_dnd_overrides();

-- Should be called periodically (e.g., daily cron job)
```

## Testing

### Test Coverage

1. **Timezone Tests** (`timezone_test.go`)
   - Location loading and caching
   - UTC ↔ Local timezone conversion
   - DST detection
   - DST transition calculation
   - Travel detection
   - Concurrent access

2. **DND Override Tests** (`dnd_override_test.go`)
   - Override creation and validation
   - Active override detection
   - Priority-based override logic
   - Travel overrides
   - Emergency overrides

### Test Timezones

Tests use real timezones:
- `America/New_York` - Eastern Time (DST observed)
- `Asia/Tokyo` - Japan Standard Time (no DST)
- `Europe/London` - British Time (DST observed)

### Running Tests

```bash
# Run timezone tests
go test ./internal/notification/timezone_test.go ./internal/notification/timezone.go -v

# Run DND override tests
go test ./internal/notification/dnd_override_test.go ./internal/notification/dnd_override.go -v

# Run all notification tests
go test ./internal/notification/... -v
```

## Common Timezones

The system includes 50+ common timezones organized by region:

- **Americas**: New York, Chicago, Denver, Los Angeles, Toronto, etc.
- **Europe**: London, Paris, Berlin, Moscow, etc.
- **Asia**: Tokyo, Seoul, Shanghai, Singapore, etc.
- **Oceania**: Sydney, Melbourne, Auckland, etc.
- **Africa**: Cairo, Johannesburg, Lagos, etc.

## Migration Guide

### From UTC-only to Timezone-aware

1. **Run migration**: `000049_user_timezone_preferences.up.sql`
2. **Update user records**: Set default timezone based on location
3. **Migrate existing schedules**: Convert to user timezones
4. **Test DND rules**: Verify quiet hours work correctly
5. **Enable travel detection**: Monitor timezone changes

### Configuration

```go
// Initialize components
tzHandler := NewTimezoneHandler(logger)
tzManager := NewTimezoneManager(logger)
dndScheduler := NewDNDScheduler(logger)
dndOverrideRepo := NewDNDOverridePostgresRepository(db, logger)
dndOverrideService := NewDNDOverrideService(logger, dndOverrideRepo)

// Create settings engine with timezone support
settingsEngine := NewSettingsEngine(logger, settingsProvider)
```

## Error Handling

### Invalid Timezone
```go
err := handler.ValidateTimezone("Invalid/Timezone")
// Returns error, falls back to UTC
```

### DST Transition Edge Cases
```go
// Non-existent time (spring forward)
springTime := time.Date(2024, 3, 10, 2, 30, 0, 0, loc)
// Automatically adjusted to 3:30 AM

// Ambiguous time (fall back)
fallTime := time.Date(2024, 11, 3, 1, 30, 0, 0, loc)
// Uses first occurrence (DST active)
```

### Override Validation
```go
err := service.ValidateOverride(override)
// Checks:
// - User ID not nil
// - End time after start time
// - Reason provided
// - Duration <= 7 days
```

## Best Practices

1. **Always store times in UTC** in the database
2. **Convert to user timezone** only for display or scheduling
3. **Handle DST transitions** for recurring events
4. **Validate timezones** before storing
5. **Use override reasons** for audit trails
6. **Monitor travel patterns** for user experience
7. **Cleanup expired overrides** regularly
8. **Cache location objects** for performance
9. **Test with real timezones** not just UTC
10. **Log timezone operations** for debugging

## Security Considerations

1. **Rate limiting**: Prevent abuse of override creation
2. **Audit logging**: Track timezone changes in history table
3. **IP address tracking**: Detect suspicious timezone changes
4. **Override limits**: Maximum 7-day duration
5. **User validation**: Ensure user can only modify their own settings

## Monitoring

### Key Metrics

- Active DND overrides count
- Timezone change frequency
- DST transition handling success rate
- DND bypass rate by priority
- Average override duration
- Travel detection accuracy

### Logging

All components use structured logging (zap):
- Info: Normal operations, status changes
- Warn: Failed validations, fallbacks to UTC
- Error: Database errors, critical failures
- Debug: Detailed timezone conversions

## Future Enhancements

1. **Smart scheduling**: ML-based optimal notification times
2. **Timezone prediction**: Predict travel based on patterns
3. **Custom DND profiles**: Multiple DND schedules
4. **Location-based DND**: Auto-enable based on GPS
5. **Calendar integration**: Sync with user's calendar quiet hours
6. **Group DND**: Team-wide quiet hours
7. **Holiday detection**: Adjust schedules for holidays
8. **Notification batching**: Bundle notifications outside quiet hours

## References

- IANA Time Zone Database: https://www.iana.org/time-zones
- Go time package: https://pkg.go.dev/time
- PostgreSQL timezone support: https://www.postgresql.org/docs/current/datatype-datetime.html
