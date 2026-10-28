package ui

import (
	"strconv"
	"time"
)

const (
	day   = 24 * time.Hour
	week  = 7 * day
	month = 30 * day
	year  = 365 * day
)

// relAge renders how long before now t was, in its largest whole unit:
// now, Nm, Nh, Nd, Nw, Nmo or Ny. A time in the future is now.
func relAge(t, now time.Time) string {
	d := now.Sub(t)

	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return count(d, time.Minute, "m")
	case d < day:
		return count(d, time.Hour, "h")
	case d < week:
		return count(d, day, "d")
	case d < month:
		return count(d, week, "w")
	case d < year:
		return count(d, month, "mo")
	default:
		return count(d, year, "y")
	}
}

func count(d, unit time.Duration, suffix string) string {
	return strconv.FormatInt(int64(d/unit), 10) + suffix
}
