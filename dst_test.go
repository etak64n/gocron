package gocron

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// On the day daylight saving time starts, a time that the clock skips runs
// at the moment it would have come; on the day it ends, a time that the
// clock shows twice runs once, at the first of the two moments.
func TestDaylightSaving(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	s := mustParse(t, "30 2 * * *", "America/New_York")
	spring := s.Next(time.Date(2026, 3, 7, 12, 0, 0, 0, ny))
	if spring.In(ny).Day() != 8 || spring.In(ny).Hour() != 3 || spring.In(ny).Minute() != 30 {
		t.Errorf("2:30 on the day it does not exist = %s, want 3:30 EDT on March 8", spring.In(ny))
	}
	// Lord Howe Island moves its clock by half an hour.
	lh, err := time.LoadLocation("Australia/Lord_Howe")
	if err != nil {
		t.Skip(err)
	}
	gap := mustParse(t, "15 2 * * *", "Australia/Lord_Howe").Next(time.Date(2026, 10, 3, 12, 0, 0, 0, lh))
	if gap.In(lh).Day() != 4 || gap.In(lh).Hour() != 2 || gap.In(lh).Minute() != 45 {
		t.Errorf("2:15 on the day it does not exist = %s, want 2:45 on October 4", gap.In(lh))
	}
	// Every skipped time runs, each at the moment it would have come, and
	// a time that the clock shows twice runs at the first of the two
	// instants, on either side of UTC.
	utc := func(mo time.Month, d, h, mi int) time.Time { return time.Date(2026, mo, d, h, mi, 0, 0, time.UTC) }
	for _, c := range []struct {
		spec, tz string
		from     time.Time
		want     []time.Time
	}{
		// 2:00, 2:20 and 2:40 EST run at 3:00, 3:20 and 3:40 EDT.
		{"*/20 2 * * *", "America/New_York", utc(3, 7, 17, 0), []time.Time{utc(3, 8, 7, 0), utc(3, 8, 7, 20), utc(3, 8, 7, 40), utc(3, 9, 6, 0)}},
		// 2:35 exists at UTC+11 and runs before the skipped 2:10, which
		// comes at 2:40.
		{"10,35 2 * * *", "Australia/Lord_Howe", utc(10, 3, 1, 30), []time.Time{utc(10, 3, 15, 35), utc(10, 3, 15, 40), utc(10, 4, 15, 10), utc(10, 4, 15, 35)}},
		// 1:30 EDT, then 1:30 EST on the next day.
		{"30 1 * * *", "America/New_York", utc(10, 31, 16, 0), []time.Time{utc(11, 1, 5, 30), utc(11, 2, 6, 30)}},
		// 2:30 CEST, then 2:30 CET on the next day.
		{"30 2 * * *", "Europe/Berlin", utc(10, 24, 10, 0), []time.Time{utc(10, 25, 0, 30), utc(10, 26, 1, 30)}},
		// 1:45 at UTC+11, then 1:45 at UTC+10:30 on the next day.
		{"45 1 * * *", "Australia/Lord_Howe", utc(4, 4, 1, 0), []time.Time{utc(4, 4, 14, 45), utc(4, 5, 15, 15)}},
		// From 1:30 CEST: 2:00 CEST, then 3:00 CET, without 2:00 CET.
		{"0 * * * *", "Europe/Berlin", utc(10, 24, 23, 30), []time.Time{utc(10, 25, 0, 0), utc(10, 25, 2, 0)}},
	} {
		if _, err := time.LoadLocation(c.tz); err != nil {
			t.Skip(err)
		}
		s := mustParse(t, c.spec, c.tz)
		got := c.from
		for i, want := range c.want {
			if got = s.Next(got); !got.Equal(want) {
				t.Errorf("%q in %s: run %d at %s, want %s", c.spec, c.tz, i+1, got.UTC(), want)
				break
			}
		}
	}
}

