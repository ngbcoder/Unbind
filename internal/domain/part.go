package domain

import (
	"math"
	"sort"
	"time"

	"github.com/ngbcoder/Unbind/internal/settings"
)

// Part is one input file of a book.
//
// A book always has at least one part, even a single-file book, which has a
// part number of zero. That removes a special case from almost every
// algorithm.
type Part struct {
	// Book is the owning book.
	Book *Book

	// AaxFileItem is the source file record.
	AaxFileItem *AaxFileItem

	// PartNumber is 0 for a standalone file, n for part n of a multi-part book.
	PartNumber int

	// Chapters is the working chapter list.
	Chapters []*Chapter

	// Chapters2 is the other chapter list.
	//
	// A part can hold two independent chapter lists: the *embedded* chapters
	// from the container and the *named* chapters from the Audible sidecar.
	// The two lists are swapped several times during the pipeline. Code that
	// reasons about "named versus embedded" must use the NamedChapters and
	// EmbeddedChapters accessors, never the raw fields.
	Chapters2 []*Chapter

	// Tracks are the output files derived from this part.
	Tracks []*Track

	// ActivationCode is the code that successfully decrypted this part.
	ActivationCode string

	// BrandIntro and BrandOutro are Audible's brand intro and outro lengths.
	BrandIntro time.Duration
	BrandOutro time.Duration

	// Duration is the total running time of the part.
	Duration time.Duration

	// ChaptersCurtailed is set when chapters were removed or truncated at the
	// end of the book. It makes the last chapter participate in the adjustment.
	ChaptersCurtailed bool

	// SKU is the Audible SKU, needed to locate series metadata.
	SKU string

	// TmpFileName is the intermediate decrypted file.
	TmpFileName string

	// TmpFileNameIntermedCopy is the intermediate copy for the iTunes path.
	TmpFileNameIntermedCopy string

	// TmpFileNameAACFixed is the intermediate copy with the AAC esds atom patched.
	TmpFileNameAACFixed string

	// FFmpegPrefer64Bit is true when ffmpeg64.exe should be used.
	FFmpegPrefer64Bit bool

	// isMp3Stream caches the answer; nil means "not probed yet".
	isMp3Stream *bool

	// namedChaptersIn1 records which slot currently holds the named chapters.
	namedChaptersIn1 bool
}

// NewPart returns a part for the given file.
func NewPart(book *Book, fi *AaxFileItem, partNumber int) *Part {
	p := &Part{
		Book:        book,
		AaxFileItem: fi,
		PartNumber:  partNumber,
	}
	if fi != nil {
		p.Duration = fi.Duration
	}
	return p
}

// AddTrack appends a track to the part.
func (p *Part) AddTrack(t *Track) {
	p.Tracks = append(p.Tracks, t)
}

// HasNamedChapters is true only when *both* chapter lists are non-empty.
func (p *Part) HasNamedChapters() bool {
	return len(p.Chapters) > 0 && len(p.Chapters2) > 0
}

// NamedChapters returns the named chapter list, if there is one.
func (p *Part) NamedChapters() []*Chapter {
	if !p.HasNamedChapters() {
		return p.Chapters
	}
	if p.namedChaptersIn1 {
		return p.Chapters
	}
	return p.Chapters2
}

// EmbeddedChapters returns the embedded chapter list, if there is one.
func (p *Part) EmbeddedChapters() []*Chapter {
	if !p.HasNamedChapters() {
		return p.Chapters
	}
	if p.namedChaptersIn1 {
		return p.Chapters2
	}
	return p.Chapters
}

// SwapChapters exchanges the two chapter lists. It is how the engine says
// "from here on, Chapters holds the list I want to work with".
func (p *Part) SwapChapters() {
	p.Chapters, p.Chapters2 = p.Chapters2, p.Chapters
	p.namedChaptersIn1 = !p.namedChaptersIn1
}

// ComplementChapterNames replaces empty chapter names with "(n)", so that every
// chapter has a printable name without changing the source data.
func (p *Part) ComplementChapterNames() {
	for i, ch := range p.Chapters {
		if isNullOrWhiteSpace(ch.Name) {
			ch.Name = "(" + itoa(i+1) + ")"
		}
	}
}

