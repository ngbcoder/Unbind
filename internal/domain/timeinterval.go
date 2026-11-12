package domain

import (
	"fmt"
	"time"
)

// TimeInterval is a mutable [Begin, End] range with a cached Duration.
//
// The clamping rules are inherited from the C# original (TimeInterval.cs) and
// are load-bearing throughout the engine:
//
//   - Setting Begin clamps End up to Begin.
//   - Setting End clamps Begin down to End.
//   - Duration is always recomputed as End-Begin on either assignment.
//   - Setting Duration resets Begin to zero and End to the new duration.
//
// The Duration setter's reset is what makes NewChapterDuration(d) mean
// "chapter starting at zero with this length" — the dominant convention in the
// engine.
type TimeInterval struct {
	begin    time.Duration
	end      time.Duration
	duration time.Duration
}

// NewTimeInterval returns an empty interval.
func NewTimeInterval() TimeInterval {
	return TimeInterval{}
}

// NewTimeIntervalDuration returns an interval spanning [0, d].
func NewTimeIntervalDuration(d time.Duration) TimeInterval {
	return TimeInterval{end: d, duration: d}
}

// NewTimeIntervalRange returns an interval spanning [begin, end]. The duration
// is only computed when end > begin, mirroring the C# constructor.
func NewTimeIntervalRange(begin, end time.Duration) TimeInterval {
	ti := TimeInterval{begin: begin, end: end}
	if end > begin {
		ti.duration = end - begin
	}
	return ti
}

// NewTimeIntervalCopy returns a copy of other, optionally displaced in time.
func NewTimeIntervalCopy(other TimeInterval, offset time.Duration) TimeInterval {
	ti := TimeInterval{begin: other.begin + offset, end: other.end + offset}
	if ti.end > ti.begin {
		ti.duration = ti.end - ti.begin
	}
	return ti
}

// Begin returns the start of the interval.
func (t *TimeInterval) Begin() time.Duration { return t.begin }

// SetBegin sets the start, clamping the end up and recomputing the duration.
func (t *TimeInterval) SetBegin(d time.Duration) {
	t.begin = d
	if t.begin > t.end {
		t.end = t.begin
	}
	t.duration = t.end - t.begin
}

// End returns the end of the interval.
func (t *TimeInterval) End() time.Duration { return t.end }

// SetEnd sets the end, clamping the begin down and recomputing the duration.
func (t *TimeInterval) SetEnd(d time.Duration) {
	t.end = d
	if t.end < t.begin {
		t.begin = t.end
	}
	t.duration = t.end - t.begin
}

// Duration returns the length of the interval.
func (t *TimeInterval) Duration() time.Duration { return t.duration }

// SetDuration sets the length, resetting the interval to [0, d].
func (t *TimeInterval) SetDuration(d time.Duration) {
	t.duration = d
	t.begin = 0
	t.end = d
}

// Shifted returns a copy of the interval displaced by offset.
func (t TimeInterval) Shifted(offset time.Duration) TimeInterval {
	return NewTimeIntervalRange(t.begin+offset, t.end+offset)
}

// String renders the interval in the "hh:mm:ss.mmm" form used by the C#
// original's ToStringHMSm.
func (t TimeInterval) String() string {
	return fmt.Sprintf("%s -> %s (%s)",
		FormatHMSm(t.begin), FormatHMSm(t.end), FormatHMSm(t.duration))
}

// FormatHMSm formats a duration as "hh:mm:ss.mmm". Negative durations keep
// their sign in front of the hours field.
func FormatHMSm(d time.Duration) string {
	sign := ""
	if d < 0 {
		sign = "-"
		d = -d
	}
	total := int64(d / time.Millisecond)
	ms := total % 1000
	total /= 1000
	s := total % 60
	total /= 60
	m := total % 60
	h := total / 60
	return fmt.Sprintf("%s%02d:%02d:%02d.%03d", sign, h, m, s, ms)
}