// TestClockChanges compares the times of schedules around every
// clock change of 2026 in zones with unusual changes with times worked out
// by brute force from the readings of the clock.
func TestClockChanges(t *testing.T) {
	zones := []string{
		"America/New_York", "Europe/Berlin", "Europe/Dublin", "Australia/Sydney",
		"America/St_Johns",    // UTC-3:30
		"Australia/Lord_Howe", // moves the clock by half an hour
		"Pacific/Chatham",     // UTC+12:45, changes at 2:45
		"Antarctica/Troll",    // moves the clock by two hours
		"America/Havana",      // changes at midnight
		"America/Santiago",    // changes at midnight, back into the previous date
		"Africa/Cairo",        // changes at midnight
	}
	specs := []string{
		"30 2 * * *", "*/20 2 * * *", "10,35 2 * * *", "* 2 * * *", "45 1 * * *",
		"0 * * * *", "*/15 * * * *", "0 */7 * * *", "0 0 * * *", "30 0 * * *",
		"*/10 23 * * *", "0 23,0,1 * * *", "0 0 * * 0", "*/30 * 2 * * *",
	}
	checked := 0
	for _, zone := range zones {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Logf("skipping %s: %v", zone, err)
			continue
		}
		changes := clockChanges(loc, 2026)
		for _, change := range changes {
			for _, spec := range specs {
				checkAround(t, spec, zone, mustParse(t, spec, zone).(*calendar), loc, change, near(changes, change))
				checked++
			}
		}
	}
	if checked == 0 {
		t.Skip("no time zone data")
	}
}

// clockChanges returns the moments in year at which the UTC offset of loc
// changes.
func clockChanges(loc *time.Location, year int) []time.Time {
	offset := func(t time.Time) int {
		_, o := t.In(loc).Zone()
		return o
	}
	var out []time.Time
	end := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
	for a := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC); a.Before(end); a = a.Add(15 * time.Minute) {
		b := a.Add(15 * time.Minute)
		if offset(a) == offset(b) {
			continue
		}
		for b.Sub(a) > time.Second {
			mid := a.Add(b.Sub(a) / 2).Truncate(time.Second)
			if offset(mid) == offset(a) {
				a = mid
			} else {
				b = mid
			}
		}
		out = append(out, b)
	}
	return out
}

// bruteForce returns, in order, the times of s on the five local dates
// around a clock change. A reading of the clock runs at the first moment
// the clock shows it. A reading that the clock skips runs at the moment it
// would have come with the offset from before the change that skipped it.
// changes lists every change of loc near the dates.
func bruteForce(s *calendar, loc *time.Location, change time.Time, changes []time.Time) []time.Time {
	offsets := map[int]bool{}
	for _, c := range changes {
		_, before := c.Add(-time.Second).In(loc).Zone()
		_, after := c.In(loc).Zone()
		offsets[before], offsets[after] = true, true
	}
	y, m, d := change.In(loc).Date()
	var out []time.Time
	for i := -2; i <= 2; i++ {
		day := time.Date(y, m, d+i, 0, 0, 0, 0, time.UTC)
		if s.days(day.Year(), day.Month())&(1<<uint(day.Day())) == 0 {
			continue
		}
		for h := 0; h < 24; h++ {
			for mi := 0; mi < 60; mi++ {
				for se := 0; se < 60; se++ {
					if s.hour&(1<<uint(h)) == 0 || s.minute&(1<<uint(mi)) == 0 || s.second&(1<<uint(se)) == 0 {
						continue
					}
					w := time.Duration(h)*time.Hour + time.Duration(mi)*time.Minute + time.Duration(se)*time.Second
					var first time.Time
					for o := range offsets {
						c := day.Add(w - time.Duration(o)*time.Second)
						l := c.In(loc)
						ly, lm, ld := l.Date()
						lh, lmi, lse := l.Clock()
						if ly == day.Year() && lm == day.Month() && ld == day.Day() && lh == h && lmi == mi && lse == se &&
							(first.IsZero() || c.Before(first)) {
							first = c
						}
					}
					if first.IsZero() {
						// Find the change whose gap holds the reading.
						for _, c := range changes {
							_, before := c.Add(-time.Second).In(loc).Zone()
							_, after := c.In(loc).Zone()
							wall := day.Add(w)
							if after > before && !wall.Before(c.Add(time.Duration(before)*time.Second)) &&
								wall.Before(c.Add(time.Duration(after)*time.Second)) {
								first = day.Add(w - time.Duration(before)*time.Second)
							}
						}
					}
					if !first.IsZero() {
						out = append(out, first)
					}
				}
			}
		}
	}
	slices.SortFunc(out, time.Time.Compare)
	return slices.CompactFunc(out, time.Time.Equal)
}

