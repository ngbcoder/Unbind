package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ngbcoder/Unbind/internal/domain"
	"github.com/ngbcoder/Unbind/internal/logging"
)

// asinRegex extracts the ASIN from a file name.
var asinRegex = regexp.MustCompile(`_([0-9A-Z]{10})(_|$|\.)`)

// FindFile searches for a sidecar file in three places: local (beside the
// source file), relative (in a subdirectory of the parent), and absolute
// (in every Audible package directory).
//
// This is the port of AsinJsonFile.FindFile (AudibleAppContentMetadata.cs:30-60).
func FindFile(asinFile, contentDirPath, subDir, asin string) *domain.AsinJsonFile {
	// local
	localPath := filepath.Join(contentDirPath, asinFile)
	if _, err := os.Stat(localPath); err == nil {
		return &domain.AsinJsonFile{Filename: localPath, ASIN: asin}
	}

	// relative
	if subDirPath := getRelativePath(contentDirPath, subDir); subDirPath != nil {
		relativePath := filepath.Join(*subDirPath, asinFile)
		if _, err := os.Stat(relativePath); err == nil {
			return &domain.AsinJsonFile{Filename: relativePath, ASIN: asin}
		}
	}

	// absolute
	if subDirPaths := getAbsolutePaths(subDir); subDirPaths != nil {
		for _, subDirPath := range subDirPaths {
			absolutePath := filepath.Join(subDirPath, asinFile)
			if _, err := os.Stat(absolutePath); err == nil {
				return &domain.AsinJsonFile{Filename: absolutePath, ASIN: asin}
			}
		}
	}

	logging.Log(3, "AsinJsonFile", asinFile+" not found.")
	return nil
}

func getRelativePath(contentDirPath, subDir string) *string {
	diLocalState := filepath.Dir(contentDirPath)
	subDirPath := filepath.Join(diLocalState, subDir)
	if info, err := os.Stat(subDirPath); err == nil && info.IsDir() {
		return &subDirPath
	}
	return nil
}

func getAbsolutePaths(subDir string) []string {
	// Audible package directories — in the C# original these come from
	// ActivationCodeApp.GetPackageDirectories(). For now, return nil; the
	// absolute search is a fallback for the Audible desktop app's data.
	return nil
}

// --- content_metadata_<ASIN>.json ---

// ContentMetadata is the JSON model for content_metadata_<ASIN>.json.
type ContentMetadata struct {
	ContentMetadata struct {
		ChapterInfo struct {
			BrandIntroDurationMs int `json:"brandIntroDurationMs"`
			BrandOutroDurationMs int `json:"brandOutroDurationMs"`
			Chapters             []struct {
				LengthMs      int    `json:"length_ms"`
				StartOffsetMs int    `json:"start_offset_ms"`
				Title         string `json:"title"`
			} `json:"chapters"`
			RuntimeLengthMs  int `json:"runtime_length_ms"`
			RuntimeLengthSec int `json:"runtime_length_sec"`
		} `json:"chapter_info"`
		ContentReference struct {
			ACR           string `json:"acr"`
			ASIN          string `json:"asin"`
			ContentFormat string `json:"content_format"`
			ContentSize   int64  `json:"content_size_in_bytes"`
			Marketplace   string `json:"marketplace"`
			SKU           string `json:"sku"`
			Tempo         string `json:"tempo"`
			Version       string `json:"version"`
		} `json:"content_reference"`
	} `json:"content_metadata"`
	ResponseGroups []string `json:"response_groups"`
}

// GetContentMetadata reads the content_metadata_<ASIN>.json sidecar.
//
// It is the port of AudibleAppContentMetadata.GetContentMetadata.
func GetContentMetadata(part *domain.Part, flags int) {
	if part == nil || part.AaxFileItem == nil {
		return
	}

	filename := part.AaxFileItem.FileName
	logging.Log(3, "AudibleAppContentMetadata", logging.SubstitUser(filename))

	asin := extractASIN(filename)
	if asin == "" {
		logging.Log(3, "AudibleAppContentMetadata", "ASIN not found.")
		part.AaxFileItem.ContentMetadataFile = &domain.AsinJsonFile{Filename: "", ASIN: ""}
		return
	}

	contentDir := filepath.Dir(filename)
	contentMetaFile := "content_metadata_" + asin + ".json"

	asinJsonFile := FindFile(contentMetaFile, contentDir, "filescache", asin)
	if asinJsonFile == nil {
		part.AaxFileItem.ContentMetadataFile = &domain.AsinJsonFile{Filename: "", ASIN: ""}
		return
	}

	part.AaxFileItem.ContentMetadataFile = asinJsonFile
	logging.Log(3, "AudibleAppContentMetadata", logging.SubstitUser(asinJsonFile.Filename))

	if flags&flagFileOnly != 0 {
		return
	}

	getContentMetaChapters(part, asinJsonFile.Filename, flags&flagSKUOnly != 0)
}

const (
	flagFileOnly = 1
	flagSKUOnly  = 2
)

