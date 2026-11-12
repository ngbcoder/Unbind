package domain

import (
	"testing"
	"time"
)

// silence builds a silence interval.
func silence(begin, end time.Duration) TimeInterval {
	return NewTimeIntervalRange(begin, end)
}

// newTestPart returns a part with one chapter, ready for a cue sheet.
func newTestPart(chapter *Chapter) *Part {
	b := NewBook("Test")
	p := NewPart(b, &AaxFileItem{}, 0)
	p.Chapters = []*Chapter{chapter}
	b.Parts = []*Part{p}
	return p
}

func TestCreateCueSheetNoSilences(t *testing.T) {
	// A ten-minute chapter with a five-minute target and no silence at all.
	// The algorithm extends the track until it reaches the chapter end.
	ch := NewChapterDuration(600 * time.Second)
	part := newTestPart(ch)

	ch.CreateCueSheet(part, 5, false)

	if len(part.Tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(part.Tracks))
	}
	tr := part.Tracks[0]
	if got := tr.Time.Begin(); got != 0 {
		t.Errorf("track begin = %v, want 0", got)
	}
	// the chapter end minus the 10 ms epsilon
	if want := 600*time.Second - 10*time.Millisecond; tr.Time.End() != want {
		t.Errorf("track end = %v, want %v", tr.Time.End(), want)
	}
}

func TestCreateCueSheetSplitsAtSilenceMidpoint(t *testing.T) {
	// A ten-minute chapter with a two-second silence at 295s. The track
	// boundary lands in the middle of the silence.
	ch := NewChapterDuration(600 * time.Second)
	ch.Silences = []TimeInterval{silence(295*time.Second, 297*time.Second)}
	part := newTestPart(ch)

	ch.CreateCueSheet(part, 5, false)

	if len(part.Tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(part.Tracks))
	}
	if got := part.Tracks[0].Time.End(); got != 296*time.Second {
		t.Errorf("first track end = %v, want 296s (the silence midpoint)", got)
	}
	if got := part.Tracks[1].Time.Begin(); got != 296*time.Second {
		t.Errorf("second track begin = %v, want 296s", got)
	}
}

func TestCreateCueSheetSplitTimeMode(t *testing.T) {
	// In the split-time mode the chapter starts at its real begin, not at zero.
	ch := NewChapterRange(100*time.Second, 700*time.Second)
	part := newTestPart(ch)

	ch.CreateCueSheet(part, 5, true)

	if len(part.Tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(part.Tracks))
	}
	if got := part.Tracks[0].Time.Begin(); got != 100*time.Second {
		t.Errorf("track begin = %v, want 100s", got)
	}
	if got := part.Tracks[0].Time.End(); got != 700*time.Second {
		t.Errorf("track end = %v, want 700s", got)
	}
}

func TestCreateCueSheetIgnoresShortSilences(t *testing.T) {
	// A silence shorter than 500 ms must not become a track boundary.
	ch := NewChapterDuration(600 * time.Second)
	ch.Silences = []TimeInterval{silence(295*time.Second, 295*time.Second+200*time.Millisecond)}
	part := newTestPart(ch)

	ch.CreateCueSheet(part, 5, false)

	if len(part.Tracks) != 1 {
		t.Fatalf("got %d tracks, want 1, the short silence must be ignored", len(part.Tracks))
	}
}

func TestDetermineTimeAdjustmentShortTrailingSilence(t *testing.T) {
	// A trailing silence that does not reach the chapter end, and a next
	// chapter that does not start with silence: the general path looks for the
	// silence closest to the chapter mark.
	ch := NewChapterDuration(300 * time.Second)
	ch.Silences = []TimeInterval{silence(297*time.Second, 299*time.Second)}
	next := NewChapterDuration(300 * time.Second)
	next.Silences = []TimeInterval{silence(500*time.Millisecond, 1500*time.Millisecond)}

	ch.DetermineTimeAdjustment(next)

	if ch.TimeAdjustment == nil {
		t.Fatal("no adjustment was recorded")
	}
	// new end = silence begin + (duration - 300 ms ante) = 297 + 1.7 = 298.7
	if want := -1300 * time.Millisecond; ch.TimeAdjustment.End != want {
		t.Errorf("TimeAdjustment.End = %v, want %v", ch.TimeAdjustment.End, want)
	}
	if next.TimeAdjustment == nil {
		t.Fatal("the next chapter got no forward adjustment")
	}
	if want := -1300 * time.Millisecond; next.TimeAdjustment.Begin != want {
		t.Errorf("next TimeAdjustment.Begin = %v, want %v", next.TimeAdjustment.Begin, want)
	}
}