// ApplicableInFileNameForTranscode returns the input file to use for a given
// stage.
//
// Every FFmpeg invocation downstream of the probe stage must read from the
// right file, because different preprocessing stages write different
// intermediates. The activation code is only needed when reading the original.
func (p *Part) ApplicableInFileNameForTranscode() string {
	if p.TmpFileName != "" {
		return p.TmpFileName
	}
	if p.TmpFileNameAACFixed != "" {
		return p.TmpFileNameAACFixed
	}
	if p.AaxFileItem != nil {
		return p.AaxFileItem.FileName
	}
	return ""
}

// IsMp3Stream reports whether the source payload is already MP3.
func (p *Part) IsMp3Stream() bool {
	if p.isMp3Stream != nil {
		return *p.isMp3Stream
	}
	if p.AaxFileItem != nil {
		return p.AaxFileItem.IsAA
	}
	return false
}

// SetIsMp3Stream overrides the detected stream type.
func (p *Part) SetIsMp3Stream(v bool) {
	p.isMp3Stream = &v
}

// IsTrancodeConversion reports whether the conversion re-encodes the audio
// rather than copying the stream.
func (p *Part) IsTrancodeConversion(s *settings.Settings) bool {
	isMp3 := p.IsMp3Stream()
	wantMp3 := s.ConvFormat == settings.ConvFormatMP3
	isTranscode := isMp3 != wantMp3

	applicable, _ := s.ApplicableBitRate(p.AaxFileItem.AvgBitRate)
	if applicable != 0 {
		isTranscode = true
	}
	return isTranscode
}

// durationFFmpeg64Bit is the default threshold above which the 64-bit FFmpeg
// is used. 32-bit FFmpeg runs out of address space on very long books.
const durationFFmpeg64Bit = 32 * time.Hour

// SetFFmpegPrefer64Bit decides whether ffmpeg64.exe should be used for this
// part. Only the single mode loads the whole book into memory.
func (p *Part) SetFFmpegPrefer64Bit(s *settings.Settings) {
	if s.ConvMode != settings.ConvModeSingle {
		return
	}
	if s.FFMpegDirectory != "" {
		return
	}
	threshold := durationFFmpeg64Bit
	if s.FFmpeg64bitHours > 0 {
		threshold = time.Duration(s.FFmpeg64bitHours) * time.Hour
	}
	if p.Duration < threshold {
		return
	}
	p.FFmpegPrefer64Bit = is64BitOperatingSystem
}

// adjustTimes applies the pending adjustments of every chapter in the list.
func (p *Part) adjustTimes() {
	for i, chapter := range p.Chapters {
		var next *Chapter
		if i < len(p.Chapters)-1 {
			next = p.Chapters[i+1]
		}
		chapter.AdjustTime(next)
	}
}

// AdjustTimesWithPreference applies the silence-derived adjustments and then,
// if requested, prefers the embedded chapter times.
func (p *Part) AdjustTimesWithPreference(preference settings.PreferEmbeddedChapterTimes) []*Chapter {
	p.adjustTimes()

	if p.HasNamedChapters() && preference != settings.PreferEmbeddedChapterTimesNo {
		return p.preferEmbeddedChapterTimes(preference == settings.PreferEmbeddedChapterTimesAlways)
	}
	return nil
}

// preferEmbeddedChapterTimes replaces the named chapters' end times with the
// matching embedded chapters' end times.
func (p *Part) preferEmbeddedChapterTimes(always bool) []*Chapter {
	if !p.HasNamedChapters() {
		return nil
	}

	matches := p.findMatchingChapters()

	var chapters []*Chapter
	for _, m := range matches {
		nmCh := m.named

		idx := -1
		named := p.NamedChapters()
		for i, c := range named {
			if c == nmCh {
				idx = i
				break
			}
		}

		var nmChNxt *Chapter
		if idx >= 0 && idx < len(named)-1 {
			nmChNxt = named[idx+1]
		}

		result := nmCh.SetNewEnd(m.embedded.Time.End(), nmChNxt, always)
		chapters = append(chapters, result...)
	}

	p.adjustTimes()

	return distinctChapters(chapters)
}

