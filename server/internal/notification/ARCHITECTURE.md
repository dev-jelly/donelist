# Notification System Architecture with Timezone & DND

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         API Layer                                    │
│  ┌────────────┐  ┌──────────────┐  ┌─────────────────┐             │
│  │  Schedule  │  │   Timezone   │  │  DND Override   │             │
│  │Notification│  │  Management  │  │   Management    │             │
│  └────────────┘  └──────────────┘  └─────────────────┘             │
└───────────────────────────┬─────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     Service Layer                                    │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │              SettingsEngine                                  │   │
│  │  • Checks notification eligibility                          │   │
│  │  • Applies DND rules                                        │   │
│  │  • Handles batch processing                                 │   │
│  └────────────┬──────────────────────┬────────────────────────┘   │
│               │                      │                              │
│               ▼                      ▼                              │
│  ┌───────────────────┐   ┌──────────────────────┐                 │
│  │  DNDScheduler     │   │ DNDOverrideService   │                 │
│  │  • Check DND      │   │ • Create overrides   │                 │
│  │  • Calculate      │   │ • Priority logic     │                 │
│  │    windows        │   │ • Travel overrides   │                 │
│  └─────────┬─────────┘   └──────────┬───────────┘                 │
│            │                        │                              │
│            └────────────┬───────────┘                              │
│                         │                                          │
│                         ▼                                          │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │           TimezoneHandler & TimezoneManager              │    │
│  │  • Timezone conversion (UTC ↔ Local)                     │    │
│  │  • DST detection & transition calculation                │    │
│  │  • Travel detection                                      │    │
│  │  • Location caching                                      │    │
│  └──────────────────────────────────────────────────────────┘    │
└───────────────────────────┬─────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     Repository Layer                                 │
│  ┌──────────────────────┐  ┌────────────────────────────────┐      │
│  │  SettingsRepository  │  │ DNDOverrideRepository          │      │
│  │  • Get user settings │  │ • Create/Get/Delete overrides  │      │
│  │  • Update DND config │  │ • Cleanup expired              │      │
│  └──────────────────────┘  └────────────────────────────────┘      │
└───────────────────────────┬─────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     Database (PostgreSQL)                            │
│  ┌────────────┐  ┌──────────────────┐  ┌─────────────────────┐    │
│  │   users    │  │ dnd_overrides    │  │ user_timezone_      │    │
│  │  timezone  │  │  start_time      │  │      history        │    │
│  │  dnd_*     │  │  end_time        │  │  old_timezone       │    │
│  └────────────┘  │  reason          │  │  new_timezone       │    │
│                  └──────────────────┘  └─────────────────────┘    │
│  ┌──────────────────────────────────┐                             │
│  │  notification_settings           │                             │
│  │    dnd_enabled                   │                             │
│  │    dnd_start_time                │                             │
│  │    dnd_end_time                  │                             │
│  │    dnd_days                      │                             │
│  │    timezone                      │                             │
│  └──────────────────────────────────┘                             │
└─────────────────────────────────────────────────────────────────────┘
```

## Component Relationships

```
┌──────────────────────────────────────────────────────────┐
│                 Notification Scheduler                    │
│                                                           │
│  1. Get user timezone and DND settings                   │
│  2. Convert notification time to user's local time       │
│  3. Check if time falls in DND period                    │
│  4. Check for active DND overrides                       │
│  5. Apply priority-based override logic                  │
│  6. Calculate next available time if needed              │
│  7. Schedule notification                                │
└──────────────────────────────────────────────────────────┘
         │                      │                    │
         │                      │                    │
         ▼                      ▼                    ▼
┌──────────────┐    ┌────────────────┐    ┌─────────────────┐
│  Timezone    │    │  DND Scheduler │    │  DND Override   │
│  Handler     │    │                │    │    Service      │
│              │    │  • Check DND   │    │                 │
│  • Convert   │    │  • Calculate   │    │  • Check active │
│    UTC/Local │    │    windows     │    │  • Priority     │
│  • DST check │    │  • Next time   │    │    logic        │
│  • Validate  │    │                │    │                 │
└──────────────┘    └────────────────┘    └─────────────────┘
```

## Data Flow: Scheduling a Notification

```
User Request (Schedule at 9 AM local)
          │
          ▼