func TestDetermineTimeAdjustmentNextStartsWithSilence(t *testing.T) {
	// A long trailing silence that reaches the chapter end, followed by a
	// leading silence in the next chapter: a genuine pause, no adjustment.
	ch := NewChapterDuration(300 * time.Second)
	ch.Silences = []TimeInterval{silence(296*time.Second, 300500*time.Millisecond)}
	next := NewChapterDuration(300 * time.Second)
	next.Silences = []TimeInterval{silence(0, time.Second)}

	ch.DetermineTimeAdjustment(next)

	if ch.TimeAdjustment != nil {
		t.Errorf("an adjustment was recorded (%v), want none", ch.TimeAdjustment)
	}
}

func TestDetermineTimeAdjustmentShortTrailingSilenceIsSplit(t *testing.T) {
	// A short trailing silence that reaches the chapter end, with a next
	// chapter that does not start with silence: the silence is split in half,
	// so the chapter ends at silence begin + duration - 300 ms.
	ch := NewChapterDuration(300 * time.Second)
	ch.Silences = []TimeInterval{silence(298*time.Second, 300*time.Second)}
	next := NewChapterDuration(300 * time.Second)

	ch.DetermineTimeAdjustment(next)

	if ch.TimeAdjustment == nil {
		t.Fatal("no adjustment was recorded")
	}
	// new end = 298 + (2 - 0.3) = 299.7, adjust = 300 - 299.7 = 0.3
	if want := -300 * time.Millisecond; ch.TimeAdjustment.End != want {
		t.Errorf("TimeAdjustment.End = %v, want %v", ch.TimeAdjustment.End, want)
	}
}

func TestDetermineTimeAdjustmentNoSilences(t *testing.T) {
	ch := NewChapterDuration(300 * time.Second)
	next := NewChapterDuration(300 * time.Second)

	ch.DetermineTimeAdjustment(next)

	if ch.TimeAdjustment != nil {
		t.Errorf("an adjustment was recorded (%v), want none", ch.TimeAdjustment)
	}
}

func TestAdjustTimeMovesTheEndEarlier(t *testing.T) {
	ch := NewChapterDuration(300 * time.Second)
	ch.Silences = []TimeInterval{silence(297*time.Second, 299*time.Second)}
	next := NewChapterDuration(300 * time.Second)
	next.Silences = []TimeInterval{silence(500*time.Millisecond, 1500*time.Millisecond)}

	ch.DetermineTimeAdjustment(next)
	ch.AdjustTime(next)

	if want := 298700 * time.Millisecond; ch.Time.End() != want {
		t.Errorf("chapter end = %v, want %v", ch.Time.End(), want)
	}
	if ch.TimeAdjustment != nil {
		t.Error("the adjustment was not cleared")
	}
	// the trailing silence must be clipped to the new end
	if got := ch.Silences[len(ch.Silences)-1].End(); got != 298700*time.Millisecond {
		t.Errorf("last silence end = %v, want 298.7s", got)
	}
}

func TestAdjustTimeMigratesSilencesToTheNextChapter(t *testing.T) {
	// The chapter becomes longer, so silence at the start of the next chapter
	// moves here.
	ch := NewChapterDuration(300 * time.Second)
	next := NewChapterDuration(300 * time.Second)
	next.Silences = []TimeInterval{silence(0, 2*time.Second)}

	ch.TimeAdjustment = &TimeAdjustments{End: 2 * time.Second}
	ch.AdjustTime(next)

	if len(ch.Silences) != 1 {
		t.Fatalf("got %d silences in the chapter, want 1", len(ch.Silences))
	}
	// the migrated silence is shifted by the chapter duration
	if got := ch.Silences[0].Begin(); got != 300*time.Second {
		t.Errorf("migrated silence begin = %v, want 300s", got)
	}
}

