package handoff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsURI(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"unbind://add?p=x", true},
		{"UNBIND://add", true},
		{"Unbind://Show", true},
		{`C:\Books\a.aax`, false},
		{"unbind:/add", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsURI(tc.in); got != tc.want {
			t.Errorf("IsURI(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseURIAdd(t *testing.T) {
	raw := "unbind://add?p=C%3A%5CBooks%5Ca.aax&p=C%3A%5CBooks%5Cb.aax&activate=1&src=unseal"

	req, err := ParseURI(raw)
	if err != nil {
		t.Fatalf("ParseURI() error: %v", err)
	}
	if req.Verb != VerbAdd {
		t.Errorf("Verb = %q, want %q", req.Verb, VerbAdd)
	}
	if len(req.Paths) != 2 {
		t.Fatalf("got %d paths, want 2", len(req.Paths))
	}
	if req.Paths[0] != `C:\Books\a.aax` {
		t.Errorf("path 0 = %q", req.Paths[0])
	}
	if req.Paths[1] != `C:\Books\b.aax` {
		t.Errorf("path 1 = %q", req.Paths[1])
	}
	if !req.Activate {
		t.Error("Activate = false, want true")
	}
	if req.Source != "unseal" {
		t.Errorf("Source = %q, want %q", req.Source, "unseal")
	}
}

func TestParseURIDoc(t *testing.T) {
	raw := "unbind://add?doc=C%3A%5Ctmp%5Chandoff.json&activate=0&src=unseal"

	req, err := ParseURI(raw)
	if err != nil {
		t.Fatalf("ParseURI() error: %v", err)
	}
	if req.Doc != `C:\tmp\handoff.json` {
		t.Errorf("Doc = %q", req.Doc)
	}
	if req.Activate {
		t.Error("Activate = true, want false")
	}
}

func TestParseURIDefaultActivate(t *testing.T) {
	req, err := ParseURI("unbind://add?p=x")
	if err != nil {
		t.Fatalf("ParseURI() error: %v", err)
	}
	if !req.Activate {
		t.Error("Activate = false, want true by default")
	}
}

func TestParseURIUnknownVerb(t *testing.T) {
	if _, err := ParseURI("unbind://frobnicate"); !errors.Is(err, ErrUnknownVerb) {
		t.Errorf("error = %v, want ErrUnknownVerb", err)
	}
}

func TestParseURINotAURI(t *testing.T) {
	if _, err := ParseURI(`C:\Books\a.aax`); !errors.Is(err, ErrNotAURI) {
		t.Errorf("error = %v, want ErrNotAURI", err)
	}
}

func TestParseURIShow(t *testing.T) {
	req, err := ParseURI("unbind://show")
	if err != nil {
		t.Fatalf("ParseURI() error: %v", err)
	}
	if req.Verb != VerbShow {
		t.Errorf("Verb = %q, want %q", req.Verb, VerbShow)
	}
}

func TestIsAllowedExtension(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"book.aax", true},
		{"book.AAX", true},
		{"book.aa", true},
		{"book.m4b", true},
		{"book.mp3", false},
		{"book.exe", false},
		{"book", false},
	}
	for _, tc := range tests {
		if got := IsAllowedExtension(tc.in); got != tc.want {
			t.Errorf("IsAllowedExtension(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestResolveDropsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.aax")
	if err := os.WriteFile(good, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	notThere := filepath.Join(dir, "missing.aax")
	badExt := filepath.Join(dir, "bad.exe")
	if err := os.WriteFile(badExt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := &Request{Paths: []string{good, notThere, badExt, "relative.aax"}}
	accepted, skipped := Resolve(req, nil, nil)

	if len(accepted) != 1 || accepted[0] != good {
		t.Errorf("accepted = %v, want just %q", accepted, good)
	}
	if skipped != 3 {
		t.Errorf("skipped = %d, want 3", skipped)
	}
}

func TestResolveDeduplicates(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.aax")
	if err := os.WriteFile(good, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := &Request{Paths: []string{good, good}}
	accepted, skipped := Resolve(req, nil, []string{good})

	if len(accepted) != 0 {
		t.Errorf("accepted = %v, want none, the file is already present", accepted)
	}
	if skipped != 2 {
		t.Errorf("skipped = %d, want 2", skipped)
	}
}

func TestLoadDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoff.json")
	body := `{
	  "protocol": 1,
	  "source": "unseal",
	  "files": [
	    {"path": "C:\\Books\\a.aax", "asin": "B0XXXXXXXXXX", "title": "The Book"}
	  ],
	  "options": {"format": "mp4", "mode": "chapters"}
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument() error: %v", err)
	}
	if doc.Protocol != Protocol {
		t.Errorf("Protocol = %d, want %d", doc.Protocol, Protocol)
	}
	if doc.Source != "unseal" {
		t.Errorf("Source = %q", doc.Source)
	}
	if len(doc.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(doc.Files))
	}
	if doc.Files[0].ASIN != "B0XXXXXXXXXX" {
		t.Errorf("ASIN = %q", doc.Files[0].ASIN)
	}
	if doc.Options == nil || doc.Options.Format != "mp4" {
		t.Errorf("Options = %+v", doc.Options)
	}
}

func TestLoadDocumentRejectsANewerProtocol(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoff.json")
	if err := os.WriteFile(path, []byte(`{"protocol": 99, "files": []}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadDocument(path); !errors.Is(err, ErrUnsupportedProtocol) {
		t.Errorf("error = %v, want ErrUnsupportedProtocol", err)
	}
}

func TestLoadDocumentRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoff.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadDocument(path); err == nil {
		t.Error("LoadDocument() error = nil, want an error")
	}
}

func TestResolveUsesTheDocumentFiles(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.aax")
	if err := os.WriteFile(good, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := &Request{}
	doc := &Document{Protocol: Protocol, Files: []File{{Path: good}}}

	accepted, skipped := Resolve(req, doc, nil)
	if len(accepted) != 1 || accepted[0] != good {
		t.Errorf("accepted = %v, want %q", accepted, good)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
}
