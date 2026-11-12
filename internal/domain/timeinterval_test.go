package domain

import (
	"testing"
	"time"
)

func TestTimeIntervalBeginClampsEnd(t *testing.T) {
	ti := NewTimeIntervalRange(0, 100*time.Second)
	ti.SetBegin(200 * time.Second)

	if got := ti.End(); got != 200*time.Second {
		t.Errorf("End() = %v, want 200s", got)
	}
	if got := ti.Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0", got)
	}
}

func TestTimeIntervalEndClampsBegin(t *testing.T) {
	ti := NewTimeIntervalRange(100*time.Second, 200*time.Second)
	ti.SetEnd(50 * time.Second)

	if got := ti.Begin(); got != 50*time.Second {
		t.Errorf("Begin() = %v, want 50s", got)
	}
	if got := ti.Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0", got)
	}
}

func TestTimeIntervalDurationResets(t *testing.T) {
	ti := NewTimeIntervalRange(100*time.Second, 200*time.Second)
	ti.SetDuration(42 * time.Second)

	if got := ti.Begin(); got != 0 {
		t.Errorf("Begin() = %v, want 0", got)
	}
	if got := ti.End(); got != 42*time.Second {
		t.Errorf("End() = %v, want 42s", got)
	}
	if got := ti.Duration(); got != 42*time.Second {
		t.Errorf("Duration() = %v, want 42s", got)
	}
}

func TestTimeIntervalRangeDurationOnlyWhenEndAfterBegin(t *testing.T) {
	ti := NewTimeIntervalRange(200*time.Second, 100*time.Second)
	if got := ti.Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0 for an inverted range", got)
	}
}

func TestTimeIntervalDurationConstructor(t *testing.T) {
	ti := NewTimeIntervalDuration(90 * time.Second)
	if got := ti.Begin(); got != 0 {
		t.Errorf("Begin() = %v, want 0", got)
	}
	if got := ti.End(); got != 90*time.Second {
		t.Errorf("End() = %v, want 90s", got)
	}
	if got := ti.Duration(); got != 90*time.Second {
		t.Errorf("Duration() = %v, want 90s", got)
	}
}

func TestTimeIntervalShifted(t *testing.T) {
	ti := NewTimeIntervalRange(10*time.Second, 20*time.Second)
	sh := ti.Shifted(5 * time.Second)

	if got := sh.Begin(); got != 15*time.Second {
		t.Errorf("Shifted Begin() = %v, want 15s", got)
	}
	if got := sh.End(); got != 25*time.Second {
		t.Errorf("Shifted End() = %v, want 25s", got)
	}
	// the original must be untouched
	if got := ti.Begin(); got != 10*time.Second {
		t.Errorf("Begin() = %v, want 10s, the original was modified", got)
	}
}

func TestTimeIntervalCopyWithOffset(t *testing.T) {
	src := NewTimeIntervalRange(10*time.Second, 20*time.Second)
	cp := NewTimeIntervalCopy(src, -5*time.Second)

	if got := cp.Begin(); got != 5*time.Second {
		t.Errorf("Begin() = %v, want 5s", got)
	}
	if got := cp.End(); got != 15*time.Second {
		t.Errorf("End() = %v, want 15s", got)
	}
}

func TestFormatHMSm(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "00:00:00.000"},
		{time.Millisecond, "00:00:00.001"},
		{3661 * time.Second, "01:01:01.000"},
		{90 * time.Minute, "01:30:00.000"},
		{-time.Second, "-00:00:01.000"},
	}
	for _, tc := range tests {
		if got := FormatHMSm(tc.in); got != tc.want {
			t.Errorf("FormatHMSm(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
