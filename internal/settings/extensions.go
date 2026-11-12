package settings

import "regexp"

// bitRatePattern extracts the numeric part of a reduced bit rate name.
var bitRatePattern = regexp.MustCompile(`(\d+)`)

// UInt32 returns the bit rate in kbit/s, or 0 for "off".
func (v ReducedBitRate) UInt32() uint {
	m := bitRatePattern.FindStringSubmatch(v.String())
	if m == nil {
		return 0
	}
	var n uint
	for _, c := range m[1] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + uint(c-'0')
	}
	return n
}

// TrackDuration returns the permitted track length range in minutes for the
// given mode, and the clamped value.
func TrackDuration(value byte, mode ConvMode) (min, max, val int) {
	maxTrackDurMins := TrackDurationMaxSplitChapter
	if mode == ConvModeSplitTime {
		maxTrackDurMins = TrackDurationMaxTimeSplit
	}
	min = TrackDurationMin
	max = maxTrackDurMins
	val = min
	if int(value) < min {
		val = min
	} else if int(value) > max {
		val = max
	} else {
		val = int(value)
	}
	return min, max, val
}

// ApplicableBitRate returns the bit rate to apply and the requested one.
//
// An applicable bit rate of 0 means "no change". The C# original returns the
// same pair (Extensions.cs:46-56).
func (s *Settings) ApplicableBitRate(sourceBitrate uint) (applicable, requested uint) {
	settingsBitrate := s.ReducedBitRate.UInt32()
	if (settingsBitrate == 0 || settingsBitrate >= sourceBitrate) && !s.VariableBitRate {
		return 0, 0
	}

	applicable = sourceBitrate
	if settingsBitrate != 0 {
		if settingsBitrate < applicable {
			applicable = settingsBitrate
		}
	}
	return applicable, settingsBitrate
}

// UseEmbeddedChapterTimes reports whether the container's own chapter times
// should be preferred over the sidecar's.
func (s *Settings) UseEmbeddedChapterTimes() bool {
	return s.PreferEmbeddedChapterTimes == PreferEmbeddedChapterTimesIfSilent &&
		s.NamedChapters != NamedChaptersNo
}
