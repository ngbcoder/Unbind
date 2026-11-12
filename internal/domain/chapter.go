package domain

import (
	"math"
	"time"

	"github.com/ngbcoder/Unbind/internal/logging"
)

// Chapter timing constants, inherited from Chapter.cs.
const (
	// MsMinChapterLength is the minimum chapter length in milliseconds.
	MsMinChapterLength = 1

	// TsMinLastChapterLength is the minimum length of the last chapter.
	TsMinLastChapterLength = time.Second

	tsEpsSilence       = 10 * time.Millisecond
	tsMaxLastSilence   = 2 * time.Second
	tsCatchRange       = 30 * time.Second
	tsPartTitleLength  = 10 * time.Second
	tsLongerSilence    = 900 * time.Millisecond
	tsShortSilence     = 500 * time.Millisecond
	tsSilenceAnte      = 300 * time.Millisecond
	tsMinChapterLength = time.Millisecond
)

// TimeAdjustments holds the pending delta for a chapter's Begin and End.
type TimeAdjustments struct {
	Begin time.Duration
	End   time.Duration
}

func (a *TimeAdjustments) String() string {
	return fmtAdjust(a.Begin, a.End)
}

func fmtAdjust(begin, end time.Duration) string {
	return "adj beg=" + FormatHMSm(begin) + ", end=" + FormatHMSm(end)
}

// Chapter is a named time range within a part.
type Chapter struct {
	// Time is the chapter's range.
	Time TimeInterval

	// Name is the chapter title. It is "(1)", "(2)", … when unnamed.
	Name string

	// Silences are the detected silence intervals, relative to the chapter start.
	Silences []TimeInterval

	// IsPaddingChapter marks a synthetic chapter covering a head or tail gap.
	IsPaddingChapter bool

	// TmpFileName is the chapter-scoped intermediate file.
	TmpFileName string

	// TimeAdjustment is the pending adjustment, nil when there is none.
	TimeAdjustment *TimeAdjustments
}

// NewChapter returns an empty chapter.
func NewChapter() *Chapter {
	return &Chapter{}
}

// NewChapterDuration returns a chapter spanning [0, d].
func NewChapterDuration(d time.Duration) *Chapter {
	ch := &Chapter{}
	ch.Time.SetDuration(d)
	return ch
}

// NewChapterRange returns a chapter spanning [begin, end].
func NewChapterRange(begin, end time.Duration) *Chapter {
	ch := &Chapter{}
	ch.Time.SetBegin(begin)
	ch.Time.SetEnd(end)
	return ch
}

// NewChapterCopy returns a copy of other, optionally carrying the silences.
func NewChapterCopy(other *Chapter, withSilences bool) *Chapter {
	ch := &Chapter{Name: other.Name}
	ch.Time.SetBegin(other.Time.Begin())
	ch.Time.SetEnd(other.Time.End())
	if withSilences {
		ch.Silences = append(ch.Silences, other.Silences...)
	}
	return ch
}

