package calendar

import (
	"testing"
)

func TestGetColorIntensity(t *testing.T) {
	tests := []struct {
		name               string
		completionPercent  float64
		expectedIntensity int
	}{
		{"No checkins", 0, 0},
		{"Very low", 10, 1},
		{"Low", 25, 1},
		{"Medium-low", 30, 2},
		{"Medium-low max", 50, 2},
		{"Medium-high", 60, 3},
		{"Medium-high max", 75, 3},
		{"High", 80, 4},
		{"Complete", 100, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intensity := GetColorIntensity(tt.completionPercent)
			if intensity != tt.expectedIntensity {
				t.Errorf("GetColorIntensity(%f) = %d, want %d", tt.completionPercent, intensity, tt.expectedIntensity)
			}
		})
	}
}

func TestCalculateCompletionPercent(t *testing.T) {
	tests := []struct {
		name         string
		totalMinutes int
		expected     float64
	}{
		{"No minutes", 0, 0.0},
		{"One hour", 60, 4.166666666666667},
		{"Half day", 720, 50.0},
		{"Full day", 1440, 100.0},
		{"Over day", 2000, 100.0},
		{"Quarter day", 360, 25.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			percent := CalculateCompletionPercent(tt.totalMinutes)
			// Allow small floating point differences
			if percent < tt.expected-0.01 || percent > tt.expected+0.01 {
				t.Errorf("CalculateCompletionPercent(%d) = %f, want %f", tt.totalMinutes, percent, tt.expected)
			}
		})
	}
}

func TestStartDay(t *testing.T) {
	tests := []struct {
		name     string
		startDay StartDay
		valid    bool
	}{
		{"Sunday start", StartDaySunday, true},
		{"Monday start", StartDayMonday, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.startDay != StartDaySunday && tt.startDay != StartDayMonday {
				t.Errorf("Invalid start day: %s", tt.startDay)
			}
		})
	}
}
