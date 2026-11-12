package ffmpeg

import (
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestSplitArgsRespectsQuotes(t *testing.T) {
	got := splitArgs(`-i "C:\path with spaces\file.aax" -y`)
	want := []string{"-i", `C:\path with spaces\file.aax`, "-y"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitArgsSimple(t *testing.T) {
	got := splitArgs("-hide_banner -y -i file.aax")
	want := []string{"-hide_banner", "-y", "-i", "file.aax"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"00:00:00.000", 0},
		{"00:05:00.000", 5 * time.Minute},
		{"01:01:01.000", time.Hour + time.Minute + time.Second},
		{"12:34:56.789", 12*time.Hour + 34*time.Minute + 56*time.Second + 789*time.Millisecond},
		{"00:00:00.001", time.Millisecond},
	}
	for _, tc := range tests {
		if got := parseTimestamp(tc.in); got != tc.want {
			t.Errorf("parseTimestamp(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseSeconds(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"0", 0},
		{"1.5", 1500 * time.Millisecond},
		{"4602.43", 4602*time.Second + 430*time.Millisecond},
		{"60", time.Minute},
	}
	for _, tc := range tests {
		if got := parseSeconds(tc.in); got != tc.want {
			t.Errorf("parseSeconds(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSubstitActiv(t *testing.T) {
	in := `-activation_bytes 0f01f010 -i file.aax`
	got := SubstitActiv(in)
	if got != `-activation_bytes XXXXXXXX -i file.aax` {
		t.Errorf("SubstitActiv() = %q, want %q", got, `-activation_bytes XXXXXXXX -i file.aax`)
	}

	// no activation bytes: unchanged
	in2 := `-i file.aax`
	if got := SubstitActiv(in2); got != in2 {
		t.Errorf("SubstitActiv() = %q, want %q", got, in2)
	}
}

func TestRegexesMatchSampleOutput(t *testing.T) {
	cases := []struct {
		name  string
		regex *regexp.Regexp
		line  string
	}{
		{"duration", rgxDuration, "Duration: 00:12:34.56, start: 0.000, bitrate: 64 kb/s"},
		{"duration ex", rgxDurationEx, "Duration: 00:12:34.56, start: 0.000, bitrate: 64 kb/s"},
		{"audio stream", rgxAudioStream, "  Stream #0:0: Audio: aac (LC) (mp4a / 0x6134706D), 22050 Hz, stereo, fltp, 64 kb/s"},
		{"audio stream ex", rgxAudioStreamEx, "  Stream #0:0: Audio: aac (LC) (mp4a / 0x6134706D), 22050 Hz, stereo, fltp, 64 kb/s"},
		{"chapter", rgxChapter, "    Chapter #0: start 0.000000, end 300.000000"},
		{"chapter meta", rgxChapterMeta, "    Metadata:"},
		{"chapter title", rgxChapterTitle, "      title           : Chapter 1"},
		{"silence start", rgxSilenceStart, "[silencedetect @ 0000000002a54400] silence_start: 4602.43"},
		{"silence end", rgxSilenceEnd, "[silencedetect @ 0000000002a54400] silence_end: 4603.68 | silence_duration: 1.24948"},
		{"timestamp 1", rgxTimestamp1, "frame=   12 fps=0.5 q=1.6 size=N/A time=00:05:00.00 bitrate=N/A"},
		{"timestamp 2", rgxTimestamp2, "size= 1234kB time=00:05:00.00 bitrate=  64.0kbits/s"},
		{"mux final", rgxMuxFinal, "video:0kB audio:1234kB subtitle:0kB other streams:0kB global headers:0kB muxing overhead: 0.123%"},
		{"aa file", rgxAaFile, "Input #0, aa, from 'file.aa':"},
		{"no activation", rgxNoActivOpt, "Option activation_bytes not found"},
		{"error activation", rgxErrorActOpt, "Error setting option activation_bytes"},
		{"any stream", rgxAnyStream, "  Stream #0:1: Video: png, 100x100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.regex.MatchString(tc.line) {
				t.Errorf("regex %s did not match %q", tc.name, tc.line)
			}
		})
	}
}

func TestAudioStreamExCaptures(t *testing.T) {
	line := "  Stream #0:0: Audio: aac (LC) (mp4a / 0x6134706D), 22050 Hz, stereo, fltp, 64 kb/s"
	m := rgxAudioStreamEx.FindStringSubmatch(line)
	if m == nil {
		t.Fatal("no match")
	}
	if m[1] != "aac" {
		t.Errorf("codec = %q, want aac", m[1])
	}
	if m[2] != "22050" {
		t.Errorf("sample rate = %q, want 22050", m[2])
	}
	if m[3] != "stereo" {
		t.Errorf("channels = %q, want stereo", m[3])
	}
	if m[4] != "64" {
		t.Errorf("bit rate = %q, want 64", m[4])
	}
}

func TestChapterRegexCaptures(t *testing.T) {
	line := "    Chapter #0: start 0.000000, end 300.000000"
	m := rgxChapter.FindStringSubmatch(line)
	if m == nil {
		t.Fatal("no match")
	}
	if m[1] != "0.000000" {
		t.Errorf("begin = %q, want 0.000000", m[1])
	}
	if m[2] != "300.000000" {
		t.Errorf("end = %q, want 300.000000", m[2])
	}
}

func TestSilenceRegexCaptures(t *testing.T) {
	line := "[silencedetect @ 0000000002a54400] silence_end: 4603.68 | silence_duration: 1.24948"
	m := rgxSilenceEnd.FindStringSubmatch(line)
	if m == nil {
		t.Fatal("no match")
	}
	if m[1] != "4603.68" {
		t.Errorf("end = %q", m[1])
	}
	if m[2] != "1.24948" {
		t.Errorf("duration = %q", m[2])
	}
}

func TestVersionRegex(t *testing.T) {
	line := "ffmpeg version 4.1.3 Copyright (c) 2000-2019 the FFmpeg developers"
	m := rgxVersion.FindStringSubmatch(line)
	if m == nil {
		t.Fatal("no match")
	}
	if m[1] != "4.1.3" {
		t.Errorf("version = %q, want 4.1.3", m[1])
	}
}

func TestErrorRegexMatches(t *testing.T) {
	lines := []string{
		"Error while decoding",
		"corrupt input packet",
		"Conversion failed",
		"aborting",
		"Invalid data found when processing input",
	}
	for _, line := range lines {
		if !rgxErrorVarious.MatchString(line) {
			t.Errorf("rgxErrorVarious did not match %q", line)
		}
	}
}

func TestFFmpegExePath(t *testing.T) {
	// without a directory, the bare name is returned
	if got := FFmpegExePath(false, nil); got != "ffmpeg.exe" {
		t.Errorf("got %q, want ffmpeg.exe", got)
	}
	// with a directory that exists, the path is joined
	dir := t.TempDir()
	got := FFmpegExePath(false, func() string { return dir })
	if got != filepath.Join(dir, "ffmpeg.exe") {
		t.Errorf("got %q, want %q", got, filepath.Join(dir, "ffmpeg.exe"))
	}
	// with a directory that does not exist, the bare name is returned
	got = FFmpegExePath(false, func() string { return `C:\does\not\exist` })
	if got != "ffmpeg.exe" {
		t.Errorf("got %q, want ffmpeg.exe", got)
	}
}
