package timeline

import (
	"testing"
	"time"

	"github.com/dev-jelly/donelist/internal/category"
	"github.com/dev-jelly/donelist/internal/checkin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestValidateBlockGranularity(t *testing.T) {
	tests := []struct {
		name        string
		minutes     int
		expected    BlockGranularity
		expectError bool
	}{
		{
			name:        "15 minute blocks",
			minutes:     15,
			expected:    Block15Min,
			expectError: false,
		},
		{
			name:        "30 minute blocks",
			minutes:     30,
			expected:    Block30Min,
			expectError: false,
		},
		{
			name:        "45 minute blocks",
			minutes:     45,
			expected:    Block45Min,
			expectError: false,
		},
		{
			name:        "2 hour blocks",
			minutes:     120,
			expected:    Block2Hour,
			expectError: false,
		},
		{
			name:        "invalid granularity",
			minutes:     60,
			expected:    Block30Min, // Default fallback
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateBlockGranularity(tt.minutes)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateTimeBlocks(t *testing.T) {
	// Create a test date
	loc, _ := time.LoadLocation("America/New_York")
	startOfDay := time.Date(2025, 11, 13, 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	// Create mock check-ins
	categoryID := uuid.New()
	checkins := []*CheckinWithMeta{
		{
			Checkin: &checkin.Checkin{
				ID:              uuid.New(),
				UserID:          uuid.New(),
				CategoryID:      &categoryID,
				Content:         "Morning workout",
				CheckinTime:     time.Date(2025, 11, 13, 8, 15, 0, 0, loc),
				DurationMinutes: 30,
			},
		},
		{
			Checkin: &checkin.Checkin{
				ID:              uuid.New(),
				UserID:          uuid.New(),
				CategoryID:      &categoryID,
				Content:         "Lunch break",
				CheckinTime:     time.Date(2025, 11, 13, 12, 0, 0, 0, loc),
				DurationMinutes: 45,
			},
		},
	}

	s := &Service{}

	tests := []struct {
		name          string
		blockMinutes  int
		expectedCount int
	}{
		{
			name:          "15 minute blocks",
			blockMinutes:  15,
			expectedCount: 96, // 24 hours * 4 blocks per hour
		},
		{
			name:          "30 minute blocks",
			blockMinutes:  30,
			expectedCount: 48, // 24 hours * 2 blocks per hour
		},
		{
			name:          "45 minute blocks",
			blockMinutes:  45,
			expectedCount: 32, // 24 hours / 0.75 hours
		},
		{
			name:          "2 hour blocks",
			blockMinutes:  120,
			expectedCount: 12, // 24 hours / 2 hours
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := s.generateTimeBlocks(startOfDay, endOfDay, tt.blockMinutes, checkins, loc)
			assert.Equal(t, tt.expectedCount, len(blocks))

			// Verify first and last blocks
			assert.Equal(t, startOfDay, blocks[0].StartTime)
			assert.Equal(t, endOfDay, blocks[len(blocks)-1].EndTime)
		})
	}
}

func TestDetectGaps(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")

	tests := []struct {
		name         string
		checkins     []*CheckinWithMeta
		expectedGaps int
	}{
		{
			name:         "no check-ins",
			checkins:     []*CheckinWithMeta{},
			expectedGaps: 0,
		},
		{
			name: "single check-in",
			checkins: []*CheckinWithMeta{
				{
					Checkin: &checkin.Checkin{
						CheckinTime:     time.Date(2025, 11, 13, 8, 0, 0, 0, loc),
						DurationMinutes: 30,
					},
				},
			},
			expectedGaps: 0,
		},
		{
			name: "two check-ins with gap",
			checkins: []*CheckinWithMeta{
				{
					Checkin: &checkin.Checkin{
						CheckinTime:     time.Date(2025, 11, 13, 8, 0, 0, 0, loc),
						DurationMinutes: 30,
					},
				},
				{
					Checkin: &checkin.Checkin{
						CheckinTime:     time.Date(2025, 11, 13, 9, 0, 0, 0, loc),
						DurationMinutes: 30,
					},
				},
			},
			expectedGaps: 1,
		},
		{
			name: "consecutive check-ins no gap",
			checkins: []*CheckinWithMeta{
				{
					Checkin: &checkin.Checkin{
						CheckinTime:     time.Date(2025, 11, 13, 8, 0, 0, 0, loc),
						DurationMinutes: 30,
					},
				},
				{
					Checkin: &checkin.Checkin{
						CheckinTime:     time.Date(2025, 11, 13, 8, 30, 0, 0, loc),
						DurationMinutes: 30,
					},
				},
			},
			expectedGaps: 0,
		},
	}

	s := &Service{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gaps := s.detectGaps(tt.checkins, loc)
			assert.Equal(t, tt.expectedGaps, len(gaps))

			// If there's a gap, verify it has positive duration
			for _, gap := range gaps {
				assert.Greater(t, gap.DurationMins, 0)
				assert.True(t, gap.EndTime.After(gap.StartTime))
			}
		})
	}
}

func TestCalculateDailySummary(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	date := time.Date(2025, 11, 13, 0, 0, 0, 0, loc)

	tests := []struct {
		name     string
		checkins []*CheckinWithMeta
		gaps     []*Gap
		validate func(*testing.T, *DailySummary)
	}{
		{
			name:     "no check-ins",
			checkins: []*CheckinWithMeta{},
			gaps:     []*Gap{},
			validate: func(t *testing.T, summary *DailySummary) {
				assert.Equal(t, 0, summary.TotalCheckins)
				assert.Equal(t, 0, summary.TotalMinutes)
				assert.Nil(t, summary.FirstCheckinTime)
				assert.Nil(t, summary.LastCheckinTime)
			},
		},
		{
			name: "single check-in",
			checkins: []*CheckinWithMeta{
				{
					Checkin: &checkin.Checkin{
						CheckinTime:     time.Date(2025, 11, 13, 8, 0, 0, 0, loc),
						DurationMinutes: 30,
					},
				},
			},
			gaps: []*Gap{},
			validate: func(t *testing.T, summary *DailySummary) {
				assert.Equal(t, 1, summary.TotalCheckins)
				assert.Equal(t, 30, summary.TotalMinutes)
				assert.NotNil(t, summary.FirstCheckinTime)
				assert.NotNil(t, summary.LastCheckinTime)
				assert.Equal(t, float64(1), summary.ActiveHours)
			},
		},
		{
			name: "multiple check-ins with gaps",
			checkins: []*CheckinWithMeta{
				{
					Checkin: &checkin.Checkin{
						CheckinTime:     time.Date(2025, 11, 13, 8, 0, 0, 0, loc),
						DurationMinutes: 30,
					},
				},
				{
					Checkin: &checkin.Checkin{
						CheckinTime:     time.Date(2025, 11, 13, 12, 0, 0, 0, loc),
						DurationMinutes: 45,
					},
				},
			},
			gaps: []*Gap{
				{
					StartTime:    time.Date(2025, 11, 13, 8, 30, 0, 0, loc),
					EndTime:      time.Date(2025, 11, 13, 12, 0, 0, 0, loc),
					DurationMins: 210,
				},
			},
			validate: func(t *testing.T, summary *DailySummary) {
				assert.Equal(t, 2, summary.TotalCheckins)
				assert.Equal(t, 75, summary.TotalMinutes)
				assert.Equal(t, 1, summary.GapCount)
				assert.Equal(t, 210, summary.TotalGapMinutes)
				assert.InDelta(t, 210.0, summary.AverageGapMinutes, 0.1)
				assert.Equal(t, float64(2), summary.ActiveHours)
			},
		},
	}

	s := &Service{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := s.calculateDailySummary(date, tt.checkins, tt.gaps)
			tt.validate(t, summary)
		})
	}
}

