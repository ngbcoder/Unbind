package tags

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	mp4 "github.com/abema/go-mp4"
	"github.com/ngbcoder/Unbind/internal/domain"
	"github.com/ngbcoder/Unbind/internal/util"
)

// MP4Reader reads metadata from an MP4 container (.aax, .m4a, .m4b) using
// the go-mp4 library.
type MP4Reader struct{}

// Read fills the AaxFileItem with the metadata from the file.
func (r *MP4Reader) Read(item *domain.AaxFileItem) error {
	f, err := os.Open(item.FileName)
	if err != nil {
		return err
	}
	defer f.Close()

	// Walk the box tree and collect metadata from the ilst box.
	type metaEntry struct {
		atomType string
		data     []byte
		dataType uint32
		mean     string
		name     string
	}

	var entries []metaEntry

	_, err = mp4.ReadBoxStructure(f, func(h *mp4.ReadHandle) (interface{}, error) {
		// Only collect boxes that are under the ilst.
		underIlst := false
		for _, p := range h.Path {
			if p == mp4.BoxTypeIlst() {
				underIlst = true
				break
			}
		}
		if !underIlst {
			_, err := h.Expand()
			return nil, err
		}

		// Direct child of ilst: an ilst meta container.
		if len(h.Path) > 0 && h.Path[len(h.Path)-1] == mp4.BoxTypeIlst() {
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, nil
			}
			imc, ok := box.(*mp4.IlstMetaContainer)
			if ok {
				entry := metaEntry{atomType: string(imc.Type[:])}
				entries = append(entries, entry)
			} else if it, ok := box.(*mp4.Item); ok {
				entry := metaEntry{
					atomType: string(it.ItemName),
					data:     it.Data.Data,
					dataType: it.Data.DataType,
				}
				entries = append(entries, entry)
			}
			// Expand to read mean/name/data children.
			children, err := h.Expand()
			if err != nil {
				return nil, nil
			}
			_ = children
			return nil, nil
		}

		// Under an ilst item: check for mean/name/data.
		if len(entries) > 0 {
			idx := len(entries) - 1
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, nil
			}
			if sd, ok := box.(*mp4.StringData); ok {
				if h.BoxInfo.Type == mp4.StrToBoxType("mean") {
					entries[idx].mean = string(sd.Data)
				} else if h.BoxInfo.Type == mp4.StrToBoxType("name") {
					entries[idx].name = string(sd.Data)
				}
			} else if d, ok := box.(*mp4.Data); ok {
				entries[idx].data = d.Data
				entries[idx].dataType = d.DataType
			}
		}
		return nil, nil
	})
	if err != nil {
		return err
	}

	// Process the collected entries.
	for _, e := range entries {
		data := string(e.data)
		switch e.atomType {
		case "\xa9nam":
			item.BookTitle = data
		case "aART":
			item.Authors = util.SplitTrim(data, ';')
		case "\xa9gen":
			item.Genre = data
		case "\xa9day":
			if t, err := parseDate(data); err == nil {
				item.PublishingDate = &t
			}
		case "cprt":
			item.Copyright = data
		case "covr":
			item.Cover = e.data
		case "----":
			switch strings.ToLower(e.name) {
			case "nrt", "narrator":
				item.Narrators = util.SplitTrim(data, ';')
			case "des", "description":
				if item.Abstract == "" {
					item.Abstract = data
				}
			case "pub", "publisher":
				if item.Publisher == "" {
					item.Publisher = data
				}
			case "rldt", "releasedate":
				if item.PublishingDate == nil {
					if t, err := parseDate(data); err == nil {
						item.PublishingDate = &t
					}
				}
			}
		case "desc":
			if item.Abstract == "" {
				item.Abstract = data
			}
		case "\xa9des":
			if item.Abstract == "" {
				item.Abstract = data
			}
		case "\xa9pub":
			if item.Publisher == "" {
				item.Publisher = data
			}
		}
	}

	// Cover extension detection.
	if item.Cover != nil && len(item.Cover) > 0 {
		if bytes.HasPrefix(item.Cover, []byte{0x89, 'P', 'N', 'G'}) {
			item.CoverExt = ".png"
		} else {
			item.CoverExt = ".jpg"
		}
	}

	return nil
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006",
		"01/02/2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date %q", s)
}
