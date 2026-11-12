package metadata

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngbcoder/Unbind/internal/domain"
)

func TestExtractASIN(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"the Unseal naming scheme", "Title_B0ABCD1234_LC_64_22050", "B0ABCD1234"},
		{"at the end", "Book_B012345678", "B012345678"},
		{"followed by a dot", "Book_B012345678.part", "B012345678"},
		{"no ASIN", "JustATitle", ""},
		{"too short", "Book_B0123456", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractASIN(tc.in); got != tc.want {
				t.Errorf("extractASIN(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFindFileLocal(t *testing.T) {
	dir := t.TempDir()
	name := "B0ABCD1234.json"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := FindFile(name, dir, "SimsBySeries", "B0ABCD1234")
	if got == nil {
		t.Fatal("FindFile() = nil, want the local file")
	}
	if got.ASIN != "B0ABCD1234" {
		t.Errorf("ASIN = %q", got.ASIN)
	}
	if filepath.Base(got.Filename) != name {
		t.Errorf("Filename = %q, want %q", got.Filename, name)
	}
}

func TestFindFileNotFound(t *testing.T) {
	dir := t.TempDir()
	if got := FindFile("missing.json", dir, "SimsBySeries", "B0ABCD1234"); got != nil {
		t.Errorf("FindFile() = %v, want nil", got)
	}
}

func TestGetContentMetadata(t *testing.T) {
	dir := t.TempDir()
	asin := "B0ABCD1234"

	body := `{
	  "content_metadata": {
	    "chapter_info": {
	      "brandIntroDurationMs": 1500,
	      "brandOutroDurationMs": 2500,
	      "runtime_length_ms": 3600000,
	      "runtime_length_sec": 3600,
	      "chapters": [
	        {"length_ms": 60000, "start_offset_ms": 0, "title": "Chapter 1"},
	        {"length_ms": 0, "start_offset_ms": 60000, "title": ""},
	        {"length_ms": 60000, "start_offset_ms": 120000, "title": "Chapter 3"}
	      ]
	    },
	    "content_reference": {
	      "asin": "B0ABCD1234",
	      "sku": "BKTEST-12345",
	      "content_size_in_bytes": 123456
	    }
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "content_metadata_"+asin+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// The source file must carry the ASIN in its name.
	src := filepath.Join(dir, "Title_"+asin+"_LC.aax")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := domain.NewBook("Title")
	part := domain.NewPart(b, &domain.AaxFileItem{FileName: src}, 0)
	b.Parts = []*domain.Part{part}

	GetContentMetadata(part, 0)

	if part.SKU != "BKTEST-12345" {
		t.Errorf("SKU = %q, want BKTEST-12345", part.SKU)
	}
	if part.BrandIntro != 1500*time.Millisecond {
		t.Errorf("BrandIntro = %v, want 1.5s", part.BrandIntro)
	}
	if part.BrandOutro != 2500*time.Millisecond {
		t.Errorf("BrandOutro = %v, want 2.5s", part.BrandOutro)
	}
	if part.Duration != 3600*time.Second {
		t.Errorf("Duration = %v, want 1h", part.Duration)
	}

	if len(part.Chapters2) != 3 {
		t.Fatalf("got %d chapters, want 3", len(part.Chapters2))
	}

	// The empty title is repaired to a single dot.
	if part.Chapters2[1].Name != "." {
		t.Errorf("chapter 2 name = %q, want %q", part.Chapters2[1].Name, ".")
	}

	// The zero-length chapter is repaired to 1 ms.
	if part.Chapters2[1].Time.Duration() != time.Millisecond {
		t.Errorf("chapter 2 duration = %v, want 1ms", part.Chapters2[1].Time.Duration())
	}
	// The third chapter is not moved: the repair only closes an overlap, and
	// chapter 3 starts well after chapter 2 ends.
	if part.Chapters2[2].Time.Begin() != 120*time.Second {
		t.Errorf("chapter 3 begin = %v, want 2m", part.Chapters2[2].Time.Begin())
	}
	if part.Chapters2[2].Time.Duration() != 60*time.Second {
		t.Errorf("chapter 3 duration = %v, want 60s", part.Chapters2[2].Time.Duration())
	}
}

func TestGetContentMetadataFileOnly(t *testing.T) {
	dir := t.TempDir()
	asin := "B0ABCD1234"
	if err := os.WriteFile(filepath.Join(dir, "content_metadata_"+asin+".json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "Title_"+asin+".aax")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := domain.NewBook("Title")
	part := domain.NewPart(b, &domain.AaxFileItem{FileName: src}, 0)
	b.Parts = []*domain.Part{part}

	GetContentMetadata(part, flagFileOnly)

	if part.AaxFileItem.ContentMetadataFile == nil {
		t.Fatal("the content metadata file was not located")
	}
	if part.AaxFileItem.ContentMetadataFile.ASIN != asin {
		t.Errorf("ASIN = %q, want %q", part.AaxFileItem.ContentMetadataFile.ASIN, asin)
	}
	// fileOnly means no chapters
	if part.Chapters2 != nil {
		t.Errorf("chapters were parsed, want none")
	}
}

func TestGetSimsBySeries(t *testing.T) {
	dir := t.TempDir()
	asin := "B0ABCD1234"

	body := `{
	  "product": {
	    "asin": "B0ABCD1234",
	    "title": "The Long Book",
	    "sku": "BKTEST-12345",
	    "sku_lite": "BKTEST-12345-L",
	    "authors": [{"asin": "A1", "name": "Jane Doe"}],
	    "series": [{"asin": "S1", "title": "The Series", "sequence": "1.5"}]
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, asin+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "Title_"+asin+".aax")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := domain.NewBook("Title")
	part := domain.NewPart(b, &domain.AaxFileItem{FileName: src}, 0)
	b.Parts = []*domain.Part{part}

	GetSimsBySeries(part)

	meta := b.ExternalMeta
	if meta == nil {
		t.Fatal("no external metadata was set")
	}
	if meta.Book != "The Long Book" {
		t.Errorf("Book = %q, want %q", meta.Book, "The Long Book")
	}
	if len(meta.Authors) != 1 || meta.Authors[0] != "Jane Doe" {
		t.Errorf("Authors = %v, want [Jane Doe]", meta.Authors)
	}
	if len(meta.Series) != 1 || meta.Series[0].Title != "The Series" {
		t.Errorf("Series = %v, want The Series", meta.Series)
	}
	if meta.Series[0].Seq != "1.5" {
		t.Errorf("Series sequence = %q, want 1.5", meta.Series[0].Seq)
	}
}

func TestActivationCodesFromUserSetting(t *testing.T) {
	code := uint32(0x0F01F010)
	ac := NewActivationCodes(&code)

	if !ac.HasCode() {
		t.Fatal("no codes found")
	}
	found := false
	for _, c := range ac.Codes {
		if c == "0F01F010" {
			found = true
		}
	}
	if !found {
		t.Errorf("codes = %v, want 0F01F010", ac.Codes)
	}
}

func TestActivationCodesDeduplicate(t *testing.T) {
	code := uint32(0xAABBCCDD)
	ac := NewActivationCodes(&code)

	seen := map[string]bool{}
	for _, c := range ac.Codes {
		if seen[c] {
			t.Errorf("code %q appears twice", c)
		}
		seen[c] = true
		if len(c) != 8 {
			t.Errorf("code %q is not 8 hex digits", c)
		}
	}
}