// chapterMatch pairs a named chapter with the embedded chapter it matched.
type chapterMatch struct {
	named    *Chapter
	embedded *Chapter
}

// findMatchingChapters matches the named chapters to the embedded ones by end
// time.
//
// The last named chapter is excluded from matching. The last embedded chapter
// is excluded as a candidate. A match further apart than ten seconds is
// rejected. When two named chapters match the same embedded chapter, the one
// whose duration is closest to the embedded chapter's duration wins and the
// other is re-matched.
func (p *Part) findMatchingChapters() []chapterMatch {
	var result []chapterMatch
	embeddedExcludes := map[*Chapter]bool{}

	named := p.NamedChapters()
	if len(named) == 0 {
		return nil
	}
	// The last named chapter is not matched.
	namedChapters := make([]*Chapter, 0, len(named)-1)
	namedChapters = append(namedChapters, named[:len(named)-1]...)

	for {
		// named chapter -> embedded chapter
		matching := map[*Chapter]*Chapter{}
		var order []*Chapter
		// embedded chapter -> the named chapters that matched it
		embeddedToNamed := map[*Chapter][]*Chapter{}
		var embeddedOrder []*Chapter

		for _, nmCh := range namedChapters {
			minDelta := math.MaxFloat64
			var ebMatchCh *Chapter
			embedded := p.EmbeddedChapters()
			for j := 0; j < len(embedded)-1; j++ {
				ebCh := embedded[j]
				if embeddedExcludes[ebCh] {
					continue
				}
				delta := math.Abs((nmCh.Time.End() - ebCh.Time.End()).Seconds())
				if delta < minDelta {
					minDelta = delta
					ebMatchCh = ebCh
				}
			}

			// if nearer than threshold then replace times
			const threshold = 10.0 // sec
			if minDelta > threshold {
				delete(matching, nmCh)
				removeNamed(&order, nmCh)
				continue
			}

			if _, ok := matching[nmCh]; !ok {
				order = append(order, nmCh)
			}
			matching[nmCh] = ebMatchCh

			if _, ok := embeddedToNamed[ebMatchCh]; !ok {
				embeddedOrder = append(embeddedOrder, ebMatchCh)
			}
			embeddedToNamed[ebMatchCh] = append(embeddedToNamed[ebMatchCh], nmCh)
		}

		// The accumulated result keeps the chapters that are still matched.
		result = result[:0]
		for _, nmCh := range order {
			if eb, ok := matching[nmCh]; ok {
				result = append(result, chapterMatch{named: nmCh, embedded: eb})
			}
		}

		namedChapters = namedChapters[:0]

		var multipleReplacements []*Chapter
		for _, ebCh := range embeddedOrder {
			if len(embeddedToNamed[ebCh]) > 1 {
				multipleReplacements = append(multipleReplacements, ebCh)
			}
		}
		if len(multipleReplacements) == 0 {
			break
		}

		for _, ebCh := range multipleReplacements {
			minDurDelta := math.MaxFloat64
			var nmMatchCh *Chapter
			for _, nmCh := range embeddedToNamed[ebCh] {
				delta := math.Abs((nmCh.Time.Duration() - ebCh.Time.Duration()).Seconds())
				if delta < minDurDelta {
					minDurDelta = delta
					nmMatchCh = nmCh
				}
			}

			embeddedExcludes[ebCh] = true
			for _, nmCh := range embeddedToNamed[ebCh] {
				if nmCh != nmMatchCh {
					namedChapters = append(namedChapters, nmCh)
				}
			}
		}
	}

	return result
}

// removeNamed removes a chapter from an ordered slice, preserving order.
func removeNamed(order *[]*Chapter, ch *Chapter) {
	for i, c := range *order {
		if c == ch {
			*order = append((*order)[:i], (*order)[i+1:]...)
			return
		}
	}
}

