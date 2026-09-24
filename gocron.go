// Package gocron reads cron expressions and works out when they fire, in
// any time zone and on the days when daylight saving time starts or ends.
//
// gocron computes times only. It does not run jobs: a program calls
// [Schedule.Next] and waits for that time with its own timer.
//
// # Expressions
//
// An expression has five fields, separated by spaces:
//
//	┌───────────── minute (0-59)
//	│ ┌─────────── hour (0-23)
//	│ │ ┌───────── day of month (1-31)
//	│ │ │ ┌─────── month (1-12 or jan-dec)
//	│ │ │ │ ┌───── day of week (0-6 or sun-sat, 0 is Sunday)
//	│ │ │ │ │
//	* * * * *
//
// A sixth field in front of them gives the seconds (0-59). Without it, the
// times fall on second 0.
//
// A field is a comma-separated list of terms:
//
//	5           one value
//	1-5         a range
//	* or ?      every value
//	*/15        every 15th value from the smallest
//	10-40/10    every 10th value in a range
//	30/5        every 5th value from 30 to the largest
//
// Names of months and days of the week are not case-sensitive. When both
// the day of month and the day of week are restricted, a day that matches
// either of them fires, as in the classic cron: "0 12 1 * sun" fires at
// noon on the 1st of every month and on every Sunday.
//
// # Descriptors
//
// A descriptor stands for a whole expression:
//
//	@yearly, @annually    0 0 1 1 *
//	@monthly              0 0 1 * *
//	@weekly               0 0 * * 0
//	@daily, @midnight     0 0 * * *
//	@hourly               0 * * * *
//	@every 1h30m          1 hour 30 minutes after the time passed to Next
//
// @every takes a duration in the format of [time.ParseDuration], cut to
// whole seconds with a minimum of one second. Its times fall on whole
// seconds.
//
// # Time zones
//
// The times in an expression are readings of a clock. [ParseInLocation]
// reads them in the given time zone, and [Parse] in the local one. A
// "CRON_TZ=zone " or "TZ=zone " prefix sets the zone in the expression
// itself and takes precedence, as in "CRON_TZ=Europe/Berlin 30 2 * * *".
//
// # Daylight saving time
//
// On the day daylight saving time starts, the clock skips some readings,
// such as 2:00 to 2:59 in New York. A time that the clock skips fires at
// the moment it would have come with the UTC offset from before the
// change, which the clock shows as a time after the gap: "30 2 * * *"
// fires at 3:30, and "*/20 2 * * *" at 3:00, 3:20 and 3:40.
//
// On the day daylight saving time ends, the clock shows some readings
// twice, such as 1:00 to 1:59 in New York. A time that the clock shows
// twice fires once, at the first of the two moments.
//
// Zones that change the clock at midnight, by half an hour or by two hours
// follow the same rules.
package gocron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule gives the times at which an expression fires.
type Schedule interface {
	// Next returns the first time after t at which the schedule fires, in
	// the location of t. It returns the zero time when the schedule does
	// not fire within five years after t, as with "0 0 30 2 *".
	Next(t time.Time) time.Time
}

// Parse reads an expression whose times are in the local time zone.
func Parse(spec string) (Schedule, error) {
	return ParseInLocation(spec, time.Local)
}

// MustParse is like [Parse] but panics when spec is not a valid expression.
// It simplifies initializing variables that hold schedules.
func MustParse(spec string) Schedule {
	s, err := Parse(spec)
	if err != nil {
		panic("gocron: MustParse(" + strconv.Quote(spec) + "): " + err.Error())
	}
	return s
}

// ParseInLocation reads an expression whose times are in loc. A
// "CRON_TZ=zone " or "TZ=zone " prefix on spec sets the time zone instead.
func ParseInLocation(spec string, loc *time.Location) (Schedule, error) {
	if loc == nil {
		return nil, errors.New("nil location")
	}
	spec = strings.TrimSpace(spec)
	if rest, ok := cutZonePrefix(spec); ok {
		zone, expr, _ := strings.Cut(rest, " ")
		if expr = strings.TrimSpace(expr); expr == "" {
			return nil, fmt.Errorf("no expression after the time zone in %q", spec)
		}
		l, err := time.LoadLocation(zone)
		if err != nil {
			return nil, fmt.Errorf("time zone %q: %w", zone, err)
		}
		loc, spec = l, expr
	}
	if strings.HasPrefix(spec, "@") {
		return parseDescriptor(spec, loc)
	}
	fields := strings.Fields(spec)
	switch len(fields) {
	case 5:
		fields = append([]string{"0"}, fields...)
	case 6:
	default:
		return nil, fmt.Errorf("expected 5 or 6 fields, found %d in %q", len(fields), spec)
	}
	s := &calendar{loc: loc}
	for i, f := range []struct {
		dst  *uint64
		star *bool
		r    fieldRange
	}{
		{&s.second, nil, secondRange},
		{&s.minute, nil, minuteRange},
		{&s.hour, nil, hourRange},
		{&s.dom, &s.domStar, domRange},
		{&s.month, nil, monthRange},
		{&s.dow, &s.dowStar, dowRange},
	} {
		bits, star, err := parseField(fields[i], f.r)
		if err != nil {
			return nil, err
		}
		*f.dst = bits
		if f.star != nil {
			*f.star = star
		}
	}
	return s, nil
}

