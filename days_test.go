package gocron

import (
	"fmt"
	"testing"
	"time"
)

// TestDayRules compares the days on which schedules with L, W and # fire
// from 2000 to 2100 with days worked out from the definitions, one date at
// a time.
func TestDayRules(t *testing.T) {
	type date struct {
		y       int
		m       time.Month
		d, last int
		wd      time.Weekday
	}
	weekday := func(y int, m time.Month, d int) time.Weekday {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Weekday()
	}
	workday := func(wd time.Weekday) bool { return wd != time.Saturday && wd != time.Sunday }
	// nearest is the workday of the month nearest to day n; there are no ties.
	nearest := func(x date, n int) bool {
		if n > x.last {
			return false
		}
		best, dist := 0, 99
		for d := 1; d <= x.last; d++ {
			if k := max(d-n, n-d); workday(weekday(x.y, x.m, d)) && k < dist {
				best, dist = d, k
			}
		}
		return x.d == best
	}
	lastWorkday := func(x date) bool {
		d := x.last
		for !workday(weekday(x.y, x.m, d)) {
			d--
		}
		return x.d == d
	}
	// nth counts the days of the same weekday from the 1st up to x.
	nth := func(x date) int {
		n := 0
		for d := 1; d <= x.d; d++ {
			if weekday(x.y, x.m, d) == x.wd {
				n++
			}
		}
		return n
	}
	lastOfWeekday := func(x date) bool { return x.d+7 > x.last }
	cases := []struct {
		spec  string
		fires func(date) bool
	}{
		{"0 0 L * *", func(x date) bool { return x.d == x.last }},
		{"0 0 L-3 * *", func(x date) bool { return x.d == x.last-3 }},
		{"0 0 L-27 feb *", func(x date) bool { return x.m == 2 && x.d == x.last-27 }},
		{"0 0 LW * *", lastWorkday},
		{"0 0 1W * *", func(x date) bool { return nearest(x, 1) }},
		{"0 0 15W * *", func(x date) bool { return nearest(x, 15) }},
		{"0 0 30W * *", func(x date) bool { return nearest(x, 30) }},
		{"0 0 31W * *", func(x date) bool { return nearest(x, 31) }},
		{"0 0 * * 5L", func(x date) bool { return x.wd == time.Friday && lastOfWeekday(x) }},
		{"0 0 * * 0L", func(x date) bool { return x.wd == time.Sunday && lastOfWeekday(x) }},
		{"0 0 * * 7L", func(x date) bool { return x.wd == time.Sunday && lastOfWeekday(x) }},
		{"0 0 * * 1#1", func(x date) bool { return x.wd == time.Monday && nth(x) == 1 }},
		{"0 0 * * sat#5", func(x date) bool { return x.wd == time.Saturday && nth(x) == 5 }},
		{"0 0 * * 7#2,3#4", func(x date) bool {
			return x.wd == time.Sunday && nth(x) == 2 || x.wd == time.Wednesday && nth(x) == 4
		}},
		{"0 0 * * 7", func(x date) bool { return x.wd == time.Sunday }},
		{"0 0 * * 1-7/2", func(x date) bool {
			return x.wd == time.Monday || x.wd == time.Wednesday || x.wd == time.Friday || x.wd == time.Sunday
		}},
		{"0 0 * * fri-sun", func(x date) bool { return x.wd == time.Friday || x.wd == time.Saturday || x.wd == time.Sunday }},
		{"0 0 13 * fri", func(x date) bool { return x.d == 13 || x.wd == time.Friday }},
		{"0 0 L * 1#1", func(x date) bool { return x.d == x.last || x.wd == time.Monday && nth(x) == 1 }},
		{"0 0 LW,1 * *", func(x date) bool { return x.d == 1 || lastWorkday(x) }},
		{"0 0 15W * 5L", func(x date) bool { return nearest(x, 15) || x.wd == time.Friday && lastOfWeekday(x) }},
		{"0 0 ? feb,aug 5#2", func(x date) bool { return (x.m == 2 || x.m == 8) && x.wd == time.Friday && nth(x) == 2 }},
	}
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2101, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range cases {
		var want []time.Time
		for day := from; day.Before(until); day = day.AddDate(0, 0, 1) {
			y, m, d := day.Date()
			x := date{y, m, d, time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day(), day.Weekday()}
			if c.fires(x) {
				want = append(want, day)
			}
		}
		s := mustParse(t, c.spec, "UTC")
		var got []time.Time
		for next := s.Next(from.Add(-time.Second)); next.Before(until); next = s.Next(next) {
			got = append(got, next)
		}
		if err := sameTimes(got, want); err != nil {
			t.Errorf("%q, Next: %v", c.spec, err)
		}
		got = got[:0]
		for prev := s.Prev(until); !prev.Before(from); prev = s.Prev(prev) {
			got = append([]time.Time{prev}, got...)
		}
		if err := sameTimes(got, want); err != nil {
			t.Errorf("%q, Prev: %v", c.spec, err)
		}
	}
}

// sameTimes describes the first difference between got and want.
func sameTimes(got, want []time.Time) error {
	for i := 0; i < len(got) || i < len(want); i++ {
		switch {
		case i >= len(got):
			return fmt.Errorf("missing %s", want[i])
		case i >= len(want):
			return fmt.Errorf("extra %s", got[i])
		case !got[i].Equal(want[i]):
			return fmt.Errorf("got %s, want %s", got[i], want[i])
		}
	}
	return nil
}
