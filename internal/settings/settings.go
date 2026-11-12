package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Settings is the complete configuration of the application.
//
// The C# original exposed a hierarchy of interfaces so that the engine would
// never see the designer-generated Properties.Settings class. In Go there is no
// designer magic: one plain struct is the shared contract, the user interface
// edits its fields directly, and the engine reads it.
//
// The engine never mutates a Settings value. A conversion takes a Snapshot at
// the start so that changing a setting mid-run cannot alter the work in
// progress.
type Settings struct {
	// --- Naming ---
	FileNaming     FileNaming      `json:"fileNaming"`
	TitleNaming    TitleNaming     `json:"titleNaming"`
	TrackNumbering TrackNumbering  `json:"trackNumbering"`
	TotalTracks    bool            `json:"totalTracks"`
	GenreNaming    GeneralNaming   `json:"genreNaming"`
	GenreName      string          `json:"genreName"`
	ChapterNaming  GeneralNamingEx `json:"chapterNaming"`
	ChapterName    string          `json:"chapterName"`
	PartNaming     GeneralNaming   `json:"partNaming"`
	PartName       string          `json:"partName"`
	ExtraMetaFiles bool            `json:"extraMetaFiles"`
	NamedChapters  NamedChapters   `json:"namedChapters"`
	M4B            bool            `json:"m4b"`

	// --- Mode ---
	ConvFormat      ConvFormat     `json:"convFormat"`
	ConvMode        ConvMode       `json:"convMode"`
	VariableBitRate bool           `json:"variableBitRate"`
	ReducedBitRate  ReducedBitRate `json:"reducedBitRate"`

	// --- Titles ---
	SeriesTitleLeft    bool      `json:"seriesTitleLeft"`
	LongBookTitle      LongTitle `json:"longBookTitle"`
	AddnlValTitlePunct string    `json:"addnlValTitlePunct"`

	// --- Tags ---
	TagArtist      RoleTagAssignment `json:"tagArtist"`
	TagAlbumArtist RoleTagAssignment `json:"tagAlbumArtist"`
	TagComposer    RoleTagAssignment `json:"tagComposer"`
	TagConductor   RoleTagAssignment `json:"tagConductor"`
	Narrator       *bool             `json:"narrator"`

	// --- Activation ---
	ActivationCode *uint32 `json:"activationCode"`

	// --- Update ---
	OnlineUpdate               OnlineUpdate `json:"onlineUpdate"`
	OnlineUpdateOthersDeclined string       `json:"onlineUpdateOthersDeclined"`

	// --- Archive ---
	AaxCopyMode      AaxCopyMode `json:"aaxCopyMode"`
	AaxCopyDirectory string      `json:"aaxCopyDirectory"`

	// --- Operational ---
	NonParallel                bool                       `json:"nonParallel"`
	FFmpeg64bitHours           int                        `json:"ffmpeg64bitHours"`
	InputDirectory             string                     `json:"inputDirectory"`
	OutputDirectory            string                     `json:"outputDirectory"`
	FFMpegDirectory            string                     `json:"ffmpegDirectory"`
	RelaxedFFmpegVersionCheck  bool                       `json:"relaxedFFmpegVersionCheck"`
	ConvertByFileDate          bool                       `json:"convertByFileDate"`
	TrkDurMins                 byte                       `json:"trkDurMins"`
	PartNames                  string                     `json:"partNames"`
	Genres                     string                     `json:"genres"`
	FlatFolders                bool                       `json:"flatFolders"`
	FlatFolderNaming           FlatFolderNaming           `json:"flatFolderNaming"`
	WithSeriesTitle            bool                       `json:"withSeriesTitle"`
	NumDigitsSeriesSeqNo       byte                       `json:"numDigitsSeriesSeqNo"`
	FullCaptionBookFolder      bool                       `json:"fullCaptionBookFolder"`
	OutFolderConflict          OutFolderConflict          `json:"outFolderConflict"`
	ShortChapterSec            uint                       `json:"shortChapterSec"`
	VeryShortChapterSec        uint                       `json:"veryShortChapterSec"`
	VerifyAdjustChapterMarks   VerifyAdjustChapterMarks   `json:"verifyAdjustChapterMarks"`
	PreferEmbeddedChapterTimes PreferEmbeddedChapterTimes `json:"preferEmbeddedChapterTimes"`
	Latin1EncodingForPlaylist  bool                       `json:"latin1EncodingForPlaylist"`
	IntermedCopySingle         bool                       `json:"intermedCopySingle"`
	FixAACEncoding             FixAACEncoding             `json:"fixAACEncoding"`
	AutoLaunchPlayer           bool                       `json:"autoLaunchPlayer"`
	Version                    string                     `json:"version"`

	// --- Application only, ignored by the engine ---
	FileAssoc      bool `json:"fileAssoc"`
	ShowStartupTip bool `json:"showStartupTip"`
	FileDateColumn bool `json:"fileDateColumn"`
	SettingsTab    int  `json:"settingsTab"`
}

