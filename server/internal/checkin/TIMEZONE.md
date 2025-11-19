# Timezone and DST Safety Documentation

## Overview

This document describes how the Donelist check-in system handles timezones and Daylight Saving Time (DST) transitions to ensure consistent behavior across all users regardless of their location.

## Architecture

### Three-Layer Approach

1. **Storage Layer (PostgreSQL)**
   - Uses `TIMESTAMP WITH TIME ZONE` for all time fields
   - Stores all times in UTC internally
   - Automatically converts incoming times to UTC

2. **Application Layer (Go)**
   - All calculations use UTC: `time.Now().UTC()`
   - Incoming times converted to UTC: `time.UTC()`
   - No local timezone dependencies

3. **API Layer (JSON/RFC3339)**
   - Times exchanged in RFC3339 format (ISO 8601 with timezone)
   - Clients can send times in their local timezone
   - Server automatically converts to UTC for calculations

## Time Format Examples

### Valid RFC3339 Formats

```json
{
  "checkin_time": "2025-03-09T15:00:00Z"           // UTC
}

{
  "checkin_time": "2025-03-09T10:00:00-05:00"      // EST (Eastern)
}

{
  "checkin_time": "2025-03-09T23:00:00+09:00"      // JST (Japan)
}
```

All of these are equivalent and stored as the same UTC time.

## DST Handling

### Spring Forward (Lost Hour)

When clocks "spring forward" (e.g., 2:00 AM → 3:00 AM):

**Problem:** What if a user tries to check in at 2:30 AM?

**Solution:**
- Client sends time in their timezone with proper offset
- Server converts to UTC (which doesn't have DST)
- No ambiguity in storage or calculations

**Example:**
```
User in EST tries to check in at "2:30 AM" on March 9, 2025
Client sends: "2025-03-09T03:30:00-04:00" (already adjusted for DST)
Server stores: "2025-03-09T07:30:00Z" (UTC)
```

### Fall Back (Repeated Hour)

When clocks "fall back" (e.g., 2:00 AM → 1:00 AM):

**Problem:** The hour from 1:00-2:00 AM occurs twice

**Solution:**
- RFC3339 includes timezone offset
- First 1:30 AM: "2025-11-02T01:30:00-04:00" (EDT)
- Second 1:30 AM: "2025-11-02T01:30:00-05:00" (EST)
- Different offsets = different UTC times

## Check-in Interval Rules with DST

### Scenario: User checks in before DST transition

```
User in New York (EST → EDT transition at 2:00 AM on March 9, 2025)

Check-in 1: March 9, 1:30 AM EST = 06:30 UTC
User requests 30-minute interval check-in

Next eligible time: 06:30 UTC + 30 minutes = 07:00 UTC
                   = March 9, 3:00 AM EDT (after DST transition)
```

The interval calculation is **correct** because:
- UTC doesn't have DST
- 30 minutes in UTC = 30 minutes in any timezone
- User sees "3:00 AM" (not "2:00 AM") due to DST shift

### Scenario: Check-in spans DST boundary

```
User in New York

Check-in 1: March 9, 12:00 AM EST = 05:00 UTC
User requests 2-hour interval check-in

Next eligible time: 05:00 UTC + 2 hours = 07:00 UTC
                   = March 9, 3:00 AM EDT

User experiences:
- Checked in at 12:00 AM
- Must wait until 3:00 AM (clock skipped 2:00 AM due to DST)
- Actually waited 3 wall-clock hours, but 2 absolute hours
```

This is **correct behavior** - we enforce absolute time intervals, not wall-clock intervals.

## Clock Skew Tolerance

The system includes a 1-second tolerance (`ClockSkewTolerance`) to handle:

1. **Network Latency**
   - Request sent at exactly 120:00.000
   - Arrives at server at 120:00.100
   - Tolerance prevents rejection

2. **Client Clock Drift**
   - Client clock 0.5 seconds fast
   - Would violate exact boundary checks
   - Tolerance normalizes minor differences

3. **Floating Point Precision**
   - Duration calculations may have sub-second precision
   - Tolerance prevents spurious boundary violations

## Implementation Details

### Database Schema

```sql
CREATE TABLE checkins (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    checkin_time TIMESTAMP WITH TIME ZONE NOT NULL,  -- Stored in UTC
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ...
);
```

### Go Time Handling

```go
// ✅ Correct: Always use UTC
now := time.Now().UTC()
checkinTime.UTC()

// ❌ Wrong: Don't use local time
now := time.Now()  // Uses server's local timezone
```

### API Request/Response

**Request (Client in Tokyo):**
```json
POST /api/v1/checkins
{
  "content": "Finished morning workout",
  "checkin_time": "2025-03-09T23:00:00+09:00",
  "duration_minutes": 30
}
```

**Response:**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "checkin_time": "2025-03-09T14:00:00Z",  // Converted to UTC
  "created_at": "2025-03-09T14:00:00Z",
  ...
}
```

**Client Handling:**
```javascript
// JavaScript example
const checkinTime = new Date("2025-03-09T14:00:00Z");
console.log(checkinTime.toLocaleString('ja-JP', { timeZone: 'Asia/Tokyo' }));
// Output: "2025/3/9 23:00:00" (converted to user's timezone)
```

## Testing Considerations

### Unit Tests Should Cover

1. **Timezone Conversions**
   - UTC → Local → UTC round-trip
   - Multiple IANA timezones (America/New_York, Asia/Tokyo, Europe/London)

2. **DST Transitions**
   - Spring forward scenarios
   - Fall back scenarios
   - Before/after transition calculations

3. **Boundary Conditions**
   - Exactly 120 minutes
   - 119:59.999 vs 120:00.001
   - Clock skew tolerance edge cases

4. **Long-term Intervals**
   - Multi-day intervals across DST
   - Year boundaries (Dec 31 → Jan 1)
   - Leap year handling (Feb 29)

### Integration Test Example

```go
func TestDSTTransition(t *testing.T) {
    // Set up: User in New York, DST transition at 2 AM on March 9
    loc, _ := time.LoadLocation("America/New_York")

    // Check-in before DST (1:30 AM EST)
    checkinTime := time.Date(2025, 3, 9, 1, 30, 0, 0, loc)

    // Request 30-minute interval
    nextEligible := NextEligibleCheckinTime(checkinTime, 30*time.Minute)

    // Should be 3:00 AM EDT (not 2:00 AM, which doesn't exist)
    expected := time.Date(2025, 3, 9, 3, 0, 0, 0, loc)

    if !nextEligible.Equal(expected) {
        t.Errorf("Expected %v, got %v", expected, nextEligible)
    }
}
```

## Security Considerations

### Timezone Manipulation Prevention

**Attack:** User manipulates timezone to bypass interval limits

**Example:**
- User checks in at 10:00 AM UTC
- Claims next check-in is "10:15 AM" in UTC-5 timezone
- Actually equals 15:15 UTC (5 hours later in absolute time)

**Defense:**
- Server converts all times to UTC before validation
- Interval checks use absolute UTC time differences
- Timezone offset cannot affect calculation

```go
// This is why we do this:
now := time.Now().UTC()
lastCheckinUTC := lastCheckin.UTC()
elapsed := now.Sub(lastCheckinUTC)  // Absolute time difference
```

### Server Timezone Independence

**Problem:** Server running in Tokyo processes requests from US users

**Solution:**
- Server never uses `time.Now()` without `.UTC()`
- Server timezone configuration doesn't affect behavior
- Can redeploy to different regions without code changes

## Troubleshooting

### Issue: User sees unexpected "next eligible time"

**Cause:** DST transition occurred between check-ins

**Example:**
```
User: "I checked in at midnight and chose 2-hour interval.
       Why does it say I can check in at 3 AM instead of 2 AM?"

