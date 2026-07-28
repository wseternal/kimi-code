package protocol

import "time"

// NowISO returns the current time as an ISO 8601 string (UTC, with millisecond precision).
func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// ParseISO parses an ISO 8601 datetime string into a time.Time.
func ParseISO(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}
