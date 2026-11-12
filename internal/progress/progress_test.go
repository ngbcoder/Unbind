package progress

import "testing"

func TestAccumulatorRollsTheSegmentOnReset(t *testing.T) {
	a := NewAccumulator(1000)

	a.Set(400)
	if got := a.AccuValue(); got != 1 {
		t.Errorf("AccuValue() = %d, want 1 (the ceiling of 400/1000)", got)
	}

	// The plan changed: the work done so far still counts.
	a.Reset()
	if got := a.Value(); got != 0 {
		t.Errorf("Value() = %d, want 0 after a reset", got)
	}
	a.Set(600)
	if got := a.AccuValue(); got != 1 {
		t.Errorf("AccuValue() = %d, want 1 (the ceiling of 1000/1000)", got)
	}
}

func TestAccumulatorResetAll(t *testing.T) {
	a := NewAccumulator(1000)
	a.Set(900)
	a.ResetAll()
	if got := a.AccuValue(); got != 0 {
		t.Errorf("AccuValue() = %d, want 0", got)
	}
}

func TestThreadProgressReportsDeltas(t *testing.T) {
	var total uint
	p := NewThreadProgress(1000, func(m Message) {
		if m.IncStepsPerMille != nil {
			total += *m.IncStepsPerMille
		}
	})

	p.Report(0.25)
	if total != 250 {
		t.Errorf("total = %d, want 250", total)
	}

	// The same ratio again must not report anything.
	p.Report(0.25)
	if total != 250 {
		t.Errorf("total = %d, want 250, the delta must not repeat", total)
	}

	p.Report(0.5)
	if total != 500 {
		t.Errorf("total = %d, want 500", total)
	}
}

func TestThreadProgressFlushesTheRemainder(t *testing.T) {
	var total uint
	p := NewThreadProgress(1000, func(m Message) {
		if m.IncStepsPerMille != nil {
			total += *m.IncStepsPerMille
		}
	})

	p.Report(0.3)
	p.Close()
	if total != 1000 {
		t.Errorf("total = %d, want 1000, the step must reach exactly 100 percent", total)
	}

	// Closing twice must not report again.
	p.Close()
	if total != 1000 {
		t.Errorf("total = %d, want 1000", total)
	}
}

func TestThreadProgressClampsTheRatio(t *testing.T) {
	var total uint
	p := NewThreadProgress(1000, func(m Message) {
		if m.IncStepsPerMille != nil {
			total += *m.IncStepsPerMille
		}
	})

	p.Report(-1)
	p.Report(2)
	p.Close()

	if total != 1000 {
		t.Errorf("total = %d, want 1000", total)
	}
}

func TestPhaseNames(t *testing.T) {
	names := map[Phase]string{
		PhaseNone:        "none",
		PhaseSilence:     "silence",
		PhaseAdjust:      "adjust",
		PhaseTranscoding: "transcoding",
		PhaseCopying:     "copying",
	}
	for p, want := range names {
		if got := p.String(); got != want {
			t.Errorf("Phase(%d).String() = %q, want %q", int(p), got, want)
		}
	}
}
