package gocron

import (
	"strings"
	"testing"
	"time"
)

// mustParse parses spec in the named zone, and skips the test when the
// zone is not available.
func mustParse(t *testing.T, spec, zone string) Schedule {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Skip(err)
	}
	s, err := ParseInLocation(spec, loc)
	if err != nil {
		t.Fatalf("ParseInLocation(%q, %s): %v", spec, zone, err)
	}
	return s
}

// nexts returns the next n times after from.
func nexts(s Schedule, from time.Time, n int) []time.Time {
	var out []time.Time
	for i := 0; i < n; i++ {
		from = s.Next(from)
		out = append(out, from)
	}
	return out
}

func TestFields(t *testing.T) {
	utc := time.UTC
	from := time.Date(2026, 9, 24, 10, 7, 30, 0, utc) // a Thursday
	cases := []struct {
		spec string
		want []time.Time
	}{
		{"*/15 * * * *", []time.Time{time.Date(2026, 9, 24, 10, 15, 0, 0, utc), time.Date(2026, 9, 24, 10, 30, 0, 0, utc)}},
		{"0 9-17/4 * * *", []time.Time{time.Date(2026, 9, 24, 13, 0, 0, 0, utc), time.Date(2026, 9, 24, 17, 0, 0, 0, utc), time.Date(2026, 9, 25, 9, 0, 0, 0, utc)}},
		{"30 8 * * mon-fri", []time.Time{time.Date(2026, 9, 25, 8, 30, 0, 0, utc), time.Date(2026, 9, 28, 8, 30, 0, 0, utc)}},
		{"0 0 1 jan,jul *", []time.Time{time.Date(2027, 1, 1, 0, 0, 0, 0, utc), time.Date(2027, 7, 1, 0, 0, 0, 0, utc)}},
		// Both day fields restricted: the 1st of the month or any Sunday.
		{"0 12 1 * sun", []time.Time{time.Date(2026, 9, 27, 12, 0, 0, 0, utc), time.Date(2026, 10, 1, 12, 0, 0, 0, utc), time.Date(2026, 10, 4, 12, 0, 0, 0, utc)}},
		// "N/step" runs from N to the end of the field.
		{"50/5 10 * * *", []time.Time{time.Date(2026, 9, 24, 10, 50, 0, 0, utc), time.Date(2026, 9, 24, 10, 55, 0, 0, utc), time.Date(2026, 9, 25, 10, 50, 0, 0, utc)}},
		// Six fields: a leading seconds field.
		{"*/20 8 10 * * *", []time.Time{time.Date(2026, 9, 24, 10, 8, 0, 0, utc), time.Date(2026, 9, 24, 10, 8, 20, 0, utc)}},
		{"@hourly", []time.Time{time.Date(2026, 9, 24, 11, 0, 0, 0, utc)}},
		{"@daily", []time.Time{time.Date(2026, 9, 25, 0, 0, 0, 0, utc)}},
		{"@weekly", []time.Time{time.Date(2026, 9, 27, 0, 0, 0, 0, utc)}},
		{"@monthly", []time.Time{time.Date(2026, 10, 1, 0, 0, 0, 0, utc)}},
		{"@yearly", []time.Time{time.Date(2027, 1, 1, 0, 0, 0, 0, utc)}},
		// Leap days come every four years.
		{"0 0 29 feb *", []time.Time{time.Date(2028, 2, 29, 0, 0, 0, 0, utc), time.Date(2032, 2, 29, 0, 0, 0, 0, utc)}},
		// 7 is Sunday, and a range of days of the week can end on it.
		{"0 0 * * 7", []time.Time{time.Date(2026, 9, 27, 0, 0, 0, 0, utc), time.Date(2026, 10, 4, 0, 0, 0, 0, utc)}},
		{"0 0 * * 5-7", []time.Time{time.Date(2026, 9, 25, 0, 0, 0, 0, utc), time.Date(2026, 9, 26, 0, 0, 0, 0, utc), time.Date(2026, 9, 27, 0, 0, 0, 0, utc), time.Date(2026, 10, 2, 0, 0, 0, 0, utc)}},
		{"0 0 * * mon-sun", []time.Time{time.Date(2026, 9, 25, 0, 0, 0, 0, utc), time.Date(2026, 9, 26, 0, 0, 0, 0, utc)}},
		// The last day of the month, and two days before it.
		{"0 0 L * *", []time.Time{time.Date(2026, 9, 30, 0, 0, 0, 0, utc), time.Date(2026, 10, 31, 0, 0, 0, 0, utc), time.Date(2026, 11, 30, 0, 0, 0, 0, utc)}},
		{"0 0 L-2 * *", []time.Time{time.Date(2026, 9, 28, 0, 0, 0, 0, utc), time.Date(2026, 10, 29, 0, 0, 0, 0, utc), time.Date(2026, 11, 28, 0, 0, 0, 0, utc)}},
		// The last weekday: October 31, 2026 is a Saturday.
		{"0 0 LW * *", []time.Time{time.Date(2026, 9, 30, 0, 0, 0, 0, utc), time.Date(2026, 10, 30, 0, 0, 0, 0, utc), time.Date(2026, 11, 30, 0, 0, 0, 0, utc)}},
		// The weekday nearest to the 15th: November 15, 2026 is a Sunday.
		{"0 0 15W * *", []time.Time{time.Date(2026, 10, 15, 0, 0, 0, 0, utc), time.Date(2026, 11, 16, 0, 0, 0, 0, utc), time.Date(2026, 12, 15, 0, 0, 0, 0, utc)}},
		// The last Friday, and the third Friday, of the month.
		{"0 0 * * 5L", []time.Time{time.Date(2026, 9, 25, 0, 0, 0, 0, utc), time.Date(2026, 10, 30, 0, 0, 0, 0, utc), time.Date(2026, 11, 27, 0, 0, 0, 0, utc)}},
		{"0 0 * * fri#3", []time.Time{time.Date(2026, 10, 16, 0, 0, 0, 0, utc), time.Date(2026, 11, 20, 0, 0, 0, 0, utc)}},
		// Terms combine within a field, and restricted day fields combine with OR.
		{"0 0 * * 1#1,5L", []time.Time{time.Date(2026, 9, 25, 0, 0, 0, 0, utc), time.Date(2026, 10, 5, 0, 0, 0, 0, utc), time.Date(2026, 10, 30, 0, 0, 0, 0, utc)}},
		{"0 0 L * fri", []time.Time{time.Date(2026, 9, 25, 0, 0, 0, 0, utc), time.Date(2026, 9, 30, 0, 0, 0, 0, utc), time.Date(2026, 10, 2, 0, 0, 0, 0, utc)}},
		{"0 0 1,L * *", []time.Time{time.Date(2026, 9, 30, 0, 0, 0, 0, utc), time.Date(2026, 10, 1, 0, 0, 0, 0, utc), time.Date(2026, 10, 31, 0, 0, 0, 0, utc)}},
	}
	for _, c := range cases {
		got := nexts(mustParse(t, c.spec, "UTC"), from, len(c.want))
		for i := range c.want {
			if !got[i].Equal(c.want[i]) {
				t.Errorf("%q: next %d = %s, want %s", c.spec, i+1, got[i], c.want[i])
			}
		}
	}
}

