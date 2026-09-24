package gocron

import (
	"testing"
	"time"
)

// FuzzSchedule parses any input and checks the times of the expressions
// that parse: Next moves forward and Prev backward, on whole seconds, and
// the two agree with each other.
func FuzzSchedule(f *testing.F) {
	for _, spec := range []string{
		"0 3 * * *", "*/20 2 * * *", "0 0 29 feb *", "0 0 30 feb *", "@every 1h30m", "@daily",
		"CRON_TZ=Australia/Lord_Howe 15 2 * * *", "TZ=America/Havana 30 0 * * *", "1-5/2 * * * * *",
		"0 12 1 * sun", "0 0 L * *", "0 0 LW * *", "0 0 15W * *", "0 0 * * 5L", "0 0 * * 1#1,5#5", "0 0 * * mon-sun",
	} {
		f.Add(spec, int64(1_774_000_000))
		f.Add(spec, int64(2_240_524_800)) // December 31, 2040
	}
	zones := []*time.Location{time.UTC}
	for _, name := range []string{"America/New_York", "Australia/Lord_Howe", "America/Santiago", "Antarctica/Troll"} {
		if loc, err := time.LoadLocation(name); err == nil {
			zones = append(zones, loc)
		}
	}
	f.Fuzz(func(t *testing.T, spec string, unix int64) {
		at := time.Unix(unix%4_000_000_000, 0) // from 1843 to 2096
		for _, loc := range zones {
			s, err := ParseInLocation(spec, loc)
			if err != nil {
				continue
			}
			if next := s.Next(at); !next.IsZero() {
				if !next.After(at) || next.Nanosecond() != 0 {
					t.Fatalf("%q in %s: Next(%s) = %s", spec, loc, at, next)
				}
				// Nothing fires between at and next.
				if prev := s.Prev(next); prev.IsZero() || prev.After(at) || !s.Next(prev).Equal(next) {
					t.Fatalf("%q in %s: Next(%s) = %s, but Prev(%s) = %s", spec, loc, at, next, next, prev)
				}
			}
			if prev := s.Prev(at); !prev.IsZero() {
				if !prev.Before(at) || prev.Nanosecond() != 0 {
					t.Fatalf("%q in %s: Prev(%s) = %s", spec, loc, at, prev)
				}
				// Nothing fires between prev and at.
				if next := s.Next(prev); next.IsZero() || next.Before(at) {
					t.Fatalf("%q in %s: Prev(%s) = %s, but Next(%s) = %s", spec, loc, at, prev, prev, next)
				}
			}
		}
	})
}
