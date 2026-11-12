// Package settings holds the enumerations and the settings contract that the
// conversion engine consumes.
//
// The enumerations keep the *exact* names of the C# original
// (AaxAudioConverterLib/Enums.cs). That matters: the user interface renders
// an enum by splitting its name on "_" and looking up each part in the
// resource manager, so "track_book_author" must stay exactly that. Each
// enumeration therefore exposes a String method that returns the C# name, and
// a parser that accepts it.
package settings

import "strings"

// ConvFormat is the target container.
type ConvFormat int

// Target containers.
const (
	ConvFormatMP3 ConvFormat = iota
	ConvFormatMP4
)

var convFormatNames = [...]string{"mp3", "mp4"}

// String returns the C# name of the value.
func (v ConvFormat) String() string {
	if int(v) < 0 || int(v) >= len(convFormatNames) {
		return "mp3"
	}
	return convFormatNames[v]
}

// ParseConvFormat parses a C# enum name.
func ParseConvFormat(s string) (ConvFormat, bool) {
	for i, n := range convFormatNames {
		if n == s {
			return ConvFormat(i), true
		}
	}
	return 0, false
}

// ConvMode is the output structure.
type ConvMode int

// Output structures.
const (
	ConvModeSingle ConvMode = iota
	ConvModeChapters
	ConvModeSplitChapters
	ConvModeSplitTime
)

var convModeNames = [...]string{"single", "chapters", "splitChapters", "splitTime"}

// String returns the C# name of the value.
func (v ConvMode) String() string {
	if int(v) < 0 || int(v) >= len(convModeNames) {
		return "single"
	}
	return convModeNames[v]
}

// ParseConvMode parses a C# enum name.
func ParseConvMode(s string) (ConvMode, bool) {
	for i, n := range convModeNames {
		if n == s {
			return ConvMode(i), true
		}
	}
	return 0, false
}

// FileNaming is the output file name pattern.
type FileNaming int

// File name patterns.
const (
	FileNamingTrack FileNaming = iota
	FileNamingTrackBook
	FileNamingTrackBookAuthor
	FileNamingAuthorBookTrack
	FileNamingBookTrack
)

var fileNamingNames = [...]string{
	"track", "track_book", "track_book_author", "author_book_track", "book_track",
}

// String returns the C# name of the value.
func (v FileNaming) String() string {
	if int(v) < 0 || int(v) >= len(fileNamingNames) {
		return "track"
	}
	return fileNamingNames[v]
}

// ParseFileNaming parses a C# enum name.
func ParseFileNaming(s string) (FileNaming, bool) {
	for i, n := range fileNamingNames {
		if n == s {
			return FileNaming(i), true
		}
	}
	return 0, false
}

// TitleNaming is the tag title pattern.
type TitleNaming int

// Tag title patterns.
const (
	TitleNamingTrack TitleNaming = iota
	TitleNamingTrackBook
	TitleNamingTrackBookAuthor
	TitleNamingBookAuthor
	TitleNamingAuthorBook
	TitleNamingAuthorBookTrack
	TitleNamingBookTrack
)

var titleNamingNames = [...]string{
	"track", "track_book", "track_book_author", "book_author",
	"author_book", "author_book_track", "book_track",
}

// String returns the C# name of the value.
func (v TitleNaming) String() string {
	if int(v) < 0 || int(v) >= len(titleNamingNames) {
		return "track"
	}
	return titleNamingNames[v]
}

// ParseTitleNaming parses a C# enum name.
func ParseTitleNaming(s string) (TitleNaming, bool) {
	for i, n := range titleNamingNames {
		if n == s {
			return TitleNaming(i), true
		}
	}
	return 0, false
}

// TrackNumbering is the track numbering scheme in the split modes.
type TrackNumbering int

// Track numbering schemes.
const (
	TrackNumberingTrack TrackNumbering = iota
	TrackNumberingChapterATrack
	TrackNumberingTrackBChapterC
)

var trackNumberingNames = [...]string{"track", "chapter_a_track", "track_b_chapter_c"}

