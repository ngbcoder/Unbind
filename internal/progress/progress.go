// Package progress carries the progress information from the conversion
// engine to the user interface.
//
// The protocol is inherited from the C# original (ProgressMessage.cs). The
// important detail is that the step count may be *negative*: the number of
// tracks is only known after the silence detection and the cue sheeting, so
// the difference against the initial estimate can be negative.
package progress

// Phase is a coarse stage of the pipeline.
type Phase int

// The pipeline phases.
const (
	PhaseNone Phase = iota
	PhaseSilence
	PhaseAdjust
	PhaseTranscoding
	PhaseCopying
)

var phaseNames = [...]string{"none", "silence", "adjust", "transcoding", "copying"}

// String returns the C# name of the phase.
func (p Phase) String() string {
	if int(p) < 0 || int(p) >= len(phaseNames) {
		return "none"
	}
	return phaseNames[p]
}

// Info is the semantic state of the run: which book, part, chapter and track
// are being worked on, separate from the numeric progress.
type Info struct {
	// Book is the title of the book.
	Book string

	// Part is the part number, if any.
	Part *uint

	// Chapter is the chapter number, if any.
	Chapter *uint

	// Track is the track number, if any.
	Track *uint

	// Chapters is the chapter count of the book.
	Chapters *uint

	// Tracks is the track count of the book.
	Tracks *uint

	// Phase is the current phase.
	Phase Phase

	// Cancel asks the interface to remove this item from its display.
	Cancel bool
}

// Message is one progress report.
type Message struct {
	// Reset asks the interface to reset everything.
	Reset bool

	// ResetSteps asks the interface to re-plan the step count.
	ResetSteps bool

	// AddTotalWeightedPhases extends the phase bar's maximum.
	AddTotalWeightedPhases *uint

	// IncWeightedPhases advances the phase bar.
	IncWeightedPhases *uint

	// AddTotalSteps extends the step bar's maximum. It may be negative.
	AddTotalSteps *int

	// IncSteps advances the step bar by whole steps.
	IncSteps *uint

	// IncStepsPerMille advances the step bar by a fraction of a step.
	IncStepsPerMille *uint

	// Info is the human-readable status.
	Info *Info
}

// Uint returns a pointer to the value.
func Uint(v uint) *uint { return &v }

// Int returns a pointer to the value.
func Int(v int) *int { return &v }

// Accumulator is the two-level progress bar accumulator.
//
// A segment is the value within the current plan. The accumulator carries the
// value of the finished segments. The bar shows the ceiling of the sum,
// because a bar must reach its maximum exactly.
type Accumulator struct {
	// M is the scale of the accumulator. The step bar uses 1000, so that a
	// step can be divided into per-mille increments.
	M uint

	accuValue uint
	value     uint
}

// NewAccumulator returns an accumulator with the given scale.
func NewAccumulator(m uint) *Accumulator {
	return &Accumulator{M: m}
}

// Value returns the current segment value.
func (a *Accumulator) Value() uint { return a.value }

// AccuValue returns the accumulated value, the ceiling of the sum.
func (a *Accumulator) AccuValue() uint {
	if a.M == 0 {
		return a.accuValue + a.value
	}
	sum := a.accuValue + a.value
	return (sum + a.M - 1) / a.M
}

// Set sets the current segment value.
func (a *Accumulator) Set(v uint) { a.value = v }

// Inc advances the current segment value.
func (a *Accumulator) Inc(v uint) { a.value += v }

// Reset rolls the current segment into the accumulator and starts a new
// segment.
//
// This is what "the plan changed, but the work done so far still counts"
// means. It is used when the step count is re-planned.
func (a *Accumulator) Reset() {
	a.accuValue += a.value
	a.value = 0
}

// ResetAll discards everything.
func (a *Accumulator) ResetAll() {
	a.accuValue = 0
	a.value = 0
}

// ThreadProgress converts a continuous ratio into per-mille increments.
//
// It remembers the last reported step and only emits the difference, so that
// a long-running operation does not flood the interface. On close it flushes
// the remainder, so that the bar always reaches exactly 100 %.
type ThreadProgress struct {
	report func(Message)
	mille  uint
	last   uint
	done   bool
}

// NewThreadProgress returns a progress reporter for one step.
//
// mille is the number of per-mille increments in the step.
func NewThreadProgress(mille uint, report func(Message)) *ThreadProgress {
	if report == nil {
		report = func(Message) {}
	}
	return &ThreadProgress{report: report, mille: mille}
}

// Report converts the ratio, which must be between 0 and 1, into an increment.
func (p *ThreadProgress) Report(ratio float64) {
	if p.done {
		return
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	current := uint(ratio * float64(p.mille))
	if current < p.last {
		current = p.last
	}
	delta := current - p.last
	if delta == 0 {
		return
	}
	p.last = current
	p.report(Message{IncStepsPerMille: Uint(delta)})
}

// Close flushes the remainder, so that the step reaches exactly 100 %.
func (p *ThreadProgress) Close() {
	if p.done {
		return
	}
	p.done = true
	if p.last < p.mille {
		p.report(Message{IncStepsPerMille: Uint(p.mille - p.last)})
	}
	p.last = p.mille
}
