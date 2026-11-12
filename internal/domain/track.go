package domain

import (
	"fmt"
	"time"
)

// TrackState is the lifecycle state of an output file.
type TrackState int

// Track states.
const (
	TrackInitial TrackState = iota
	TrackCurrent
	TrackComplete
	TrackAborted
)

// Track is one output audio file.
type Track struct {
	// Time is the absolute range within the source, used for FFmpeg's -ss/-to.
	Time TimeInterval

	// FileName is the absolute output path.
	FileName string

	// Title is the tag title.
	Title string

	// State is the lifecycle state.
	State TrackState

	// Chapter is the owning chapter. It is nil in single mode.
	Chapter *Chapter

	// MetaChapters are the chapter markers to embed inside the output file.
	MetaChapters []*Chapter
}

// minTrackDuration is the 1 ms floor inherited from Chapter.MS_MIN_CHAPTER_LENGTH.
const minTrackDuration = time.Millisecond

// NewTrackDuration returns a track spanning [0, d].
func NewTrackDuration(d time.Duration) *Track {
	return &Track{Time: NewTimeIntervalDuration(d)}
}

// NewTrackRange returns a track spanning [begin, end].
func NewTrackRange(begin, end time.Duration) *Track {
	return &Track{Time: NewTimeIntervalRange(begin, end)}
}

// HasMinDuration reports whether the track is long enough to be written.
func (t *Track) HasMinDuration() bool {
	return t.Time.Duration() >= minTrackDuration
}

// MatchesMinDuration reports whether the given interval is long enough.
func MatchesMinDuration(ti *TimeInterval) bool {
	if ti == nil {
		return true
	}
	return ti.Duration() >= minTrackDuration
}

func (t *Track) String() string {
	if t.Chapter == nil {
		return t.Time.String()
	}
	return fmt.Sprintf("%s, Ch: %s", t.Time.String(), t.Chapter.String())
}
