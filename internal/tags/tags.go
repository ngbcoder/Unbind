// Package tags reads and writes the metadata of audio files.
//
// The C# original used the ATL library (z440.atl.core) for both MP3 and MP4
// containers. In Go the responsibilities are split:
//
//   - MP3: the bogem/id3v2 library, which supports ID3v2.3/2.4, cover art,
//     custom TXXX frames and native CHAP chapter frames.
//   - MP4: FFmpeg writes the metadata and the chapters during the transcode,
//     so the Go side only needs to read the source container.
//   - The source .aax is read with go-mp4, which exposes the ilst atoms and
//     the freeform ---- atoms that carry narrator, publisher and description.
package tags

import (
	"github.com/ngbcoder/Unbind/internal/domain"
)

// Reader reads metadata from an audio file.
type Reader interface {
	// Read fills the AaxFileItem with the metadata from the file.
	Read(item *domain.AaxFileItem) error
}

// Writer writes metadata to an audio file.
type Writer interface {
	// Write applies the metadata to the file.
	Write(item *domain.AaxFileItem, book *domain.Book, track *domain.Track, withChapters bool) error
}

// AudioQuality is the small interface that an input file satisfies for the
// bit-rate handling. It mirrors the C# IAudioQuality.
type AudioQuality interface {
	SampleRate() uint
	AvgBitRate() uint
}

// AaxFileItemQuality adapts an AaxFileItem to AudioQuality.
func AaxFileItemQuality(item *domain.AaxFileItem) AudioQuality {
	return aaxQuality{item: item}
}

type aaxQuality struct{ item *domain.AaxFileItem }

func (q aaxQuality) SampleRate() uint { return q.item.SampleRate }
func (q aaxQuality) AvgBitRate() uint { return q.item.AvgBitRate }
