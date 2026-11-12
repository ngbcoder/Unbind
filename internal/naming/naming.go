// Package naming computes the output file names, the folder names and the
// metadata titles.
//
// It is the port of TagAndFileNamingHelper and its nested Numbers class. The
// naming and tagging subsystem is the most regression-prone part of the
// application, because it is a large matrix of settings, so every branch here
// is covered by a test.
package naming

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ngbcoder/Unbind/internal/domain"
	"github.com/ngbcoder/Unbind/internal/settings"
	"github.com/ngbcoder/Unbind/internal/util"
)

// The file name extensions.
const (
	ExtM4A = ".m4a"
	ExtM4B = ".m4b"
	ExtMP3 = ".mp3"
)

// The fallbacks, used when every other source failed.
const (
	FallbackGenre   = "Audiobook"
	FallbackChapter = "Chapter"
	FallbackPart    = "Part"
)

// The punctuation chains. The dash separates the components of a name, the dot
// chain separates the parts of a track number.
const (
	sepDash   = " - "
	sepDot0   = "."
	sepDot1   = "("
	sepDot2   = ")"
	spaceChar = " "
)

// Resources supplies the localized strings that the naming needs.
type Resources struct {
	// PartNamePrefixStandard is the localized word for "Part".
	PartNamePrefixStandard string

	// ChapterNamePrefixStandard is the localized word for "Chapter".
	ChapterNamePrefixStandard string
}

// NamedChaptersMode is how a chapter is named in a file name or a tag.
//
// It is derived from the NamedChapters setting and whether the book has
// unique chapter names. The C# original nests a second enum with this name
// inside the naming helper, distinct from the setting's own enum.
type NamedChaptersMode int

// The chapter naming modes.
const (
	// ChaptersNumber names the chapter by its number.
	ChaptersNumber NamedChaptersMode = iota
	// ChaptersName names the chapter by its own name.
	ChaptersName
	// ChaptersNumberName prefixes the name with the number.
	ChaptersNumberName
)