// String returns the C# name of the value.
func (v TrackNumbering) String() string {
	if int(v) < 0 || int(v) >= len(trackNumberingNames) {
		return "track"
	}
	return trackNumberingNames[v]
}

// ParseTrackNumbering parses a C# enum name.
func ParseTrackNumbering(s string) (TrackNumbering, bool) {
	for i, n := range trackNumberingNames {
		if n == s {
			return TrackNumbering(i), true
		}
	}
	return 0, false
}

// LongTitle is the "long book title" style.
type LongTitle int

// Long title styles.
const (
	LongTitleNo LongTitle = iota
	LongTitleBookSeries
	LongTitleSeriesBook
	LongTitleAsIs
)

var longTitleNames = [...]string{"no", "book_series", "series_book", "as_is"}

// String returns the C# name of the value.
func (v LongTitle) String() string {
	if int(v) < 0 || int(v) >= len(longTitleNames) {
		return "no"
	}
	return longTitleNames[v]
}

// ParseLongTitle parses a C# enum name.
func ParseLongTitle(s string) (LongTitle, bool) {
	for i, n := range longTitleNames {
		if n == s {
			return LongTitle(i), true
		}
	}
	return 0, false
}

// GeneralNaming selects where the genre or part prefix comes from.
type GeneralNaming int

// Prefix sources.
const (
	GeneralNamingSource GeneralNaming = iota
	GeneralNamingStandard
	GeneralNamingCustom
)

var generalNamingNames = [...]string{"source", "standard", "custom"}

// String returns the C# name of the value.
func (v GeneralNaming) String() string {
	if int(v) < 0 || int(v) >= len(generalNamingNames) {
		return "source"
	}
	return generalNamingNames[v]
}

// ParseGeneralNaming parses a C# enum name.
func ParseGeneralNaming(s string) (GeneralNaming, bool) {
	for i, n := range generalNamingNames {
		if n == s {
			return GeneralNaming(i), true
		}
	}
	return 0, false
}

// GeneralNamingEx selects where the chapter prefix comes from.
type GeneralNamingEx int

// Chapter prefix sources.
const (
	GeneralNamingExSource GeneralNamingEx = iota
	GeneralNamingExStandard
	GeneralNamingExCustom
	GeneralNamingExNoFolders
)

var generalNamingExNames = [...]string{"source", "standard", "custom", "_nofolders"}

// String returns the C# name of the value.
func (v GeneralNamingEx) String() string {
	if int(v) < 0 || int(v) >= len(generalNamingExNames) {
		return "source"
	}
	return generalNamingExNames[v]
}

// ParseGeneralNamingEx parses a C# enum name.
func ParseGeneralNamingEx(s string) (GeneralNamingEx, bool) {
	for i, n := range generalNamingExNames {
		if n == s {
			return GeneralNamingEx(i), true
		}
	}
	return 0, false
}

// FlatFolderNaming is the order of the components in a flat folder name.
type FlatFolderNaming int

// Flat folder name orders.
const (
	FlatFolderNamingAuthorBook FlatFolderNaming = iota
	FlatFolderNamingBookAuthor
)

var flatFolderNamingNames = [...]string{"author_book", "book_author"}

// String returns the C# name of the value.
func (v FlatFolderNaming) String() string {
	if int(v) < 0 || int(v) >= len(flatFolderNamingNames) {
		return "author_book"
	}
	return flatFolderNamingNames[v]
}

// ParseFlatFolderNaming parses a C# enum name.
func ParseFlatFolderNaming(s string) (FlatFolderNaming, bool) {
	for i, n := range flatFolderNamingNames {
		if n == s {
			return FlatFolderNaming(i), true
		}
	}
	return 0, false
}

// AaxCopyMode is the archive layout for the original files. It is a bit mask.
//
//	bit 1  book first in the file name
//	bit 2  include the author in the file name
//	bit 4  group into per-author folders
//	bit 8  reserved, unused
type AaxCopyMode int

