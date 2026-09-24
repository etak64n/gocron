// Package gocron reads cron expressions and works out when they fire, in
// any time zone and on the days when daylight saving time starts or ends.
//
// gocron computes times only. It does not run jobs: a program calls
// [Schedule.Next] and waits for that time with its own timer.
// [Schedule.Prev] gives the last time before a given one.
//
// # Expressions
//
// An expression has five fields, separated by spaces:
//
//	┌───────────── minute (0-59)
//	│ ┌─────────── hour (0-23)
//	│ │ ┌───────── day of month (1-31)
//	│ │ │ ┌─────── month (1-12 or jan-dec)
//	│ │ │ │ ┌───── day of week (0-7 or sun-sat, where 0 and 7 are Sunday)
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
// Names of months and days of the week are not case-sensitive. A range of
// days of the week can end on Sunday, as in mon-sun or 5-7.
//
// # Days that move within the month
//
// The day of month also takes these terms:
//
//	L           the last day of the month
//	L-3         the third day before the last day
//	15W         the weekday nearest to the 15th
//	LW          the last weekday of the month
//
// A weekday is a day from Monday to Friday. "15W" fires on Friday the 14th
// when the 15th is a Saturday, and on Monday the 16th when it is a Sunday.
// W stays within the month: "1W" fires on Monday the 3rd when the 1st is a
// Saturday. A month without the given day, such as April for "31W", does
// not fire.
//
// The day of week also takes these terms:
//
//	5L          the last Friday of the month
//	fri#3       the third Friday of the month, from #1 to #5
//
// When both the day of month and the day of week are restricted, a day that
// matches either of them fires, as in the classic cron: "0 12 1 * sun"
// fires at noon on the 1st of every month and on every Sunday.
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
// whole seconds with a minimum of one second. Next returns the time one
// interval after the time passed to it, and Prev the time one interval
// before it, on whole seconds.
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
// The same rules hold for every change of a zone's UTC offset, including
// changes at midnight, by half an hour and by two hours.
//
// # Times that never come
//
// Next and Prev return the zero time when an expression never fires, as
// with "0 0 30 2 *". The Gregorian calendar repeats every 400 years, so
// they look 400 years ahead or back before they give up.
package gocron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule gives the times at which an expression fires. A Schedule is
// safe for concurrent use.
type Schedule interface {
	// Next returns the first time after t at which the schedule fires, in
	// the location of t. It returns the zero time when the schedule never
	// fires, as with "0 0 30 2 *".
	Next(t time.Time) time.Time
	// Prev returns the last time before t at which the schedule fires, in
	// the location of t, or the zero time when there is none.
	Prev(t time.Time) time.Time
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
	var err error
	if s.second, err = parseField(fields[0], secondRange); err != nil {
		return nil, err
	}
	if s.minute, err = parseField(fields[1], minuteRange); err != nil {
		return nil, err
	}
	if s.hour, err = parseField(fields[2], hourRange); err != nil {
		return nil, err
	}
	if err = s.parseDays(fields[3]); err != nil {
		return nil, err
	}
	if s.month, err = parseField(fields[4], monthRange); err != nil {
		return nil, err
	}
	if err = s.parseWeekdays(fields[5]); err != nil {
		return nil, err
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
	// every is the largest value that "*" and "N/step" reach. It is below
	// max only for the day of week, where 7 is another name for Sunday.
	every int
	names map[string]int
}

var (
	secondRange = fieldRange{name: "seconds", min: 0, max: 59, every: 59}
	minuteRange = fieldRange{name: "minutes", min: 0, max: 59, every: 59}
	hourRange   = fieldRange{name: "hours", min: 0, max: 23, every: 23}
	domRange    = fieldRange{name: "day of month", min: 1, max: 31, every: 31}
	monthRange  = fieldRange{name: "month", min: 1, max: 12, every: 12, names: map[string]int{
		"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	}}
	dowRange = fieldRange{name: "day of week", min: 0, max: 7, every: 6, names: map[string]int{
		"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
	}}
)

// parseField parses a comma-separated list of values, ranges and steps
// into a bit set.
func parseField(expr string, r fieldRange) (uint64, error) {
	var bits uint64
	for _, term := range strings.Split(expr, ",") {
		b, _, err := parseTerm(term, r)
		if err != nil {
			return 0, err
		}
		bits |= b
	}
	return bits, nil
}

// parseDays parses the day-of-month field. Besides the terms of every
// field, it takes L for the last day of the month, L-n for n days before
// it, nW for the weekday nearest to day n, and LW for the last weekday.
func (s *calendar) parseDays(expr string) error {
	for _, term := range strings.Split(expr, ",") {
		t := strings.ToUpper(term)
		switch {
		case t == "L":
			s.domLast |= 1
		case t == "LW":
			s.lastWeekday = true
		case strings.HasPrefix(t, "L-"):
			n, err := strconv.Atoi(t[2:])
			if err != nil || n < 0 || n > 30 {
				return fmt.Errorf("day of month: %q needs a number from 0 to 30 after L-", term)
			}
			s.domLast |= 1 << uint(n)
		case len(t) > 1 && t[len(t)-1] == 'W':
			d, err := strconv.Atoi(t[:len(t)-1])
			if err != nil {
				return fmt.Errorf("day of month: %q needs a single day before W", term)
			}
			if d < domRange.min || d > domRange.max {
				return fmt.Errorf("day of month: %d is outside 1-31", d)
			}
			s.domNear |= 1 << uint(d)
		default:
			b, star, err := parseTerm(term, domRange)
			if err != nil {
				return err
			}
			s.dom |= b
			s.domStar = s.domStar || star
		}
	}
	return nil
}

// parseWeekdays parses the day-of-week field. Besides the terms of every
// field, it takes dL for the last day d of the month, as in 5L for the
// last Friday, and d#n for the n-th day d, as in 1#1 for the first Monday.
func (s *calendar) parseWeekdays(expr string) error {
	for _, term := range strings.Split(expr, ",") {
		dayPart, nth, isNth := strings.Cut(term, "#")
		switch {
		case isNth:
			d, err := dowRange.value(dayPart)
			if err != nil {
				return err
			}
			n, err := strconv.Atoi(nth)
			if err != nil || n < 1 || n > 5 {
				return fmt.Errorf("day of week: %q needs #1 to #5", term)
			}
			s.dowNth[d%7] |= 1 << uint(n)
		case strings.EqualFold(term, "L"):
			return fmt.Errorf("day of week: %q needs a day before L, as in 5L", term)
		case len(term) > 1 && (term[len(term)-1] == 'L' || term[len(term)-1] == 'l'):
			d, err := dowRange.value(term[:len(term)-1])
			if err != nil {
				return err
			}
			s.dowLast |= 1 << uint(d%7)
		default:
			b, star, err := parseTerm(term, dowRange)
			if err != nil {
				return err
			}
			s.dow |= b
			s.dowStar = s.dowStar || star
		}
	}
	if s.dow&(1<<7) != 0 {
		s.dow = s.dow&^(1<<7) | 1 // 7 is Sunday
	}
	return nil
}

// parseTerm parses "*", "?", "N", "N-M", and any of them followed by
// "/step". "N/step" runs from N to the largest value of the field. star
// reports a term that allows every value; the day fields need it to
// combine days of month and of week.
func parseTerm(term string, r fieldRange) (bits uint64, star bool, err error) {
	rangePart, stepPart, hasStep := strings.Cut(term, "/")
	lowPart, highPart, hasHigh := strings.Cut(rangePart, "-")
	var low, high int
	switch {
	case lowPart == "*" || lowPart == "?":
		if hasHigh {
			return 0, false, fmt.Errorf("%s: %q cannot have a range", r.name, term)
		}
		low, high, star = r.min, r.every, true
	default:
		if low, err = r.value(lowPart); err != nil {
			return 0, false, err
		}
		high = low
		if hasHigh {
			if high, err = r.value(highPart); err != nil {
				return 0, false, err
			}
			// A range of days of the week can end on Sunday, as in mon-sun.
			if r.max > r.every && high == r.min && low > high {
				high = r.max
			}
		}
	}
	step := 1
	if hasStep {
		if step, err = strconv.Atoi(stepPart); err != nil || step <= 0 {
			return 0, false, fmt.Errorf("%s: %q has an invalid step", r.name, term)
		}
		if !hasHigh && !star {
			high = r.every
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
	s := &calendar{loc: loc, second: 1, minute: 1, hour: 1, dom: every(domRange), month: every(monthRange),
		dow: every(dowRange), domStar: true, dowStar: true}
	switch strings.ToLower(spec) {
	case "@yearly", "@annually":
		s.dom, s.month, s.domStar = 1<<1, 1<<1, false
	case "@monthly":
		s.dom, s.domStar = 1<<1, false
	case "@weekly":
		s.dow, s.dowStar = 1<<0, false
	case "@daily", "@midnight":
	case "@hourly":
		s.hour = every(hourRange)
	default:
		d, ok := strings.CutPrefix(spec, "@every ")
		if !ok {
			return nil, fmt.Errorf("unknown descriptor %q", spec)
		}
		delay, err := time.ParseDuration(strings.TrimSpace(d))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec, err)
		}
		return interval{delay: max(delay.Truncate(time.Second), time.Second)}, nil
	}
	return s, nil
}

// every returns the values that "*" allows in a field.
func every(r fieldRange) uint64 {
	var b uint64
	for v := r.min; v <= r.every; v++ {
		b |= 1 << uint(v)
	}
	return b
}

// interval fires at a fixed delay after or before the time passed to it,
// on whole seconds.
type interval struct{ delay time.Duration }

func (e interval) Next(t time.Time) time.Time {
	return t.Add(e.delay - time.Duration(t.Nanosecond()))
}

func (e interval) Prev(t time.Time) time.Time {
	return t.Add(-e.delay - time.Duration(t.Nanosecond()))
}