func TestBuildCategoryLegend(t *testing.T) {
	categoryID1 := uuid.New()
	categoryID2 := uuid.New()

	categoryName1 := "Work"
	categoryName2 := "Exercise"
	color1 := "#FF0000"
	color2 := "#00FF00"

	categoryMap := map[uuid.UUID]*category.Category{
		categoryID1: {
			ID:    categoryID1,
			Name:  categoryName1,
			Color: &color1,
		},
		categoryID2: {
			ID:    categoryID2,
			Name:  categoryName2,
			Color: &color2,
		},
	}

	checkins := []*CheckinWithMeta{
		{Checkin: &checkin.Checkin{CategoryID: &categoryID1}},
		{Checkin: &checkin.Checkin{CategoryID: &categoryID1}},
		{Checkin: &checkin.Checkin{CategoryID: &categoryID1}},
		{Checkin: &checkin.Checkin{CategoryID: &categoryID2}},
		{Checkin: &checkin.Checkin{CategoryID: &categoryID2}},
	}

	s := &Service{}
	legend := s.buildCategoryLegend(checkins, categoryMap)

	// Should be sorted by count descending
	assert.Equal(t, 2, len(legend))
	assert.Equal(t, categoryID1, legend[0].CategoryID)
	assert.Equal(t, 3, legend[0].Count)
	assert.Equal(t, categoryName1, legend[0].CategoryName)
	assert.Equal(t, categoryID2, legend[1].CategoryID)
	assert.Equal(t, 2, legend[1].Count)
	assert.Equal(t, categoryName2, legend[1].CategoryName)
}
