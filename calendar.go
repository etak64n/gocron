package gocron

import (
	"math/bits"
	"time"
)

// calendar is a schedule of cron fields, as bit sets of allowed values.
type calendar struct {
	second, minute, hour, month uint64
	// dom holds the plain days of month (bits 1-31), domLast the days
	// counted back from the last one (bit n for L-n), and domNear the days
	// whose nearest weekday fires (bit n for nW). lastWeekday marks LW.
	dom, domLast, domNear uint64
	lastWeekday           bool
	// dow holds the plain days of week (bits 0-6, 0 is Sunday), dowLast the
	// days whose last occurrence in the month fires (dL), and dowNth[d]
	// the occurrences of day d that fire (bit n for d#n).
	dow, dowLast uint64
	dowNth       [7]uint8
	// domStar and dowStar mark day fields written as "*" or "?". When
	// both day fields are restricted, a day matches either of them, as in
	// the classic cron.
	domStar, dowStar bool
	loc              *time.Location
}

// searchMonths bounds the search. The Gregorian calendar repeats every
// 400 years, so a schedule that does not fire within them never fires.
const searchMonths = 400*12 + 2

// offsetBound bounds the distance of any UTC offset from UTC, including
// the local mean times of the 19th century.
const offsetBound = 16 * time.Hour

// Next returns the first time after t at which the schedule fires.
//
// A clock reading w after midnight comes at day+w-offset, with the UTC
// offset in effect then. The search walks the days that match and keeps
// the earliest moment at or after start. A reading that the clock skips
// can come on the next day, so the search starts a day early and goes on
// until no later day can come first.
func (s *calendar) Next(t time.Time) time.Time {
	start := t.Add(time.Second - time.Duration(t.Nanosecond()))
	y, m, d := start.In(s.loc).Date()
	y, m, d = time.Date(y, m, d-1, 0, 0, 0, 0, time.UTC).Date()
	var best time.Time
	for i := 0; i < searchMonths; i++ {
		if !best.IsZero() && !time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).Add(-offsetBound).Before(best) {
			break
		}
		days := s.days(y, m) >> uint(d) << uint(d)
		for days != 0 {
			dd := bits.TrailingZeros64(days)
			days &^= 1 << uint(dd)
			day := time.Date(y, m, dd, 0, 0, 0, 0, time.UTC)
			if !best.IsZero() && !day.Add(-offsetBound).Before(best) {
				return best.In(t.Location())
			}
			if c := s.firstOn(day, start); !c.IsZero() && (best.IsZero() || c.Before(best)) {
				best = c
			}
		}
		d = 1
		if m++; m > 12 {
			m, y = 1, y+1
		}
	}
	if best.IsZero() {
		return best
	}
	return best.In(t.Location())
}

// Prev returns the last time before t at which the schedule fires. It
// walks the days that match backward, as Next walks them forward.
func (s *calendar) Prev(t time.Time) time.Time {
	y, m, d := t.In(s.loc).Date()
	var best time.Time
	for i := 0; i < searchMonths; i++ {
		if !best.IsZero() && !time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC).Add(offsetBound).After(best) {
			break
		}
		days := s.days(y, m) & (1<<uint(d+1) - 1)
		for days != 0 {
			dd := 63 - bits.LeadingZeros64(days)
			days &^= 1 << uint(dd)
			day := time.Date(y, m, dd, 0, 0, 0, 0, time.UTC)
			if !best.IsZero() && !day.Add(24*time.Hour+offsetBound).After(best) {
				return best.In(t.Location())
			}
			if c := s.lastOn(day, t); !c.IsZero() && (best.IsZero() || c.After(best)) {
				best = c
			}
		}
		d = 31
		if m--; m < 1 {
			m, y = 12, y-1
		}
	}
	if best.IsZero() {
		return best
	}
	return best.In(t.Location())
}