func TestNeverFires(t *testing.T) {
	for _, spec := range []string{"0 0 30 feb *", "0 0 31 4,6,9,11 *", "0 0 L-30 feb *"} {
		s := mustParse(t, spec, "UTC")
		if next := s.Next(time.Now()); !next.IsZero() {
			t.Errorf("%q never fires, but Next returned %s", spec, next)
		}
		if prev := s.Prev(time.Now()); !prev.IsZero() {
			t.Errorf("%q never fires, but Prev returned %s", spec, prev)
		}
	}
}

// The year 2100 is not a leap year, so February 29 skips eight years.
func TestLongGaps(t *testing.T) {
	s := mustParse(t, "0 0 29 feb *", "UTC")
	if got, want := s.Next(time.Date(2097, 1, 1, 0, 0, 0, 0, time.UTC)), time.Date(2104, 2, 29, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Next = %s, want %s", got, want)
	}
	if got, want := s.Prev(time.Date(2104, 1, 1, 0, 0, 0, 0, time.UTC)), time.Date(2096, 2, 29, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Prev = %s, want %s", got, want)
	}
}

func TestPrev(t *testing.T) {
	utc := time.UTC
	from := time.Date(2026, 9, 24, 10, 7, 30, 0, utc)
	cases := []struct {
		spec string
		want []time.Time
	}{
		{"*/15 * * * *", []time.Time{time.Date(2026, 9, 24, 10, 0, 0, 0, utc), time.Date(2026, 9, 24, 9, 45, 0, 0, utc)}},
		// 10:08:00, 10:08:20 and 10:08:40 every day, so yesterday's come first.
		{"*/20 8 10 * * *", []time.Time{time.Date(2026, 9, 23, 10, 8, 40, 0, utc), time.Date(2026, 9, 23, 10, 8, 20, 0, utc), time.Date(2026, 9, 23, 10, 8, 0, 0, utc)}},
		{"0 0 L * *", []time.Time{time.Date(2026, 8, 31, 0, 0, 0, 0, utc), time.Date(2026, 7, 31, 0, 0, 0, 0, utc)}},
		{"0 0 * * 5L", []time.Time{time.Date(2026, 8, 28, 0, 0, 0, 0, utc), time.Date(2026, 7, 31, 0, 0, 0, 0, utc)}},
		{"0 0 29 feb *", []time.Time{time.Date(2024, 2, 29, 0, 0, 0, 0, utc), time.Date(2020, 2, 29, 0, 0, 0, 0, utc)}},
		{"@hourly", []time.Time{time.Date(2026, 9, 24, 10, 0, 0, 0, utc), time.Date(2026, 9, 24, 9, 0, 0, 0, utc)}},
		{"@every 90s", []time.Time{time.Date(2026, 9, 24, 10, 6, 0, 0, utc), time.Date(2026, 9, 24, 10, 4, 30, 0, utc)}},
	}
	for _, c := range cases {
		s := mustParse(t, c.spec, "UTC")
		got := from
		for i, want := range c.want {
			if got = s.Prev(got); !got.Equal(want) {
				t.Errorf("%q: prev %d = %s, want %s", c.spec, i+1, got, want)
				break
			}
		}
	}
	// At a time at which the schedule fires, Prev and Next both move away.
	s := mustParse(t, "*/15 * * * *", "UTC")
	at := time.Date(2026, 9, 24, 10, 15, 0, 0, utc)
	if got := s.Prev(at); !got.Equal(at.Add(-15 * time.Minute)) {
		t.Errorf("Prev(%s) = %s", at, got)
	}
	if got := s.Next(at); !got.Equal(at.Add(15 * time.Minute)) {
		t.Errorf("Next(%s) = %s", at, got)
	}
}

