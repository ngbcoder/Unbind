package naming

import (
	"testing"

	"github.com/ngbcoder/Unbind/internal/domain"
	"github.com/ngbcoder/Unbind/internal/settings"
)

func TestDigits(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{0, 1},
		{1, 1},
		{9, 1},
		{10, 2},
		{99, 2},
		{100, 3},
		{1234, 4},
		{-42, 2},
	}
	for _, tc := range tests {
		if got := Digits(tc.in); got != tc.want {
			t.Errorf("Digits(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestStr(t *testing.T) {
	tests := []struct {
		n      int
		digits int
		want   string
	}{
		{1, 2, "01"},
		{12, 2, "12"},
		{123, 2, "123"},
		{0, 3, "000"},
		{7, 0, "7"},
	}
	for _, tc := range tests {
		if got := Str(tc.n, tc.digits); got != tc.want {
			t.Errorf("Str(%d, %d) = %q, want %q", tc.n, tc.digits, got, tc.want)
		}
	}
}

// newBook builds a book with one part and the given tracks.
func newBook(partsType settings.PartsType, partNumbers []int) *domain.Book {
	b := domain.NewBook("The Book")
	for _, pn := range partNumbers {
		fi := &domain.AaxFileItem{
			BookTitle: "The Book",
			Authors:   []string{"Jane Doe"},
			Genre:     "Fiction",
		}
		b.AddPart(fi, pn)
	}
	b.PartsType = partsType
	return b
}

// newChapteredBook builds a book whose single part carries the given chapters.
func newChapteredBook(t *testing.T, chapters ...string) (*domain.Book, *domain.Part) {
	t.Helper()
	b := newBook(settings.PartsNone, []int{0})
	p := b.Parts[0]
	for _, name := range chapters {
		ch := domain.NewChapterDuration(60 * 1e9)
		ch.Name = name
		p.Chapters = append(p.Chapters, ch)
	}
	return b, p
}

func TestFileNamingMatrix(t *testing.T) {
	tests := []struct {
		name        string
		fileNaming  settings.FileNaming
		partsType   settings.PartsType
		partNumbers []int
		mode        settings.ConvMode
		want        string
	}{
		{"track only", settings.FileNamingTrack, settings.PartsNone, []int{0}, settings.ConvModeChapters, "1"},
		{"track and book", settings.FileNamingTrackBook, settings.PartsNone, []int{0}, settings.ConvModeChapters, "1 - The Book"},
		{"track, book and author", settings.FileNamingTrackBookAuthor, settings.PartsNone, []int{0}, settings.ConvModeChapters, "1 - The Book - Jane Doe"},
		{"author, book and track", settings.FileNamingAuthorBookTrack, settings.PartsNone, []int{0}, settings.ConvModeChapters, "Jane Doe - The Book - 1"},
		{"book and track", settings.FileNamingBookTrack, settings.PartsNone, []int{0}, settings.ConvModeChapters, "The Book - 1"},
		{"single mode, no parts", settings.FileNamingTrackBookAuthor, settings.PartsNone, []int{0}, settings.ConvModeSingle, "The Book - Jane Doe"},
		{"single mode, author book track", settings.FileNamingAuthorBookTrack, settings.PartsNone, []int{0}, settings.ConvModeSingle, "Jane Doe - The Book"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := settings.DefaultSettings()
			s.FileNaming = tc.fileNaming
			s.ConvMode = tc.mode
			s.NamedChapters = settings.NamedChaptersNo

			b := newBook(tc.partsType, tc.partNumbers)
			b.InitAuthorTitle(s)
			b.OutDirectoryLong = `C:\out`

			p := b.Parts[0]
			ch := domain.NewChapterDuration(60 * 1e9)
			ch.Name = "Chapter 1"
			p.Chapters = []*domain.Chapter{ch}
			tr := domain.NewTrackDuration(60 * 1e9)
			tr.Chapter = ch
			p.Tracks = []*domain.Track{tr}

			h := NewHelper(s, nil, b, p, tr, true)
			if got := h.FileName(); got != tc.want {
				t.Errorf("FileName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSingleModeWithPartsUsesThePartNumber(t *testing.T) {
	// An incomplete set of parts: the part number replaces the track.
	s := settings.DefaultSettings()
	s.ConvMode = settings.ConvModeSingle
	s.FileNaming = settings.FileNamingTrack
	s.NamedChapters = settings.NamedChaptersNo

	b := newBook(settings.PartsSome, []int{2})
	b.InitAuthorTitle(s)

	p := b.Parts[0]
	tr := domain.NewTrackDuration(60 * 1e9)
	p.Tracks = []*domain.Track{tr}

	h := NewHelper(s, nil, b, p, tr, true)
	if got := h.FileName(); got != "2" {
		t.Errorf("FileName() = %q, want %q", got, "2")
	}
}

func TestFullPathLayout(t *testing.T) {
	s := settings.DefaultSettings()
	s.ConvMode = settings.ConvModeChapters
	s.NamedChapters = settings.NamedChaptersNo
	s.FileNaming = settings.FileNamingTrack

	b := newBook(settings.PartsNone, []int{0})
	b.InitAuthorTitle(s)
	b.OutDirectoryLong = `C:\out`

	p := b.Parts[0]
	ch := domain.NewChapterDuration(60 * 1e9)
	ch.Name = "Chapter 1"
	p.Chapters = []*domain.Chapter{ch}
	tr := domain.NewTrackDuration(60 * 1e9)
	tr.Chapter = ch
	p.Tracks = []*domain.Track{tr}

	h := NewHelper(s, nil, b, p, tr, true)
	want := `C:\out\1.m4a`
	if got := h.FullPath(); got != want {
		t.Errorf("FullPath() = %q, want %q", got, want)
	}
}

func TestFullPathChapterFolder(t *testing.T) {
	s := settings.DefaultSettings()
	s.ConvMode = settings.ConvModeSplitChapters
	s.NamedChapters = settings.NamedChaptersNo
	s.FileNaming = settings.FileNamingTrack
	s.ChapterNaming = settings.GeneralNamingExStandard

	b := newBook(settings.PartsNone, []int{0})
	b.InitAuthorTitle(s)
	b.OutDirectoryLong = `C:\out`

	p := b.Parts[0]
	ch := domain.NewChapterDuration(60 * 1e9)
	ch.Name = "Chapter 1"
	p.Chapters = []*domain.Chapter{ch}
	tr := domain.NewTrackDuration(60 * 1e9)
	tr.Chapter = ch
	p.Tracks = []*domain.Track{tr}

	h := NewHelper(s, &Resources{ChapterNamePrefixStandard: "Chapter"}, b, p, tr, true)
	want := `C:\out\Chapter 1\1.m4a`
	if got := h.FullPath(); got != want {
		t.Errorf("FullPath() = %q, want %q", got, want)
	}
}

func TestFullPathNoChapterFolders(t *testing.T) {
	s := settings.DefaultSettings()
	s.ConvMode = settings.ConvModeSplitChapters
	s.NamedChapters = settings.NamedChaptersNo
	s.FileNaming = settings.FileNamingTrack
	s.ChapterNaming = settings.GeneralNamingExNoFolders

	b := newBook(settings.PartsNone, []int{0})
	b.InitAuthorTitle(s)
	b.OutDirectoryLong = `C:\out`

	p := b.Parts[0]
	ch := domain.NewChapterDuration(60 * 1e9)
	ch.Name = "Chapter 1"
	p.Chapters = []*domain.Chapter{ch}
	tr := domain.NewTrackDuration(60 * 1e9)
	tr.Chapter = ch
	p.Tracks = []*domain.Track{tr}

	h := NewHelper(s, nil, b, p, tr, true)
	want := `C:\out\1.m4a`
	if got := h.FullPath(); got != want {
		t.Errorf("FullPath() = %q, want %q", got, want)
	}
}

func TestNumbersLocalForIncompleteParts(t *testing.T) {
	s := settings.DefaultSettings()
	s.NamedChapters = settings.NamedChaptersNo

	b := newBook(settings.PartsSome, []int{1})
	p := b.Parts[0]
	for i := 0; i < 3; i++ {
		tr := domain.NewTrackDuration(60 * 1e9)
		p.Tracks = append(p.Tracks, tr)
	}

	n := NewNumbers(p.Tracks[1], p)
	if n.NTrk != 2 {
		t.Errorf("NTrk = %d, want 2", n.NTrk)
	}
	if n.NTrks != 3 {
		t.Errorf("NTrks = %d, want 3", n.NTrks)
	}
	if n.NDsk != 1 {
		t.Errorf("NDsk = %d, want 1", n.NDsk)
	}
	if n.NDsks != 0 {
		t.Errorf("NDsks = %d, want 0", n.NDsks)
	}
}

func TestNumbersGlobalAcrossParts(t *testing.T) {
	s := settings.DefaultSettings()
	s.NamedChapters = settings.NamedChaptersNo

	b := newBook(settings.PartsAll, []int{1, 2})
	p1, p2 := b.Parts[0], b.Parts[1]
	for i := 0; i < 3; i++ {
		p1.Tracks = append(p1.Tracks, domain.NewTrackDuration(60*1e9))
	}
	for i := 0; i < 2; i++ {
		p2.Tracks = append(p2.Tracks, domain.NewTrackDuration(60*1e9))
	}

	n := NewNumbers(p2.Tracks[0], p2)
	// three tracks in the first part, so the second part starts at four
	if n.NTrk != 4 {
		t.Errorf("NTrk = %d, want 4", n.NTrk)
	}
	if n.NTrks != 5 {
		t.Errorf("NTrks = %d, want 5", n.NTrks)
	}
	if n.NDsks != 2 {
		t.Errorf("NDsks = %d, want 2", n.NDsks)
	}
	if n.NDsk != 2 {
		t.Errorf("NDsk = %d, want 2", n.NDsk)
	}
}

func TestChapterDirectoryName(t *testing.T) {
	s := settings.DefaultSettings()
	s.ConvMode = settings.ConvModeSplitChapters
	s.ChapterNaming = settings.GeneralNamingExStandard
	s.NamedChapters = settings.NamedChaptersNo

	b, p := newChapteredBook(t, "Chapter 1")
	b.InitAuthorTitle(s)
	ch := p.Chapters[0]
	tr := domain.NewTrackDuration(60 * 1e9)
	tr.Chapter = ch
	p.Tracks = []*domain.Track{tr}

	h := NewHelper(s, &Resources{ChapterNamePrefixStandard: "Chapter"}, b, p, tr, true)
	if got := h.ChapterDirectoryName(); got != "Chapter 1" {
		t.Errorf("ChapterDirectoryName() = %q, want %q", got, "Chapter 1")
	}
}

func TestPartDirectoryName(t *testing.T) {
	s := settings.DefaultSettings()
	s.PartNaming = settings.GeneralNamingStandard

	b := newBook(settings.PartsSome, []int{3})
	b.InitAuthorTitle(s)
	p := b.Parts[0]
	tr := domain.NewTrackDuration(60 * 1e9)
	p.Tracks = []*domain.Track{tr}

	h := NewHelper(s, &Resources{PartNamePrefixStandard: "Part"}, b, p, tr, true)
	if got := h.PartDirectoryName(); got != "Part 3" {
		t.Errorf("PartDirectoryName() = %q, want %q", got, "Part 3")
	}
}

func TestGetGenre(t *testing.T) {
	s := settings.DefaultSettings()
	afi := &domain.AaxFileItem{Genre: "Fiction"}

	s.GenreNaming = settings.GeneralNamingSource
	if got := GetGenre(s, afi); got != "Fiction" {
		t.Errorf("GetGenre() = %q, want %q", got, "Fiction")
	}

	s.GenreNaming = settings.GeneralNamingCustom
	s.GenreName = "Audiobook"
	if got := GetGenre(s, afi); got != "Audiobook" {
		t.Errorf("GetGenre() = %q, want %q", got, "Audiobook")
	}

	// no genre anywhere falls back
	afi2 := &domain.AaxFileItem{}
	s.GenreNaming = settings.GeneralNamingSource
	if got := GetGenre(s, afi2); got != FallbackGenre {
		t.Errorf("GetGenre() = %q, want %q", got, FallbackGenre)
	}
}

func TestTrackForFileReplacesTheSlash(t *testing.T) {
	s := settings.DefaultSettings()
	s.NamedChapters = settings.NamedChaptersNo
	s.FileNaming = settings.FileNamingTrack
	s.TotalTracks = true

	b := newBook(settings.PartsNone, []int{0})
	b.InitAuthorTitle(s)
	p := b.Parts[0]
	tr := domain.NewTrackDuration(60 * 1e9)
	p.Tracks = []*domain.Track{tr}

	h := NewHelper(s, nil, b, p, tr, true)
	if got := h.TrackForFile(); got != "1,1" {
		t.Errorf("TrackForFile() = %q, want %q", got, "1,1")
	}
}