// CreateCueSheet splits the chapter into several tracks at natural pause
// points. It is the core of the "split into short tracks" feature.
//
// The chapter duration is divided into numTracks tracks of roughly equal
// length; each track boundary is then moved to the nearest suitable silence,
// landing in its midpoint so that half of the pause stays with the outgoing
// track and half with the incoming one.
func (ch *Chapter) CreateCueSheet(part *Part, trackDurMins int, splitTimeMode bool) {
	logging.Log(3, "Chapter", ch.String())

	// chapter duration in sec
	chLenSec := ch.Time.Duration().Seconds()

	// rounded number of tracks in chapter, observing desired track duration
	numTracks := int(math.Max(chLenSec/(float64(trackDurMins)*60)+0.5, 1))

	// average track length in sec
	aveTrackLen := chLenSec / float64(numTracks)

	tsAveTrackLen := time.Duration(aveTrackLen * float64(time.Second))

	// search range for silence at desired end of track is +/- 1/3 ave track length
	tsSearchRange := time.Duration(aveTrackLen / 3 * float64(time.Second))

	// start 1st track at zero, unless in time split mode where we start at the
	// actual beginning of the chapter
	tsStart := time.Duration(0)
	if splitTimeMode {
		tsStart = ch.Time.Begin()
	}

	// desired end of 1st track
	tsEnd := tsStart + tsAveTrackLen

	// max end is chapter duration unless in time split mode
	tsChEnd := ch.Time.Duration() - tsEpsSilence
	if splitTimeMode {
		tsChEnd = ch.Time.End()
	}

	// filter silences
	silences := make([]TimeInterval, 0, len(ch.Silences))
	for _, s := range ch.Silences {
		if s.Duration() >= tsShortSilence {
			silences = append(silences, s)
		}
	}

	for {
		if tsEnd < tsChEnd {
			tsUpper := tsEnd + tsSearchRange
			tsLower := tsEnd - tsSearchRange

			// find nearest silence above and below the desired end
			var silUp, silDn *TimeInterval
			for i := range silences {
				s := &silences[i]
				if s.Begin() >= tsEnd && s.Begin() < tsUpper {
					silUp = s
					break
				}
			}
			for i := len(silences) - 1; i >= 0; i-- {
				s := &silences[i]
				if s.End() > tsLower && s.End() <= tsEnd {
					silDn = s
					break
				}
			}

			// which silence shall it be
			var sil *TimeInterval
			if silUp != nil && silDn != nil {
				deltaUp := silUp.Begin() - tsEnd
				deltaDn := tsEnd - silDn.End()
				if deltaUp < deltaDn {
					sil = silUp
				} else {
					sil = silDn
				}
			} else if silUp != nil {
				sil = silUp
			} else {
				sil = silDn
			}

			if sil != nil {
				// actual track end: add half of the silence interval, the other
				// half goes into the next track. A cut in a stream is never exact.
				tsActualEnd := sil.Begin() + sil.Duration()/2

				// check for overshooting
				if tsActualEnd > tsChEnd {
					tsActualEnd = tsChEnd
				}

				tsDelta := tsActualEnd - tsEnd

				track := NewTrackRange(tsStart, tsActualEnd)
				track.Chapter = ch
				part.AddTrack(track)

				// set for next track
				tsStart = tsActualEnd
				tsEnd = tsStart + tsAveTrackLen - tsDelta
				if tsEnd+tsSearchRange > tsChEnd {
					tsEnd = tsChEnd
				}
			} else {
				// silence not found, extend track
				tsEnd += tsAveTrackLen
			}
		}

		// at the end of the chapter
		if tsEnd >= tsChEnd {
			track := NewTrackRange(tsStart, tsChEnd)
			track.Chapter = ch
			part.AddTrack(track)
			break
		}
	}
}

// DetermineTimeAdjustment finds the true chapter boundary by looking at where
// the silence actually is, and records a pending adjustment.
func (ch *Chapter) DetermineTimeAdjustment(nextChapter *Chapter) {
	if len(ch.Silences) == 0 {
		return
	}
	logging.Log(3, "Chapter", ch.String())

	if ch.handlePartTitle(nextChapter) {
		return
	}

	ch.handleChapterMark(nextChapter)
}

// setNewBgn records a new begin time for this chapter.
func (ch *Chapter) setNewBgn(begin time.Duration) {
	logging.Log(3, "Chapter", "named: "+ch.String()+", bgn time replaced with: "+FormatHMSm(begin))
	if ch.TimeAdjustment == nil {
		ch.TimeAdjustment = &TimeAdjustments{}
	}
	ch.TimeAdjustment.Begin = begin - ch.Time.Begin()
}