func TestEveryRoundsToWholeSeconds(t *testing.T) {
	from := time.Date(2026, 9, 24, 10, 0, 0, 400_000_000, time.UTC)
	if got, want := MustParse("@every 90s").Next(from), time.Date(2026, 9, 24, 10, 1, 30, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Next = %s, want %s", got, want)
	}
	if got, want := MustParse("@every 200ms").Next(from), time.Date(2026, 9, 24, 10, 0, 1, 0, time.UTC); !got.Equal(want) {
		t.Errorf("a delay below a second runs every second: got %s, want %s", got, want)
	}
}

func TestParse(t *testing.T) {
	for _, spec := range []string{
		"0 3 * * *", "*/5 * * * * *", "@every 10m", "@hourly", "30 9 * * 1-5", "  0 3 * * *  ",
		"CRON_TZ=UTC 0 0 * * *", "TZ=Asia/Tokyo 0 9 * * *", "CRON_TZ=Asia/Tokyo @daily",
	} {
		if _, err := Parse(spec); err != nil {
			t.Errorf("Parse(%q) = %v", spec, err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, spec := range []string{
		"", "every day", "0 3 * *", "* * * * * * *", "60 * * * * *",
		"60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8",
		"5-1 * * * *", "*/0 * * * *", "*-5 * * * *", "a * * * *", "* * * foo *", "1,,2 * * * *",
		"@every", "@every soon", "@often",
		"CRON_TZ=Mars/Olympus @hourly", "CRON_TZ=UTC", "TZ=UTC   ",
		// L, W and # belong to the day fields and take no ranges or steps.
		"0 L * * *", "0 0 * L *", "0 0 L/2 * *", "0 0 W * *", "0 0 1-5W * *", "0 0 1W-3 * *", "0 0 LW-1 * *",
		"0 0 L-31 * *", "0 0 L-x * *", "0 0 0W * *", "0 0 32W * *",
		"0 0 * * L", "0 0 * * 8L", "0 0 * * 1-5L", "0 0 * * 5#0", "0 0 * * 5#6", "0 0 * * 5#", "0 0 * * #3", "0 0 * * 8#1",
		"0 0 * * sun-sat-mon", "0 0 * * sat-fri",
	} {
		if _, err := Parse(spec); err == nil {
			t.Errorf("Parse(%q) should fail", spec)
		}
	}
	if _, err := ParseInLocation("@hourly", nil); err == nil {
		t.Error("ParseInLocation with a nil location should fail")
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), `gocron: MustParse("61 * * * *")`) {
			t.Errorf("recovered %v", r)
		}
	}()
	MustParse("61 * * * *")
}

func TestNextInLocation(t *testing.T) {
	s := mustParse(t, "0 9 * * *", "Asia/Tokyo")
	// 00:00 UTC is 9:00 in Tokyo, so the next time is a day later.
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next := s.Next(from)
	if want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("Next = %s, want %s", next, want)
	}
	if next.Location() != time.UTC {
		t.Errorf("Next returned a time in %s, want the location of its argument", next.Location())
	}
}

func TestZonePrefixTakesPrecedence(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip(err)
	}
	s, err := ParseInLocation("CRON_TZ=UTC 0 3 * * *", tokyo)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got, want := s.Next(from), time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Next = %s, want %s", got, want)
	}
}
