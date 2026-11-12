package util

import (
	"strings"
	"testing"
)

func TestPruneRemovesInvalidCharacters(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"the Windows reserved punctuation", `a<b>c:d"e/f\g|h?i*j`, "abcdefghij"},
		{"a control character", "a\x01b", "ab"},
		{"a null byte", "a\x00b", "ab"},
		{"nothing to remove", "Plain Title", "Plain Title"},
		{"an empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Prune(tc.in); got != tc.want {
				t.Errorf("Prune(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPruneRemovesDoubtfulCharacters(t *testing.T) {
	// The doubtful set is the curated list of exotic typographic characters.
	// The euro sign is deliberately not in it, matching the C# original.
	for _, c := range []rune{'™', '—', '–', '…', '«', '»', '¡', '¿', '½', '¶'} {
		in := "a" + string(c) + "b"
		if got := Prune(in); got != "ab" {
			t.Errorf("Prune(%q) = %q, want %q", in, got, "ab")
		}
	}
	if got := Prune("a€b"); got != "a€b" {
		t.Errorf("Prune() = %q, want the euro sign to survive", got)
	}
}

func TestPruneTrimsTrailingDots(t *testing.T) {
	if got := Prune("file..."); got != "file" {
		t.Errorf("Prune() = %q, want %q", got, "file")
	}
	// a dot in the middle is kept
	if got := Prune("a.b"); got != "a.b" {
		t.Errorf("Prune() = %q, want %q", got, "a.b")
	}
}

func TestPruneKeepsUnicodeLetters(t *testing.T) {
	if got := Prune("Книга Подзаголовок"); got != "Книга Подзаголовок" {
		t.Errorf("Prune() = %q, want the Cyrillic text to survive", got)
	}
	// the em dash is doubtful and is removed, but the spaces around it stay
	if got := Prune("Книга — Подзаголовок"); got != "Книга  Подзаголовок" {
		t.Errorf("Prune() = %q, want %q", got, "Книга  Подзаголовок")
	}
}

func TestSplitTrim(t *testing.T) {
	got := SplitTrim("a; b , c ;; d")
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitTrimEmpty(t *testing.T) {
	if got := SplitTrim("   "); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestCombine(t *testing.T) {
	got := Combine([]string{"a", "", "b", "  "}, ';')
	if got != "a; b" {
		t.Errorf("got %q, want %q", got, "a; b")
	}
}

func TestShorten(t *testing.T) {
	long := "The quick brown fox jumps over the lazy dog and keeps on running"
	got := Shorten(long, 20)
	if len([]rune(got)) > 20 {
		t.Errorf("Shorten() = %q, %d runes, want at most 20", got, len([]rune(got)))
	}
	if !strings.Contains(got, "…") {
		t.Errorf("Shorten() = %q, want an ellipsis", got)
	}
	if !strings.HasPrefix(got, "The quick") {
		t.Errorf("Shorten() = %q, want it to keep the head", got)
	}
}

func TestShortenShortString(t *testing.T) {
	if got := Shorten("short", 20); got != "short" {
		t.Errorf("Shorten() = %q, want %q", got, "short")
	}
}
