// Package util holds the pure string and path helpers shared by the domain
// and the naming layers.
//
// They live in their own package so that the domain and the naming code can
// both use them without an import cycle.
package util

import "strings"

// invalidFileNameChars are the characters that Windows forbids in a file name.
// It mirrors Path.GetInvalidFileNameChars on .NET: the control characters plus
// the reserved punctuation.
var invalidFileNameChars = func() map[rune]bool {
	m := map[rune]bool{}
	for _, c := range []rune{'"', '<', '>', '|', 0, ':', '*', '?', '\\', '/'} {
		m[c] = true
	}
	for c := rune(1); c < 32; c++ {
		m[c] = true
	}
	return m
}()

// doubtfulFileNameChars is the curated set of exotic typographic characters
// that the C# original strips from names (AuxLib/ExtensionMethods.cs:84-89).
//
// These are not illegal in a Windows file name, but they cause trouble in
// shells, in synchronisation tools and on other platforms.
var doubtfulFileNameChars = func() map[rune]bool {
	m := map[rune]bool{}
	const doubtful = "¡¢£¤¥¦§¨©ª«¬®¯°±" +
		"²³´µ¶·¸¹º»¼½¾¿×÷" +
		"‘’ƒ„…†‡ˆ‰‹’“”•–—" +
		"˜™›"
	for _, c := range doubtful {
		m[c] = true
	}
	return m
}()

// Prune removes the characters that must not appear in a file or folder name:
// the ones Windows forbids, plus the doubtful typographic set, plus trailing
// dots.
//
// The C# original exposes this as the string extension Prune
// (AuxLib/ExtensionMethods.cs:112-118).
func Prune(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, c := range s {
		if invalidFileNameChars[c] {
			continue
		}
		if doubtfulFileNameChars[c] {
			continue
		}
		sb.WriteRune(c)
	}
	return strings.TrimRight(sb.String(), ".")
}

// IsNullOrWhiteSpace reports whether the string is empty or only whitespace.
func IsNullOrWhiteSpace(s string) bool {
	return strings.TrimSpace(s) == ""
}

// SplitTrim splits the value on the given separators and trims each part,
// dropping the empty ones.
func SplitTrim(value string, separators ...rune) []string {
	if IsNullOrWhiteSpace(value) {
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

// Combine joins the values with the separator, skipping the blanks.
func Combine(values []string, separator rune) string {
	var sb strings.Builder
	for _, v := range values {
		if IsNullOrWhiteSpace(v) {
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

// maxShortString is the default maximum length of Shorten.
const maxShortString = 40

// Shorten shortens the string to roughly maxlen characters, keeping the head
// and the tail and eliding the middle.
//
// It mirrors the C# original (AuxLib/ExtensionMethods.cs:133-146), which keeps
// two thirds at the front, one third at the back and joins them with a single
// ellipsis character.
func Shorten(s string, maxlen int) string {
	if s == "" {
		return s
	}
	if maxlen == 0 {
		maxlen = maxShortString
	}
	runes := []rune(s)
	if maxlen < 0 || len(runes) <= maxlen {
		return s
	}

	partLen1 := maxlen * 2 / 3
	partLen2 := maxlen - partLen1 - 1

	p2 := len(runes) - partLen2
	return strings.TrimSpace(string(runes[:partLen1])) + "…" + strings.TrimSpace(string(runes[p2:]))
}
