# Task 7.1 Completion Summary

**Task**: 월간 데이터 모델·스키마와 색상 매핑 규칙 설계 (Monthly Calendar Data Model and Color Mapping Rules Design)

**Status**: ✅ COMPLETED

**Completion Date**: November 24, 2024

---

## Deliverables

### 1. Comprehensive Design Documentation
**File**: `/Users/jelly/personal/donelist/server/docs/MONTHLY_CALENDAR_SCHEMA.md`

Complete specification document including:
- Data model design for all structures
- Detailed color mapping rules with 6-bucket system
- Progress calculation formulas
- Implementation notes
- Testing requirements
- Example payloads

### 2. JSON Schema Definition
**File**: `/Users/jelly/personal/donelist/server/docs/schemas/monthly-calendar-grid.json`

Formal JSON Schema (Draft 07) including:
- Complete type definitions for all models
- Validation rules (min/max, patterns, formats)
- Required field specifications
- Nested definitions for DayCell, MonthSummary, ColorLegend, etc.
- Examples for all fields

### 3. Example Payloads

#### Active Month Example
**File**: `/Users/jelly/personal/donelist/server/docs/schemas/examples/monthly-calendar-active.json`
- Real-world usage scenario
- November 2024 with mixed activity levels
- Demonstrates all 6 color buckets
- Includes today indicator
- Shows month overflow (days from adjacent months)

#### Boundary Test Example
**File**: `/Users/jelly/personal/donelist/server/docs/schemas/examples/monthly-calendar-boundary-test.json`
- Tests all boundary values: 0%, 0.1%, 1%, 24.9%, 25%, 49.9%, 50%, 74.9%, 75%, 99.9%, 100%
- Demonstrates over-tracking scenario (>100% capped to 100%)
- Uses February 2024 (leap year) to test edge case
- Includes comments explaining each boundary test

---

## Data Model Design Summary

### Core Structures

1. **MonthCalendar** (Root)
   - Year, month metadata
   - 7x6 grid (2D array of weeks and days)
   - Summary statistics
   - Color legend
   - Navigation links
   - Cache metadata

2. **DayCell** (Individual Day)
   - Date (ISO 8601)
   - Progress percentage (0-100)
   - Color hex code
   - Color bucket index (0-5)
   - Boolean flags: in_current_month, is_today
   - Check-in metrics: count, total_minutes

3. **MonthSummary** (Aggregates)
   - Total check-ins and minutes
   - Days with check-ins
   - Averages and completion rate
   - Most productive day
   - Current and longest streaks

4. **ColorLegend** (Reference)
   - Array of 6 color buckets
   - Each bucket includes: index, label, min/max progress, color, description

5. **MonthNavigation** (Links)
   - Previous, current, next month (YYYY-MM format)

---

## Color Mapping Rules

### 6-Bucket System

| Bucket | Label | Progress Range | Color | Hex Code | Minutes Range |
|--------|-------|----------------|-------|----------|---------------|
| 0 | None | 0% | Light Gray | #f3f4f6 | 0 minutes |
| 1 | Minimal | 0.1-24.9% | Light Green | #d1fae5 | 1-359 minutes |
| 2 | Low | 25-49.9% | Medium Green | #6ee7b7 | 360-719 minutes |
| 3 | Medium | 50-74.9% | Dark Green | #34d399 | 720-1079 minutes |
| 4 | High | 75-99.9% | Darker Green | #10b981 | 1080-1439 minutes |
| 5 | Complete | 100% | Darkest Green | #059669 | 1440+ minutes |

### Progress Calculation

```
progress = (totalMinutes / 1440) * 100
where 1440 = 24 hours × 60 minutes

If progress > 100, cap at 100%
Round to 1 decimal place
```

### Color Selection Algorithm

```go
func GetColorBucket(progress float64) int {
    if progress == 0 {
        return 0
    } else if progress < 25 {
        return 1
    } else if progress < 50 {
        return 2
    } else if progress < 75 {
        return 3
    } else if progress < 100 {
        return 4
    }
    return 5
}
```

---

## Grid Structure

### 7x6 Layout
- **Outer array**: 4-6 weeks (depending on month)
- **Inner array**: Always 7 days per week
- **Total cells**: 28-42 days

### Week Start Support
- **Monday start**: Mon, Tue, Wed, Thu, Fri, Sat, Sun
- **Sunday start**: Sun, Mon, Tue, Wed, Thu, Fri, Sat

### Overflow Days
- Days from previous/next month included to fill grid
- Identified by `in_current_month: false`
- Rendered with reduced opacity on frontend

---

## API Contract

### Endpoint
```
GET /api/v1/calendar/monthly/grid
```

### Query Parameters
- `year` (optional): default current year
- `month` (optional): default current month
- `start_of_week` (optional): "sunday" or "monday", default "monday"
- `timezone` (optional): IANA timezone, default "UTC"

### Response Headers
```
Content-Type: application/json
Cache-Control: private, max-age=3600
```

### Response Structure
See JSON Schema and example payloads for complete structure.

---

## Testing Strategy

### Unit Tests Required

