# Calendar API Usage Examples

This document provides practical examples for using the Calendar API endpoints.

## Table of Contents

- [Authentication](#authentication)
- [Monthly Calendar Examples](#monthly-calendar-examples)
- [Heatmap Examples](#heatmap-examples)
- [Frontend Integration](#frontend-integration)
- [Error Handling](#error-handling)
- [Advanced Use Cases](#advanced-use-cases)

## Authentication

All calendar endpoints require JWT authentication. Include the bearer token in the Authorization header:

```bash
export TOKEN="your_jwt_token_here"
```

## Monthly Calendar Examples

### 1. Current Month Calendar

Get the calendar for the current month:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

Response:
```json
{
  "year": 2024,
  "month": 11,
  "month_name": "November",
  "start_day": "monday",
  "weeks": [
    {
      "days": [
        {
          "date": "2024-10-28",
          "checkin_count": 0,
          "total_minutes": 0,
          "completion_percent": 0,
          "color_intensity": 0,
          "is_current_month": false,
          "is_today": false,
          "has_checkins": false
        },
        // ... 6 more days
      ]
    },
    // ... more weeks
  ],
  "summary": {
    "total_checkins": 150,
    "total_minutes": 7200,
    "days_with_checkins": 20,
    "total_days_in_month": 30,
    "average_per_day": 5.0,
    "completion_rate": 66.67,
    "most_productive_day": "2024-11-15",
    "most_productive_count": 15,
    "current_streak": 5,
    "longest_streak": 7
  },
  "categories": [
    {
      "category_id": "550e8400-e29b-41d4-a716-446655440000",
      "category_name": "Work",
      "count": 100,
      "percentage": 66.67
    },
    {
      "category_id": "550e8400-e29b-41d4-a716-446655440001",
      "category_name": "Personal",
      "count": 50,
      "percentage": 33.33
    }
  ],
  "previous_month": "2024-10",
  "next_month": "2024-12",
  "generated_at": "2024-11-24T10:00:00Z",
  "cache_expiration": "2024-11-25T00:00:00Z"
}
```

### 2. Specific Month Calendar

Get the calendar for January 2024:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly?year=2024&month=1" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

### 3. Calendar with Sunday Start

Get a calendar starting weeks on Sunday:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly?start_day=sunday" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

### 4. Calendar with Timezone

Get calendar in New York timezone:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly?timezone=America/New_York" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

Get calendar in Seoul timezone:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly?timezone=Asia/Seoul" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

### 5. Extract Specific Information

Get only the summary statistics:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '.summary'
```

Get only productive days:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '.weeks[].days[] | select(.has_checkins == true)'
```

Get the most productive day:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '.summary.most_productive_day, .summary.most_productive_count'
```

## Heatmap Examples

### 1. Year-Long Heatmap

Get heatmap for entire year 2024:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=2024-01-01&end_date=2024-12-31" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

Response:
```json
{
  "start_date": "2024-01-01",
  "end_date": "2024-12-31",
  "days": [
    {
      "date": "2024-01-01",
      "checkin_count": 8,
      "total_minutes": 360,
      "completion_percent": 25.0,
      "color_intensity": 1
    },
    {
      "date": "2024-01-02",
      "checkin_count": 12,
      "total_minutes": 540,
      "completion_percent": 37.5,
      "color_intensity": 2
    },
    // ... 364 more days
  ],
  "total_days": 366,
  "active_days": 250,
  "total_checkins": 2000,
  "generated_at": "2024-11-24T10:00:00Z"
}
```

### 2. Last 90 Days Heatmap

```bash
START_DATE=$(date -u -d '90 days ago' '+%Y-%m-%d')
END_DATE=$(date -u '+%Y-%m-%d')

curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=$START_DATE&end_date=$END_DATE" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

### 3. Current Month Heatmap

```bash
YEAR=$(date '+%Y')
MONTH=$(date '+%m')
START_DATE="${YEAR}-${MONTH}-01"
END_DATE=$(date -d "${START_DATE} +1 month -1 day" '+%Y-%m-%d')

curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=$START_DATE&end_date=$END_DATE" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

### 4. Quarter Heatmap

Q1 2024 (Jan-Mar):

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=2024-01-01&end_date=2024-03-31" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
```

### 5. Extract Intensity Distribution

Get count of days by intensity level:

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=2024-01-01&end_date=2024-12-31" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '[.days[].color_intensity] | group_by(.) | map({intensity: .[0], count: length})'
```

Output:
```json
[
  {"intensity": 0, "count": 116},
  {"intensity": 1, "count": 80},
  {"intensity": 2, "count": 70},
  {"intensity": 3, "count": 60},
  {"intensity": 4, "count": 40}
]
```

## Frontend Integration

### React Example

```typescript
// API client
import axios from 'axios';

const API_BASE = 'http://localhost:8080/api/v1';
const getAuthHeaders = () => ({
  headers: { Authorization: `Bearer ${localStorage.getItem('token')}` }
});

// Fetch monthly calendar
export async function getMonthlyCalendar(
  year: number,
  month: number,
  options?: {
    startDay?: 'sunday' | 'monday';
    timezone?: string;
  }
): Promise<MonthlyCalendar> {
  const params = new URLSearchParams({
    year: year.toString(),
    month: month.toString(),
    ...(options?.startDay && { start_day: options.startDay }),
    ...(options?.timezone && { timezone: options.timezone }),
  });

  const response = await axios.get(
    `${API_BASE}/calendar/monthly?${params}`,
    getAuthHeaders()
  );
  return response.data;
}

// Fetch heatmap
export async function getHeatmap(
  startDate: string,
  endDate: string,
  timezone?: string
): Promise<HeatmapData> {
  const params = new URLSearchParams({
    start_date: startDate,
    end_date: endDate,
    ...(timezone && { timezone }),
  });

  const response = await axios.get(
    `${API_BASE}/calendar/heatmap?${params}`,
    getAuthHeaders()
  );
  return response.data;
}

// React component
function CalendarView() {
  const [calendar, setCalendar] = useState<MonthlyCalendar | null>(null);
  const [currentDate, setCurrentDate] = useState(new Date());

  useEffect(() => {
    getMonthlyCalendar(
      currentDate.getFullYear(),
      currentDate.getMonth() + 1,
      {
        startDay: 'monday',
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone
      }
    ).then(setCalendar);
  }, [currentDate]);

  if (!calendar) return <Loading />;

  return (
    <div className="calendar">
      <h2>{calendar.month_name} {calendar.year}</h2>

      {/* Navigation */}
      <div className="navigation">
        <button onClick={() => navigateMonth(-1)}>
          Previous
        </button>
        <button onClick={() => navigateMonth(1)}>
          Next
        </button>
      </div>

      {/* Calendar grid */}
      <div className="weeks">
        {calendar.weeks.map((week, i) => (
          <div key={i} className="week">
            {week.days.map((day) => (
              <CalendarDay
                key={day.date}
                day={day}
                onClick={() => handleDayClick(day)}
              />
            ))}
          </div>
        ))}
      </div>

      {/* Summary */}
      <CalendarSummary summary={calendar.summary} />
    </div>
  );
}

// Calendar day component
function CalendarDay({ day, onClick }: { day: CalendarDay; onClick: () => void }) {
  const intensityClass = `intensity-${day.color_intensity}`;
  const classes = [
    'calendar-day',
    intensityClass,
    day.is_current_month ? 'current-month' : 'other-month',
    day.is_today ? 'today' : '',
  ].filter(Boolean).join(' ');

  return (
    <div className={classes} onClick={onClick}>
      <div className="date">{new Date(day.date).getDate()}</div>
      <div className="count">{day.checkin_count}</div>
    </div>
  );
}
```

### CSS for Color Intensity

```css
/* Calendar day base styles */
.calendar-day {
  border: 1px solid #e0e0e0;
  padding: 8px;
  cursor: pointer;
  transition: all 0.2s;
}

.calendar-day.today {
  border-color: #2196f3;
  border-width: 2px;
}

.calendar-day.other-month {
  opacity: 0.4;
}

/* Intensity color scheme (GitHub-style) */
.calendar-day.intensity-0 {
  background-color: #ebedf0;
}

.calendar-day.intensity-1 {
  background-color: #9be9a8;
}

.calendar-day.intensity-2 {
  background-color: #40c463;
}

.calendar-day.intensity-3 {
  background-color: #30a14e;
}

.calendar-day.intensity-4 {
  background-color: #216e39;
  color: white;
}

.calendar-day:hover {
  transform: scale(1.05);
  box-shadow: 0 2px 8px rgba(0,0,0,0.15);
}
```

### Heatmap with D3.js

```typescript
import * as d3 from 'd3';

function renderHeatmap(data: HeatmapData, container: HTMLElement) {
  const cellSize = 12;
  const cellPadding = 2;
  const width = 900;
  const height = 140;

  const svg = d3.select(container)
    .append('svg')
    .attr('width', width)
    .attr('height', height);

  // Color scale
  const colorScale = d3.scaleQuantize()
    .domain([0, 4])
    .range(['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39']);

  // Group days by week
  const weeks = d3.groups(
    data.days,
    d => d3.timeWeek.count(new Date(data.start_date), new Date(d.date))
  );

  // Draw cells
  const cells = svg.selectAll('g')
    .data(weeks)
    .join('g')
    .attr('transform', (d, i) => `translate(${i * (cellSize + cellPadding)}, 0)`)
    .selectAll('rect')
    .data(d => d[1])
    .join('rect')
    .attr('width', cellSize)
    .attr('height', cellSize)
    .attr('y', d => new Date(d.date).getDay() * (cellSize + cellPadding))
    .attr('fill', d => colorScale(d.color_intensity))
    .attr('data-date', d => d.date)
    .attr('data-count', d => d.checkin_count);

  // Add tooltips
  cells.append('title')
    .text(d => `${d.date}: ${d.checkin_count} check-ins`);
}
```

## Error Handling

### Handle Invalid Year

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly?year=1800" \
  -H "Authorization: Bearer $TOKEN"
```

Response (400):
```json
{
  "error": "invalid year, must be between 1970 and 2100"
}
```

### Handle Missing Date

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?end_date=2024-12-31" \
  -H "Authorization: Bearer $TOKEN"
```

Response (400):
```json
{
  "error": "start_date parameter is required (format: YYYY-MM-DD)"
}
```

### Handle Date Range Too Large

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=2024-01-01&end_date=2025-12-31" \
  -H "Authorization: Bearer $TOKEN"
```

Response (400):
```json
{
  "error": "date range cannot exceed 366 days"
}
```

### Handle Unauthorized

```bash
curl -X GET "http://localhost:8080/api/v1/calendar/monthly"
```

Response (401):
```json
{
  "error": "unauthorized"
}
```

## Advanced Use Cases

### 1. Compare Two Months

```bash
# Get November 2024
NOV_2024=$(curl -s -X GET "http://localhost:8080/api/v1/calendar/monthly?year=2024&month=11" \
  -H "Authorization: Bearer $TOKEN" | jq '.summary')

# Get November 2023
NOV_2023=$(curl -s -X GET "http://localhost:8080/api/v1/calendar/monthly?year=2023&month=11" \
  -H "Authorization: Bearer $TOKEN" | jq '.summary')

# Compare
echo "November 2024: $NOV_2024"
echo "November 2023: $NOV_2023"
```

### 2. Track Streak Over Time

```bash
# Get streak for last 6 months
for month in {6..11}; do
  STREAK=$(curl -s -X GET "http://localhost:8080/api/v1/calendar/monthly?year=2024&month=$month" \
    -H "Authorization: Bearer $TOKEN" \
    | jq -r '.summary.longest_streak')
  echo "Month $month: $STREAK days"
done
```

### 3. Find Most Productive Period

```bash
# Get heatmap for year
curl -s -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=2024-01-01&end_date=2024-12-31" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '[.days[] | select(.checkin_count > 0)] | sort_by(.checkin_count) | reverse | .[0:10]'
```

### 4. Calculate Activity Rate

```bash
# Get monthly calendar
curl -s -X GET "http://localhost:8080/api/v1/calendar/monthly" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '{
      month: .month_name,
      active_days: .summary.days_with_checkins,
      total_days: .summary.total_days_in_month,
      activity_rate: (.summary.days_with_checkins / .summary.total_days_in_month * 100 | round)
    }'
```

### 5. Export to CSV

```bash
# Export heatmap data to CSV
curl -s -X GET "http://localhost:8080/api/v1/calendar/heatmap?start_date=2024-01-01&end_date=2024-12-31" \
  -H "Authorization: Bearer $TOKEN" \
  | jq -r '.days[] | [.date, .checkin_count, .total_minutes, .completion_percent, .color_intensity] | @csv' \
  > heatmap_2024.csv
```

### 6. Webhook Integration

```bash
# Check if productivity dropped below threshold
COMPLETION_RATE=$(curl -s -X GET "http://localhost:8080/api/v1/calendar/monthly" \
  -H "Authorization: Bearer $TOKEN" \
  | jq -r '.summary.completion_rate')

if (( $(echo "$COMPLETION_RATE < 50" | bc -l) )); then
  # Send notification
  curl -X POST "https://hooks.slack.com/services/YOUR/WEBHOOK/URL" \
    -H "Content-Type: application/json" \
    -d "{\"text\": \"Productivity alert: Only $COMPLETION_RATE% active days this month\"}"
fi
```

## Performance Tips

1. **Use Cache Headers**: The API returns cache headers. Respect them in your client.

2. **Batch Requests**: When loading multiple months, consider using parallel requests:

```typescript
const months = [1, 2, 3, 4, 5, 6];
const calendars = await Promise.all(
  months.map(month => getMonthlyCalendar(2024, month))
);
```

3. **Optimize Heatmap Range**: Request only the date range you need. Smaller ranges = faster responses.

4. **Use User's Timezone**: Always pass the user's actual timezone for accurate day boundaries:

```typescript
const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
```

5. **Prefetch Adjacent Months**: When user views a month, prefetch previous and next months for smooth navigation.

## Testing

Test the API with curl scripts:

```bash
#!/bin/bash
# test_calendar_api.sh

set -e

TOKEN="your_token_here"
BASE_URL="http://localhost:8080/api/v1"

echo "Testing monthly calendar..."
curl -s -X GET "$BASE_URL/calendar/monthly?year=2024&month=11" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '.summary' || echo "FAILED"

echo "Testing heatmap..."
curl -s -X GET "$BASE_URL/calendar/heatmap?start_date=2024-01-01&end_date=2024-12-31" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '.total_days' || echo "FAILED"

echo "All tests passed!"
```

Run with:
```bash
chmod +x test_calendar_api.sh
./test_calendar_api.sh
```