// SetNewEnd moves this chapter's end to the given time, and the next chapter's
// begin to the same time. Unless always is set, the new end is only accepted
// when it falls inside a detected silence.
//
// It returns the chapters that were affected.
func (ch *Chapter) SetNewEnd(end time.Duration, nextChapter *Chapter, always bool) []*Chapter {
	if !always {
		isSil := true
		if end < ch.Time.End() {
			if ch.Silences != nil {
				relEnd := end - ch.Time.Begin()
				isSil = false
				for _, s := range ch.Silences {
					if s.Begin() <= relEnd && s.End() >= relEnd {
						isSil = true
						break
					}
				}
			}
		} else if nextChapter != nil && nextChapter.Silences != nil {
			relBgn := end - nextChapter.Time.Begin()
			isSil = false
			for _, s := range nextChapter.Silences {
				if s.Begin() <= relBgn && s.End() >= relBgn {
					isSil = true
					break
				}
			}
		}
		if !isSil {
			return nil
		}
	}

	logging.Log(3, "Chapter", "named: "+ch.String()+", end time replaced with: "+FormatHMSm(end))
	if ch.TimeAdjustment == nil {
		ch.TimeAdjustment = &TimeAdjustments{}
	}
	ch.TimeAdjustment.End = end - ch.Time.End()

	chapters := []*Chapter{ch}
	if nextChapter != nil {
		nextChapter.setNewBgn(end)
		chapters = append(chapters, nextChapter)
	}
	return chapters
}

// AdjustTime applies the pending adjustment and migrates the affected silence
// intervals so that the silence lists stay consistent with the new boundaries.
//
// If the chapter becomes longer, silence at the beginning of the next chapter
// moves here; if it becomes shorter, silence at the end of this chapter moves
// to the next one. Partial silences spanning the boundary are split by simply
// duplicating them — a deliberate simplification that trades exactness for
// robustness.
func (ch *Chapter) AdjustTime(nextChapter *Chapter) {
	if ch.TimeAdjustment == nil {
		return
	}

	logging.Log(3, "Chapter", ch.String()+"; "+ch.TimeAdjustment.String())

	if nextChapter != nil && nextChapter.Silences != nil && ch.TimeAdjustment.End != 0 {
		adjEnd := ch.Time.Duration() + ch.TimeAdjustment.End
		if ch.TimeAdjustment.End > 0 {
			// need all silences in next with a begin earlier than adjustment
			for _, aff := range nextChapter.Silences {
				if aff.Begin() <= ch.TimeAdjustment.End {
					ch.Silences = append(ch.Silences, NewTimeIntervalCopy(aff, ch.Time.Duration()))
				}
			}
		} else {
			// need all silences in this with an end later than adjustment
			for _, aff := range ch.Silences {
				if aff.End() >= adjEnd {
					nextChapter.Silences = append(nextChapter.Silences, NewTimeIntervalCopy(aff, -ch.Time.Duration()))
				}
			}
			sortSilences(nextChapter.Silences)
		}
	}

	ch.Time.SetBegin(ch.Time.Begin() + ch.TimeAdjustment.Begin)
	ch.Time.SetEnd(ch.Time.End() + ch.TimeAdjustment.End)

	if len(ch.Silences) > 0 {
		var zeros []int
		if ch.TimeAdjustment.Begin != 0 {
			for i := range ch.Silences {
				sil := &ch.Silences[i]
				if ch.TimeAdjustment.Begin > 0 {
					sil.SetBegin(sil.Begin() - ch.TimeAdjustment.Begin)
					sil.SetEnd(sil.End() - ch.TimeAdjustment.Begin)
				} else {
					sil.SetEnd(sil.End() - ch.TimeAdjustment.Begin)
					sil.SetBegin(sil.Begin() - ch.TimeAdjustment.Begin)
				}

				if sil.Begin() < 0 {
					sil.SetBegin(0)
				}
				if sil.Duration() == 0 {
					zeros = append(zeros, i)
				}
			}

			for i := len(zeros) - 1; i >= 0; i-- {
				ch.Silences = append(ch.Silences[:zeros[i]], ch.Silences[zeros[i]+1:]...)
			}
		}
		if ch.TimeAdjustment.End != 0 {
			for len(ch.Silences) > 0 && ch.Silences[len(ch.Silences)-1].Begin() > ch.Time.Duration() {
				ch.Silences = ch.Silences[:len(ch.Silences)-1]
			}
			// The C# original calls Silences.Last() unguarded here, which throws
			// when the loop above removed every silence. Guard instead of crashing.
			if len(ch.Silences) > 0 {
				sil := &ch.Silences[len(ch.Silences)-1]
				if sil.End() > ch.Time.Duration() {
					sil.SetEnd(ch.Time.Duration())
				}
			}
		}
	}

	ch.TimeAdjustment = nil
	logging.Log(3, "Chapter", ch.String()+", adjusted")
}

