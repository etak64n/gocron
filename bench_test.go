package gocron

import (
	"testing"
	"time"
)

func BenchmarkParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := ParseInLocation("0 9 * * mon-fri", time.UTC); err != nil {
			b.Fatal(err)
		}
	}
}

var benchSchedules = []struct{ name, spec, zone string }{
	{"EveryMinute", "* * * * *", "UTC"},
	{"Weekdays", "0 9 * * mon-fri", "Asia/Tokyo"},
	{"DailyNewYork", "30 2 * * *", "America/New_York"},
	{"LastWeekday", "0 0 LW * *", "UTC"},
	{"LeapDay", "0 0 29 feb *", "UTC"},
	{"Never", "0 0 30 feb *", "UTC"},
}

func BenchmarkNext(b *testing.B) {
	for _, c := range benchSchedules {
		b.Run(c.name, func(b *testing.B) {
			s := benchParse(b, c.spec, c.zone)
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			t := start
			for i := 0; i < b.N; i++ {
				if t = s.Next(t); t.IsZero() {
					t = start
				}
			}
		})
	}
}

func BenchmarkPrev(b *testing.B) {
	for _, c := range benchSchedules {
		b.Run(c.name, func(b *testing.B) {
			s := benchParse(b, c.spec, c.zone)
			start := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
			t := start
			for i := 0; i < b.N; i++ {
				if t = s.Prev(t); t.IsZero() {
					t = start
				}
			}
		})
	}
}

func benchParse(b *testing.B, spec, zone string) Schedule {
	b.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		b.Skip(err)
	}
	s, err := ParseInLocation(spec, loc)
	if err != nil {
		b.Fatal(err)
	}
	return s
}