// checkAround compares the times of s around change with bruteForce, in
// both directions.
func checkAround(t *testing.T, spec, zone string, s *calendar, loc *time.Location, change time.Time, changes []time.Time) {
	t.Helper()
	want := bruteForce(s, loc, change, changes)
	if len(want) == 0 {
		return
	}
	var got []time.Time
	for c := s.Next(want[0].Add(-time.Second)); !c.After(want[len(want)-1]); c = s.Next(c) {
		if c.IsZero() || len(got) > len(want) {
			t.Errorf("%q in %s around the change at %s: Next went on with %s after %s",
				spec, zone, change.In(loc), c, readings(got, loc))
			return
		}
		got = append(got, c)
	}
	if !slices.EqualFunc(got, want, time.Time.Equal) {
		t.Errorf("%q in %s around the change at %s, Next:\n got %s\nwant %s",
			spec, zone, change.In(loc), readings(got, loc), readings(want, loc))
	}
	got = got[:0]
	for c := s.Prev(want[len(want)-1].Add(time.Second)); !c.Before(want[0]); c = s.Prev(c) {
		if len(got) > len(want) {
			t.Errorf("%q in %s around the change at %s: Prev went on with %s", spec, zone, change.In(loc), c)
			return
		}
		got = append([]time.Time{c}, got...)
	}
	if !slices.EqualFunc(got, want, time.Time.Equal) {
		t.Errorf("%q in %s around the change at %s, Prev:\n got %s\nwant %s",
			spec, zone, change.In(loc), readings(got, loc), readings(want, loc))
	}
}

// near returns the changes within five days of change.
func near(changes []time.Time, change time.Time) []time.Time {
	var out []time.Time
	for _, c := range changes {
		if d := c.Sub(change); d > -5*24*time.Hour && d < 5*24*time.Hour {
			out = append(out, c)
		}
	}
	return out
}

func readings(ts []time.Time, loc *time.Location) string {
	var b strings.Builder
	for _, t := range ts {
		fmt.Fprintf(&b, "%s ", t.In(loc).Format("01-02 15:04:05 -0700"))
	}
	return b.String()
}

// TestAllZones runs the comparison of TestClockChanges for every zone in
// the time zone database of the system, over every clock change from 1900
// to 2040. It takes minutes, so it runs only with GOCRON_ALL_ZONES=1.
func TestAllZones(t *testing.T) {
	if os.Getenv("GOCRON_ALL_ZONES") == "" {
		t.Skip("set GOCRON_ALL_ZONES=1 to check every zone")
	}
	// On macOS, /usr/share/zoneinfo is a link, and WalkDir does not follow
	// links.
	root, err := filepath.EvalSymlinks("/usr/share/zoneinfo")
	if err != nil {
		t.Skip(err)
	}
	seen := map[[32]byte]bool{}
	var zones []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "posix" || d.Name() == "right" {
				return filepath.SkipDir
			}
			return nil
		}
		name, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil || !strings.HasPrefix(string(b), "TZif") || seen[sha256.Sum256(b)] {
			return nil // not a zone, or a link to one already listed
		}
		if _, err := time.LoadLocation(filepath.ToSlash(name)); err == nil {
			seen[sha256.Sum256(b)] = true
			zones = append(zones, filepath.ToSlash(name))
		}
		return nil
	})
	if err != nil {
		t.Skip(err)
	}
	specs := []string{"30 2 * * *", "*/20 2 * * *", "10,35 2 * * *", "0 * * * *", "0 0 * * *", "30 0 * * *", "*/10 23 * * *", "45 1 * * *"}
	count := 0
	for _, zone := range zones {
		began := time.Now()
		loc, _ := time.LoadLocation(zone)
		changes := zoneChanges(loc, time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2041, 1, 1, 0, 0, 0, 0, time.UTC))
		for _, change := range changes {
			around := near(changes, change)
			for _, spec := range specs {
				s, err := ParseInLocation(spec, loc)
				if err != nil {
					t.Fatal(err)
				}
				checkAround(t, spec, zone, s.(*calendar), loc, change, around)
			}
			count++
		}
		if d := time.Since(began); d > time.Second {
			t.Logf("%s: %d changes in %s", zone, len(changes), d.Round(time.Millisecond))
		}
	}
	t.Logf("checked %d clock changes in %d zones", count, len(zones))
}