// days returns the days of month m of year y on which the schedule fires,
// as bits 1-31.
func (s *calendar) days(y int, m time.Month) uint64 {
	if s.month&(1<<uint(m)) == 0 {
		return 0
	}
	last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	first := int(time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).Weekday())
	weekday := func(d int) int { return (first + d - 1) % 7 }

	dom := s.dom & (1<<uint(last+1) - 2)
	for n := 0; n < last; n++ {
		if s.domLast&(1<<uint(n)) != 0 {
			dom |= 1 << uint(last-n)
		}
	}
	for n := 1; n <= last; n++ {
		if s.domNear&(1<<uint(n)) != 0 {
			dom |= 1 << uint(nearestWeekday(n, last, weekday))
		}
	}
	if s.lastWeekday {
		dom |= 1 << uint(nearestWeekday(last, last, weekday))
	}

	var dow uint64
	for d := 1; d <= last; d++ {
		wd := weekday(d)
		if s.dow&(1<<uint(wd)) != 0 ||
			s.dowLast&(1<<uint(wd)) != 0 && d+7 > last ||
			s.dowNth[wd]&(1<<uint((d-1)/7+1)) != 0 {
			dow |= 1 << uint(d)
		}
	}
	if s.domStar || s.dowStar {
		return dom & dow
	}
	return dom | dow
}

// nearestWeekday returns the day from Monday to Friday nearest to day n of
// a month with last days, without leaving the month: a Saturday moves to
// the Friday before it, a Sunday to the Monday after it, a Saturday on the
// 1st to Monday the 3rd, and a Sunday on the last day to the Friday before.
func nearestWeekday(n, last int, weekday func(int) int) int {
	switch weekday(n) {
	case 6:
		if n == 1 {
			return 3
		}
		return n - 1
	case 0:
		if n == last {
			return n - 2
		}
		return n + 1
	}
	return n
}

// period is a stretch of time with one UTC offset. start and end are zero
// for a period without a start or an end.
type period struct {
	offset     time.Duration
	start, end time.Time
}

// periods returns, in order, the periods that cover the moments of the
// readings of day, given at midnight UTC. Those moments lie within
// offsetBound of the day, so the first period is taken to have no start
// and the last to have no end.
func (s *calendar) periods(day time.Time) []period {
	from, to := day.Add(-offsetBound), day.Add(24*time.Hour+offsetBound)
	_, off := from.In(s.loc).Zone()
	ps := []period{{offset: time.Duration(off) * time.Second}}
	for t := from; ; {
		end := s.nextChange(t, to)
		if end.IsZero() {
			return ps
		}
		_, off := end.In(s.loc).Zone()
		ps[len(ps)-1].end = end
		ps = append(ps, period{offset: time.Duration(off) * time.Second, start: end})
		t = end
	}
}

// nextChange returns the first moment after t and before to at which the
// UTC offset changes, or the zero time.
func (s *calendar) nextChange(t, to time.Time) time.Time {
	_, off := t.In(s.loc).Zone()
	for {
		_, end := t.In(s.loc).ZoneBounds()
		if !end.IsZero() && !end.After(t) {
			// After the last change listed in the time zone database, Go
			// computes changes from a rule and reports the end of each year
			// as the end of a zone, a day early in leap years, and there it
			// returns bounds that do not move forward. Probe hour by hour
			// instead.
			return s.probeChange(t, to, off)
		}
		if end.IsZero() || !end.Before(to) {
			return time.Time{}
		}
		if _, o := end.In(s.loc).Zone(); o != off {
			return end
		}
		t = end // the end of a year, where the offset stays
	}
}

// probeChange looks for the first moment after t at which the offset
// differs from off, one hour at a time until to. A change found in the
// last hour can lie a little after to, which does no harm to the periods.
func (s *calendar) probeChange(t, to time.Time, off int) time.Time {
	for a := t; a.Before(to); {
		b := a.Add(time.Hour)
		if _, o := b.In(s.loc).Zone(); o == off {
			a = b
			continue
		}
		for b.Sub(a) > time.Second {
			mid := a.Add(b.Sub(a) / 2).Truncate(time.Second)
			if _, o := mid.In(s.loc).Zone(); o == off {
				a = mid
			} else {
				b = mid
			}
		}
		return b
	}
	return time.Time{}
}

// moment returns the moment at which the clock shows the reading w of day.
// A reading that the clock shows twice comes at the first of them. A
// reading that the clock skips comes at the moment it would have come
// with the offset from before the change, which lies after the gap.
func moment(day time.Time, w time.Duration, ps []period) time.Time {
	for _, p := range ps {
		c := day.Add(w - p.offset)
		if (p.start.IsZero() || !c.Before(p.start)) && (p.end.IsZero() || c.Before(p.end)) {
			return c
		}
	}
	wall := day.Add(w)
	for i := 0; i+1 < len(ps); i++ {
		a, b := ps[i], ps[i+1]
		if b.offset > a.offset && !wall.Before(a.end.Add(a.offset)) && wall.Before(a.end.Add(b.offset)) {
			return day.Add(w - a.offset)
		}
	}
	return time.Time{}
}

