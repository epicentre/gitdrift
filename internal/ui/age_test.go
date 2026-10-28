package ui

import (
	"testing"
	"time"
)

func TestRelAge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"future", -time.Hour, "now"},
		{"zero", 0, "now"},
		{"just under a minute", time.Minute - time.Second, "now"},
		{"one minute", time.Minute, "1m"},
		{"just under an hour", time.Hour - time.Second, "59m"},
		{"one hour", time.Hour, "1h"},
		{"just under a day", day - time.Second, "23h"},
		{"one day", day, "1d"},
		{"just under a week", 7*day - time.Second, "6d"},
		{"one week", 7 * day, "1w"},
		{"just under thirty days", 30*day - time.Second, "4w"},
		{"thirty days", 30 * day, "1mo"},
		{"just under a year", 365*day - time.Second, "12mo"},
		{"one year", 365 * day, "1y"},
		{"three years", 3 * 365 * day, "3y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := relAge(fixedNow.Add(-tt.ago), fixedNow); got != tt.want {
				t.Errorf("relAge(now - %s, now) = %q, want %q", tt.ago, got, tt.want)
			}
		})
	}
}