// getContentMetaChapters parses the content metadata file and populates the
// part's chapters, brand intro/outro and duration.
func getContentMetaChapters(part *domain.Part, metafile string, skuOnly bool) {
	data, err := os.ReadFile(metafile)
	if err != nil {
		logging.Log(1, "AudibleAppContentMetadata", err.Error())
		return
	}

	var metadata ContentMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		logging.Log(1, "AudibleAppContentMetadata", err.Error())
		return
	}

	// get the SKU
	part.SKU = metadata.ContentMetadata.ContentReference.SKU
	logging.Log(3, "AudibleAppContentMetadata", "SKU="+part.SKU)

	if skuOnly {
		return
	}

	// set the chapters
	chapters := metadata.ContentMetadata.ChapterInfo.Chapters
	if len(chapters) == 0 {
		return
	}

	// handle chapters with no name. Set '.' as a placeholder
	for i := range chapters {
		if strings.TrimSpace(chapters[i].Title) == "" {
			chapters[i].Title = "."
		}
	}

	// handle chapters of zero length. Min length must be 1 ms.
	var zeros []int
	for i, ch := range chapters {
		if ch.LengthMs == 0 {
			zeros = append(zeros, i)
		}
	}
	if len(zeros) > 0 {
		for _, i := range zeros {
			chapters[i].LengthMs = domain.MsMinChapterLength
		}
		// Close the gap that the zero-length chapter left behind.
		for i := 1; i < len(chapters); i++ {
			ch0 := chapters[i-1]
			ch := &chapters[i]
			chOffsNew := ch0.StartOffsetMs + ch0.LengthMs
			if ch.StartOffsetMs >= chOffsNew {
				continue
			}
			chLenNew := ch.LengthMs + ch.StartOffsetMs - chOffsNew
			if chLenNew <= 0 {
				chLenNew = domain.MsMinChapterLength
			}
			ch.StartOffsetMs = chOffsNew
			ch.LengthMs = chLenNew
		}
	}

	var chs []*domain.Chapter
	for _, ch := range chapters {
		chapter := domain.NewChapter()
		chapter.Name = strings.TrimSpace(ch.Title)
		chapter.Time.SetBegin(time.Duration(ch.StartOffsetMs) * time.Millisecond)
		chapter.Time.SetEnd(time.Duration(ch.StartOffsetMs+ch.LengthMs) * time.Millisecond)
		chs = append(chs, chapter)
	}
	part.Chapters2 = chs

	// set brand intro and outro times
	part.BrandIntro = time.Duration(metadata.ContentMetadata.ChapterInfo.BrandIntroDurationMs) * time.Millisecond
	part.BrandOutro = time.Duration(metadata.ContentMetadata.ChapterInfo.BrandOutroDurationMs) * time.Millisecond
	part.Duration = time.Duration(metadata.ContentMetadata.ChapterInfo.RuntimeLengthMs) * time.Millisecond

	logging.Log(3, "AudibleAppContentMetadata", "brand intro="+part.BrandIntro.String()+", outro="+part.BrandOutro.String()+", duration="+part.Duration.String())
}

// --- SimsBySeries / series_titles_<ASIN>.json ---

// AppSimsBySeries is the JSON model for <ASIN>.json.
type AppSimsBySeries struct {
	Product        Product  `json:"product"`
	ResponseGroups []string `json:"response_groups"`
}

// Product is the product record in a sidecar.
type Product struct {
	ASIN             string     `json:"asin"`
	Title            string     `json:"title"`
	SKU              string     `json:"sku"`
	SKULite          string     `json:"sku_lite"`
	IsListenable     bool       `json:"is_listenable"`
	RuntimeLengthMin int        `json:"runtime_length_min"`
	HasChildren      bool       `json:"has_children"`
	IsAdultProduct   bool       `json:"is_adult_product"`
	Authors          []Author   `json:"authors"`
	Narrators        []Narrator `json:"narrators"`
	Series           []Series   `json:"series"`
}

// Author is an author in a sidecar.
type Author struct {
	ASIN string `json:"asin"`
	Name string `json:"name"`
}

// Narrator is a narrator in a sidecar.
type Narrator struct {
	Name string `json:"name"`
}

