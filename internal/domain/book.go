package domain

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ngbcoder/Unbind/internal/settings"
	"github.com/ngbcoder/Unbind/internal/util"
)

// Caption is an author and title pair.
type Caption struct {
	Author string
	Title  string
}

func (c Caption) String() string {
	return `Author="` + c.Author + `", Title="` + c.Title + `"`
}

// ExternalBookMeta is the author and series information lifted from the
// Audible sidecars.
type ExternalBookMeta struct {
	// Authors are the author names.
	Authors []string

	// Series is the list of series with their sequence numbers.
	Series []SeriesEntry

	// Book is the book title.
	Book string
}

// SeriesEntry is one series and its sequence number.
type SeriesEntry struct {
	Title string
	Seq   string
}

// HasSeries reports whether the meta carries a series.
func (m *ExternalBookMeta) HasSeries() bool {
	return m != nil && m.SeriesString() != ""
}

// SeriesString returns the title of the first series, if any.
func (m *ExternalBookMeta) SeriesString() string {
	if m == nil || len(m.Series) == 0 {
		return ""
	}
	return m.Series[0].Title
}

// seqNoPattern matches a sequence number that may carry a fractional part,
// as in "1.5" for a sub-part.
var seqNoPattern = regexp.MustCompile(`(\D*)((\d*)(\.\d+)?)(\D*)`)

// SeqString formats the sequence number with the given number of digits.
//
// The integer part is zero-padded; a fractional part such as ".5" is kept.
func (m *ExternalBookMeta) SeqString(numDigits byte) string {
	if m == nil || len(m.Series) == 0 {
		return ""
	}
	sn := m.Series[0].Seq
	match := seqNoPattern.FindStringSubmatch(sn)
	if match == nil {
		return sn
	}

	p1 := match[1]
	p2 := match[2]
	p2a := match[3]
	p2b := match[4]
	p3 := match[5]

	if util.IsNullOrWhiteSpace(p2) {
		return p1 + p3
	}

	p2ai, _ := strconv.Atoi(p2a)
	p2af := padLeft(p2ai, int(numDigits))

	return p1 + p2af + p2b + p3
}

// SeqStringFramed returns the sequence number in brackets.
func (m *ExternalBookMeta) SeqStringFramed(numDigits byte) string {
	seq := m.SeqString(numDigits)
	if util.IsNullOrWhiteSpace(seq) {
		return ""
	}
	return "[" + seq + "]"
}