// startsWithSilence reports whether the chapter begins with a silence.
func (ch *Chapter) startsWithSilence() bool {
	if len(ch.Silences) == 0 {
		return false
	}
	return ch.Silences[0].Begin() < tsEpsSilence
}

// endsWithSilence reports whether the chapter ends with a silence.
func (ch *Chapter) endsWithSilence(silence *TimeInterval) bool {
	if silence == nil {
		if len(ch.Silences) == 0 {
			return false
		}
		silence = &ch.Silences[len(ch.Silences)-1]
	}
	return silence.End() >= (ch.Time.Duration() - tsEpsSilence)
}

// forwardAdjustment pushes the start of this chapter later by adjust.
func (ch *Chapter) forwardAdjustment(adjust time.Duration) {
	if ch.TimeAdjustment == nil {
		ch.TimeAdjustment = &TimeAdjustments{}
	}
	ch.TimeAdjustment.Begin -= adjust
	logging.Log(3, "Chapter", ch.String()+"; "+ch.TimeAdjustment.String())
}

// handleChapterMark is the general case of the chapter-mark adjustment.
func (ch *Chapter) handleChapterMark(nextChapter *Chapter) {
	lastSilence := &ch.Silences[len(ch.Silences)-1]

	// last silence extends into next chapter?
	if ch.endsWithSilence(lastSilence) {
		nextStartsWithSilence := nextChapter != nil && nextChapter.startsWithSilence()

		// is this a long silence?
		if lastSilence.Duration() > tsMaxLastSilence {
			// but next chapter already starts with silence?
			if nextStartsWithSilence {
				return
			}
		} else {
			// If we end with a silence but the next chapter starts without one,
			// then let's split our silence in half.
			if !nextStartsWithSilence {
				ch.setAdjustment(nextChapter, lastSilence)
			}
			return
		}
	}

	// the next chapter may start with silence but it might not be the right one.
	if nextChapter != nil && nextChapter.startsWithSilence() {
		var sil *TimeInterval
		for i := len(ch.Silences) - 1; i >= 0; i-- {
			s := &ch.Silences[i]
			if s.End() > ch.Time.Duration()-tsPartTitleLength && s.Duration() > tsLongerSilence {
				sil = s
				break
			}
		}
		if sil == nil {
			return
		}
	}

	// this chapter does not end and next does not begin with silence, so we
	// look closer

	// only treat it, if close to the end
	if lastSilence.End() < ch.Time.Duration()-tsCatchRange {
		logging.Log(3, "Chapter", ch.String()+"; out of range: "+lastSilence.String())
		return
	}

	if nextChapter != nil {
		// alternatively merge nearest longer silences from next and this into tmp
		var silences []TimeInterval
		for _, s := range ch.Silences {
			if s.End() > ch.Time.Duration()-tsPartTitleLength && s.Duration() > tsLongerSilence {
				silences = append(silences, s)
			}
		}
		for _, s := range nextChapter.Silences {
			if s.Begin() < tsPartTitleLength && s.Duration() > tsLongerSilence {
				silences = append(silences, NewTimeIntervalCopy(s, ch.Time.Duration()))
			}
		}

		// pick the one closest to the chapter mark
		silence := closestToChapterMark(silences, ch.Time.Duration(), true)
		if silence != nil {
			lastSilence = silence
		}
	}

	ch.setAdjustment(nextChapter, lastSilence)
}