// DefaultSettings returns the settings with the C# defaults applied.
func DefaultSettings() *Settings {
	return &Settings{
		FileNaming:                 FileNamingTrackBookAuthor,
		TitleNaming:                TitleNamingBookAuthor,
		TrackNumbering:             TrackNumberingTrack,
		GenreNaming:                GeneralNamingSource,
		ChapterNaming:              GeneralNamingExSource,
		PartNaming:                 GeneralNamingSource,
		NamedChapters:              NamedChaptersYes,
		ConvFormat:                 ConvFormatMP4,
		ConvMode:                   ConvModeChapters,
		ReducedBitRate:             ReducedBitRateOff,
		LongBookTitle:              LongTitleNo,
		TagArtist:                  RoleTagAssignmentAuthor,
		TagAlbumArtist:             RoleTagAssignmentAuthor,
		TagComposer:                RoleTagAssignmentAuthor,
		TagConductor:               RoleTagAssignmentNone,
		OnlineUpdate:               OnlineUpdatePromptForDownload,
		AaxCopyMode:                AaxCopyModeNone,
		FlatFolderNaming:           FlatFolderNamingAuthorBook,
		OutFolderConflict:          OutFolderConflictAsk,
		ShortChapterSec:            25,
		VeryShortChapterSec:        10,
		VerifyAdjustChapterMarks:   VerifyAdjustChapterMarksSplitChapterOrTimeMode,
		PreferEmbeddedChapterTimes: PreferEmbeddedChapterTimesIfSilent,
		TrkDurMins:                 5,
		NumDigitsSeriesSeqNo:       2,
	}
}

// Snapshot returns a deep copy of the settings. The conversion pipeline takes
// one at the start so that the run is not affected by later edits.
func (s *Settings) Snapshot() *Settings {
	if s == nil {
		return nil
	}
	c := *s
	if s.ActivationCode != nil {
		v := *s.ActivationCode
		c.ActivationCode = &v
	}
	if s.Narrator != nil {
		v := *s.Narrator
		c.Narrator = &v
	}
	return &c
}

// FixNarrator migrates the deprecated narrator role assignments. The C#
// original does this at startup (Settings.cs:26-51).
func (s *Settings) FixNarrator() {
	switch s.TagArtist {
	case RoleTagAssignmentAuthorNarratorDeprecated:
		s.TagArtist = RoleTagAssignmentAuthorNarrator
	case RoleTagAssignmentNarratorDeprecated:
		s.TagArtist = RoleTagAssignmentNarrator
	}
	switch s.TagAlbumArtist {
	case RoleTagAssignmentAuthorNarratorDeprecated:
		s.TagAlbumArtist = RoleTagAssignmentAuthorNarrator
	case RoleTagAssignmentNarratorDeprecated:
		s.TagAlbumArtist = RoleTagAssignmentNarrator
	}
	switch s.TagComposer {
	case RoleTagAssignmentAuthorNarratorDeprecated:
		s.TagComposer = RoleTagAssignmentAuthorNarrator
	case RoleTagAssignmentNarratorDeprecated:
		s.TagComposer = RoleTagAssignmentNarrator
	}
	switch s.TagConductor {
	case RoleTagAssignmentAuthorNarratorDeprecated:
		s.TagConductor = RoleTagAssignmentAuthorNarrator
	case RoleTagAssignmentNarratorDeprecated:
		s.TagConductor = RoleTagAssignmentNarrator
	}
}

// Store persists the settings as JSON at a fixed path.
//
// The C# original needed a custom settings provider because .NET derives the
// user.config path from a hash of the executable path and version, so the
// settings appeared to vanish after an upgrade. A JSON file at a fixed path
// has that problem by construction.
type Store struct {
	path string
}

// NewStore returns a store writing to the given path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Path returns the path of the settings file.
func (s *Store) Path() string { return s.path }

// Load reads the settings from disk. A missing file yields the defaults. A
// corrupt file also yields the defaults, because a broken settings file must
// never prevent the application from starting.
func (s *Store) Load() (*Settings, error) {
	def := DefaultSettings()
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return def, nil
		}
		return def, err
	}
	loaded := DefaultSettings()
	if err := json.Unmarshal(b, loaded); err != nil {
		return def, err
	}
	loaded.FixNarrator()
	return loaded, nil
}

// Save writes the settings to disk atomically: it writes a sibling temporary
// file and renames it over the target, so an interrupted write cannot leave a
// truncated settings file behind.
func (s *Store) Save(settings *Settings) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Now returns the current time. It is a variable so that tests can freeze it.
var Now = time.Now
