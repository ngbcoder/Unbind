package tags

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	id3v2 "github.com/bogem/id3v2/v2"
	"github.com/ngbcoder/Unbind/internal/domain"
	"github.com/ngbcoder/Unbind/internal/naming"
	"github.com/ngbcoder/Unbind/internal/settings"
	"github.com/ngbcoder/Unbind/internal/util"
)

var copyrightYearRegex = regexp.MustCompile(`\D(\d{4})\D`)

// MP3Reader reads metadata from an MP3 file using ID3v2 tags.
type MP3Reader struct{}

// Read fills the AaxFileItem with the metadata from the file.
func (r *MP3Reader) Read(item *domain.AaxFileItem) error {
	tag, err := id3v2.Open(item.FileName, id3v2.Options{Parse: true})
	if err != nil {
		return err
	}
	defer tag.Close()

	item.BookTitle = tag.Title()

	// The tag mapping differs between .aa and .aax files.
	if item.HasExtAA {
		// Legacy .aa: composer for author, artist for narrator.
		item.Authors = util.SplitTrim(getTextFrame(tag, "TCOM"), ';')
		item.Narrators = util.SplitTrim(tag.Artist(), ';')
		item.Abstract = getComment(tag)
		item.Publisher = getTextFrame(tag, "TPUB")
	} else {
		// .aax: album artist for author, custom tags for the rest.
		item.Authors = util.SplitTrim(getTextFrame(tag, "TPE2"), ';')
		item.Narrators = util.SplitTrim(customTag(tag, "nrt"), ';')
		item.Abstract = getComment(tag)
		if item.Abstract == "" {
			item.Abstract = customTag(tag, "des")
		}
		item.Publisher = getTextFrame(tag, "TPUB")
		if item.Publisher == "" {
			item.Publisher = customTag(tag, "pub")
		}
	}

	item.Genre = tag.Genre()

	// Year with the fallback chain.
	if y := tag.Year(); y != "" {
		if n, err := strconv.Atoi(y); err == nil {
			if n > 0 && n <= time.Now().Year()+2 {
				d := time.Date(n, 1, 1, 0, 0, 0, 0, time.UTC)
				item.PublishingDate = &d
			}
		}
	}
	if item.PublishingDate == nil {
		if rldt := customTag(tag, "rldt"); rldt != "" {
			if t, err := time.Parse("2006-01-02", rldt); err == nil {
				item.PublishingDate = &t
			} else if t, err := time.Parse("01/02/2006", rldt); err == nil {
				item.PublishingDate = &t
			}
		}
	}
	if item.PublishingDate == nil && item.Copyright != "" {
		if m := copyrightYearRegex.FindStringSubmatch(item.Copyright); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				d := time.Date(n, 1, 1, 0, 0, 0, 0, time.UTC)
				item.PublishingDate = &d
			}
		}
	}

	item.Copyright = getTextFrame(tag, "TCOP")

	// Cover image.
	for _, f := range tag.GetFrames("APIC") {
		if pf, ok := f.(id3v2.PictureFrame); ok {
			item.Cover = pf.Picture
			switch pf.MimeType {
			case "image/jpeg":
				item.CoverExt = ".jpg"
			case "image/png":
				item.CoverExt = ".png"
			}
			break
		}
	}

	return nil
}

// getTextFrame reads a text frame by its ID.
func getTextFrame(tag *id3v2.Tag, id string) string {
	tf := tag.GetTextFrame(id)
	return tf.Text
}

// getComment reads the first comment frame.
func getComment(tag *id3v2.Tag) string {
	for _, f := range tag.GetFrames("COMM") {
		if cf, ok := f.(id3v2.CommentFrame); ok {
			return cf.Text
		}
	}
	return ""
}

// customTag reads a custom TXXX tag with the given description.
func customTag(tag *id3v2.Tag, key string) string {
	if len(key) == 3 {
		key = "©" + key
	}
	for _, f := range tag.GetFrames("TXXX") {
		if udtf, ok := f.(id3v2.UserDefinedTextFrame); ok {
			if udtf.Description == key {
				return udtf.Value
			}
		}
	}
	return ""
}

// MP3Writer writes metadata to an MP3 file using ID3v2 tags.
type MP3Writer struct {
	Settings *settings.Settings
	Version  string
}