// padLeft zero-pads a number to the given width.
func padLeft(v, width int) string {
	s := itoa(v)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// Book is the logical work, possibly composed of several parts.
type Book struct {
	// SortingTitle is the title with any part suffix removed. It is the
	// identity of the work.
	SortingTitle string

	// Parts are the constituent files.
	Parts []*Part

	// PartsType describes how complete the set of parts is.
	PartsType settings.PartsType

	// PartNameStub is the part prefix as the source used it.
	PartNameStub string

	// ChapterNameStub is the chapter prefix as the source used it.
	ChapterNameStub string

	// IsNewAuthor records that this run created the author folder, so that
	// the rollback can remove it.
	IsNewAuthor bool

	// IsNewSeries records that this run created the series folder.
	IsNewSeries bool

	// OutDirectoryLong is the output folder of this book.
	OutDirectoryLong string

	// ExternalMeta is the author and series information from the sidecars.
	ExternalMeta *ExternalBookMeta

	// FileCaption is the author and title pair used for file and folder names.
	FileCaption Caption

	// TagCaption is the author and title pair used for the metadata.
	TagCaption Caption

	// CustomNames holds the per-book overrides from the preview dialog.
	CustomNames *CustomTagFileNames

	// DefaultAudioFile is the file to hand to the media player.
	DefaultAudioFile string
}

// NewBook returns a book with the given sorting title.
func NewBook(sortingTitle string) *Book {
	return &Book{SortingTitle: sortingTitle}
}

// NewBookFromFileItem returns a book with one part for the given file.
func NewBookFromFileItem(fi *AaxFileItem) *Book {
	b := NewBook(fi.BookTitle)
	b.AddPart(fi, 0)
	return b
}

// AddPart appends a part and re-evaluates the part completeness.
func (b *Book) AddPart(fi *AaxFileItem, partNumber int) {
	b.Parts = append(b.Parts, NewPart(b, fi, partNumber))
	if b.CustomNames == nil {
		b.CustomNames = fi.CustomNames
	}
	b.CheckParts()
}

// CheckParts decides whether the parts form a complete multi-part book.
//
// The decision is arithmetic. A book with the parts 1, 2 and 3 has a part sum
// of 6, which is the triangular number for 3, so the parts are complete and
// are merged. A book with only part 2 has a sum of 2, which is not the
// triangular number for 2, so each part is treated as an independent book.
//
// The sum of 1 is special: a lone "part 1" is a single-part book, not a
// multi-part one, so the parts are never merged.
func (b *Book) CheckParts() settings.PartsType {
	hasParts := false
	for _, p := range b.Parts {
		if p.PartNumber > 0 {
			hasParts = true
			break
		}
	}

	allParts := false
	if hasParts {
		sortPartsByNumber(b.Parts)
		highestPart := 0
		partsSum := 0
		for _, p := range b.Parts {
			if p.PartNumber > highestPart {
				highestPart = p.PartNumber
			}
			partsSum += p.PartNumber
		}
		expectedSum := highestPart * (highestPart + 1) / 2
		allParts = partsSum != 1 && partsSum == expectedSum
	}

	switch {
	case !hasParts:
		b.PartsType = settings.PartsNone
	case allParts:
		b.PartsType = settings.PartsAll
	default:
		b.PartsType = settings.PartsSome
	}
	return b.PartsType
}

// sortPartsByNumber sorts the parts by their part number.
func sortPartsByNumber(parts []*Part) {
	for i := 1; i < len(parts); i++ {
		for j := i; j > 0 && parts[j].PartNumber < parts[j-1].PartNumber; j-- {
			parts[j], parts[j-1] = parts[j-1], parts[j]
		}
	}
}

// HasNamedChapters reports whether every part has named chapters.
func (b *Book) HasNamedChapters() bool {
	for _, p := range b.Parts {
		if !p.HasNamedChapters() {
			return false
		}
	}
	return len(b.Parts) > 0
}

// ChapterNumber returns the number of a chapter, either local to the part or
// global across the parts.
func (b *Book) ChapterNumber(chapter *Chapter) uint {
	if chapter == nil {
		return 0
	}

	var part *Part
	for _, p := range b.Parts {
		for _, c := range p.Chapters {
			if c == chapter {
				part = p
				break
			}
		}
		if part != nil {
			break
		}
	}
	if part == nil {
		return 0
	}

	nChapter := 0
	for i, c := range part.Chapters {
		if c == chapter {
			nChapter = i + 1
			break
		}
	}
	if b.PartsType == settings.PartsSome {
		return uint(nChapter)
	}

	nChapters := 0
	for _, p := range b.Parts {
		if p.PartNumber < part.PartNumber {
			nChapters += len(p.Chapters)
		}
	}
	nChapter += nChapters
	return uint(nChapter)
}

// Counts returns the title and the chapter, track and part counters.
func (b *Book) Counts(mode settings.ConvMode, allChapters bool, part *Part) (title string, chapters, tracks, partNum *uint) {
	var numChapters, numTracks uint

	// the part only matters in the "some" mode
	if b.PartsType == settings.PartsSome {
		if part == nil {
			return "", nil, nil, nil
		}
		numChapters = uint(len(part.Chapters))
		numTracks = uint(len(part.Tracks))
		pn := uint(part.PartNumber)
		partNum = &pn
	} else {
		for _, p := range b.Parts {
			numChapters += uint(len(p.Chapters))
			numTracks += uint(len(p.Tracks))
		}
		if mode == settings.ConvModeSplitTime && numTracks > 0 {
			numChapters = 0
		}
	}

	if mode != settings.ConvModeSingle || allChapters {
		if numChapters > 0 {
			c := numChapters
			chapters = &c
		}
	}
	if numTracks > 0 {
		t := numTracks
		tracks = &t
	}

	return b.TitleTag(), chapters, tracks, partNum
}

// TitleTag returns the title for the metadata.
func (b *Book) TitleTag() string {
	if b.CustomNames != nil && b.CustomNames.TitleTag != "" {
		return b.CustomNames.TitleTag
	}
	return b.TagCaption.Title
}

// TitleFile returns the title for file and folder names.
func (b *Book) TitleFile() string {
	if b.CustomNames != nil && b.CustomNames.TitleFile != "" {
		return b.CustomNames.TitleFile
	}
	return b.FileCaption.Title
}

// AuthorTag returns the author for the metadata.
func (b *Book) AuthorTag() string {
	if b.CustomNames != nil && b.CustomNames.AuthorTag != "" {
		return b.CustomNames.AuthorTag
	}
	return b.TagCaption.Author
}

// AuthorFile returns the author for file and folder names.
func (b *Book) AuthorFile() string {
	if b.CustomNames != nil && b.CustomNames.AuthorFile != "" {
		return b.CustomNames.AuthorFile
	}
	return b.FileCaption.Author
}

// InitAuthorTitle derives the author and title captions from the sorting title.
func (b *Book) InitAuthorTitle(s *settings.Settings) {
	if len(b.Parts) == 0 {
		return
	}

	rgxTitle := initRegexTitle(s)

	fi := b.Parts[0].AaxFileItem

	title := b.SortingTitle
	if s.LongBookTitle != settings.LongTitleNo {
		title = strings.ReplaceAll(title, " -:", ":")
	}

	if m := rgxTitle.FindStringSubmatch(title); m != nil {
		title = strings.TrimSpace(m[1])
	}

	if !strings.ContainsRune(s.AddnlValTitlePunct, '-') {
		if idx := strings.Index(title, " -"); idx >= 0 {
			title = title[:idx]
		}
	}

	title = b.makeLongBookTitle(title, s)

	b.TagCaption = Caption{Author: fi.Author(), Title: title}
	title = PruneTitle(title)
	b.FileCaption = Caption{Author: util.Prune(b.TagCaption.Author), Title: title}
}

// PruneTitle makes a title safe for a file name: the colon and the slash
// become semicolons, and the doubtful characters are removed.
func PruneTitle(title string) string {
	replaced := strings.ReplaceAll(title, ":", ";")
	replaced = strings.ReplaceAll(replaced, "/", ";")
	replaced = strings.ReplaceAll(replaced, " ;", ";")
	return util.Prune(replaced)
}

// makeLongBookTitle implements the "long book title" styles. The series or
// subtitle moves in front of, or behind, the title.
func (b *Book) makeLongBookTitle(title string, s *settings.Settings) string {
	if s.LongBookTitle == settings.LongTitleNo || s.LongBookTitle == settings.LongTitleAsIs {
		return title
	}

	if (s.SeriesTitleLeft && s.LongBookTitle == settings.LongTitleBookSeries) ||
		(!s.SeriesTitleLeft && s.LongBookTitle == settings.LongTitleSeriesBook) {

		pos := -1
		if s.SeriesTitleLeft {
			pos = strings.Index(title, ":")
		} else {
			pos = strings.LastIndex(title, ":")
		}
		if pos > 0 {
			title1 := strings.TrimSpace(title[:pos])
			title2 := strings.TrimSpace(title[pos+1:])
			title = title2 + ": " + title1
		}
	}

	return title
}

// The title regex is assembled at runtime from the settings, so that a user
// can extend the set of characters that terminate a title.
//
// The character classes use explicit Unicode properties rather than \w and \W.
// .NET's \w is Unicode-aware, but Go's is ASCII-only, and a title in a
// non-Latin script would otherwise be truncated at its first character.
const (
	rgxWord    = `\p{L}\p{Mn}\p{Nd}\p{Pc}`
	rgxSpace   = `\s\p{Z}`
	rgxNotWord = `[^\p{L}\p{Mn}\p{Nd}\p{Pc}]`
)

// rgxTitlePunct is the built-in set of characters that terminate a title.
const rgxTitlePunct = `.,'/\-&`

// escChars are the characters that must be escaped in a character class.
const escChars = `\^$.|?*+()-`

// rgxWordOrSpace matches the characters that are stripped from the dynamic
// punctuation set.
var rgxWordOrSpace = regexp.MustCompile(`([` + rgxWord + rgxSpace + `])`)

// titleClass builds the character class of a title.
func titleClass(punct string) string {
	return `[` + rgxWord + `+` + rgxSpace + `+` + punct + `]`
}

// escapeRegexp escapes the characters that are special inside a character class.
func escapeRegexp(s string) string {
	var sb strings.Builder
	for _, c := range s {
		if strings.ContainsRune(escChars, c) {
			sb.WriteRune('\\')
		}
		sb.WriteRune(c)
	}
	return sb.String()
}

// initRegexTitle builds the title regex for the given settings.
//
// Two forms are cached: the plain one and the one that also accepts a leading
// series title. The dynamic form is built only when the user's custom
// punctuation requires it.
func initRegexTitle(s *settings.Settings) *regexp.Regexp {
	if s == nil {
		return rgxTitle0
	}

	if util.IsNullOrWhiteSpace(s.AddnlValTitlePunct) && s.LongBookTitle == settings.LongTitleNo {
		if s.SeriesTitleLeft {
			return rgxTitleL
		}
		return rgxTitle0
	}

	class := titleClass("")
	rgxTitle1 := "^(" + class

	chars := []rune(rgxTitlePunct)

	if !util.IsNullOrWhiteSpace(s.AddnlValTitlePunct) {
		seen := map[rune]bool{}
		var extra []rune
		for _, c := range s.AddnlValTitlePunct {
			if !seen[c] {
				seen[c] = true
				extra = append(extra, c)
			}
		}
		chars = append(chars, extra...)
	}

	if s.LongBookTitle != settings.LongTitleNo {
		chars = append(chars, ':')
	} else if s.SeriesTitleLeft {
		rgxTitle1 = "^(?:" + titleClass(escapeRegexp(rgxTitlePunct)) + ":)?(" + class
	}

	// strip the word and whitespace characters from the punctuation set
	str := string(chars)
	for {
		loc := rgxWordOrSpace.FindStringIndex(str)
		if loc == nil {
			break
		}
		str = str[:loc[0]] + str[loc[1]:]
	}

	pattern := rgxTitle1 + escapeRegexp(str) + "]+)" + rgxNotWord + "*"
	return regexp.MustCompile(pattern)
}

// rgxTitle0 matches a title that may be preceded by nothing.
var rgxTitle0 = regexp.MustCompile("^(" + titleClass(escapeRegexp(rgxTitlePunct)) + "+)" + rgxNotWord + "*")

// rgxTitleL matches a title that may be preceded by a series title.
var rgxTitleL = regexp.MustCompile("^(?:" + titleClass(escapeRegexp(rgxTitlePunct)) + ":)?(" +
	titleClass(escapeRegexp(rgxTitlePunct)) + "+)" + rgxNotWord + "*")
