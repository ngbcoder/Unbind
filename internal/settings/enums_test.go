package settings

import "testing"

// TestEnumNamesRoundTrip guards the C# enum names. The user interface renders
// an enum by splitting its name on "_" and looking up each part in the
// resource manager, so a renamed value would silently lose its translation.
func TestEnumNamesRoundTrip(t *testing.T) {
	check := func(name string, s string, ok bool) {
		t.Helper()
		if !ok {
			t.Errorf("%s: %q was not recognised", name, s)
		}
	}

	{
		got, ok := ParseConvFormat("mp4")
		check("ConvFormat", "mp4", ok)
		if got.String() != "mp4" {
			t.Errorf("ConvFormat round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseConvMode("splitChapters")
		check("ConvMode", "splitChapters", ok)
		if got.String() != "splitChapters" {
			t.Errorf("ConvMode round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseFileNaming("track_book_author")
		check("FileNaming", "track_book_author", ok)
		if got.String() != "track_book_author" {
			t.Errorf("FileNaming round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseTitleNaming("author_book_track")
		check("TitleNaming", "author_book_track", ok)
		if got.String() != "author_book_track" {
			t.Errorf("TitleNaming round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseTrackNumbering("track_b_chapter_c")
		check("TrackNumbering", "track_b_chapter_c", ok)
		if got.String() != "track_b_chapter_c" {
			t.Errorf("TrackNumbering round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseGeneralNamingEx("_nofolders")
		check("GeneralNamingEx", "_nofolders", ok)
		if got.String() != "_nofolders" {
			t.Errorf("GeneralNamingEx round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseReducedBitRate("_128k")
		check("ReducedBitRate", "_128k", ok)
		if got.String() != "_128k" {
			t.Errorf("ReducedBitRate round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseRoleTagAssignment("author__narrator__")
		check("RoleTagAssignment", "author__narrator__", ok)
		if got.String() != "author__narrator__" {
			t.Errorf("RoleTagAssignment round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseRoleTagAssignment("__narrator__")
		check("RoleTagAssignment", "__narrator__", ok)
		if got.String() != "__narrator__" {
			t.Errorf("RoleTagAssignment round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseVerifyAdjustChapterMarks("splitChapterOrTimeMode")
		check("VerifyAdjustChapterMarks", "splitChapterOrTimeMode", ok)
		if got.String() != "splitChapterOrTimeMode" {
			t.Errorf("VerifyAdjustChapterMarks round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseOnlineUpdate("promptForDownload")
		check("OnlineUpdate", "promptForDownload", ok)
		if got.String() != "promptForDownload" {
			t.Errorf("OnlineUpdate round trip = %q", got.String())
		}
	}
	{
		got, ok := ParsePreferEmbeddedChapterTimes("ifSilent")
		check("PreferEmbeddedChapterTimes", "ifSilent", ok)
		if got.String() != "ifSilent" {
			t.Errorf("PreferEmbeddedChapterTimes round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseOutFolderConflict("new_folder")
		check("OutFolderConflict", "new_folder", ok)
		if got.String() != "new_folder" {
			t.Errorf("OutFolderConflict round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseAaxCopyMode("author__book_author")
		check("AaxCopyMode", "author__book_author", ok)
		if got.String() != "author__book_author" {
			t.Errorf("AaxCopyMode round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseLongTitle("book_series")
		check("LongTitle", "book_series", ok)
		if got.String() != "book_series" {
			t.Errorf("LongTitle round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseNamedChapters("yesAlwaysWithNumbers")
		check("NamedChapters", "yesAlwaysWithNumbers", ok)
		if got.String() != "yesAlwaysWithNumbers" {
			t.Errorf("NamedChapters round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseFixAACEncoding("withIntermediateCopy")
		check("FixAACEncoding", "withIntermediateCopy", ok)
		if got.String() != "withIntermediateCopy" {
			t.Errorf("FixAACEncoding round trip = %q", got.String())
		}
	}
	{
		got, ok := ParseFlatFolderNaming("book_author")
		check("FlatFolderNaming", "book_author", ok)
		if got.String() != "book_author" {
			t.Errorf("FlatFolderNaming round trip = %q", got.String())
		}
	}
}

func TestRoleTagAssignmentValues(t *testing.T) {
	// The C# original assigns explicit ordinals. Go would repeat the previous
	// expression without them, so the values are pinned here.
	tests := []struct {
		v    RoleTagAssignment
		want int
	}{
		{RoleTagAssignmentNone, 0},
		{RoleTagAssignmentAuthor, 1},
		{RoleTagAssignmentAuthorNarratorDeprecated, 2},
		{RoleTagAssignmentAuthorNarrator, 3},
		{RoleTagAssignmentNarratorDeprecated, 4},
		{RoleTagAssignmentNarrator, 5},
	}
	for _, tc := range tests {
		if int(tc.v) != tc.want {
			t.Errorf("%v = %d, want %d", tc.v, int(tc.v), tc.want)
		}
	}
}

func TestReducedBitRateUInt32(t *testing.T) {
	tests := []struct {
		v    ReducedBitRate
		want uint
	}{
		{ReducedBitRateOff, 0},
		{ReducedBitRate128k, 128},
		{ReducedBitRate96k, 96},
		{ReducedBitRate64k, 64},
		{ReducedBitRate48k, 48},
		{ReducedBitRate32k, 32},
		{ReducedBitRate24k, 24},
	}
	for _, tc := range tests {
		if got := tc.v.UInt32(); got != tc.want {
			t.Errorf("%v.UInt32() = %d, want %d", tc.v, got, tc.want)
		}
	}
}

func TestTrackDurationClamping(t *testing.T) {
	tests := []struct {
		value byte
		mode  ConvMode
		want  int
	}{
		{1, ConvModeSplitChapters, 3},
		{5, ConvModeSplitChapters, 5},
		{20, ConvModeSplitChapters, 15},
		{20, ConvModeSplitTime, 20},
		{100, ConvModeSplitTime, 90},
	}
	for _, tc := range tests {
		_, _, val := TrackDuration(tc.value, tc.mode)
		if val != tc.want {
			t.Errorf("TrackDuration(%d, %v) = %d, want %d", tc.value, tc.mode, val, tc.want)
		}
	}
}

func TestApplicableBitRate(t *testing.T) {
	s := DefaultSettings()

	// "off" and no variable bit rate means no change.
	applicable, requested := s.ApplicableBitRate(64)
	if applicable != 0 || requested != 0 {
		t.Errorf("got (%d, %d), want (0, 0)", applicable, requested)
	}

	// A cap below the source bit rate applies.
	s.ReducedBitRate = ReducedBitRate32k
	applicable, requested = s.ApplicableBitRate(64)
	if applicable != 32 || requested != 32 {
		t.Errorf("got (%d, %d), want (32, 32)", applicable, requested)
	}

	// A cap above the source bit rate does not apply.
	applicable, _ = s.ApplicableBitRate(24)
	if applicable != 0 {
		t.Errorf("got %d, want 0", applicable)
	}
}

func TestFixNarratorMigratesDeprecatedValues(t *testing.T) {
	s := DefaultSettings()
	s.TagArtist = RoleTagAssignmentAuthorNarratorDeprecated
	s.TagAlbumArtist = RoleTagAssignmentNarratorDeprecated
	s.TagComposer = RoleTagAssignmentAuthorNarratorDeprecated
	s.TagConductor = RoleTagAssignmentNarratorDeprecated

	s.FixNarrator()

	if s.TagArtist != RoleTagAssignmentAuthorNarrator {
		t.Errorf("TagArtist = %v, want author_narrator", s.TagArtist)
	}
	if s.TagAlbumArtist != RoleTagAssignmentNarrator {
		t.Errorf("TagAlbumArtist = %v, want narrator", s.TagAlbumArtist)
	}
	if s.TagComposer != RoleTagAssignmentAuthorNarrator {
		t.Errorf("TagComposer = %v, want author_narrator", s.TagComposer)
	}
	if s.TagConductor != RoleTagAssignmentNarrator {
		t.Errorf("TagConductor = %v, want narrator", s.TagConductor)
	}
}

func TestUseEmbeddedChapterTimes(t *testing.T) {
	s := DefaultSettings()
	s.PreferEmbeddedChapterTimes = PreferEmbeddedChapterTimesIfSilent
	s.NamedChapters = NamedChaptersYes
	if !s.UseEmbeddedChapterTimes() {
		t.Error("UseEmbeddedChapterTimes() = false, want true")
	}

	s.NamedChapters = NamedChaptersNo
	if s.UseEmbeddedChapterTimes() {
		t.Error("UseEmbeddedChapterTimes() = true, want false when named chapters are off")
	}

	s.NamedChapters = NamedChaptersYes
	s.PreferEmbeddedChapterTimes = PreferEmbeddedChapterTimesNo
	if s.UseEmbeddedChapterTimes() {
		t.Error("UseEmbeddedChapterTimes() = true, want false when the preference is off")
	}
}

func TestSnapshotIsDeep(t *testing.T) {
	s := DefaultSettings()
	code := uint32(0xAABBCCDD)
	s.ActivationCode = &code

	snap := s.Snapshot()
	*snap.ActivationCode = 0

	if *s.ActivationCode != 0xAABBCCDD {
		t.Error("the snapshot shares the activation code pointer with the original")
	}
}