// distinctChapters removes duplicates, preserving order.
func distinctChapters(chapters []*Chapter) []*Chapter {
	if len(chapters) == 0 {
		return nil
	}
	seen := make(map[*Chapter]bool, len(chapters))
	out := make([]*Chapter, 0, len(chapters))
	for _, c := range chapters {
		if c == nil || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// TrackId returns the identifier of a track within its part.
func (p *Part) TrackId(track *Track) string {
	for i, t := range p.Tracks {
		if t == track {
			return itoa(p.PartNumber) + "." + itoa(i+1)
		}
	}
	return ""
}

// ChapterId returns the identifier of a chapter within its part.
func (p *Part) ChapterId(chapter *Chapter) string {
	for i, c := range p.Chapters {
		if c == chapter {
			return itoa(p.PartNumber) + "." + itoa(i+1)
		}
	}
	return ""
}

// SetMetaChapters assigns the chapter markers to be embedded in each output
// file.
func (p *Part) SetMetaChapters(s *settings.Settings) {
	switch s.ConvMode {
	case settings.ConvModeChapters, settings.ConvModeSplitChapters:
		p.setMetaChaptersSingle()
	case settings.ConvModeSplitTime:
		p.setMetaChaptersMulti()
	}
}

// setMetaChaptersSingle gives every track the chapter it came from.
func (p *Part) setMetaChaptersSingle() {
	if len(p.Tracks) == 0 {
		return
	}
	for _, track := range p.Tracks {
		if track.Chapter == nil {
			continue
		}
		ch := NewChapterDuration(track.Time.Duration())
		ch.Name = track.Chapter.Name
		track.MetaChapters = []*Chapter{ch}
	}
}

// setMetaChaptersMulti gives every track all the chapters that overlap its
// time range, rebased to the track's start.
func (p *Part) setMetaChaptersMulti() {
	if len(p.Chapters2) == 0 || len(p.Tracks) == 0 {
		return
	}
	for _, track := range p.Tracks {
		var chapters []*Chapter
		for _, ch := range p.Chapters2 {
			overlaps := (ch.Time.End() >= track.Time.Begin() && ch.Time.Begin() <= track.Time.End()) ||
				(ch.Time.Begin() <= track.Time.Begin() && ch.Time.End() >= track.Time.End())
			if overlaps {
				chapters = append(chapters, NewChapterCopy(ch, false))
			}
		}
		if len(chapters) == 0 {
			continue
		}

		for _, chapter := range chapters {
			chapter.Time.SetBegin(chapter.Time.Begin() - track.Time.Begin())
			chapter.Time.SetEnd(chapter.Time.End() - track.Time.Begin())
		}

		chapters[0].Time.SetBegin(0)
		chapters[len(chapters)-1].Time.SetEnd(track.Time.Duration())

		track.MetaChapters = chapters
	}
}

// InsertPaddingChapters inserts up to two synthetic chapters so that the
// chapter list covers the file exactly.
func (p *Part) InsertPaddingChapters() int {
	chCount := len(p.Chapters)
	if chCount == 0 {
		return 0
	}

	ch := p.Chapters[0]
	if ch.Time.Begin() > 0 && !ch.IsPaddingChapter {
		preChapter := NewChapterDuration(ch.Time.Begin())
		preChapter.IsPaddingChapter = true
		p.Chapters = append([]*Chapter{preChapter}, p.Chapters...)
	}

	ch = p.Chapters[len(p.Chapters)-1]
	if ch.Time.End() < p.Duration && !ch.IsPaddingChapter {
		postChapter := NewChapterRange(ch.Time.End(), p.Duration)
		postChapter.IsPaddingChapter = true
		p.Chapters = append(p.Chapters, postChapter)
	}

	return len(p.Chapters) - chCount
}

// RemovePaddingChapters removes the leading and trailing padding chapters.
func (p *Part) RemovePaddingChapters() {
	if len(p.Chapters) > 0 && p.Chapters[0].IsPaddingChapter {
		p.Chapters = p.Chapters[1:]
	}
	if len(p.Chapters) > 0 && p.Chapters[len(p.Chapters)-1].IsPaddingChapter {
		p.Chapters = p.Chapters[:len(p.Chapters)-1]
	}
}

// MergeSilences collapses all chapters of the part into one aggregate chapter
// whose silence list is the concatenation of every chapter's silences, shifted
// into absolute part-relative time.
//
// This is the split-time special case: the chapters are ignored and the whole
// book is cut into equal-length tracks.
func (p *Part) MergeSilences() {
	if len(p.Chapters) == 0 {
		return
	}

	aggrChapter := NewChapterCopy(p.Chapters[0], true)
	startTimeThisChapter := aggrChapter.Time.Begin()
	if startTimeThisChapter != 0 {
		shifted := make([]TimeInterval, 0, len(aggrChapter.Silences))
		for _, ivl := range aggrChapter.Silences {
			shifted = append(shifted, ivl.Shifted(startTimeThisChapter))
		}
		aggrChapter.Silences = shifted
	}

	for i := 1; i < len(p.Chapters); i++ {
		chapter := p.Chapters[i]
		if chapter.Silences == nil {
			continue
		}

		startTimeThisChapter = chapter.Time.Begin()

		for j := range chapter.Silences {
			ivl := chapter.Silences[j].Shifted(startTimeThisChapter)

			if j == 0 && len(aggrChapter.Silences) > 0 {
				last := &aggrChapter.Silences[len(aggrChapter.Silences)-1]
				if ivl.Begin() <= last.End() {
					ivl = NewTimeIntervalRange(last.Begin(), ivl.End())
					aggrChapter.Silences = aggrChapter.Silences[:len(aggrChapter.Silences)-1]
				}
			}

			aggrChapter.Silences = append(aggrChapter.Silences, ivl)
		}
	}

	aggrChapter.Time.SetEnd(p.Chapters[len(p.Chapters)-1].Time.End())
	aggrChapter.TmpFileName = p.TmpFileName
	aggrChapter.Name = ""

	p.SwapChapters()
	p.Chapters = []*Chapter{aggrChapter}
}

// CreateCueSheet splits every chapter of the part into tracks.
func (p *Part) CreateCueSheet(s *settings.Settings) {
	_, _, trackDurMins := settings.TrackDuration(s.TrkDurMins, s.ConvMode)
	splitTimeMode := s.ConvMode == settings.ConvModeSplitTime

	for _, chapter := range p.Chapters {
		chapter.CreateCueSheet(p, trackDurMins, splitTimeMode)
	}
}

// DetermineTimeAdjustments works out the true chapter boundaries from the
// detected silences. It returns the number of affected chapters.
func (p *Part) DetermineTimeAdjustments(book *Book) int {
	for i := range p.Chapters {
		chapter := p.Chapters[i]
		var next *Chapter
		if i < len(p.Chapters)-1 {
			next = p.Chapters[i+1]
		} else if !p.ChaptersCurtailed {
			break
		}
		chapter.DetermineTimeAdjustment(next)
	}

	affected := 0
	for _, c := range p.Chapters {
		if c.TimeAdjustment != nil && !c.IsPaddingChapter {
			affected++
		}
	}
	return affected
}

func (p *Part) String() string {
	part := ""
	if p.PartNumber != 0 {
		part = " Part " + itoa(p.PartNumber)
	}
	dur := time.Duration(0)
	nch, ntr := 0, 0
	if p.AaxFileItem != nil {
		dur = p.AaxFileItem.Duration
	}
	if p.Chapters != nil {
		nch = len(p.Chapters)
	}
	if p.Tracks != nil {
		ntr = len(p.Tracks)
	}
	return "\"" + p.Book.SortingTitle + "\"" + part + ", " + FormatHMSm(dur) +
		", #C=" + itoa(nch) + ", #Tr=" + itoa(ntr)
}

// sortChaptersByBegin sorts a chapter list by its begin time.
func sortChaptersByBegin(chapters []*Chapter) {
	sort.SliceStable(chapters, func(i, j int) bool {
		return chapters[i].Time.Begin() < chapters[j].Time.Begin()
	})
}