func cutZonePrefix(spec string) (string, bool) {
	if rest, ok := strings.CutPrefix(spec, "CRON_TZ="); ok {
		return rest, true
	}
	return strings.CutPrefix(spec, "TZ=")
}

// fieldRange is the name and the allowed values of one field.
type fieldRange struct {
	name     string
	min, max int
	names    map[string]int
}

var (
	secondRange = fieldRange{name: "seconds", min: 0, max: 59}
	minuteRange = fieldRange{name: "minutes", min: 0, max: 59}
	hourRange   = fieldRange{name: "hours", min: 0, max: 23}
	domRange    = fieldRange{name: "day of month", min: 1, max: 31}
	monthRange  = fieldRange{name: "month", min: 1, max: 12, names: map[string]int{
		"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	}}
	dowRange = fieldRange{name: "day of week", min: 0, max: 6, names: map[string]int{
		"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
	}}
)

// parseField parses a comma-separated list of values, ranges and steps
// into a bit set. star reports a field that allows every value with "*"
// or "?"; the day fields need it to combine days of month and of week.
func parseField(expr string, r fieldRange) (bits uint64, star bool, err error) {
	for _, term := range strings.Split(expr, ",") {
		b, s, err := parseTerm(term, r)
		if err != nil {
			return 0, false, err
		}
		bits |= b
		star = star || s
	}
	return bits, star, nil
}

// parseTerm parses "*", "?", "N", "N-M", and any of them followed by
// "/step". "N/step" runs from N to the largest value of the field.
func parseTerm(term string, r fieldRange) (bits uint64, star bool, err error) {
	rangePart, stepPart, hasStep := strings.Cut(term, "/")
	lowPart, highPart, hasHigh := strings.Cut(rangePart, "-")
	var low, high int
	switch {
	case lowPart == "*" || lowPart == "?":
		if hasHigh {
			return 0, false, fmt.Errorf("%s: %q cannot have a range", r.name, term)
		}
		low, high, star = r.min, r.max, true
	default:
		if low, err = r.value(lowPart); err != nil {
			return 0, false, err
		}
		high = low
		if hasHigh {
			if high, err = r.value(highPart); err != nil {
				return 0, false, err
			}
		}
	}
	step := 1
	if hasStep {
		if step, err = strconv.Atoi(stepPart); err != nil || step <= 0 {
			return 0, false, fmt.Errorf("%s: %q has an invalid step", r.name, term)
		}
		if !hasHigh && !star {
			high = r.max
		}
		if step > 1 {
			star = false
		}
	}
	if low > high {
		return 0, false, fmt.Errorf("%s: %q starts after it ends", r.name, term)
	}
	for v := low; v <= high; v += step {
		bits |= 1 << uint(v)
	}
	return bits, star, nil
}

// value parses one number or name of the field.
func (r fieldRange) value(s string) (int, error) {
	if v, ok := r.names[strings.ToLower(s)]; ok {
		return v, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number", r.name, s)
	}
	if v < r.min || v > r.max {
		return 0, fmt.Errorf("%s: %d is outside %d-%d", r.name, v, r.min, r.max)
	}
	return v, nil
}

func parseDescriptor(spec string, loc *time.Location) (Schedule, error) {
	all := func(r fieldRange) uint64 {
		var b uint64
		for v := r.min; v <= r.max; v++ {
			b |= 1 << uint(v)
		}
		return b
	}
	s := &calendar{loc: loc, second: 1, minute: 1, hour: 1, dom: all(domRange), month: all(monthRange),
		dow: all(dowRange), domStar: true, dowStar: true}
	switch strings.ToLower(spec) {
	case "@yearly", "@annually":
		s.dom, s.month, s.domStar = 1<<1, 1<<1, false
	case "@monthly":
		s.dom, s.domStar = 1<<1, false
	case "@weekly":
		s.dow, s.dowStar = 1<<0, false
	case "@daily", "@midnight":
	case "@hourly":
		s.hour = all(hourRange)
	default:
		d, ok := strings.CutPrefix(spec, "@every ")
		if !ok {
			return nil, fmt.Errorf("unknown descriptor %q", spec)
		}
		delay, err := time.ParseDuration(strings.TrimSpace(d))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec, err)
		}
		return every{delay: max(delay.Truncate(time.Second), time.Second)}, nil
	}
	return s, nil
}

// every fires at a fixed delay after the time passed to Next, on whole
// seconds.
type every struct{ delay time.Duration }

func (e every) Next(t time.Time) time.Time {
	return t.Add(e.delay - time.Duration(t.Nanosecond()))
}