// Archive layouts.
const (
	AaxCopyModeNone             AaxCopyMode = 0
	AaxCopyModeFlatAuthorBook   AaxCopyMode = 10
	AaxCopyModeFlatBookAuthor   AaxCopyMode = 11
	AaxCopyModeAuthorBook       AaxCopyMode = 12
	AaxCopyModeAuthorAuthorBook AaxCopyMode = 14
	AaxCopyModeAuthorBookAuthor AaxCopyMode = 15
)

// The individual flags of AaxCopyMode.
const (
	AaxCopyFlagBookFirst      = 1
	AaxCopyFlagBookWithAuthor = 2
	AaxCopyFlagFoldersByAuth  = 4
	AaxCopyFlagFolders        = 8
)

var aaxCopyModeNames = map[AaxCopyMode]string{
	AaxCopyModeNone:             "no",
	AaxCopyModeFlatAuthorBook:   "flat__author_book",
	AaxCopyModeFlatBookAuthor:   "flat__book_author",
	AaxCopyModeAuthorBook:       "author__book",
	AaxCopyModeAuthorAuthorBook: "author__author_book",
	AaxCopyModeAuthorBookAuthor: "author__book_author",
}

// String returns the C# name of the value.
func (v AaxCopyMode) String() string {
	if n, ok := aaxCopyModeNames[v]; ok {
		return n
	}
	return "no"
}

// ParseAaxCopyMode parses a C# enum name.
func ParseAaxCopyMode(s string) (AaxCopyMode, bool) {
	for v, n := range aaxCopyModeNames {
		if n == s {
			return v, true
		}
	}
	return 0, false
}

// NamedChapters selects whether Audible's named chapters are used.
type NamedChapters int

// Named chapter modes.
const (
	NamedChaptersNo NamedChapters = iota
	NamedChaptersYes
	NamedChaptersYesAlwaysWithNumbers
)

var namedChaptersNames = [...]string{"no", "yes", "yesAlwaysWithNumbers"}

// String returns the C# name of the value.
func (v NamedChapters) String() string {
	if int(v) < 0 || int(v) >= len(namedChaptersNames) {
		return "no"
	}
	return namedChaptersNames[v]
}

// ParseNamedChapters parses a C# enum name.
func ParseNamedChapters(s string) (NamedChapters, bool) {
	for i, n := range namedChaptersNames {
		if n == s {
			return NamedChapters(i), true
		}
	}
	return 0, false
}

// OnlineUpdate is the update behaviour.
type OnlineUpdate int

// Update behaviours.
const (
	OnlineUpdateNo OnlineUpdate = iota
	OnlineUpdatePromptForDownload
	OnlineUpdatePromptForInstall
)

var onlineUpdateNames = [...]string{"no", "promptForDownload", "promptForInstall"}

// String returns the C# name of the value.
func (v OnlineUpdate) String() string {
	if int(v) < 0 || int(v) >= len(onlineUpdateNames) {
		return "no"
	}
	return onlineUpdateNames[v]
}

// ParseOnlineUpdate parses a C# enum name.
func ParseOnlineUpdate(s string) (OnlineUpdate, bool) {
	for i, n := range onlineUpdateNames {
		if n == s {
			return OnlineUpdate(i), true
		}
	}
	return 0, false
}

// VerifyAdjustChapterMarks selects when chapter marks are re-derived from
// silence detection.
type VerifyAdjustChapterMarks int

// Chapter mark verification modes.
const (
	VerifyAdjustChapterMarksNo VerifyAdjustChapterMarks = iota
	VerifyAdjustChapterMarksSplitChapterOrTimeMode
	VerifyAdjustChapterMarksAllModes
)

var verifyAdjustChapterMarksNames = [...]string{"no", "splitChapterOrTimeMode", "allModes"}

// String returns the C# name of the value.
func (v VerifyAdjustChapterMarks) String() string {
	if int(v) < 0 || int(v) >= len(verifyAdjustChapterMarksNames) {
		return "no"
	}
	return verifyAdjustChapterMarksNames[v]
}

