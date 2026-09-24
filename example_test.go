package gocron_test

import (
	"fmt"
	"time"

	"github.com/etak64n/gocron"
)

func ExampleParseInLocation() {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		fmt.Println(err)
		return
	}
	s, err := gocron.ParseInLocation("0 9 * * mon-fri", tokyo)
	if err != nil {
		fmt.Println(err)
		return
	}
	t := time.Date(2026, 9, 25, 12, 0, 0, 0, tokyo) // a Friday
	for range 3 {
		t = s.Next(t)
		fmt.Println(t.Format("Mon Jan 2 15:04 MST"))
	}
	// Output:
	// Mon Sep 28 09:00 JST
	// Tue Sep 29 09:00 JST
	// Wed Sep 30 09:00 JST
}

var nightly = gocron.MustParse("CRON_TZ=UTC 0 3 * * *")

func ExampleMustParse() {
	fmt.Println(nightly.Next(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)))
	// Output: 2026-09-25 03:00:00 +0000 UTC
}

// In New York, the clock jumps from 2:00 to 3:00 on March 8, 2026, and
// falls back from 2:00 to 1:00 on November 1, 2026.
func Example_daylightSaving() {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		fmt.Println(err)
		return
	}
	// Each skipped time fires at the moment it would have come.
	skipped := gocron.MustParse("CRON_TZ=America/New_York */20 2 * * *")
	t := time.Date(2026, 3, 7, 12, 0, 0, 0, ny)
	for range 4 {
		t = skipped.Next(t)
		fmt.Println(t.Format("Jan 2 15:04 MST"))
	}
	// A time that the clock shows twice fires once.
	twice := gocron.MustParse("CRON_TZ=America/New_York 30 1 * * *")
	t = time.Date(2026, 10, 31, 12, 0, 0, 0, ny)
	for range 2 {
		t = twice.Next(t)
		fmt.Println(t.Format("Jan 2 15:04 MST"))
	}
	// Output:
	// Mar 8 03:00 EDT
	// Mar 8 03:20 EDT
	// Mar 8 03:40 EDT
	// Mar 9 02:00 EDT
	// Nov 1 01:30 EDT
	// Nov 2 01:30 EST
}