// Write applies the metadata to the file.
func (w *MP3Writer) Write(item *domain.AaxFileItem, book *domain.Book, track *domain.Track, withChapters bool) error {
	tag, err := id3v2.Open(track.FileName, id3v2.Options{Parse: true})
	if err != nil {
		return err
	}
	defer tag.Close()

	// Force ID3v2.3 on Windows 7 and 8 for compatibility.
	tag.SetVersion(4)

	tag.SetTitle(track.Title)
	tag.SetAlbum(bookTitleTag(book))
	tag.SetArtist(buildPerformerTag(w.Settings.TagArtist, bookAuthorTag(book), bookNarrators(book)))
	tag.AddTextFrame("TPE2", id3v2.EncodingUTF8, buildPerformerTag(w.Settings.TagAlbumArtist, bookAuthorTag(book), bookNarrators(book)))
	tag.AddTextFrame("TCOM", id3v2.EncodingUTF8, buildPerformerTag(w.Settings.TagComposer, bookAuthorTag(book), bookNarrators(book)))
	tag.AddTextFrame("TPE3", id3v2.EncodingUTF8, buildPerformerTag(w.Settings.TagConductor, bookAuthorTag(book), bookNarrators(book)))
	tag.AddCommentFrame(id3v2.CommentFrame{
		Encoding:    id3v2.EncodingUTF8,
		Language:    "eng",
		Description: "",
		Text:        bookAbstract(book),
	})
	if bookPublishingDate(book) != nil {
		tag.SetYear(fmt.Sprintf("%d", bookPublishingDate(book).Year()))
	}
	tag.SetGenre(bookGenre(book))
	tag.AddTextFrame("TCOP", id3v2.EncodingUTF8, bookCopyright(book))
	tag.AddTextFrame("TPUB", id3v2.EncodingUTF8, bookPublisher(book))

	// Disc and track numbers.
	part := GetTrackPart(track)
	if part != nil {
		n := naming.NewNumbers(track, part)
		tag.AddTextFrame("TPOS", id3v2.EncodingUTF8, fmt.Sprintf("%d/%d", n.NDsk, n.NDsks))
		tag.AddTextFrame("TRCK", id3v2.EncodingUTF8, fmt.Sprintf("%d/%d", n.NTrk, n.NTrks))
	}

	// Cover art.
	if item.Cover != nil {
		mime := "image/jpeg"
		if item.CoverExt == ".png" {
			mime = "image/png"
		}
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    mime,
			PictureType: id3v2.PTFrontCover,
			Description: "",
			Picture:     item.Cover,
		})
	}

	// Watermark.
	tag.AddUserDefinedTextFrame(id3v2.UserDefinedTextFrame{
		Encoding:    id3v2.EncodingUTF8,
		Description: "wmrk",
		Value:       "Processed by Unbind " + w.Version,
	})

	// Chapters.
	if withChapters && track.MetaChapters != nil {
		for i, ch := range track.MetaChapters {
			cf := id3v2.ChapterFrame{
				ElementID:   fmt.Sprintf("chp%d", i),
				StartTime:   ch.Time.Begin(),
				EndTime:     ch.Time.End(),
				StartOffset: id3v2.IgnoredOffset,
				EndOffset:   id3v2.IgnoredOffset,
				Title:       &id3v2.TextFrame{Encoding: id3v2.EncodingUTF8, Text: ch.Name},
			}
			tag.AddChapterFrame(cf)
		}
	}

	return tag.Save()
}

// Helper functions for the book.
func bookTitleTag(b *domain.Book) string {
	if b == nil {
		return ""
	}
	return b.TagCaption.Title
}

func bookAuthorTag(b *domain.Book) string {
	if b == nil {
		return ""
	}
	return b.TagCaption.Author
}

func bookNarrators(b *domain.Book) []string {
	if b == nil || len(b.Parts) == 0 {
		return nil
	}
	if b.Parts[0].AaxFileItem != nil {
		return b.Parts[0].AaxFileItem.Narrators
	}
	return nil
}

func bookAbstract(b *domain.Book) string {
	if b == nil || len(b.Parts) == 0 {
		return ""
	}
	if b.Parts[0].AaxFileItem != nil {
		return b.Parts[0].AaxFileItem.Abstract
	}
	return ""
}

func bookCopyright(b *domain.Book) string {
	if b == nil || len(b.Parts) == 0 {
		return ""
	}
	if b.Parts[0].AaxFileItem != nil {
		return b.Parts[0].AaxFileItem.Copyright
	}
	return ""
}

func bookPublisher(b *domain.Book) string {
	if b == nil || len(b.Parts) == 0 {
		return ""
	}
	if b.Parts[0].AaxFileItem != nil {
		return b.Parts[0].AaxFileItem.Publisher
	}
	return ""
}

func bookGenre(b *domain.Book) string {
	if b == nil || len(b.Parts) == 0 {
		return ""
	}
	if b.Parts[0].AaxFileItem != nil {
		return b.Parts[0].AaxFileItem.Genre
	}
	return ""
}

func bookPublishingDate(b *domain.Book) *time.Time {
	if b == nil || len(b.Parts) == 0 {
		return nil
	}
	if b.Parts[0].AaxFileItem != nil {
		return b.Parts[0].AaxFileItem.PublishingDate
	}
	return nil
}

func isWindows7Or8() bool {
	return false
}

// buildPerformerTag implements the role assignment.
func buildPerformerTag(role settings.RoleTagAssignment, author string, narrators []string) string {
	switch role {
	case settings.RoleTagAssignmentAuthor:
		return author
	case settings.RoleTagAssignmentAuthorNarrator:
		narratorStr := strings.Join(narrators, "; ")
		if narratorStr == "" {
			return author
		}
		return author + "; " + narratorStr
	case settings.RoleTagAssignmentNarrator:
		return strings.Join(narrators, "; ")
	default:
		return ""
	}
}

// trackPartMap associates tracks with their parts.
var trackPartMap = make(map[*domain.Track]*domain.Part)

// SetTrackPart associates a track with its part.
func SetTrackPart(track *domain.Track, part *domain.Part) {
	trackPartMap[track] = part
}

// GetTrackPart returns the part that owns the track.
func GetTrackPart(track *domain.Track) *domain.Part {
	return trackPartMap[track]
}
