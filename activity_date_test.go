package main

import (
	"testing"
	"time"
)

func TestActivityDateStringFromInstant(t *testing.T) {
	tests := []struct {
		name     string
		instant  time.Time
		expected string
	}{
		{
			name:     "JST 03:59 は前日扱い",
			instant:  time.Date(2026, 5, 2, 18, 59, 0, 0, time.UTC),
			expected: "2026-05-02",
		},
		{
			name:     "JST 04:00 は当日扱い",
			instant:  time.Date(2026, 5, 2, 19, 0, 0, 0, time.UTC),
			expected: "2026-05-03",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := activityDateStringFromInstant(
				tt.instant,
				defaultActivityTimezone,
				defaultActivityCutoffHour,
			)
			if got != tt.expected {
				t.Fatalf("activityDateStringFromInstant() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNormalizeActivityCutoffHour(t *testing.T) {
	if got := normalizeActivityCutoffHour(-1); got != defaultActivityCutoffHour {
		t.Fatalf("normalizeActivityCutoffHour(-1) = %d, want %d", got, defaultActivityCutoffHour)
	}
}