// handlePartTitle handles the "-- Part I -- Chapter 1 --" pattern, where a very
// short chapter carries the part title and the real chapter mark sits in the
// silence that follows.
func (ch *Chapter) handlePartTitle(next *Chapter) bool {
	// ignore PreChapter
	if ch.IsPaddingChapter {
		return false
	}

	// chapter must be very short to be analysed here and must have a longer successor
	if ch.Time.Duration() > tsPartTitleLength ||
		next == nil ||
		next.Time.Duration() < tsPartTitleLength ||
		len(next.Silences) == 0 {
		return false
	}

	// pattern:       "-- Part I [- Title] -- Chapter 1 [- Title] -- Text"
	// chapter mark:        >               <

	spanning := ch.endsWithSilence(nil) && next.startsWithSilence()

	// merge the silences from next and this into tmp
	var silences []TimeInterval
	for _, s := range ch.Silences {
		silences = append(silences, s)
	}
	for _, s := range next.Silences {
		if s.Begin() < tsPartTitleLength {
			silences = append(silences, NewTimeIntervalCopy(s, ch.Time.Duration()))
		}
	}

	if spanning {
		idx := len(ch.Silences)
		if idx >= 1 && idx < len(silences) {
			silences[idx-1].SetEnd(silences[idx-1].End() + silences[idx].Duration())
			silences = append(silences[:idx], silences[idx+1:]...)
		}
	}

	if ch.startsWithSilence() && len(silences) > 0 {
		silences = silences[1:]
	}

	filtered := silences[:0]
	for _, s := range silences {
		if s.Duration() > tsLongerSilence {
			filtered = append(filtered, s)
		}
	}
	silences = filtered

	silence := closestToChapterMark(silences, ch.Time.Duration(), false)
	if silence == nil {
		logging.Log(3, "Chapter", ch.String()+"; silence is <null>")
		return true
	}

	ch.setAdjustment(next, silence)
	return true
}

// closestToChapterMark picks the silence closest to the chapter mark. When
// preferThis is set, the longer of the two candidates wins instead.
func closestToChapterMark(silences []TimeInterval, chapterEnd time.Duration, preferThis bool) *TimeInterval {
	distThis := tsPartTitleLength
	distNext := tsPartTitleLength
	var silThis, silNext *TimeInterval

	for i := range silences {
		sil := &silences[i]
		if sil.End() < chapterEnd {
			dist := chapterEnd - sil.End()
			if dist > distThis {
				continue
			}
			distThis = dist
			silThis = sil
		} else {
			dist := sil.Begin() - chapterEnd
			if dist > distNext {
				continue
			}
			distNext = dist
			silNext = sil
		}
	}

	if silThis == nil {
		return silNext
	}
	if silNext == nil {
		return silThis
	}

	if preferThis {
		if silThis.Duration() > silNext.Duration() {
			return silThis
		}
		return silNext
	}
	if distThis < distNext {
		return silThis
	}
	return silNext
}

// setAdjustment turns the chosen silence into a time delta for this chapter
// and the next one.
func (ch *Chapter) setAdjustment(nextChapter *Chapter, silence *TimeInterval) {
	if silence == nil {
		return
	}

	if ch.TimeAdjustment == nil {
		ch.TimeAdjustment = &TimeAdjustments{}
	}

	newSilDuration := silence.Duration() - tsSilenceAnte
	if newSilDuration < 0 {
		newSilDuration = silence.Duration() / 3
	}

	newEnd := silence.Begin() + newSilDuration
	adjust := ch.Time.Duration() - newEnd
	ch.TimeAdjustment.End -= adjust

	logging.Log(3, "Chapter", ch.String()+"; "+ch.TimeAdjustment.String())
	if nextChapter != nil {
		nextChapter.forwardAdjustment(adjust)
	}
}

// sortSilences sorts silence intervals by their begin time.
func sortSilences(s []TimeInterval) {
	// insertion sort: the lists are short and nearly sorted already
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Begin() < s[j-1].Begin(); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func (ch *Chapter) String() string {
	if ch.IsPaddingChapter {
		return ch.Time.String() + ": <Padding>"
	}
	return ch.Time.String() + ": \"" + ch.Name + "\""
}