┌─────────────────────────────┐
│ 1. Get User Settings        │
│    - timezone: "Asia/Tokyo" │
│    - dnd_enabled: true      │
│    - dnd_start: 22:00       │
│    - dnd_end: 08:00         │
└─────────────┬───────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│ 2. Convert to UTC                   │
│    9 AM JST → 0:00 UTC (prev day)   │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│ 3. Check DND Period                 │
│    Is 9 AM in DND (22:00-08:00)?    │
│    Result: YES (falls in DND)       │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│ 4. Check Active Overrides           │
│    Any override at this time?       │
│    Result: NO override              │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│ 5. Check Priority                   │
│    Priority: NORMAL                 │
│    Override DND? NO                 │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│ 6. Calculate Next Available Time    │
│    DND ends at 8 AM local           │
│    Reschedule to 8 AM JST           │
│    = 23:00 UTC (prev day)           │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│ 7. Schedule Notification            │
│    Scheduled for: 23:00 UTC         │
│    (8 AM JST, after DND)            │
└─────────────────────────────────────┘
```

## DST Transition Handling

```
┌────────────────────────────────────────────────────────────┐
│          DST Spring Forward (2 AM → 3 AM)                  │
│                                                            │
│  Before DST (March 9, 2024)                               │
│  ┌─────────────────────────────────────────────┐         │
│  │ User Schedule: 9 AM EST (UTC-5)             │         │
│  │ UTC Time: 14:00                             │         │
│  └─────────────────────────────────────────────┘         │
│                      │                                     │
│                      │ DST Transition                      │
│                      │ (March 10, 2 AM → 3 AM)            │
│                      ▼                                     │
│  After DST (March 10, 2024)                               │
│  ┌─────────────────────────────────────────────┐         │
│  │ User Schedule: 9 AM EDT (UTC-4)             │         │
│  │ UTC Time: 13:00                             │         │
│  │ ✓ Still 9 AM local (maintained)            │         │
│  └─────────────────────────────────────────────┘         │
│                                                            │
│  System automatically:                                     │
│  • Detects DST transition                                 │
│  • Adjusts UTC time to maintain local time                │
│  • Updates all recurring schedules                        │
└────────────────────────────────────────────────────────────┘
```

## DND Override Priority Hierarchy

```
┌────────────────────────────────────────┐
│         Notification Priority          │
└────────────────┬───────────────────────┘
                 │
                 ▼
┌────────────────────────────────────────┐
│  1. Check Active DND Override          │
│     Emergency/Travel/Manual            │
│     → SEND (bypass DND)                │
└────────────┬───────────────────────────┘
             │ No Override
             ▼
┌────────────────────────────────────────┐
│  2. Check Notification Priority        │
│     URGENT → SEND (bypass DND)         │
└────────────┬───────────────────────────┘
             │ Not Urgent
             ▼
┌────────────────────────────────────────┐
│  3. Check DND Settings                 │
│     In DND Period? → DEFER             │
│     Outside DND? → SEND                │
└────────────────────────────────────────┘
```

## Travel Detection Flow

```
User changes timezone
          │
          ▼
┌──────────────────────────────────┐
│ Detect Timezone Change           │
│ NY → Tokyo (12 hour difference)  │
└─────────────┬────────────────────┘
              │
              ▼
┌──────────────────────────────────┐
│ Calculate Offset Difference      │
│ Threshold: 1 hour                │
│ Difference: 13 hours → TRAVEL    │
└─────────────┬────────────────────┘
              │
              ▼
┌──────────────────────────────────┐
│ Log Timezone History             │
│ • old_timezone: America/New_York │
│ • new_timezone: Asia/Tokyo       │
│ • changed_at: 2024-11-24T08:00Z  │
└─────────────┬────────────────────┘
              │
              ▼
┌──────────────────────────────────┐
│ Create Travel Override           │
│ • duration: 24 hours             │
│ • reason: travel                 │
│ • Allows schedule adjustment     │
└──────────────────────────────────┘
```

## Notification Lifecycle

```
┌──────────────────┐
│ Create           │
│ Notification     │
└────────┬─────────┘
         │
         ▼
┌──────────────────────────────┐
│ Apply Timezone Conversion    │
│ (Local Time → UTC)           │
└────────┬─────────────────────┘
         │
         ▼
┌──────────────────────────────┐
│ Check DND Settings           │
│ (Is time in quiet hours?)    │
└────────┬─────────────────────┘
         │
         ├── In DND ──┐
         │            │
         │            ▼
         │      ┌────────────────────┐
         │      │ Check Overrides    │
         │      │ (Active? Priority?)│
         │      └─────┬──────────────┘
         │            │
         │            ├── Override ──┐
         │            │              │
         │            ▼              │
         │      ┌──────────────┐    │
         │      │ Calculate    │    │
         │      │ Next Time    │    │
         │      └──────┬───────┘    │
         │             │             │
         └─────────────┴─────────────┘
                       │
                       ▼
              ┌─────────────────┐
              │ Queue for       │
              │ Delivery        │
              └────────┬────────┘
                       │
                       ▼
              ┌─────────────────┐
              │ Send at         │
              │ Scheduled Time  │
              └─────────────────┘