Answer: DST spring forward happened at 2 AM.
        2 AM doesn't exist on this date.
        3 AM is the correct time (2 absolute hours later).
```

### Issue: API returns times in UTC, client expected local time

**Solution:** Client must convert to local timezone

```javascript
// ❌ Wrong: Display UTC time directly
<p>Check-in time: {checkin.checkin_time}</p>

// ✅ Correct: Convert to user's timezone
<p>Check-in time: {new Date(checkin.checkin_time).toLocaleString()}</p>
```

### Issue: Interval validation fails for valid request

**Check:**
1. Is client sending time in RFC3339 format?
2. Is timezone offset included?
3. Is server using UTC for calculations?
4. Check server logs for actual time values

## References

- [RFC3339 Specification](https://www.rfc-editor.org/rfc/rfc3339)
- [PostgreSQL TIMESTAMP WITH TIME ZONE](https://www.postgresql.org/docs/current/datatype-datetime.html)
- [Go time package](https://pkg.go.dev/time)
- [IANA Time Zone Database](https://www.iana.org/time-zones)

## Summary

✅ **Storage:** PostgreSQL TIMESTAMP WITH TIME ZONE (UTC internally)
✅ **Calculation:** Go time.Time with .UTC() (UTC explicitly)
✅ **Transport:** RFC3339 format (timezone-aware)
✅ **DST Handling:** UTC-based calculations avoid DST issues
✅ **Security:** Timezone manipulation prevented by UTC normalization

This architecture ensures consistent, predictable behavior across all timezones and DST transitions.