// Digits returns the number of digits of the value. Zero has one digit.
func Digits(n int) int {
	if n < 0 {
		n = -n
	}
	if n == 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

// Str formats the number zero-padded to the given number of digits.
func Str(n, digits int) string {
	if digits <= 0 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%0*d", digits, n)
}

// Numbers holds the counters used when composing a name.
//
// The computation differs by the part completeness. For an incomplete set of
// parts every counter is local to the part, because each part is an
// independent book. Otherwise the track and chapter numbers run continuously
// across the parts.
type Numbers struct {
	// NTrk is the current track.
	NTrk int
	// NTrks is the total number of tracks.
	NTrks int
	// NNTrk is the number of digits of NTrks.
	NNTrk int

	// NChTrk is the current track within its chapter.
	NChTrk int
	// NNChTrk is the number of digits of NChTrks.
	NNChTrk int
	// NChTrks is the number of tracks in the chapter.
	NChTrks int

	// NDsk is the current disc.
	NDsk int
	// NDsks is the total number of discs.
	NDsks int

	// NChp is the current chapter.
	NChp int
	// NNChp is the number of digits of the chapter count.
	NNChp int

	// NPrt is the current part.
	NPrt int
	// NNPrt is the number of digits of the part count.
	NNPrt int
}

// NewNumbers computes the counters for a track within a part.
func NewNumbers(track *domain.Track, part *domain.Part) *Numbers {
	n := &Numbers{NTrk: -1, NTrks: -1, NNTrk: -1, NChTrk: -1, NNChTrk: -1,
		NChTrks: -1, NDsk: -1, NDsks: -1, NChp: -1, NNChp: -1, NPrt: -1, NNPrt: -1}
	if part == nil {
		return n
	}
	book := part.Book

	n.NPrt = part.PartNumber
	highest := 0
	if book != nil {
		for _, p := range book.Parts {
			if p.PartNumber > highest {
				highest = p.PartNumber
			}
		}
	}
	n.NNPrt = Digits(highest)

	if track != nil && track.Chapter != nil {
		var chapterTracks []*domain.Track
		for _, t := range part.Tracks {
			if t.Chapter != nil && t.Chapter == track.Chapter {
				chapterTracks = append(chapterTracks, t)
			}
		}
		for i, t := range chapterTracks {
			if t == track {
				n.NChTrk = i + 1
				break
			}
		}
		n.NChTrks = len(chapterTracks)
		n.NNChTrk = Digits(n.NChTrks)
	}

	partsType := settings.PartsNone
	if book != nil {
		partsType = book.PartsType
	}

	if partsType == settings.PartsSome {
		// Every counter is local to the part.
		for i, t := range part.Tracks {
			if t == track {
				n.NTrk = i + 1
				break
			}
		}
		n.NTrks = len(part.Tracks)

		n.NDsk = part.PartNumber
		n.NDsks = 0

		if track != nil && track.Chapter != nil {
			for i, c := range part.Chapters {
				if c == track.Chapter {
					n.NChp = i + 1
					break
				}
			}
		}
		n.NNChp = Digits(len(part.Chapters))
	} else {
		// The counters run across the parts.
		n.NTrk = 0
		n.NTrks = 0
		n.NChp = 0
		totalChapters := 0
		if book != nil {
			for _, pt := range book.Parts {
				// The totals always span every part.
				n.NTrks += len(pt.Tracks)
				totalChapters += len(pt.Chapters)
				if pt == part {
					continue
				}
				if pt.PartNumber < part.PartNumber {
					n.NTrk += len(pt.Tracks)
					n.NChp += len(pt.Chapters)
				}
			}
		}
		for i, t := range part.Tracks {
			if t == track {
				n.NTrk += i + 1
				break
			}
		}
		n.NChp += func() int {
			if track == nil || track.Chapter == nil {
				return 0
			}
			for i, c := range part.Chapters {
				if c == track.Chapter {
					return i + 1
				}
			}
			return 0
		}()

		if book != nil {
			n.NDsks = len(book.Parts)
		}
		n.NNChp = Digits(totalChapters)
		if partsType == settings.PartsNone {
			n.NDsk = 1
		} else {
			n.NDsk = part.PartNumber
		}
	}

	n.NNTrk = Digits(n.NTrks)
	return n
}

// Helper computes the names for a book, a part and a track.
type Helper struct {
	settings  *settings.Settings
	resources *Resources
	book      *domain.Book
	part      *domain.Part
	track     *domain.Track

	namedChapters NamedChaptersMode
	isFileName    bool
	numbers       *Numbers
}

// NewHelper returns a naming helper.
//
// The helper is used for both file names and tag titles; isFileName selects
// which, because the two differ: a file name is pruned and a tag title is
// kept verbatim.
func NewHelper(s *settings.Settings, resources *Resources, book *domain.Book, part *domain.Part, track *domain.Track, isFileName bool) *Helper {
	h := &Helper{
		settings:   s,
		resources:  resources,
		book:       book,
		part:       part,
		track:      track,
		isFileName: isFileName,
	}
	h.numbers = NewNumbers(track, part)
	h.namedChapters = h.deriveNamedChapters()
	return h
}

// deriveNamedChapters works out how a chapter is named.
//
// The names are used only when the book has named chapters. The number is
// prefixed when the setting asks for it, or when the chapter names are not
// unique and would therefore be ambiguous.
func (h *Helper) deriveNamedChapters() NamedChaptersMode {
	if h.book == nil || h.settings == nil {
		return ChaptersNumber
	}
	useChapterNames := h.settings.NamedChapters >= settings.NamedChaptersYes && h.book.HasNamedChapters()
	useChapterNumbers := (useChapterNames && h.settings.NamedChapters == settings.NamedChaptersYesAlwaysWithNumbers) ||
		!h.book.HasUniqueChapterNames(h.settings.ConvMode)
	if useChapterNames {
		if useChapterNumbers {
			return ChaptersNumberName
		}
		return ChaptersName
	}
	return ChaptersNumber
}

// Numbers returns the counters.
func (h *Helper) Numbers() *Numbers { return h.numbers }

// PartPrefix returns the prefix of the part folder.
func (h *Helper) PartPrefix() string {
	var prefix string
	switch h.settings.PartNaming {
	case settings.GeneralNamingSource:
		prefix = h.book.PartNameStub
	case settings.GeneralNamingCustom:
		prefix = h.settings.PartName
	default:
		if h.resources != nil {
			prefix = h.resources.PartNamePrefixStandard
		}
	}
	if prefix == "" {
		prefix = FallbackPart
	}
	return prefix
}

// ChapterPrefix returns the prefix of the chapter folder.
func (h *Helper) ChapterPrefix() string {
	var prefix string
	switch h.settings.ChapterNaming {
	case settings.GeneralNamingExSource:
		prefix = h.book.ChapterNameStub
	case settings.GeneralNamingExCustom:
		prefix = h.settings.ChapterName
	default:
		if h.resources != nil {
			prefix = h.resources.ChapterNamePrefixStandard
		}
	}
	if prefix == "" {
		prefix = FallbackChapter
	}
	return prefix
}

// ExtMp4 returns the extension of the MP4 output.
func (h *Helper) ExtMp4() string {
	if h.settings.M4B {
		return ExtM4B
	}
	return ExtM4A
}

// Extension returns the extension of the output file.
func (h *Helper) Extension() string {
	if h.settings.ConvFormat == settings.ConvFormatMP4 {
		return h.ExtMp4()
	}
	return ExtMP3
}

// PartDirectoryName returns the name of the part folder.
func (h *Helper) PartDirectoryName() string {
	n := h.numbers
	return h.PartPrefix() + spaceChar + Str(n.NPrt, n.NNPrt)
}

// ChapterDirectoryName returns the name of the chapter folder.
func (h *Helper) ChapterDirectoryName() string {
	n := h.numbers
	chapter := h.ChapterPrefix()
	switch h.namedChapters {
	case ChaptersName, ChaptersNumberName:
		chapterName := h.chapterName()
		isNumeric := isNumericString(chapterName)
		if isNumeric {
			chapterName = chapter + spaceChar + chapterName
		}
		if h.namedChapters == ChaptersName {
			return chapterName
		}
		return "(" + Str(n.NChp, n.NNChp) + ") " + chapterName
	default:
		return chapter + spaceChar + Str(n.NChp, n.NNChp)
	}
}

// chapterName returns the name of the track's chapter.
func (h *Helper) chapterName() string {
	if h.track == nil || h.track.Chapter == nil {
		return ""
	}
	return h.track.Chapter.Name
}

// Track returns the track component of a name, with the total track count
// appended when the setting asks for it.
func (h *Helper) Track() string {
	return h.trackEx()
}

// TrackForFile returns the track component of a file name. A slash is illegal
// in a file name, so it becomes a comma.
func (h *Helper) TrackForFile() string {
	return strings.ReplaceAll(h.Track(), "/", ",")
}

// trackName is the plain track component, without the total track count.
func (h *Helper) trackName() string {
	n := h.numbers
	switch h.settings.ConvMode {
	case settings.ConvModeChapters:
		return h.chapterNamedTrackForChapters()
	case settings.ConvModeSplitChapters:
		return h.chapterNamedTrackForSplitChapters()
	default:
		return Str(n.NTrk, n.NNTrk)
	}
}

// trackEx is the track component, optionally with the total track count.
func (h *Helper) trackEx() string {
	trk := h.trackName()
	if h.settings.TotalTracks {
		return trk + "/" + fmt.Sprintf("%d", h.numbers.NTrks)
	}
	return trk
}

// chapterNamedTrackForChapters is the track component in the chapters mode.
func (h *Helper) chapterNamedTrackForChapters() string {
	n := h.numbers
	trackName := Str(n.NTrk, n.NNTrk)

	switch h.namedChapters {
	case ChaptersName, ChaptersNumberName:
		chapterName := h.chapterName()
		if h.isFileName {
			chapterName = util.Prune(chapterName)
			if h.namedChapters == ChaptersNumberName {
				chapterName = "(" + trackName + ") " + chapterName
			}
		}
		return chapterName
	default:
		return trackName
	}
}

// chapterNamedTrackForSplitChapters is the track component in the split-chapter
// mode.
func (h *Helper) chapterNamedTrackForSplitChapters() string {
	n := h.numbers
	trackName := Str(n.NTrk, n.NNTrk)

	if n.NChp == 0 {
		return trackName
	}

	chapterName := h.chapterName()
	if h.isFileName {
		chapterName = util.Prune(chapterName)
	}

	switch h.settings.TrackNumbering {
	case settings.TrackNumberingChapterATrack:
		var tn string
		if h.namedChapters != ChaptersNumber {
			tn = chapterName
		} else {
			tn = Str(n.NChp, n.NNChp)
		}
		if n.NChTrks > 1 {
			tn += sepDot0 + Str(n.NChTrk, n.NNChTrk)
		}
		return tn
	case settings.TrackNumberingTrackBChapterC:
		if h.namedChapters != ChaptersNumber {
			return trackName + sepDot1 + chapterName + sepDot2
		}
		return trackName + sepDot1 + Str(n.NChp, n.NNChp) + sepDot2
	default:
		return trackName
	}
}

// Title returns the tag title.
func (h *Helper) Title() string {
	n := h.numbers
	isSingleOutputFile := h.settings.ConvMode == settings.ConvModeSingle && n.NTrks == 1
	p := sepDash

	switch h.settings.TitleNaming {
	case settings.TitleNamingTrack:
		return h.Track()
	case settings.TitleNamingTrackBook:
		if isSingleOutputFile {
			return h.book.TitleTag()
		}
		return h.Track() + p + h.book.TitleTag()
	case settings.TitleNamingTrackBookAuthor:
		if isSingleOutputFile {
			return h.book.TitleTag() + p + h.book.AuthorTag()
		}
		return h.Track() + p + h.book.TitleTag() + p + h.book.AuthorTag()
	case settings.TitleNamingBookAuthor:
		return h.book.TitleTag() + p + h.book.AuthorTag()
	case settings.TitleNamingAuthorBook:
		return h.book.AuthorTag() + p + h.book.TitleTag()
	case settings.TitleNamingAuthorBookTrack:
		if isSingleOutputFile {
			return h.book.AuthorTag() + p + h.book.TitleTag()
		}
		return h.book.AuthorTag() + p + h.book.TitleTag() + p + h.Track()
	case settings.TitleNamingBookTrack:
		if isSingleOutputFile {
			return h.book.TitleTag()
		}
		return h.book.TitleTag() + p + h.Track()
	default:
		return h.Track()
	}
}

// FileName returns the name of the output file, without the extension.
func (h *Helper) FileName() string {
	switch h.settings.ConvMode {
	case settings.ConvModeSingle:
		return h.fileSingle()
	default:
		return h.fileChapters()
	}
}

// fileChapters is the file name in the chapter and split modes.
func (h *Helper) fileChapters() string {
	p := sepDash
	switch h.settings.FileNaming {
	case settings.FileNamingTrackBook:
		return h.TrackForFile() + p + h.book.TitleFile()
	case settings.FileNamingTrackBookAuthor:
		return h.TrackForFile() + p + h.book.TitleFile() + p + h.book.AuthorFile()
	case settings.FileNamingAuthorBookTrack:
		return h.book.AuthorFile() + p + h.book.TitleFile() + p + h.TrackForFile()
	case settings.FileNamingBookTrack:
		return h.book.TitleFile() + p + h.TrackForFile()
	default:
		return h.TrackForFile()
	}
}

// fileSingle is the file name in the single mode.
func (h *Helper) fileSingle() string {
	switch h.book.PartsType {
	case settings.PartsSome:
		return h.fileSingleSome()
	case settings.PartsAll:
		return h.fileChapters()
	default:
		return h.fileSingleNone()
	}
}

// fileSingleNone is the file name of a book without parts.
func (h *Helper) fileSingleNone() string {
	p := sepDash
	switch h.settings.FileNaming {
	case settings.FileNamingTrackBookAuthor:
		return h.book.TitleFile() + p + h.book.AuthorFile()
	case settings.FileNamingAuthorBookTrack:
		return h.book.AuthorFile() + p + h.book.TitleFile()
	default:
		// The track component is meaningless with a single file.
		return h.book.TitleFile()
	}
}

// fileSingleSome is the file name of a book with an incomplete set of parts.
// The part number takes the place of the track.
func (h *Helper) fileSingleSome() string {
	n := h.numbers
	p := sepDash
	prt := Str(n.NPrt, n.NNPrt)
	switch h.settings.FileNaming {
	case settings.FileNamingTrackBook:
		return prt + p + h.book.TitleFile()
	case settings.FileNamingBookTrack:
		return h.book.TitleFile() + p + prt
	case settings.FileNamingTrackBookAuthor:
		return prt + p + h.book.TitleFile() + p + h.book.AuthorFile()
	case settings.FileNamingAuthorBookTrack:
		return h.book.AuthorFile() + p + h.book.TitleFile() + p + prt
	default:
		return prt
	}
}

// FullPath returns the absolute path of the output file.
func (h *Helper) FullPath() string {
	filename := h.FileName() + h.Extension()
	base := h.book.OutDirectoryLong

	switch h.settings.ConvMode {
	case settings.ConvModeSingle:
		if h.settings.ExtraMetaFiles && h.book.PartsType == settings.PartsSome {
			return filepath.Join(base, h.PartDirectoryName(), filename)
		}
		return filepath.Join(base, filename)
	case settings.ConvModeChapters, settings.ConvModeSplitTime:
		if h.book.PartsType == settings.PartsSome {
			return filepath.Join(base, h.PartDirectoryName(), filename)
		}
		return filepath.Join(base, filename)
	case settings.ConvModeSplitChapters:
		if h.book.PartsType == settings.PartsSome {
			if h.settings.ChapterNaming != settings.GeneralNamingExNoFolders {
				return filepath.Join(base, h.PartDirectoryName(), h.ChapterDirectoryName(), filename)
			}
			return filepath.Join(base, h.PartDirectoryName(), filename)
		}
		if h.settings.ChapterNaming != settings.GeneralNamingExNoFolders {
			return filepath.Join(base, h.ChapterDirectoryName(), filename)
		}
		return filepath.Join(base, filename)
	default:
		return filepath.Join(base, filename)
	}
}

// SetFileName assigns the output path to the track.
func (h *Helper) SetFileName() {
	if h.track == nil {
		return
	}
	h.track.FileName = h.FullPath()
}

// SetTrackTitle assigns the tag title to the track.
func (h *Helper) SetTrackTitle() {
	if h.track == nil {
		return
	}
	h.track.Title = h.Title()
}

// GetGenre resolves the genre of a file.
func GetGenre(s *settings.Settings, afi *domain.AaxFileItem) string {
	if s.GenreNaming == settings.GeneralNamingSource {
		if afi != nil && afi.Genre != "" {
			return afi.Genre
		}
		return FallbackGenre
	}
	if s.GenreName != "" {
		return s.GenreName
	}
	return FallbackGenre
}

// isNumericString reports whether the string is a plain number.
func isNumericString(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