// ParseVerifyAdjustChapterMarks parses a C# enum name.
func ParseVerifyAdjustChapterMarks(s string) (VerifyAdjustChapterMarks, bool) {
	for i, n := range verifyAdjustChapterMarksNames {
		if n == s {
			return VerifyAdjustChapterMarks(i), true
		}
	}
	return 0, false
}

// FixAACEncoding selects when the AAC esds atom is patched.
type FixAACEncoding int

// AAC fix modes.
const (
	FixAACEncodingNo FixAACEncoding = iota
	FixAACEncodingWithIntermediateCopy
	FixAACEncodingAllModes
)

var fixAACEncodingNames = [...]string{"no", "withIntermediateCopy", "allModes"}

// String returns the C# name of the value.
func (v FixAACEncoding) String() string {
	if int(v) < 0 || int(v) >= len(fixAACEncodingNames) {
		return "no"
	}
	return fixAACEncodingNames[v]
}

// ParseFixAACEncoding parses a C# enum name.
func ParseFixAACEncoding(s string) (FixAACEncoding, bool) {
	for i, n := range fixAACEncodingNames {
		if n == s {
			return FixAACEncoding(i), true
		}
	}
	return 0, false
}

// ReducedBitRate caps the output bit rate.
type ReducedBitRate int

// Bit rate caps. "off" keeps the source bit rate.
const (
	ReducedBitRateOff ReducedBitRate = iota
	ReducedBitRate128k
	ReducedBitRate96k
	ReducedBitRate64k
	ReducedBitRate48k
	ReducedBitRate32k
	ReducedBitRate24k
)

var reducedBitRateNames = [...]string{
	"off", "_128k", "_96k", "_64k", "_48k", "_32k", "_24k",
}

// String returns the C# name of the value.
func (v ReducedBitRate) String() string {
	if int(v) < 0 || int(v) >= len(reducedBitRateNames) {
		return "off"
	}
	return reducedBitRateNames[v]
}

// ParseReducedBitRate parses a C# enum name.
func ParseReducedBitRate(s string) (ReducedBitRate, bool) {
	for i, n := range reducedBitRateNames {
		if n == s {
			return ReducedBitRate(i), true
		}
	}
	return 0, false
}

// PreferEmbeddedChapterTimes selects whether the container's own chapter times
// override the sidecar's.
type PreferEmbeddedChapterTimes int

// Embedded time preferences.
const (
	PreferEmbeddedChapterTimesNo PreferEmbeddedChapterTimes = iota
	PreferEmbeddedChapterTimesIfSilent
	PreferEmbeddedChapterTimesAlways
)

var preferEmbeddedChapterTimesNames = [...]string{"no", "ifSilent", "always"}

// String returns the C# name of the value.
func (v PreferEmbeddedChapterTimes) String() string {
	if int(v) < 0 || int(v) >= len(preferEmbeddedChapterTimesNames) {
		return "no"
	}
	return preferEmbeddedChapterTimesNames[v]
}

// ParsePreferEmbeddedChapterTimes parses a C# enum name.
func ParsePreferEmbeddedChapterTimes(s string) (PreferEmbeddedChapterTimes, bool) {
	for i, n := range preferEmbeddedChapterTimesNames {
		if n == s {
			return PreferEmbeddedChapterTimes(i), true
		}
	}
	return 0, false
}

// RoleTag selects which metadata field a role is written to.
type RoleTag int

// Metadata fields.
const (
	RoleTagNone RoleTag = iota
	RoleTagArtist
	RoleTagAlbumArtist
	RoleTagComposer
	RoleTagConductor
)

var roleTagNames = [...]string{"none", "artist", "albumArtist", "composer", "conductor"}

// String returns the C# name of the value.
func (v RoleTag) String() string {
	if int(v) < 0 || int(v) >= len(roleTagNames) {
		return "none"
	}
	return roleTagNames[v]
}

// ParseRoleTag parses a C# enum name.
func ParseRoleTag(s string) (RoleTag, bool) {
	for i, n := range roleTagNames {
		if n == s {
			return RoleTag(i), true
		}
	}
	return 0, false
}

