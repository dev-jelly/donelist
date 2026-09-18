# Monthly Calendar Data Model and Schema Design

**Task 7.1** - Design specification for monthly calendar view with 7x6 grid layout and progress-based color mapping.

## Table of Contents
1. [Overview](#overview)
2. [Data Model Design](#data-model-design)
3. [Color Mapping Rules](#color-mapping-rules)
4. [JSON Schema](#json-schema)
5. [Example Payloads](#example-payloads)
6. [Implementation Notes](#implementation-notes)

---

## Overview

The monthly calendar view provides a visual representation of user productivity across a month using a 7x6 grid (up to 42 days). Each cell represents a single day with progress percentage (0-100%) mapped to color intensity levels.

### Design Goals
- **Visual clarity**: Easy-to-understand color-coded progress indicators
- **Flexibility**: Support both Sunday and Monday week starts
- **Performance**: Efficient data structure for rendering
- **Consistency**: Align with existing calendar service implementation

---

## Data Model Design

### 1. MonthCalendar (Root Structure)

```go
type MonthCalendar struct {
    Year            int              `json:"year"`             // Calendar year (e.g., 2024)
    Month           int              `json:"month"`            // Month number (1-12)
    MonthName       string           `json:"month_name"`       // Human-readable month (e.g., "November")
    StartOfWeek     StartDay         `json:"start_of_week"`    // "sunday" or "monday"
    Grid            [][]*DayCell     `json:"grid"`             // 7x6 matrix (weeks x days)
    Summary         *MonthSummary    `json:"summary"`          // Aggregate statistics
    Legend          *ColorLegend     `json:"legend"`           // Color mapping reference
    Navigation      *MonthNavigation `json:"navigation"`       // Previous/next month links
    GeneratedAt     string           `json:"generated_at"`     // ISO 8601 timestamp
    CacheExpiration string           `json:"cache_expiration"` // ISO 8601 timestamp
}
```

### 2. DayCell (Individual Day)

```go
type DayCell struct {
    Date            string  `json:"date"`              // ISO date format (YYYY-MM-DD)
    InCurrentMonth  bool    `json:"in_current_month"`  // true if day belongs to displayed month
    Progress        float64 `json:"progress"`          // Completion percentage (0-100)
    Color           string  `json:"color"`             // Hex color code (e.g., "#10b981")
    ColorBucket     int     `json:"color_bucket"`      // Bucket index (0-5)
    IsToday         bool    `json:"is_today"`          // true if this is today's date
    CheckinCount    int     `json:"checkin_count"`     // Number of check-ins
    TotalMinutes    int     `json:"total_minutes"`     // Total tracked minutes
}
```

### 3. MonthSummary (Aggregates)

```go
type MonthSummary struct {
    TotalCheckins       int     `json:"total_checkins"`        // Total check-ins in month
    TotalMinutes        int     `json:"total_minutes"`         // Total tracked minutes
    DaysWithCheckins    int     `json:"days_with_checkins"`    // Days with at least 1 check-in
    TotalDaysInMonth    int     `json:"total_days_in_month"`   // Days in this month (28-31)
    AveragePerDay       float64 `json:"average_per_day"`       // Average check-ins per day
    CompletionRate      float64 `json:"completion_rate"`       // % of days with check-ins
    MostProductiveDay   string  `json:"most_productive_day"`   // Date (YYYY-MM-DD)
    MostProductiveCount int     `json:"most_productive_count"` // Check-ins on most productive day
    CurrentStreak       int     `json:"current_streak"`        // Current consecutive days
    LongestStreak       int     `json:"longest_streak"`        // Longest streak in month
}
```

### 4. ColorLegend (Reference)

```go
type ColorLegend struct {
    Buckets []ColorBucket `json:"buckets"` // Array of 6 color buckets
}

type ColorBucket struct {
    Index       int     `json:"index"`        // Bucket number (0-5)
    Label       string  `json:"label"`        // Human-readable label
    MinProgress float64 `json:"min_progress"` // Minimum progress % (inclusive)
    MaxProgress float64 `json:"max_progress"` // Maximum progress % (inclusive)
    Color       string  `json:"color"`        // Hex color code
    Description string  `json:"description"`  // Usage description
}
```

### 5. MonthNavigation

```go
type MonthNavigation struct {
    PreviousMonth string `json:"previous_month"` // YYYY-MM format
    NextMonth     string `json:"next_month"`     // YYYY-MM format
    CurrentMonth  string `json:"current_month"`  // YYYY-MM format
}
```

---

## Color Mapping Rules

### Progress Calculation

```
progress = (totalMinutes / 1440) * 100
where 1440 = 24 hours × 60 minutes per day
```

- **0 minutes** → 0% progress
- **360 minutes (6 hours)** → 25% progress
- **720 minutes (12 hours)** → 50% progress
- **1080 minutes (18 hours)** → 75% progress
- **1440+ minutes (24+ hours)** → 100% progress (capped)

### Bucket Boundaries

| Bucket | Label | Progress Range | Color | Hex Code | Description |
|--------|-------|----------------|-------|----------|-------------|
| **0** | None | 0% | Light Gray | `#f3f4f6` | No activity |
| **1** | Minimal | 1-24% | Light Green | `#d1fae5` | Started tracking |
| **2** | Low | 25-49% | Medium Green | `#6ee7b7` | Quarter to half day |
| **3** | Medium | 50-74% | Dark Green | `#34d399` | Half to three-quarters |
| **4** | High | 75-99% | Darker Green | `#10b981` | Nearly full day |
| **5** | Complete | 100% | Darkest Green | `#059669` | Full day tracked |

### Color Mapping Function

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

func GetBucketColor(bucket int) string {
    colors := []string{
        "#f3f4f6", // 0: None
        "#d1fae5", // 1: Minimal
        "#6ee7b7", // 2: Low
        "#34d399", // 3: Medium
        "#10b981", // 4: High
        "#059669", // 5: Complete
    }
    if bucket < 0 || bucket >= len(colors) {
        return colors[0]
    }
    return colors[bucket]
}
```

### Boundary Test Cases

| Progress | Bucket | Color | Expected Behavior |
|----------|--------|-------|-------------------|
| 0.0% | 0 | #f3f4f6 | No check-ins |
| 0.1% | 1 | #d1fae5 | Even 1 minute counts |
| 1.0% | 1 | #d1fae5 | Minimal activity |
| 24.9% | 1 | #d1fae5 | Just under quarter day |
| 25.0% | 2 | #6ee7b7 | Exactly quarter day |
| 49.9% | 2 | #6ee7b7 | Just under half day |
| 50.0% | 3 | #34d399 | Exactly half day |
| 74.9% | 3 | #34d399 | Just under three-quarters |
| 75.0% | 4 | #10b981 | Three-quarters day |
| 99.9% | 4 | #10b981 | Nearly complete |
| 100.0% | 5 | #059669 | Full day |
| 150.0% | 5 | #059669 | Over-tracking (capped) |

---

## JSON Schema

### OpenAPI 3.0 Schema Definition

```yaml
openapi: 3.0.3
info:
  title: Monthly Calendar API
  version: 1.0.0
  description: Monthly calendar view with 7x6 grid and progress-based coloring

components:
  schemas:
    MonthCalendar:
      type: object
      required:
        - year
        - month
        - month_name
        - start_of_week
        - grid
        - summary
        - legend
        - navigation
        - generated_at
      properties:
        year:
          type: integer
          minimum: 1970
          maximum: 2100
          example: 2024
        month:
          type: integer
          minimum: 1
          maximum: 12
          example: 11
        month_name:
          type: string
          example: "November"
        start_of_week:
          type: string
          enum: [sunday, monday]
          example: "monday"
        grid:
          type: array
          description: "7x6 matrix of day cells (weeks x days)"
          minItems: 4
          maxItems: 6
          items:
            type: array
            minItems: 7
            maxItems: 7
            items:
              $ref: '#/components/schemas/DayCell'
        summary:
          $ref: '#/components/schemas/MonthSummary'
        legend:
          $ref: '#/components/schemas/ColorLegend'
        navigation:
          $ref: '#/components/schemas/MonthNavigation'
        generated_at:
          type: string
          format: date-time
          example: "2024-11-24T10:00:00Z"
        cache_expiration:
          type: string
          format: date-time
          example: "2024-11-25T00:00:00Z"

    DayCell:
      type: object
      required:
        - date
        - in_current_month
        - progress
        - color
        - color_bucket
        - is_today
        - checkin_count
        - total_minutes
      properties:
        date:
          type: string
          format: date
          pattern: '^\d{4}-\d{2}-\d{2}$'
          example: "2024-11-15"
        in_current_month:
          type: boolean
          example: true
        progress:
          type: number
          format: float
          minimum: 0
          maximum: 100
          example: 65.5
        color:
          type: string
          pattern: '^#[0-9a-fA-F]{6}$'
          example: "#34d399"
        color_bucket:
          type: integer
          minimum: 0
          maximum: 5
          example: 3
        is_today:
          type: boolean
          example: false
        checkin_count:
          type: integer
          minimum: 0
          example: 12
        total_minutes:
          type: integer
          minimum: 0
          example: 943

    MonthSummary:
      type: object
      required:
        - total_checkins
        - total_minutes
        - days_with_checkins
        - total_days_in_month
        - average_per_day
        - completion_rate
        - current_streak
        - longest_streak
      properties:
        total_checkins:
          type: integer
          minimum: 0
          example: 250
        total_minutes:
          type: integer
          minimum: 0
          example: 12000
        days_with_checkins:
          type: integer
          minimum: 0
          example: 22
        total_days_in_month:
          type: integer
          minimum: 28
          maximum: 31
          example: 30
        average_per_day:
          type: number
          format: float
          minimum: 0
          example: 8.33
        completion_rate:
          type: number
          format: float
          minimum: 0
          maximum: 100
          example: 73.33
        most_productive_day:
          type: string
          format: date
          nullable: true
          example: "2024-11-15"
        most_productive_count:
          type: integer
          minimum: 0
          example: 18
        current_streak:
          type: integer
          minimum: 0
          example: 5
        longest_streak:
          type: integer
          minimum: 0
          example: 10

    ColorLegend:
      type: object
      required:
        - buckets
      properties:
        buckets:
          type: array
          minItems: 6
          maxItems: 6
          items:
            $ref: '#/components/schemas/ColorBucket'

    ColorBucket:
      type: object
      required:
        - index
        - label
        - min_progress
        - max_progress
        - color
        - description
      properties:
        index:
          type: integer
          minimum: 0
          maximum: 5
          example: 3
        label:
          type: string
          example: "Medium"
        min_progress:
          type: number
          format: float
          minimum: 0
          maximum: 100
          example: 50.0
        max_progress:
          type: number
          format: float
          minimum: 0
          maximum: 100
          example: 74.9
        color:
          type: string
          pattern: '^#[0-9a-fA-F]{6}$'
          example: "#34d399"
        description:
          type: string
          example: "Half to three-quarters of day tracked"

    MonthNavigation:
      type: object
      required:
        - previous_month
        - next_month
        - current_month
      properties:
        previous_month:
          type: string
          pattern: '^\d{4}-\d{2}$'
          example: "2024-10"
        next_month:
          type: string
          pattern: '^\d{4}-\d{2}$'
          example: "2024-12"
        current_month:
          type: string
          pattern: '^\d{4}-\d{2}$'
          example: "2024-11"
```

---

## Example Payloads

### Example 1: Active Month with Good Tracking

```json
{
  "year": 2024,
  "month": 11,
  "month_name": "November",
  "start_of_week": "monday",
  "grid": [
    [
      {
        "date": "2024-10-28",
        "in_current_month": false,
        "progress": 0.0,
        "color": "#f3f4f6",
        "color_bucket": 0,
        "is_today": false,
        "checkin_count": 0,
        "total_minutes": 0
      },
      {
        "date": "2024-10-29",
        "in_current_month": false,
        "progress": 0.0,
        "color": "#f3f4f6",
        "color_bucket": 0,
        "is_today": false,
        "checkin_count": 0,
        "total_minutes": 0
      },
      {
        "date": "2024-10-30",
        "in_current_month": false,
        "progress": 0.0,
        "color": "#f3f4f6",
        "color_bucket": 0,
        "is_today": false,
        "checkin_count": 0,
        "total_minutes": 0
      },
      {
        "date": "2024-10-31",
        "in_current_month": false,
        "progress": 0.0,
        "color": "#f3f4f6",
        "color_bucket": 0,
        "is_today": false,
        "checkin_count": 0,
        "total_minutes": 0
      },
      {
        "date": "2024-11-01",
        "in_current_month": true,
        "progress": 85.5,
        "color": "#10b981",
        "color_bucket": 4,
        "is_today": false,
        "checkin_count": 15,
        "total_minutes": 1231
      },
      {
        "date": "2024-11-02",
        "in_current_month": true,
        "progress": 67.3,
        "color": "#34d399",
        "color_bucket": 3,
        "is_today": false,
        "checkin_count": 12,
        "total_minutes": 969
      },
      {
        "date": "2024-11-03",
        "in_current_month": true,
        "progress": 0.0,
        "color": "#f3f4f6",
        "color_bucket": 0,
        "is_today": false,
        "checkin_count": 0,
        "total_minutes": 0
      }
    ],
    [
      {
        "date": "2024-11-04",
        "in_current_month": true,
        "progress": 92.1,
        "color": "#10b981",
        "color_bucket": 4,
        "is_today": false,
        "checkin_count": 18,
        "total_minutes": 1326
      },
      {
        "date": "2024-11-05",
        "in_current_month": true,
        "progress": 100.0,
        "color": "#059669",
        "color_bucket": 5,
        "is_today": false,
        "checkin_count": 20,
        "total_minutes": 1440
      },
      {
        "date": "2024-11-06",
        "in_current_month": true,
        "progress": 78.4,
        "color": "#10b981",
        "color_bucket": 4,
        "is_today": false,
        "checkin_count": 14,
        "total_minutes": 1129
      },
      {
        "date": "2024-11-07",
        "in_current_month": true,
        "progress": 55.2,
        "color": "#34d399",
        "color_bucket": 3,
        "is_today": false,
        "checkin_count": 10,
        "total_minutes": 795
      },
      {
        "date": "2024-11-08",
        "in_current_month": true,
        "progress": 43.1,
        "color": "#6ee7b7",
        "color_bucket": 2,
        "is_today": false,
        "checkin_count": 8,
        "total_minutes": 621
      },
      {
        "date": "2024-11-09",
        "in_current_month": true,
        "progress": 18.5,
        "color": "#d1fae5",
        "color_bucket": 1,
        "is_today": false,
        "checkin_count": 3,
        "total_minutes": 266
      },
      {
        "date": "2024-11-10",
        "in_current_month": true,
        "progress": 0.0,
        "color": "#f3f4f6",
        "color_bucket": 0,
        "is_today": false,
        "checkin_count": 0,
        "total_minutes": 0
      }
    ]
  ],
  "summary": {
    "total_checkins": 285,
    "total_minutes": 15420,
    "days_with_checkins": 22,
    "total_days_in_month": 30,
    "average_per_day": 9.5,
    "completion_rate": 73.33,
    "most_productive_day": "2024-11-15",
    "most_productive_count": 20,
    "current_streak": 5,
    "longest_streak": 9
  },
  "legend": {
    "buckets": [
      {
        "index": 0,
        "label": "None",
        "min_progress": 0.0,
        "max_progress": 0.0,
        "color": "#f3f4f6",
        "description": "No activity tracked"
      },
      {
        "index": 1,
        "label": "Minimal",
        "min_progress": 0.1,
        "max_progress": 24.9,
        "color": "#d1fae5",
        "description": "Started tracking (< 6 hours)"
      },
      {
        "index": 2,
        "label": "Low",
        "min_progress": 25.0,
        "max_progress": 49.9,
        "color": "#6ee7b7",
        "description": "Quarter to half day (6-12 hours)"
      },
      {
        "index": 3,
        "label": "Medium",
        "min_progress": 50.0,
        "max_progress": 74.9,
        "color": "#34d399",
        "description": "Half to three-quarters day (12-18 hours)"
      },
      {
        "index": 4,
        "label": "High",
        "min_progress": 75.0,
        "max_progress": 99.9,
        "color": "#10b981",
        "description": "Nearly full day (18-24 hours)"
      },
      {
        "index": 5,
        "label": "Complete",
        "min_progress": 100.0,
        "max_progress": 100.0,
        "color": "#059669",
        "description": "Full day tracked (24+ hours)"
      }
    ]
  },
  "navigation": {
    "previous_month": "2024-10",
    "next_month": "2024-12",
    "current_month": "2024-11"
  },
  "generated_at": "2024-11-24T10:30:00Z",
  "cache_expiration": "2024-11-25T00:00:00Z"
}
```

### Example 2: Current Month with Today Indicator

```json
{
  "year": 2024,
  "month": 11,
  "month_name": "November",
  "start_of_week": "sunday",
  "grid": [
    [
      {
        "date": "2024-10-27",
        "in_current_month": false,
        "progress": 0.0,
        "color": "#f3f4f6",
        "color_bucket": 0,
        "is_today": false,
        "checkin_count": 0,
        "total_minutes": 0
      },
      {
        "date": "2024-11-24",
        "in_current_month": true,
        "progress": 45.8,
        "color": "#6ee7b7",
        "color_bucket": 2,
        "is_today": true,
        "checkin_count": 8,
        "total_minutes": 660
      }
    ]
  ],
  "summary": {
    "total_checkins": 180,
    "total_minutes": 9800,
    "days_with_checkins": 18,
    "total_days_in_month": 30,
    "average_per_day": 6.0,
    "completion_rate": 60.0,
    "most_productive_day": "2024-11-10",
    "most_productive_count": 16,
    "current_streak": 3,
    "longest_streak": 7
  },
  "legend": {
    "buckets": [
      {
        "index": 0,
        "label": "None",
        "min_progress": 0.0,
        "max_progress": 0.0,
        "color": "#f3f4f6",
        "description": "No activity tracked"
      },
      {
        "index": 1,
        "label": "Minimal",
        "min_progress": 0.1,
        "max_progress": 24.9,
        "color": "#d1fae5",
        "description": "Started tracking (< 6 hours)"
      },
      {
        "index": 2,
        "label": "Low",
        "min_progress": 25.0,
        "max_progress": 49.9,
        "color": "#6ee7b7",
        "description": "Quarter to half day (6-12 hours)"
      },
      {
        "index": 3,
        "label": "Medium",
        "min_progress": 50.0,
        "max_progress": 74.9,
        "color": "#34d399",
        "description": "Half to three-quarters day (12-18 hours)"
      },
      {
        "index": 4,
        "label": "High",
        "min_progress": 75.0,
        "max_progress": 99.9,
        "color": "#10b981",
        "description": "Nearly full day (18-24 hours)"
      },
      {
        "index": 5,
        "label": "Complete",
        "min_progress": 100.0,
        "max_progress": 100.0,
        "color": "#059669",
        "description": "Full day tracked (24+ hours)"
      }
    ]
  },
  "navigation": {
    "previous_month": "2024-10",
    "next_month": "2024-12",
    "current_month": "2024-11"
  },
  "generated_at": "2024-11-24T15:45:00Z",
  "cache_expiration": "2024-11-25T00:00:00Z"
}
```

### Example 3: Empty Month (New User)

```json
{
  "year": 2024,
  "month": 11,
  "month_name": "November",
  "start_of_week": "monday",
  "grid": [
    [
      {
        "date": "2024-10-28",
        "in_current_month": false,
        "progress": 0.0,
        "color": "#f3f4f6",
        "color_bucket": 0,
        "is_today": false,
        "checkin_count": 0,
        "total_minutes": 0
      }
    ]
  ],
  "summary": {
    "total_checkins": 0,
    "total_minutes": 0,
    "days_with_checkins": 0,
    "total_days_in_month": 30,
    "average_per_day": 0.0,
    "completion_rate": 0.0,
    "most_productive_day": null,
    "most_productive_count": 0,
    "current_streak": 0,
    "longest_streak": 0
  },
  "legend": {
    "buckets": [
      {
        "index": 0,
        "label": "None",
        "min_progress": 0.0,
        "max_progress": 0.0,
        "color": "#f3f4f6",
        "description": "No activity tracked"
      },
      {
        "index": 1,
        "label": "Minimal",
        "min_progress": 0.1,
        "max_progress": 24.9,
        "color": "#d1fae5",
        "description": "Started tracking (< 6 hours)"
      },
      {
        "index": 2,
        "label": "Low",
        "min_progress": 25.0,
        "max_progress": 49.9,
        "color": "#6ee7b7",
        "description": "Quarter to half day (6-12 hours)"
      },
      {
        "index": 3,
        "label": "Medium",
        "min_progress": 50.0,
        "max_progress": 74.9,
        "color": "#34d399",
        "description": "Half to three-quarters day (12-18 hours)"
      },
      {
        "index": 4,
        "label": "High",
        "min_progress": 75.0,
        "max_progress": 99.9,
        "color": "#10b981",
        "description": "Nearly full day (18-24 hours)"
      },
      {
        "index": 5,
        "label": "Complete",
        "min_progress": 100.0,
        "max_progress": 100.0,
        "color": "#059669",
        "description": "Full day tracked (24+ hours)"
      }
    ]
  },
  "navigation": {
    "previous_month": "2024-10",
    "next_month": "2024-12",
    "current_month": "2024-11"
  },
  "generated_at": "2024-11-24T08:00:00Z",
  "cache_expiration": "2024-11-25T00:00:00Z"
}
```

---

## Implementation Notes

### 1. Grid Structure

The `grid` field is a 2D array representing:
- **Outer array**: Weeks (typically 4-6 weeks depending on month)
- **Inner array**: Days (always 7 days per week)
- **Total cells**: 28-42 days (4-6 weeks × 7 days)

**Example Grid Layout (Monday start):**
```
Week 0: [Mon, Tue, Wed, Thu, Fri, Sat, Sun]
Week 1: [Mon, Tue, Wed, Thu, Fri, Sat, Sun]
Week 2: [Mon, Tue, Wed, Thu, Fri, Sat, Sun]
Week 3: [Mon, Tue, Wed, Thu, Fri, Sat, Sun]
Week 4: [Mon, Tue, Wed, Thu, Fri, Sat, Sun]
Week 5: [Mon, Tue, Wed, Thu, Fri, Sat, Sun] (if needed)
```

### 2. Date Handling

- All dates in **ISO 8601** format (`YYYY-MM-DD`)
- All timestamps in **UTC** with timezone suffix (`Z`)
- Client should convert to user's local timezone for display
- `in_current_month` flag helps identify overflow days from adjacent months

### 3. Progress Calculation

```go
// Calculate progress percentage from minutes
progress := (float64(totalMinutes) / 1440.0) * 100.0
if progress > 100.0 {
    progress = 100.0  // Cap at 100%
}

// Round to 1 decimal place
progress = math.Round(progress * 10) / 10
```

### 4. Color Selection

```go
bucket := GetColorBucket(progress)
color := GetBucketColor(bucket)
```

### 5. Caching Strategy

- **Cache Key**: `calendar:monthly:{user_id}:{year}:{month}`
- **TTL**: Until midnight of next day (UTC)
- **Invalidation**: Automatic expiration (no manual invalidation needed)
- **Cache Headers**: Set `Cache-Control: private, max-age=3600`

### 6. API Endpoint

```
GET /api/v1/calendar/monthly/grid
Query Parameters:
  - year (optional): default current year
  - month (optional): default current month
  - start_of_week (optional): "sunday" or "monday", default "monday"
  - timezone (optional): IANA timezone, default "UTC"
```

### 7. Performance Considerations

- Grid generation should complete in **< 50ms**
- Use database aggregation (not application-level)
- Pre-calculate color buckets in database query if possible
- Return legend with every response (no separate endpoint)

### 8. Accessibility

- Include `aria-label` on frontend for each cell
- Example: `aria-label="November 15, 2024: 65% progress, 12 check-ins"`
- Color should not be the only indicator (include text labels)

### 9. Mobile Optimization

- Grid may render differently on small screens
- Consider providing abbreviated day labels (M, T, W, T, F, S, S)
- Touch targets should be at least 44x44 pixels

### 10. Testing Requirements

**Unit Tests:**
- `TestGetColorBucket` - All boundary values (0, 1, 24, 25, 49, 50, 74, 75, 99, 100)
- `TestGetBucketColor` - Valid bucket indices (0-5)
- `TestProgressCalculation` - Various minute values
- `TestGridGeneration` - Different month lengths and week starts

**Integration Tests:**
- Full calendar generation with real data
- Timezone handling
- Edge cases (February, leap years)

**JSON Schema Validation:**
- All example payloads validate against schema
- Required fields present
- Data types correct
- Value ranges respected

---

## Summary

This specification defines a complete monthly calendar data model with:
- **6-bucket color system** for progress visualization (0%, 1-24%, 25-49%, 50-74%, 75-99%, 100%)
- **7x6 grid layout** supporting both Sunday and Monday week starts
- **Comprehensive metadata** including summaries, navigation, and color legends
- **JSON Schema** for validation and contract testing
- **Example payloads** covering active, current, and empty months

The design integrates with the existing calendar service implementation while adding the grid-based structure and enhanced color mapping required for Task 7.

**Status**: Ready for implementation in Task 7.2