```

## Performance Optimization

```
┌─────────────────────────────────────────┐
│          Location Cache                  │
│  ┌────────────────────────────────┐     │
│  │ sync.Map (thread-safe)         │     │
│  │                                 │     │
│  │ "America/New_York" → *Location │     │
│  │ "Asia/Tokyo"       → *Location │     │
│  │ "Europe/London"    → *Location │     │
│  │ ...                             │     │
│  └────────────────────────────────┘     │
│                                          │
│  Benefits:                               │
│  • Avoid repeated file parsing           │
│  • O(1) lookup time                      │
│  • Thread-safe concurrent access         │
│  • Reduced memory allocation             │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│       Database Indexes                   │
│  ┌────────────────────────────────┐     │
│  │ idx_users_timezone             │     │
│  │ idx_dnd_overrides_active       │     │
│  │   WHERE end_time > NOW()       │     │
│  │ idx_dnd_overrides_cleanup      │     │
│  │   WHERE end_time < NOW()       │     │
│  └────────────────────────────────┘     │
│                                          │
│  Benefits:                               │
│  • Fast user timezone lookups            │
│  • Efficient active override queries     │
│  • Quick expired override cleanup        │
│  • Partial indexes reduce size           │
└─────────────────────────────────────────┘
```

## Error Handling Strategy

```
┌────────────────────────────────────────┐
│  Invalid Timezone                      │
│  "Invalid/Timezone"                    │
└─────────────┬──────────────────────────┘
              │
              ▼
┌────────────────────────────────────────┐
│  1. Log Warning                        │
│  2. Return Error                       │
│  3. Fallback to UTC                    │
│  4. Continue Processing                │
└────────────────────────────────────────┘

┌────────────────────────────────────────┐
│  DST Edge Case                         │
│  2:30 AM on Spring Forward Day         │
└─────────────┬──────────────────────────┘
              │
              ▼
┌────────────────────────────────────────┐
│  1. Detect non-existent time           │
│  2. Go automatically adjusts to 3:30   │
│  3. Log adjustment                     │
│  4. Use adjusted time                  │
└────────────────────────────────────────┘

┌────────────────────────────────────────┐
│  Database Connection Error             │
└─────────────┬──────────────────────────┘
              │
              ▼
┌────────────────────────────────────────┐
│  1. Log Error                          │
│  2. Retry with backoff                 │
│  3. Return error to caller             │
│  4. Update metrics                     │
└────────────────────────────────────────┘
```

## Integration Points

```
┌─────────────────────────────────────────────────────────┐
│                  External Systems                        │
└─────────────────────────────────────────────────────────┘
     │              │              │              │
     │              │              │              │
     ▼              ▼              ▼              ▼
┌─────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│  Email  │  │   Push   │  │   SMS    │  │ Webhook  │
│Provider │  │ Service  │  │ Gateway  │  │ Service  │
└────┬────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │            │              │              │
     └────────────┴──────────────┴──────────────┘
                  │
                  ▼
     ┌──────────────────────────┐
     │  Notification Processor   │
     │  • Timezone conversion    │
     │  • DND checking           │
     │  • Priority routing       │
     └──────────────────────────┘
```

## Monitoring & Metrics

```
Key Metrics to Track:

┌────────────────────────────────────┐
│ Timezone Metrics                   │
│ • Timezone changes per day         │
│ • Most common timezones            │
│ • Travel detection accuracy        │
│ • DST transition errors            │
└────────────────────────────────────┘

┌────────────────────────────────────┐
│ DND Metrics                        │
│ • Active DND users                 │
│ • Deferred notification count      │
│ • Override usage by reason         │
│ • DND bypass rate by priority      │
└────────────────────────────────────┘

┌────────────────────────────────────┐
│ Performance Metrics                │
│ • Location cache hit rate          │
│ • Average conversion time          │
│ • Database query latency           │
│ • Active override count            │
└────────────────────────────────────┘
```

## Security Model

```
┌────────────────────────────────────────┐
│          User Permissions              │
└────────────────────────────────────────┘
              │
              ▼
┌────────────────────────────────────────┐
│  Own Settings                          │
│  • Read/Update timezone                │
│  • Configure DND                       │
│  • Create/Delete own overrides         │
│  • View own history                    │
└────────────────────────────────────────┘

┌────────────────────────────────────────┐
│          Admin Permissions             │
└────────────────────────────────────────┘
              │
              ▼
┌────────────────────────────────────────┐
│  System Management                     │
│  • View all overrides                  │
│  • Monitor timezone changes            │
│  • Cleanup expired data                │
│  • Access audit logs                   │
└────────────────────────────────────────┘

┌────────────────────────────────────────┐
│          Audit Trail                   │
└────────────────────────────────────────┘
              │
              ▼
┌────────────────────────────────────────┐
│  user_timezone_history                 │
│  • Who changed                         │
│  • What changed (old → new)            │
│  • When changed                        │
│  • How changed (source)                │
│  • Where from (IP address)             │
└────────────────────────────────────────┘
```

This architecture provides a robust, scalable, and maintainable solution for timezone-aware notification scheduling with comprehensive DND support.
