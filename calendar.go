package gocron

import "time"

// calendar is a schedule of cron fields, as bit sets of allowed values.
type calendar struct {
	second, minute, hour, dom, month, dow uint64
	// domStar and dowStar mark day fields written as "*" or "?". When
	// both day fields are restricted, a day matches either of them, as in
	// the classic cron.
	domStar, dowStar bool
	loc              *time.Location
}

// maxSearchDays bounds the search for the next time to five years.
const maxSearchDays = 5*366 + 1

// Next returns the first scheduled time after t, or the zero time when
// the schedule has no time within five years.
func (s *calendar) Next(t time.Time) time.Time {
	local := t.In(s.loc)
	start := local.Add(time.Second - time.Duration(local.Nanosecond()))
	y, m, d := start.Date()
	for i := 0; i < maxSearchDays; i++ {
		// A date is counted at midnight UTC, where every day has 24 hours,
		// so a clock change at midnight cannot shift it.
		day := time.Date(y, m, d+i, 0, 0, 0, 0, time.UTC)
		if s.month&(1<<uint(day.Month())) == 0 || !s.dayMatches(day) {
			continue
		}
		if next, ok := s.firstOn(day, start); ok {
			return next.In(t.Location())
		}
	}
	return time.Time{}
}

func (s *calendar) dayMatches(day time.Time) bool {
	dom := s.dom&(1<<uint(day.Day())) != 0
	dow := s.dow&(1<<uint(day.Weekday())) != 0
	if s.domStar || s.dowStar {
		return dom && dow
	}
	return dom || dow
}

// firstOn returns the first scheduled time on the date of day, given at
// midnight UTC, that is not before start.
//
// A clock reading w after midnight comes at day+w-offset, with the UTC
// offset in effect then. Where daylight saving time starts, the clock
// skips some readings. A skipped reading runs at the moment it would have
// come with the offset from before the change, which the clock shows as a
// reading after the gap: 2:30 runs at 3:30 when the clock jumps from 2:00
// to 3:00. Where daylight saving time ends, the clock shows some readings
// twice, and such a reading runs at the first of them.
func (s *calendar) firstOn(day, start time.Time) (time.Time, bool) {
	lo, hi := s.offsets(day)
	// No reading before from comes at or after start, and a reading w
	// never comes before day+w-hi. With one offset in the day, readings
	// come in order, so the first one at or after start is the answer.
	from := max(start.Sub(day)+lo, 0)
	h0, m0, s0 := int(from/time.Hour), int(from/time.Minute%60), int(from/time.Second%60)
	var best time.Time
	for h := h0; h < 24; h++ {
		if s.hour&(1<<uint(h)) == 0 {
			continue
		}
		minFrom := 0
		if h == h0 {
			minFrom = m0
		}
		for mi := minFrom; mi < 60; mi++ {
			if s.minute&(1<<uint(mi)) == 0 {
				continue
			}
			secFrom := 0
			if h == h0 && mi == m0 {
				secFrom = s0
			}
			for se := secFrom; se < 60; se++ {
				if s.second&(1<<uint(se)) == 0 {
					continue
				}
				w := clockOf(h, mi, se)
				if !best.IsZero() && !day.Add(w-hi).Before(best) {
					return best, true
				}
				c, ok := s.moment(day, w, lo, hi)
				if !ok || c.Before(start) {
					continue
				}
				if lo == hi {
					return c, true
				}
				if best.IsZero() || c.Before(best) {
					best = c
				}
			}
		}
	}
	return best, !best.IsZero()
}

// moment returns the moment at which the clock on the date of day shows
// the reading w, given the smallest and the largest UTC offset of the day.
// It returns false when a skipped reading would come on another date.
func (s *calendar) moment(day time.Time, w, lo, hi time.Duration) (time.Time, bool) {
	// A reading that the clock shows twice comes first with the larger
	// offset.
	for _, off := range [2]time.Duration{hi, lo} {
		c := day.Add(w - off)
		if _, o := c.In(s.loc).Zone(); time.Duration(o)*time.Second == off {
			return c, true
		}
	}
	// The clock skips the reading. The offset from before the change is
	// the smaller one, and with it the reading comes after the gap.
	c := day.Add(w - lo)
	cy, cm, cd := c.In(s.loc).Date()
	y, m, d := day.Date()
	return c, cy == y && cm == m && cd == d
}

// offsets returns the smallest and the largest UTC offset in effect on the
// date of day. UTC offsets lie between -12h and +14h, so the date lies
// within day-14h and day+36h. A change on a neighboring date can widen the
// range, which costs only a longer search.
func (s *calendar) offsets(day time.Time) (lo, hi time.Duration) {
	_, a := day.Add(-14 * time.Hour).In(s.loc).Zone()
	_, b := day.Add(36 * time.Hour).In(s.loc).Zone()
	lo, hi = time.Duration(a)*time.Second, time.Duration(b)*time.Second
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func clockOf(h, m, s int) time.Duration {
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
}