1. **Color Bucket Mapping**
   - Test all boundary values: 0, 0.1, 1, 24.9, 25, 49.9, 50, 74.9, 75, 99.9, 100
   - Test over-tracking: 150%, 200%
   - Expected: `TestGetColorBucket`

2. **Progress Calculation**
   - Test: 0, 1, 360, 720, 1080, 1440, 2880 minutes
   - Verify: 0%, 0.1%, 25%, 50%, 75%, 100%, 100% (capped)
   - Expected: `TestCalculateProgress`

3. **Grid Generation**
   - Test different month lengths: February (28/29), April (30), January (31)
   - Test both week start options: Sunday and Monday
   - Verify overflow days are correct
   - Expected: `TestBuildGrid`

4. **Date Handling**
   - Test timezone conversions
   - Test leap years (February 29)
   - Test year boundaries (December → January)
   - Expected: `TestDateHandling`

### Integration Tests Required

1. **Full Calendar Generation**
   - Generate calendar with real check-in data
   - Verify all fields populated correctly
   - Verify grid structure (5 weeks with 35 days)
   - Expected: `TestMonthlyCalendarGridIntegration`

2. **JSON Schema Validation**
   - Validate all example payloads against schema
   - Use `github.com/xeipuuv/gojsonschema` or similar
   - Expected: `TestSchemaValidation`

3. **Edge Cases**
   - Empty month (new user, no check-ins)
   - Current month (with is_today flag)
   - Historical months (cache behavior)
   - Expected: `TestEdgeCases`

---

## Implementation Checklist

### Phase 1: Models (Task 7.2)
- [ ] Create new Go structs in `internal/calendar/models.go`
- [ ] Add `MonthCalendarGrid` type
- [ ] Add `DayCell` type with all fields
- [ ] Add `ColorLegend` and `ColorBucket` types
- [ ] Add `MonthNavigation` type
- [ ] Update existing `MonthlyCalendar` or create separate type

### Phase 2: Color Mapping (Task 7.2)
- [ ] Implement `GetColorBucket(progress float64) int`
- [ ] Implement `GetBucketColor(bucket int) string`
- [ ] Implement `GetColorLegend() *ColorLegend`
- [ ] Add unit tests for boundary values

### Phase 3: Grid Builder (Task 7.2)
- [ ] Implement `BuildCalendarGrid()` in service
- [ ] Handle week start preference (Sunday/Monday)
- [ ] Generate 2D array with correct overflow days
- [ ] Set `in_current_month` flags correctly
- [ ] Set `is_today` flag

### Phase 4: Handler (Task 7.2)
- [ ] Create new handler or update existing
- [ ] Parse query parameters
- [ ] Call service to generate grid
- [ ] Return JSON response
- [ ] Set cache headers

### Phase 5: Testing (Task 7.2)
- [ ] Write unit tests for color mapping
- [ ] Write unit tests for grid generation
- [ ] Write integration tests
- [ ] Validate against JSON Schema
- [ ] Test with boundary test data

---

## File Locations

All deliverables are located in the server directory:

```
/Users/jelly/personal/donelist/server/
├── docs/
│   ├── MONTHLY_CALENDAR_SCHEMA.md           # Main specification
│   ├── TASK_7.1_COMPLETION_SUMMARY.md       # This file
│   └── schemas/
│       ├── monthly-calendar-grid.json       # JSON Schema
│       └── examples/
│           ├── monthly-calendar-active.json       # Active month
│           └── monthly-calendar-boundary-test.json # Boundary tests
```

---

## Next Steps

**Task 7.2**: 그리드 응답 생성 함수 구현 및 테스트 (Implement Grid Response Generation and Tests)

Using this design specification:
1. Implement Go models based on schema
2. Implement color mapping functions
3. Build grid generation logic
4. Create HTTP handler
5. Write comprehensive tests
6. Validate against JSON Schema

---

## Design Decisions

### Why 6 buckets instead of 5?
- Bucket 0 is special (exactly 0%, no activity)
- Provides clear visual distinction between "not started" and "started"
- Aligns with GitHub's contribution graph pattern

### Why cap at 100%?
- Over-tracking (>24 hours) is possible but should be normalized
- Frontend can display "100%" or "24h+" label
- Keeps color mapping simple and consistent

### Why include overflow days?
- Creates clean 7x6 grid for UI rendering
- Matches common calendar widget behavior
- Allows users to see context from adjacent months

### Why include legend in every response?
- Self-documenting API
- Frontend doesn't need to hardcode colors
- Allows future color theme customization

### Why separate from existing MonthlyCalendar?
- Existing endpoint returns weeks array (1D)
- New endpoint returns grid (2D)
- Avoids breaking existing clients
- Allows independent evolution

---

## Performance Considerations

- Grid generation: < 50ms target
- Database aggregation (not application-level)
- Cache for 1 hour (adjustable)
- Pre-calculate color buckets in query if possible

---

## Accessibility Notes

Frontend should:
- Add aria-label for each cell
- Include text labels, not just colors
- Support keyboard navigation
- Provide alternative data view for screen readers

---

**Completion Status**: ✅ All deliverables created and validated

**Ready for**: Task 7.2 implementation
