package domain

import (
	"strings"
	"time"
)

// AaxFileItem is the record of one input file.
type AaxFileItem struct {
	// FileName is the absolute path of the input file.
	FileName string

	// HasExtAA is true for the legacy ".aa" extension.
	HasExtAA bool

	// IsAA is true when FFmpeg reports the legacy container.
	IsAA bool

	// BookTitle is the sorting key for grouping files into books.
	BookTitle string

	// Authors is the list of authors.
	Authors []string

	// Narrators is the list of narrators.
	Narrators []string

	// Duration is the total running time.
	Duration time.Duration

	// SampleRate is the audio sample rate in Hz.
	SampleRate uint

	// Channels is the audio channel count.
	Channels uint

	// AvgBitRate is the average bit rate in kbit/s.
	AvgBitRate uint

	// Cover is the embedded cover image.
	Cover []byte

	// CoverExt is the cover image file extension, ".jpg" or ".png".
	CoverExt string

	// Genre is the resolved genre.
	Genre string

	// Publisher is the publisher.
	Publisher string

	// Copyright is the copyright string.
	Copyright string

	// Abstract is the book description.
	Abstract string

	// PublishingDate is the publication date, when known.
	PublishingDate *time.Time

	// ContentMetadataFile is the located content_metadata_<ASIN>.json.
	ContentMetadataFile *AsinJsonFile

	// SimsBySeriesFile is the located series sidecar.
	SimsBySeriesFile *AsinJsonFile

	// CustomNames holds the per-book overrides from the preview dialog.
	CustomNames *CustomTagFileNames

	// Converted is true once the file has been converted successfully.
	Converted bool
}

// Author returns the authors joined with "; ".
func (a *AaxFileItem) Author() string {
	return strings.Join(a.Authors, "; ")
}

// Narrator returns the narrators joined with "; ".
func (a *AaxFileItem) Narrator() string {
	return strings.Join(a.Narrators, "; ")
}

// AsinJsonFile is a located Audible sidecar file.
type AsinJsonFile struct {
	// Filename is the absolute path of the sidecar.
	Filename string

	// ASIN is the Amazon standard item number it belongs to.
	ASIN string
}

// CustomTagFileNames holds the per-book overrides from the preview dialog.
type CustomTagFileNames struct {
	AuthorTag  string
	AuthorFile string
	TitleTag   string
	TitleFile  string
	Year       string
	Genre      string
}
