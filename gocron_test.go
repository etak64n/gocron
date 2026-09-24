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

func TestNeverReturnsZero(t *testing.T) {
	if next := mustParse(t, "0 0 30 feb *", "UTC").Next(time.Now()); !next.IsZero() {
		t.Errorf("February 30 never comes, got %s", next)
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
		"", "every day", "0 3 * *", "* * * * * * *",
		"60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 7",
		"5-1 * * * *", "*/0 * * * *", "*-5 * * * *", "a * * * *", "* * * foo *", "1,,2 * * * *",
		"@every", "@every soon", "@often",
		"CRON_TZ=Mars/Olympus @hourly", "CRON_TZ=UTC", "TZ=UTC   ",
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