// RoleTagAssignment selects what goes into a metadata field.
type RoleTagAssignment int

// Role assignments.
const (
	RoleTagAssignmentNone                     RoleTagAssignment = 0
	RoleTagAssignmentAuthor                   RoleTagAssignment = 1
	RoleTagAssignmentAuthorNarratorDeprecated RoleTagAssignment = 2
	RoleTagAssignmentAuthorNarrator           RoleTagAssignment = 3
	RoleTagAssignmentNarratorDeprecated       RoleTagAssignment = 4
	RoleTagAssignmentNarrator                 RoleTagAssignment = 5
)

var roleTagAssignmentNames = map[RoleTagAssignment]string{
	RoleTagAssignmentNone:                     "none",
	RoleTagAssignmentAuthor:                   "author",
	RoleTagAssignmentAuthorNarratorDeprecated: "author__narrator__",
	RoleTagAssignmentAuthorNarrator:           "author_narrator",
	RoleTagAssignmentNarratorDeprecated:       "__narrator__",
	RoleTagAssignmentNarrator:                 "narrator",
}

// String returns the C# name of the value.
func (v RoleTagAssignment) String() string {
	if n, ok := roleTagAssignmentNames[v]; ok {
		return n
	}
	return "none"
}

// ParseRoleTagAssignment parses a C# enum name.
func ParseRoleTagAssignment(s string) (RoleTagAssignment, bool) {
	for v, n := range roleTagAssignmentNames {
		if n == s {
			return v, true
		}
	}
	return 0, false
}

// OutFolderConflict selects what happens when the target folder is not empty.
type OutFolderConflict int

// Folder conflict behaviours.
const (
	OutFolderConflictAsk OutFolderConflict = iota
	OutFolderConflictOverwrite
	OutFolderConflictNewFolder
	OutFolderConflictSkip
)

var outFolderConflictNames = [...]string{"ask", "overwrite", "new_folder", "skip"}

// String returns the C# name of the value.
func (v OutFolderConflict) String() string {
	if int(v) < 0 || int(v) >= len(outFolderConflictNames) {
		return "ask"
	}
	return outFolderConflictNames[v]
}

// ParseOutFolderConflict parses a C# enum name.
func ParseOutFolderConflict(s string) (OutFolderConflict, bool) {
	for i, n := range outFolderConflictNames {
		if n == s {
			return OutFolderConflict(i), true
		}
	}
	return 0, false
}

// PartsType describes how complete a multi-part book is.
type PartsType int

// Part completeness.
const (
	// PartsNone is a book that has no numbered parts.
	PartsNone PartsType = iota
	// PartsSome is a book with an incomplete set of parts. Each part is an
	// independent book.
	PartsSome
	// PartsAll is a book whose parts are complete and are merged.
	PartsAll
)

var partsTypeNames = [...]string{"none", "some", "all"}

// String returns the C# name of the value.
func (v PartsType) String() string {
	if int(v) < 0 || int(v) >= len(partsTypeNames) {
		return "none"
	}
	return partsTypeNames[v]
}

// TrackDuration bounds the track length in the split modes.
const (
	// TrackDurationMin is the shortest permitted track, in minutes.
	TrackDurationMin = 3
	// TrackDurationMaxSplitChapter is the longest track in the split-chapter
	// mode, in minutes.
	TrackDurationMaxSplitChapter = 15
	// TrackDurationMaxTimeSplit is the longest track in the split-time mode, in
	// minutes.
	TrackDurationMaxTimeSplit = 90
)

// SplitTrim splits a value on the given separators and trims each part,
// dropping the empty ones.
func SplitTrim(value string, separators ...rune) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if len(separators) == 0 {
		separators = []rune{',', ';'}
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		for _, s := range separators {
			if r == s {
				return true
			}
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if t := strings.TrimSpace(f); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Combine joins the values with the given separator, skipping the blanks.
func Combine(values []string, separator rune) string {
	var sb strings.Builder
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteRune(separator)
			sb.WriteRune(' ')
		}
		sb.WriteString(v)
	}
	return sb.String()
}