// Series is a series in a sidecar.
type Series struct {
	ASIN     string `json:"asin"`
	Sequence string `json:"sequence"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

// AppSeriesTitles is the JSON model for series_titles_<ASIN>.json.
type AppSeriesTitles struct {
	ResponseGroups  []string  `json:"response_groups"`
	SimilarProducts []Product `json:"similar_products"`
}

// GetSimsBySeries resolves the series metadata from the sidecars.
//
// It is the port of AudibleAppSimsBySeries.GetSimsBySeries.
func GetSimsBySeries(part *domain.Part) {
	if part == nil || part.Book == nil {
		return
	}
	if part.Book.ExternalMeta != nil {
		return
	}

	afi := part.AaxFileItem
	if afi == nil {
		return
	}

	asin := ""
	if afi.ContentMetadataFile != nil {
		asin = afi.ContentMetadataFile.ASIN
	}
	if asin == "" {
		// try to extract from the filename
		asin = extractASIN(afi.FileName)
	}
	if asin == "" {
		logging.Log(3, "AudibleAppSimsBySeries", "ASIN not found.")
		afi.SimsBySeriesFile = &domain.AsinJsonFile{Filename: "", ASIN: ""}
		return
	}

	contentDir := filepath.Dir(afi.FileName)

	// try direct by ASIN first
	simsFile := asin + ".json"
	asinJsonFile := FindFile(simsFile, contentDir, "SimsBySeries", asin)
	if asinJsonFile != nil {
		if extractMetaDataFromProduct(asinJsonFile, part) {
			afi.SimsBySeriesFile = asinJsonFile
			return
		}
	}

	// try by SKU for multi-part books
	if part.SKU != "" {
		asinJsonFile = findSeriesFileBySKU(part, contentDir, asin)
		if asinJsonFile != nil {
			afi.SimsBySeriesFile = asinJsonFile
			return
		}
	}

	afi.SimsBySeriesFile = &domain.AsinJsonFile{Filename: "", ASIN: ""}
}

// findSeriesFileBySKU searches for a sidecar by SKU.
func findSeriesFileBySKU(part *domain.Part, contentDir, asin string) *domain.AsinJsonFile {
	// search in the content dir, the relative SimsBySeries dir, and the
	// absolute SimsBySeries dirs
	dirs := []string{contentDir}
	if rel := getRelativePath(contentDir, "SimsBySeries"); rel != nil {
		dirs = append(dirs, *rel)
	}
	if abs := getAbsolutePaths("SimsBySeries"); abs != nil {
		dirs = append(dirs, abs...)
	}

	filePattern := "??????????.json"
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, filePattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			data, err := os.ReadFile(match)
			if err != nil {
				continue
			}

			// try single product format
			var single AppSimsBySeries
			if err := json.Unmarshal(data, &single); err == nil && single.Product.ASIN != "" {
				if strings.HasPrefix(part.SKU, single.Product.SKU) || strings.HasPrefix(part.SKU, single.Product.SKULite) {
					extractMetaDataFromProductObj(single.Product, part)
					return &domain.AsinJsonFile{Filename: match, ASIN: single.Product.ASIN}
				}
			}

			// try series titles format
			var multi AppSeriesTitles
			if err := json.Unmarshal(data, &multi); err == nil {
				for _, p := range multi.SimilarProducts {
					if strings.HasPrefix(part.SKU, p.SKU) || strings.HasPrefix(part.SKU, p.SKULite) {
						extractMetaDataFromProductObj(p, part)
						return &domain.AsinJsonFile{Filename: match, ASIN: p.ASIN}
					}
				}
			}
		}
	}

	// also try series_titles_ prefix
	for _, dir := range dirs {
		pattern := "series_titles_??????????.json"
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			data, err := os.ReadFile(match)
			if err != nil {
				continue
			}
			var multi AppSeriesTitles
			if err := json.Unmarshal(data, &multi); err == nil {
				for _, p := range multi.SimilarProducts {
					if strings.HasPrefix(part.SKU, p.SKU) || strings.HasPrefix(part.SKU, p.SKULite) {
						extractMetaDataFromProductObj(p, part)
						return &domain.AsinJsonFile{Filename: match, ASIN: p.ASIN}
					}
				}
			}
		}
	}

	return nil
}

// extractMetaDataFromProduct reads the product record from a sidecar file.
func extractMetaDataFromProduct(asinJsonFile *domain.AsinJsonFile, part *domain.Part) bool {
	data, err := os.ReadFile(asinJsonFile.Filename)
	if err != nil {
		return false
	}

	var single AppSimsBySeries
	if err := json.Unmarshal(data, &single); err == nil && single.Product.ASIN != "" {
		extractMetaDataFromProductObj(single.Product, part)
		return true
	}

	var multi AppSeriesTitles
	if err := json.Unmarshal(data, &multi); err == nil {
		// find by SKU
		for _, p := range multi.SimilarProducts {
			if strings.HasPrefix(part.SKU, p.SKU) || strings.HasPrefix(part.SKU, p.SKULite) {
				extractMetaDataFromProductObj(p, part)
				return true
			}
		}
		// if no SKU match, try the first one
		if len(multi.SimilarProducts) > 0 {
			extractMetaDataFromProductObj(multi.SimilarProducts[0], part)
			return true
		}
	}

	return false
}

// extractMetaDataFromProductObj lifts the author and series info from a product.
func extractMetaDataFromProductObj(product Product, part *domain.Part) {
	if part.Book == nil {
		return
	}
	if part.Book.ExternalMeta != nil {
		return
	}

	var authors []string
	for _, a := range product.Authors {
		authors = append(authors, a.Name)
	}

	var series []domain.SeriesEntry
	for _, s := range product.Series {
		series = append(series, domain.SeriesEntry{
			Title: s.Title,
			Seq:   s.Sequence,
		})
	}

	part.Book.ExternalMeta = &domain.ExternalBookMeta{
		Authors: authors,
		Book:    product.Title,
		Series:  series,
	}
}

// extractASIN extracts the ASIN from a file name.
func extractASIN(filename string) string {
	fn := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	m := asinRegex.FindStringSubmatch(fn)
	if m == nil {
		return ""
	}
	return m[1]
}