// offsetRange returns the smallest and the largest offset of ps.
func offsetRange(ps []period) (lo, hi time.Duration) {
	lo, hi = ps[0].offset, ps[0].offset
	for _, p := range ps[1:] {
		lo, hi = min(lo, p.offset), max(hi, p.offset)
	}
	return lo, hi
}

// firstOn returns the first moment at or after start at which a reading of
// day comes, or the zero time. A reading w comes between day+w-hi and
// day+w-lo, which bounds the readings to look at.
func (s *calendar) firstOn(day, start time.Time) time.Time {
	ps := s.periods(day)
	lo, hi := offsetRange(ps)
	var best time.Time
	for w := s.nextReading(start.Sub(day) + lo); w >= 0; w = s.nextReading(w + time.Second) {
		if !best.IsZero() && !day.Add(w-hi).Before(best) {
			break
		}
		c := moment(day, w, ps)
		if c.IsZero() || c.Before(start) {
			continue
		}
		// With one offset, the readings come in order.
		if len(ps) == 1 {
			return c
		}
		if best.IsZero() || c.Before(best) {
			best = c
		}
	}
	return best
}

// lastOn returns the last moment before end at which a reading of day
// comes, or the zero time.
func (s *calendar) lastOn(day, end time.Time) time.Time {
	ps := s.periods(day)
	lo, hi := offsetRange(ps)
	var best time.Time
	for w := s.prevReading(end.Sub(day) + hi); w >= 0; w = s.prevReading(w) {
		if !best.IsZero() && !day.Add(w-lo).After(best) {
			break
		}
		c := moment(day, w, ps)
		if c.IsZero() || !c.Before(end) {
			continue
		}
		if len(ps) == 1 {
			return c
		}
		if best.IsZero() || c.After(best) {
			best = c
		}
	}
	return best
}

// nextReading returns the first reading of the schedule at or after w, as
// a time of day, or -1 when the day has none left.
func (s *calendar) nextReading(w time.Duration) time.Duration {
	w = (max(w, 0) + time.Second - 1).Truncate(time.Second)
	if w >= 24*time.Hour {
		return -1
	}
	h0, m0, s0 := int(w/time.Hour), int(w/time.Minute%60), int(w/time.Second%60)
	for h := nextBit(s.hour, h0); h >= 0; h = nextBit(s.hour, h+1) {
		mFrom := 0
		if h == h0 {
			mFrom = m0
		}
		for mi := nextBit(s.minute, mFrom); mi >= 0; mi = nextBit(s.minute, mi+1) {
			sFrom := 0
			if h == h0 && mi == m0 {
				sFrom = s0
			}
			if se := nextBit(s.second, sFrom); se >= 0 {
				return clockOf(h, mi, se)
			}
		}
	}
	return -1
}

// prevReading returns the last reading of the schedule before w, as a
// time of day, or -1 when the day has none before it.
func (s *calendar) prevReading(w time.Duration) time.Duration {
	w = (w + time.Second - 1).Truncate(time.Second) - time.Second
	if w < 0 {
		return -1
	}
	w = min(w, 24*time.Hour-time.Second)
	h0, m0, s0 := int(w/time.Hour), int(w/time.Minute%60), int(w/time.Second%60)
	for h := prevBit(s.hour, h0); h >= 0; h = prevBit(s.hour, h-1) {
		mFrom := 59
		if h == h0 {
			mFrom = m0
		}
		for mi := prevBit(s.minute, mFrom); mi >= 0; mi = prevBit(s.minute, mi-1) {
			sFrom := 59
			if h == h0 && mi == m0 {
				sFrom = s0
			}
			if se := prevBit(s.second, sFrom); se >= 0 {
				return clockOf(h, mi, se)
			}
		}
	}
	return -1
}

// nextBit returns the smallest set bit of b at or above i, or -1.
func nextBit(b uint64, i int) int {
	if b >>= uint(i); b == 0 {
		return -1
	}
	return i + bits.TrailingZeros64(b)
}

// prevBit returns the largest set bit of b at or below i, or -1.
func prevBit(b uint64, i int) int {
	if i < 0 {
		return -1
	}
	if i < 63 {
		b &= 1<<uint(i+1) - 1
	}
	if b == 0 {
		return -1
	}
	return 63 - bits.LeadingZeros64(b)
}

func clockOf(h, m, s int) time.Duration {
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
}
