package main

import (
	"time"
)

const (
	defaultActivityTimezone   = "Asia/Tokyo"
	defaultActivityCutoffHour = 4
)

// currentActivityDateString returns today's Addness activity date (YYYY-MM-DD).
// Activity days switch at cutoffHour in timezone, not at calendar midnight.
func currentActivityDateString(timezone string, cutoffHour int) string {
	return activityDateStringFromInstant(time.Now(), timezone, cutoffHour)
}

func activityDateStringFromInstant(instant time.Time, timezone string, cutoffHour int) string {
	loc := loadAppLocation(timezone)
	cutoffHour = normalizeActivityCutoffHour(cutoffHour)
	local := instant.In(loc)
	if local.Hour() < cutoffHour {
		local = local.AddDate(0, 0, -1)
	}
	return local.Format("2006-01-02")
}

func normalizeActivityCutoffHour(cutoffHour int) int {
	if cutoffHour < 0 || cutoffHour > 23 {
		return defaultActivityCutoffHour
	}
	return cutoffHour
}

func loadAppLocation(timezone string) *time.Location {
	if timezone == "" {
		timezone = defaultActivityTimezone
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc, _ = time.LoadLocation(defaultActivityTimezone)
	}
	return loc
}
