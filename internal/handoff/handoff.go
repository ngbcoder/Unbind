// Package handoff implements the receiving side of the inter-application
// handoff contract.
//
// The full specification lives in URIhandoff.md, which is kept in both this
// repository and the Unseal repository. The sender is Unseal, the Go rewrite
// of Book Lib Connect; it hands the exported .aax files to Unbind so that they
// can be converted.
//
// Nothing here is Windows-specific. The URI is the wire format and the
// document is the payload; the local endpoint that delivers a request to an
// already-running instance is a separate concern.
package handoff

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Scheme is the URI scheme of the protocol.
const Scheme = "unbind"

// Protocol is the version of the handoff document.
const Protocol = 1

// MaxDocumentSize is the largest handoff document that is accepted.
const MaxDocumentSize = 8 << 20

// The verbs.
const (
	// VerbAdd adds files to the receiver's file list.
	VerbAdd = "add"
	// VerbShow brings the receiver's window to the front.
	VerbShow = "show"
)

// The parameter names.
const (
	paramPath     = "p"
	paramDoc      = "doc"
	paramActivate = "activate"
	paramSource   = "src"
)

// Errors returned by the parser and the validator.
var (
	// ErrNotAURI is returned when the string is not a handoff URI.
	ErrNotAURI = errors.New("not a handoff URI")
	// ErrUnknownVerb is returned for a verb the receiver does not know.
	ErrUnknownVerb = errors.New("unknown verb")
	// ErrUnsupportedProtocol is returned for a document that is too new.
	ErrUnsupportedProtocol = errors.New("unsupported protocol version")
	// ErrDocumentTooLarge is returned for a document above the size cap.
	ErrDocumentTooLarge = errors.New("document too large")
)

// Request is a parsed handoff URI.
type Request struct {
	// Verb is the operation: "add" or "show".
	Verb string

	// Paths are the absolute paths of the files to add.
	Paths []string

	// Doc is the absolute path of a handoff document.
	Doc string

	// Activate asks the receiver to raise and focus its window.
	Activate bool

	// Source identifies the sender, for example "unseal".
	Source string
}

// IsURI reports whether the string is a handoff URI. The comparison is
// case-insensitive, because Windows treats the scheme that way.
func IsURI(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), Scheme+"://")
}

// ParseURI parses a handoff URI.
//
// An unknown verb is an error rather than a silent no-op: the sender would
// otherwise believe that something happened.
func ParseURI(raw string) (*Request, error) {
	if !IsURI(raw) {
		return nil, ErrNotAURI
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotAURI, err)
	}
	if !strings.EqualFold(u.Scheme, Scheme) {
		return nil, ErrNotAURI
	}

	verb := strings.ToLower(u.Host)
	if verb == "" {
		verb = strings.ToLower(u.Opaque)
	}
	switch verb {
	case VerbAdd, VerbShow:
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownVerb, verb)
	}

	q := u.Query()
	req := &Request{
		Verb:     verb,
		Doc:      q.Get(paramDoc),
		Activate: true,
		Source:   q.Get(paramSource),
	}

	// The activate parameter defaults to true. Only an explicit false turns
	// the activation off.
	if v := q.Get(paramActivate); v != "" {
		req.Activate = v != "0" && !strings.EqualFold(v, "false")
	}

	for _, p := range q[paramPath] {
		if p != "" {
			req.Paths = append(req.Paths, p)
		}
	}

	return req, nil
}

// Document is the handoff document.
//
// It carries the files and the metadata that the sender knows about. The
// receiver treats it as a supplement: the file's own tags win, and the
// document fills the gaps.
type Document struct {
	// Protocol is the document version.
	Protocol int `json:"protocol"`

	// Source identifies the sender.
	Source string `json:"source"`

	// Created is the RFC 3339 creation time, optional.
	Created string `json:"created,omitempty"`

	// Files are the files to add.
	Files []File `json:"files"`

	// Options are the optional overrides for the receiver's settings.
	Options *Options `json:"options,omitempty"`
}

// File is one file in the handoff document.
type File struct {
	// Path is the absolute path of the file. It must exist.
	Path string `json:"path"`

	// ASIN is the Amazon standard item number.
	ASIN string `json:"asin,omitempty"`

	// Title is the book title.
	Title string `json:"title,omitempty"`

	// Authors are the author names.
	Authors []string `json:"authors,omitempty"`

	// Narrators are the narrator names.
	Narrators []string `json:"narrators,omitempty"`

	// Series is the series title.
	Series string `json:"series,omitempty"`

	// SeriesSequence is the position in the series, for example "1.5".
	SeriesSequence string `json:"seriesSequence,omitempty"`

	// CoverPath is an absolute path to a cover image. It is used only when the
	// file carries no embedded cover.
	CoverPath string `json:"coverPath,omitempty"`

	// DurationMs is the running time in milliseconds.
	DurationMs int64 `json:"durationMs,omitempty"`
}

// Options are the optional overrides for the receiver's settings.
type Options struct {
	// Format is "mp3" or "mp4".
	Format string `json:"format,omitempty"`

	// Mode is "single", "chapters", "splitChapters" or "splitTime".
	Mode string `json:"mode,omitempty"`
}

// LoadDocument reads and parses a handoff document.
//
// The document is opened read-only. It is not deleted: the sender owns its
// temporary directory and cleans it up.
func LoadDocument(path string) (*Document, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > MaxDocumentSize {
		return nil, fmt.Errorf("%w: %d bytes", ErrDocumentTooLarge, fi.Size())
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc Document
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("the handoff document is not valid JSON: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Validate checks the document version.
func (d *Document) Validate() error {
	if d.Protocol > Protocol {
		return fmt.Errorf("%w: %d, this build supports up to %d", ErrUnsupportedProtocol, d.Protocol, Protocol)
	}
	return nil
}

// AllowedExtensions are the file extensions the receiver accepts.
var AllowedExtensions = []string{".aax", ".aa", ".m4b", ".m4a", ".mp4"}

// IsAllowedExtension reports whether the extension is one the receiver takes.
func IsAllowedExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range AllowedExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

// Resolve validates the files of the request and returns the ones that can be
// used.
//
// A file is dropped when its path is not absolute, when it does not exist, or
// when its extension is not accepted. The dropped files are returned
// separately so that the receiver can report the count.
func Resolve(req *Request, doc *Document, existing []string) (accepted []string, skipped int) {
	seen := map[string]bool{}
	for _, p := range existing {
		seen[normalizePath(p)] = true
	}

	add := func(p string) {
		if p == "" {
			skipped++
			return
		}
		if !filepath.IsAbs(p) {
			skipped++
			return
		}
		if !IsAllowedExtension(p) {
			skipped++
			return
		}
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			skipped++
			return
		}
		key := normalizePath(p)
		if seen[key] {
			skipped++
			return
		}
		seen[key] = true
		accepted = append(accepted, p)
	}

	for _, p := range req.Paths {
		add(p)
	}

	if doc != nil {
		for _, f := range doc.Files {
			add(f.Path)
		}
	}

	return accepted, skipped
}

// normalizePath returns the comparison key of a path. Windows paths are
// case-insensitive; POSIX paths are not.
func normalizePath(p string) string {
	p = filepath.Clean(p)
	if isWindowsPath(p) {
		return strings.ToLower(p)
	}
	return p
}

// isWindowsPath reports whether the path looks like a Windows path.
func isWindowsPath(p string) bool {
	return len(p) >= 2 && p[1] == ':'
}
