package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Hours is a daily time range in minutes after midnight.
// When End is before Start, the range crosses midnight.
type Hours struct {
	Start, End int
	Set        bool
}

// Contains reports whether the local time t is in the range.
// An unset range contains all times.
func (h Hours) Contains(t time.Time) bool {
	if !h.Set {
		return true
	}
	m := t.Hour()*60 + t.Minute()
	if h.Start <= h.End {
		return m >= h.Start && m < h.End
	}
	return m >= h.Start || m < h.End
}

// length returns the length of the range in minutes; 1440 for an unset range.
func (h Hours) length() int {
	if !h.Set {
		return 24 * 60
	}
	return ((h.End-h.Start)%(24*60) + 24*60) % (24 * 60)
}

// ParseHours parses "HH:MM-HH:MM". An empty string is an unset range.
func ParseHours(s string) (Hours, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Hours{}, nil
	}
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return Hours{}, fmt.Errorf("%q is not HH:MM-HH:MM", s)
	}
	start, err1 := parseClock(a)
	end, err2 := parseClock(b)
	if err1 != nil || err2 != nil {
		return Hours{}, fmt.Errorf("%q is not HH:MM-HH:MM", s)
	}
	if start == end {
		return Hours{}, fmt.Errorf("%q has the same start and end", s)
	}
	return Hours{Start: start, End: end, Set: true}, nil
}

func parseClock(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, errors.New("bad clock")
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, errors.New("bad clock")
	}
	return hh*60 + mm, nil
}

// onDay reports whether the schedule sends on the day of t.
func (s Schedule) onDay(t time.Time) bool {
	if len(s.Days) == 0 {
		return true
	}
	return contains(s.Days, Days[(int(t.Weekday())+6)%7])
}

// Next returns the first send time after the time after: the start of the
// hours (or 00:00) plus a multiple of every, inside the hours, on an allowed
// day, in the location of after. A range that crosses midnight belongs to the
// day of its start. Next returns the zero time when no day is allowed.
func (s Schedule) Next(after time.Time) time.Time {
	every := s.Every.D()
	if every <= 0 { // validation keeps it at 1m or more; tests use less
		every = time.Minute
	}
	h, err := ParseHours(s.Hours)
	if err != nil {
		return time.Time{}
	}
	length := time.Duration(h.length()) * time.Minute
	for d := -1; d <= 8; d++ {
		day := time.Date(after.Year(), after.Month(), after.Day()+d, 0, 0, 0, 0, after.Location())
		if !s.onDay(day) {
			continue
		}
		start := day.Add(time.Duration(h.Start) * time.Minute)
		end := start.Add(length)
		t := start
		if !after.Before(start) {
			t = start.Add((after.Sub(start)/every + 1) * every)
		}
		if t.Before(end) {
			return t
		}
	}
	return time.Time{}
}

// Text describes the schedule, for example "every 15m · Mon–Fri · 08:00–18:00".
func (s Schedule) Text() string {
	days := "every day"
	switch {
	case len(s.Days) == 0 || len(s.Days) == 7:
	case strings.Join(s.Days, ",") == "mon,tue,wed,thu,fri":
		days = "Mon–Fri"
	case strings.Join(s.Days, ",") == "sat,sun":
		days = "Sat–Sun"
	default:
		names := make([]string, len(s.Days))
		for i, d := range s.Days {
			names[i] = strings.ToUpper(d[:1]) + d[1:]
		}
		days = strings.Join(names, " ")
	}
	hours := "all day"
	if h, err := ParseHours(s.Hours); err == nil && h.Set {
		hours = fmt.Sprintf("%02d:%02d–%02d:%02d", h.Start/60, h.Start%60, h.End/60, h.End%60)
	}
	return "every " + s.Every.String() + " · " + days + " · " + hours
}