func TestSetNewEndRejectsOutsideSilence(t *testing.T) {
	ch := NewChapterDuration(300 * time.Second)
	ch.Silences = []TimeInterval{silence(100*time.Second, 110*time.Second)}

	// 200s is not inside any silence
	if got := ch.SetNewEnd(200*time.Second, nil, false); got != nil {
		t.Errorf("SetNewEnd returned %v, want nil", got)
	}
	// 105s is inside the silence
	if got := ch.SetNewEnd(105*time.Second, nil, false); got == nil {
		t.Error("SetNewEnd returned nil, want the chapter")
	}
	// "always" accepts anything
	if got := ch.SetNewEnd(200*time.Second, nil, true); got == nil {
		t.Error("SetNewEnd with always=true returned nil")
	}
}

func TestComplementChapterNames(t *testing.T) {
	ch1 := NewChapterDuration(time.Second)
	ch1.Name = ""
	ch2 := NewChapterDuration(time.Second)
	ch2.Name = "   "
	ch3 := NewChapterDuration(time.Second)
	ch3.Name = "Real"

	b := NewBook("Test")
	p := NewPart(b, &AaxFileItem{}, 0)
	p.Chapters = []*Chapter{ch1, ch2, ch3}

	p.ComplementChapterNames()

	if ch1.Name != "(1)" {
		t.Errorf("chapter 1 name = %q, want %q", ch1.Name, "(1)")
	}
	if ch2.Name != "(2)" {
		t.Errorf("chapter 2 name = %q, want %q", ch2.Name, "(2)")
	}
	if ch3.Name != "Real" {
		t.Errorf("chapter 3 name = %q, want %q", ch3.Name, "Real")
	}
}

func TestSwapChapters(t *testing.T) {
	b := NewBook("Test")
	p := NewPart(b, &AaxFileItem{}, 0)
	embedded := NewChapterDuration(time.Second)
	named := NewChapterDuration(2 * time.Second)
	p.Chapters = []*Chapter{embedded}
	p.Chapters2 = []*Chapter{named}

	if p.HasNamedChapters() != true {
		t.Fatal("HasNamedChapters() = false, want true")
	}
	if p.NamedChapters()[0] != named {
		t.Error("NamedChapters() does not return the named list")
	}
	if p.EmbeddedChapters()[0] != embedded {
		t.Error("EmbeddedChapters() does not return the embedded list")
	}

	p.SwapChapters()

	if p.NamedChapters()[0] != named {
		t.Error("after the swap NamedChapters() does not return the named list")
	}
	if p.EmbeddedChapters()[0] != embedded {
		t.Error("after the swap EmbeddedChapters() does not return the embedded list")
	}
}

func TestInsertAndRemovePaddingChapters(t *testing.T) {
	b := NewBook("Test")
	p := NewPart(b, &AaxFileItem{}, 0)
	p.Duration = 300 * time.Second
	ch := NewChapterRange(10*time.Second, 290*time.Second)
	p.Chapters = []*Chapter{ch}

	added := p.InsertPaddingChapters()
	if added != 2 {
		t.Fatalf("InsertPaddingChapters() = %d, want 2", added)
	}
	if len(p.Chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(p.Chapters))
	}
	if !p.Chapters[0].IsPaddingChapter || !p.Chapters[2].IsPaddingChapter {
		t.Error("the padding chapters are not flagged")
	}
	if p.Chapters[0].Time.End() != 10*time.Second {
		t.Errorf("the head padding ends at %v, want 10s", p.Chapters[0].Time.End())
	}

	p.RemovePaddingChapters()
	if len(p.Chapters) != 1 {
		t.Fatalf("got %d chapters after the removal, want 1", len(p.Chapters))
	}
}
