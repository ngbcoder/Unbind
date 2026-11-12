package domain

import (
	"testing"

	"github.com/ngbcoder/Unbind/internal/settings"
)

// bookWithParts returns a book with the given part numbers.
func bookWithParts(numbers ...int) *Book {
	b := NewBook("Test")
	for i, n := range numbers {
		fi := &AaxFileItem{BookTitle: "Test"}
		b.AddPart(fi, n)
		_ = i
	}
	return b
}

func TestCheckParts(t *testing.T) {
	tests := []struct {
		name  string
		parts []int
		want  settings.PartsType
	}{
		{"a single standalone file", []int{0}, settings.PartsNone},
		{"a complete three-part book", []int{1, 2, 3}, settings.PartsAll},
		{"a complete two-part book", []int{1, 2}, settings.PartsAll},
		{"a complete four-part book", []int{1, 2, 3, 4}, settings.PartsAll},
		{"only part two", []int{2}, settings.PartsSome},
		{"a missing part three", []int{1, 2, 4}, settings.PartsSome},
		{"only part one", []int{1}, settings.PartsSome},
		{"parts out of order", []int{3, 1, 2}, settings.PartsAll},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := bookWithParts(tc.parts...)
			if got := b.PartsType; got != tc.want {
				t.Errorf("PartsType = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckPartsSortsByNumber(t *testing.T) {
	b := bookWithParts(3, 1, 2)
	if b.Parts[0].PartNumber != 1 {
		t.Errorf("the parts were not sorted: first part number = %d", b.Parts[0].PartNumber)
	}
}

func TestSeqString(t *testing.T) {
	tests := []struct {
		name     string
		seq      string
		numDigit byte
		want     string
	}{
		{"a plain number", "1", 2, "01"},
		{"a number with a fraction", "1.5", 2, "01.5"},
		{"a fraction of zero", "2.0", 2, "02.0"},
		{"one digit", "7", 1, "7"},
		{"three digits", "123", 3, "123"},
		{"a non-numeric sequence", "abc", 2, "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &ExternalBookMeta{
				Series: []SeriesEntry{{Title: "S", Seq: tc.seq}},
			}
			if got := m.SeqString(tc.numDigit); got != tc.want {
				t.Errorf("SeqString(%d) = %q, want %q", tc.numDigit, got, tc.want)
			}
		})
	}
}

func TestSeqStringNoSeries(t *testing.T) {
	m := &ExternalBookMeta{}
	if got := m.SeqString(2); got != "" {
		t.Errorf("SeqString() = %q, want an empty string", got)
	}
	if got := m.SeqStringFramed(2); got != "" {
		t.Errorf("SeqStringFramed() = %q, want an empty string", got)
	}
}

func TestSeqStringFramed(t *testing.T) {
	m := &ExternalBookMeta{
		Series: []SeriesEntry{{Title: "S", Seq: "1"}},
	}
	if got := m.SeqStringFramed(2); got != "[01]" {
		t.Errorf("SeqStringFramed() = %q, want %q", got, "[01]")
	}
}

func TestPruneTitle(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Book: Subtitle / Part", "Book; Subtitle; Part"},
		{"Plain Title", "Plain Title"},
		{"With™Doubtful", "WithDoubtful"},
	}
	for _, tc := range tests {
		if got := PruneTitle(tc.in); got != tc.want {
			t.Errorf("PruneTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestInitAuthorTitle(t *testing.T) {
	s := settings.DefaultSettings()

	b := NewBook("The Book - Subtitle")
	b.AddPart(&AaxFileItem{BookTitle: "The Book - Subtitle", Authors: []string{"Jane Doe"}}, 0)

	b.InitAuthorTitle(s)

	if got := b.TagCaption.Title; got != "The Book" {
		t.Errorf("TagCaption.Title = %q, want %q", got, "The Book")
	}
	if got := b.FileCaption.Title; got != "The Book" {
		t.Errorf("FileCaption.Title = %q, want %q", got, "The Book")
	}
	if got := b.TagCaption.Author; got != "Jane Doe" {
		t.Errorf("TagCaption.Author = %q, want %q", got, "Jane Doe")
	}
}

func TestInitAuthorTitleKeepsSubtitleWhenHyphenIsCustomPunctuation(t *testing.T) {
	s := settings.DefaultSettings()
	s.AddnlValTitlePunct = "-"

	b := NewBook("The Book - Subtitle")
	b.AddPart(&AaxFileItem{BookTitle: "The Book - Subtitle", Authors: []string{"Jane Doe"}}, 0)

	b.InitAuthorTitle(s)

	if got := b.TagCaption.Title; got != "The Book - Subtitle" {
		t.Errorf("TagCaption.Title = %q, want %q", got, "The Book - Subtitle")
	}
}

func TestInitAuthorTitleHandlesNonLatinTitles(t *testing.T) {
	// .NET's \w is Unicode-aware but Go's is ASCII-only. A title in a
	// non-Latin script must not be truncated at its first character.
	s := settings.DefaultSettings()

	b := NewBook("Длинная книга - Подзаголовок")
	b.AddPart(&AaxFileItem{BookTitle: "Длинная книга - Подзаголовок", Authors: []string{"Иван Иванов"}}, 0)

	b.InitAuthorTitle(s)

	if got := b.TagCaption.Title; got != "Длинная книга" {
		t.Errorf("TagCaption.Title = %q, want %q", got, "Длинная книга")
	}
}

func TestApplicableInFileNameForTranscode(t *testing.T) {
	b := NewBook("Test")
	p := NewPart(b, &AaxFileItem{FileName: "original.aax"}, 0)

	if got := p.ApplicableInFileNameForTranscode(); got != "original.aax" {
		t.Errorf("got %q, want the original file", got)
	}

	p.TmpFileNameAACFixed = "fixed.aax"
	if got := p.ApplicableInFileNameForTranscode(); got != "fixed.aax" {
		t.Errorf("got %q, want the AAC-fixed file", got)
	}

	p.TmpFileName = "decrypted.aax"
	if got := p.ApplicableInFileNameForTranscode(); got != "decrypted.aax" {
		t.Errorf("got %q, want the decrypted file", got)
	}
}