// zoneChanges returns the moments between from and to at which the UTC
// offset of loc changes. It probes every hour and bisects, without
// time.Time.ZoneBounds, which gocron itself uses.
func zoneChanges(loc *time.Location, from, to time.Time) []time.Time {
	offset := func(t time.Time) int {
		_, o := t.In(loc).Zone()
		return o
	}
	var out []time.Time
	for a := from; a.Before(to); a = a.Add(time.Hour) {
		b := a.Add(time.Hour)
		if offset(a) == offset(b) {
			continue
		}
		lo, hi := a, b
		for hi.Sub(lo) > time.Second {
			mid := lo.Add(hi.Sub(lo) / 2).Truncate(time.Second)
			if offset(mid) == offset(lo) {
				lo = mid
			} else {
				hi = mid
			}
		}
		out = append(out, hi)
	}
	return out
}

// Go reports bounds that do not move forward on the last day of leap years
// after the changes listed in the time zone database, as on December 31,
// 2040. Next and Prev must still return, and return the right times.
func TestLeapYearEndAfterListedChanges(t *testing.T) {
	for _, zone := range []string{"America/New_York", "Australia/Sydney", "Europe/Berlin"} {
		s := mustParse(t, "0 0 * * *", zone)
		loc, _ := time.LoadLocation(zone)
		for _, day := range []int{29, 30, 31} {
			at := time.Date(2040, 12, day, 12, 0, 0, 0, time.UTC)
			done := make(chan [2]time.Time, 1)
			go func() { done <- [2]time.Time{s.Next(at), s.Prev(at)} }()
			// No clock change is near the end of the year in these zones, so
			// time.Date gives the midnights.
			y, m, d := at.In(loc).Date()
			today := time.Date(y, m, d, 0, 0, 0, 0, loc)
			select {
			case got := <-done:
				if want := today.AddDate(0, 0, 1); !got[0].Equal(want) {
					t.Errorf("%s: Next(%s) = %s, want %s", zone, at, got[0], want)
				}
				if want := today; !got[1].Equal(want) {
					t.Errorf("%s: Prev(%s) = %s, want %s", zone, at, got[1], want)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%s: Next or Prev of %s did not return", zone, at)
			}
		}
	}
}

// A zone whose rule starts daylight saving time at 12:30 on January 1 has a
// change right after the end of each leap year, where Go reports bounds
// that stop moving forward. The change must still be found, also between
// two of the hours at which gocron probes for it.
func TestChangeAfterLeapYearEnd(t *testing.T) {
	loc, err := time.LoadLocationFromTZData("Test/NewYearDST", tzif("XST-1XDT-2,J1/12:30,J180/12"))
	if err != nil {
		t.Fatal(err)
	}
	change := time.Date(2029, 1, 1, 11, 30, 0, 0, time.UTC) // 12:30 at UTC+1
	if _, before := change.Add(-time.Second).In(loc).Zone(); before != 3600 {
		t.Fatalf("offset before the change = %d", before)
	}
	if _, after := change.In(loc).Zone(); after != 7200 {
		t.Fatalf("offset after the change = %d", after)
	}
	for _, spec := range []string{"*/30 11,12,13 * * *", "0 0 * * *", "15 12 * * *"} {
		s, err := ParseInLocation(spec, loc)
		if err != nil {
			t.Fatal(err)
		}
		checkAround(t, spec, loc.String(), s.(*calendar), loc, change, []time.Time{change})
	}
}

// tzif returns TZif data for a zone that follows the POSIX TZ rule footer
// at all times, with UTC+1 as its only listed type and no listed changes.
func tzif(footer string) []byte {
	var b bytes.Buffer
	block := func() {
		b.WriteString("TZif2")
		b.Write(make([]byte, 15))
		// isutcnt, isstdcnt, leapcnt, timecnt, typecnt, charcnt
		for _, n := range []uint32{0, 0, 0, 0, 1, 4} {
			binary.Write(&b, binary.BigEndian, n)
		}
		binary.Write(&b, binary.BigEndian, int32(3600))
		b.Write([]byte{0, 0}) // not daylight saving time, designation at 0
		b.WriteString("XST\x00")
	}
	block() // the version 1 data
	block() // the version 2 data
	b.WriteString("\n" + footer + "\n")
	return b.Bytes()
}
