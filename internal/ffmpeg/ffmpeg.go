// Package ffmpeg wraps the FFmpeg executable.
//
// The original C# code (AaxAudioConverterLib/FFmpeg.cs) delegates every
// external-process call to the AuxLib.ProcessHost base class. In Go that
// machinery is replaced by os/exec, with the stdout and stderr streams read
// concurrently so that a full pipe cannot deadlock the process.
//
// FFmpeg writes its banner and its progress to stderr, and its chapter and
// metadata listing to stdout. Success is detected by the mux-overhead line,
// not by the exit code, because FFmpeg can exit non-zero for benign reasons
// while still producing a correct file.
package ffmpeg

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ngbcoder/Unbind/internal/domain"
	"github.com/ngbcoder/Unbind/internal/logging"
	"github.com/ngbcoder/Unbind/internal/settings"
)

// ETranscode selects how a track is transcoded.
type ETranscode int

// Transcode modes.
const (
	ETranscodeNormal ETranscode = iota
	ETranscodeCopy
	ETranscodeNoChapters
)

// AudioMeta is the result of probing an input file.
type AudioMeta struct {
	// Duration is the total running time.
	Duration time.Duration

	// Begin is the start offset of the selected range.
	Begin time.Duration

	// End is the end offset of the selected range.
	End time.Duration

	// SampleRate is the audio sample rate in Hz.
	SampleRate uint

	// Channels is the audio channel count.
	Channels uint

	// AvgBitRate is the average bit rate in kbit/s.
	AvgBitRate uint
}

// FFmpeg is one FFmpeg invocation. It is not reusable across invocations: the
// engine creates a fresh instance per operation, which keeps the parsing state
// trivially correct.
type FFmpeg struct {
	// filenameIn is the input file.
	filenameIn string

	// filenameMeta is the optional external metadata file.
	filenameMeta string

	// preferInternal64Bit selects ffmpeg64.exe when it exists.
	preferInternal64Bit bool

	// cancel reports whether the run should be aborted.
	cancel func() bool

	// progress receives the fractional progress, in 0..1.
	progress func(float64)

	// GetFFmpegDir resolves the directory that holds ffmpeg.exe.
	GetFFmpegDir func() string

	// process is the running process, set while it runs.
	process *exec.Cmd

	// mutex guards the mutable state below.
	mutex sync.Mutex

	// success is set when the mux-overhead line is seen.
	success bool

	// transcodingError is set when an error regex matches.
	transcodingError bool

	// aborted is set when the run was cancelled.
	aborted bool

	// matchedDuration is set once the duration has been parsed.
	matchedDuration bool

	// listComplete marks the end of the chapter list.
	listComplete bool

	// relaxedVersionParsing tolerates a looser version string.
	relaxedVersionParsing bool

	// bitrateParam is the computed -b:a / -q:a / -ar parameters.
	bitrateParam string

	// hasNoActivation is set when the file needs no activation code.
	hasNoActivation bool

	// isMp3Stream reports whether the source payload is already MP3.
	isMp3Stream bool

	// isAaFile reports whether the file is a legacy .aa container.
	isAaFile bool

	// isAaxFile reports whether the file is an .aax container.
	isAaxFile bool

	// chapters is the chapter list harvested from the probe.
	chapters []*domain.Chapter

	// silences is the silence list harvested by DetectSilence.
	silences []domain.TimeInterval

	// audioMeta is the probe result.
	audioMeta AudioMeta

	// version is the parsed FFmpeg version.
	version *Version
}

// New returns a new FFmpeg instance for the given input file.
func New(filenameIn, filenameMeta string) *FFmpeg {
	return &FFmpeg{
		filenameIn:   filenameIn,
		filenameMeta: filenameMeta,
		audioMeta:    AudioMeta{},
	}
}

// SetCancel sets the cancellation callback.
func (f *FFmpeg) SetCancel(cancel func() bool) { f.cancel = cancel }

// SetProgress sets the progress callback.
func (f *FFmpeg) SetProgress(progress func(float64)) { f.progress = progress }

// SetPreferInternal64Bit sets whether ffmpeg64.exe should be preferred.
func (f *FFmpeg) SetPreferInternal64Bit(v bool) { f.preferInternal64Bit = v }

// Chapters returns the harvested chapter list.
func (f *FFmpeg) Chapters() []*domain.Chapter { return f.chapters }

// Silences returns the harvested silence list.
func (f *FFmpeg) Silences() []domain.TimeInterval { return f.silences }

// AudioMeta returns the probe result.
func (f *FFmpeg) AudioMeta() *AudioMeta { return &f.audioMeta }

// IsMp3Stream reports whether the source payload is already MP3.
func (f *FFmpeg) IsMp3Stream() bool { return f.isMp3Stream }

// IsAaFile reports whether the file is a legacy .aa container.
func (f *FFmpeg) IsAaFile() bool { return f.isAaFile }

// IsAaxFile reports whether the file is an .aax container.
func (f *FFmpeg) IsAaxFile() bool { return f.isAaxFile }

// HasNoActivation reports whether the file needs no activation code.
func (f *FFmpeg) HasNoActivation() bool { return f.hasNoActivation }

// TranscodingError reports whether an error regex matched.
func (f *FFmpeg) TranscodingError() bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.transcodingError
}

// Version returns the parsed FFmpeg version.
func (f *FFmpeg) Version() *Version { return f.version }

// Success reports whether the run succeeded.
func (f *FFmpeg) Success() bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.success
}

// Aborted reports whether the run was cancelled.
func (f *FFmpeg) Aborted() bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.aborted
}

// The command templates.
const (
	tplTranscode   = `-hide_banner <ACTIVATION> -y <BEGIN> <END> -i "<INPUT>" -map_metadata -1 -map_chapters -1 <BITRATE> -vn <COPY> "<OUTPUT>"`
	tplTranscode2  = `-hide_banner <ACTIVATION> -y <BEGIN> <END> -i "<INPUT>" -i "<INPUT2>" -map_metadata 1 -map_chapters 1 <BITRATE> -vn <COPY> "<OUTPUT>"`
	tplExtractMeta = `-hide_banner -y -i "<INPUT>" -f ffmetadata "<OUTPUT>"`
	tplProbe       = `-hide_banner <ACTIVATION> -i "<INPUT>"`
	tplSilence     = `-hide_banner -y <ACTIVATION> -i "<INPUT>" -af silencedetect=noise=-30dB:d=0.25 -f null -`
	tplVersion     = `-version`

	tokenOutput     = "<OUTPUT>"
	tokenInput      = "<INPUT>"
	tokenInput2     = "<INPUT2>"
	tokenBitrate    = "<BITRATE>"
	tokenCopy       = "<COPY>"
	tokenActivation = "<ACTIVATION>"
	tokenActBytes   = "<ACT_BYTES>"
	tokenBegin      = "<BEGIN>"
	tokenEnd        = "<END>"
	tokenTSFrom     = "<TS_FROM>"
	tokenTSTo       = "<TS_TO>"

	activationParam = "-activation_bytes <ACT_BYTES>"
	beginParam      = "-ss <TS_FROM>"
	endParam        = "-to <TS_TO>"
	copyParam       = "-c:a copy"

	activationBytes = "-activation_bytes"
)

// The VBR quality table for MP3. The largest table entry not exceeding the
// target bit rate is the chosen quality.
var vbrMp3 = []uint{320, 250, 210, 195, 185, 150, 130, 120, 105, 85, 0}

// The regular expressions. They are compiled once.
var (
	rgxTimestamp1    = regexp.MustCompile(`(?i)frame=\s*?\d+.*time=(\d+:\d+:\d+\.\d+)`)
	rgxTimestamp2    = regexp.MustCompile(`(?i)size=\s*?\d+.*time=(\d+:\d+:\d+\.\d+)`)
	rgxDuration      = regexp.MustCompile(`(?i)Duration:\s*?(\d+:\d+:\d+\.\d+)`)
	rgxDurationEx    = regexp.MustCompile(`(?i)Duration:\s*?(\d+:\d+:\d+\.\d+).*bitrate:\s+(\d+)\s+kb/s`)
	rgxVersion       = regexp.MustCompile(`(?i)^ffmpeg version\s+([\d(\.|\-)]+)[\s\-_].*FFmpeg developers`)
	rgxVersionRel    = regexp.MustCompile(`(?i)^ffmpeg version\s+(.+)$`)
	rgxMuxFinal      = regexp.MustCompile(`(?i)video.*audio.*muxing overhead`)
	rgxErrorHeader   = regexp.MustCompile(`(?i)error reading header$`)
	rgxErrorActOpt   = regexp.MustCompile(`(?i)Error setting option activation_bytes`)
	rgxInvalid       = regexp.MustCompile(`(?i)Invalid data found when processing input$`)
	rgxNoActivOpt    = regexp.MustCompile(`(?i)Option activation_bytes not found`)
	rgxAudioStream   = regexp.MustCompile(`(?i)^\s+Stream.*audio:\s([a-z0-9]+)`)
	rgxAudioStreamEx = regexp.MustCompile(`(?i)^\s+Stream.*audio:\s([a-z0-9]+).*,\s+(\d+)\s+Hz,\s+(\w+),.*,\s+(\d+)\s+kb/s`)
	rgxAnyStream     = regexp.MustCompile(`(?i)^\s+Stream\s+`)
	rgxAaFile        = regexp.MustCompile(`(?i)Input #\d, aa, from`)
	rgxAaxFile       = regexp.MustCompile(`(?i)aax`)
	rgxErrorVarious  = regexp.MustCompile(`(?i)(Error while decoding|corrupt input packet|Conversion failed|aborting|Invalid data found when processing input)`)
	rgxChapter       = regexp.MustCompile(`(?i)^\s+Chapter.*start\s+(\d+\.?\d*).*end\s+(\d+\.?\d*)$`)
	rgxChapterMeta   = regexp.MustCompile(`(?i)^\s+Metadata:`)
	rgxChapterTitle  = regexp.MustCompile(`(?i)^\s+title.*:\s+(\w+.*)$`)
	rgxSilenceStart  = regexp.MustCompile(`(?i)^\[silencedetect.*\]\s+silence_start:\s+(\d+\.?\d*)$`)
	rgxSilenceEnd    = regexp.MustCompile(`(?i)^\[silencedetect.*\]\s+silence_end:\s+(\d+\.?\d*).*silence_duration:\s+(\d+\.?\d*)$`)
)

// SetBitRate computes the -b:a / -q:a / -ar parameters.
func (f *FFmpeg) SetBitRate(s *settings.Settings, audioQuality *AudioMeta, anyway bool) {
	sampleRate := audioQuality.SampleRate
	sourceBitrate := audioQuality.AvgBitRate
	reducing := false

	applicable, settingsBitrate := s.ApplicableBitRate(sourceBitrate)
	if applicable == 0 {
		if !anyway {
			return
		}
		applicable = sourceBitrate
	} else {
		reducing = true
		f.bitrateParam = getSampleRate(sampleRate, applicable)
	}

	if s.VariableBitRate {
		f.bitrateParam += getVariableBitrate(applicable, s.ConvFormat == settings.ConvFormatMP3)
	} else {
		if reducing {
			logging.Log(3, "FFmpeg", fmt.Sprintf("%sreduce: %d -> %d kb/s", f.id(), sourceBitrate, settingsBitrate))
		}
		f.bitrateParam += fmt.Sprintf("-b:a %dk", applicable)
	}
}

func (f *FFmpeg) id() string {
	return ""
}

func getSampleRate(sampleRate, bitrate uint) string {
	applicableSampleRate := uint(11025)
	if bitrate > 64 {
		applicableSampleRate *= 4
	} else if bitrate >= 32 {
		applicableSampleRate *= 2
	}
	applicableSampleRate = minUint(sampleRate, applicableSampleRate)
	if applicableSampleRate != sampleRate {
		logging.Log(3, "FFmpeg", fmt.Sprintf("reduce: %d -> %d Hz", sampleRate, applicableSampleRate))
		return fmt.Sprintf("-ar %d ", applicableSampleRate)
	}
	return ""
}

func getVariableBitrate(cbr uint, isMp3 bool) string {
	if isMp3 {
		return "-q:a " + getVariableBitrateMp3(cbr)
	}
	return "-q:a " + getVariableBitrateAac(cbr)
}

func getVariableBitrateAac(cbr uint) string {
	vbr := mathRound(float64(cbr)/100.0, 1)
	if vbr < 0.1 {
		vbr = 0.1
	}
	if vbr > 2.0 {
		vbr = 2.0
	}
	logging.Log(3, "FFmpeg", fmt.Sprintf("cbr %d kb/s -> vbr %v", cbr, vbr))
	return fmt.Sprintf("%v", vbr)
}

func getVariableBitrateMp3(cbr uint) string {
	vbr := 0
	for vbr < len(vbrMp3)-1 && cbr < vbrMp3[vbr+1] {
		vbr++
	}
	logging.Log(3, "FFmpeg", fmt.Sprintf("cbr %d kb/s -> vbr %d", cbr, vbr))
	return strconv.Itoa(vbr)
}

func mathRound(v float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10
	}
	return float64(int64(v*pow+0.5)) / pow
}

func minUint(a, b uint) uint {
	if a < b {
		return a
	}
	return b
}

// VerifyFFmpegPath runs `ffmpeg -version` and parses the version.
func (f *FFmpeg) VerifyFFmpegPath(relaxed bool) bool {
	f.relaxedVersionParsing = relaxed
	f.success = false

	param := tplVersion
	logging.Log(4, "FFmpeg", param)
	f.runProcess(FFmpegExePath(f.preferInternal64Bit, f.GetFFmpegDir), param, false, f.handleVersion)
	return f.success
}

// GetAudioMeta probes the input file for duration, sample rate, channels and
// bit rate.
func (f *FFmpeg) GetAudioMeta() bool {
	f.success = false
	f.aborted = false

	param := tplProbe
	param = strings.ReplaceAll(param, tokenActivation, "")
	param = strings.ReplaceAll(param, tokenInput, f.filenameIn)

	logging.Log(4, "FFmpeg", param)
	result := f.runProcess(FFmpegExePath(f.preferInternal64Bit, f.GetFFmpegDir), param, true, f.handleAudioMeta)
	_ = result
	return f.success
}

// VerifyActivation probes the input file with the given activation code. When
// withChapters is set it harvests the chapter list from the same output.
func (f *FFmpeg) VerifyActivation(actBytes string, withChapters bool) bool {
	f.success = true
	f.aborted = false
	f.hasNoActivation = false

	param := tplProbe
	if actBytes == "" {
		param = strings.ReplaceAll(param, tokenActivation, "")
	} else {
		param = strings.ReplaceAll(param, tokenActivation, activationParam)
		param = strings.ReplaceAll(param, tokenActBytes, actBytes)
	}
	param = strings.ReplaceAll(param, tokenInput, f.filenameIn)

	if withChapters {
		f.chapters = []*domain.Chapter{}
	}

	logging.Log(4, "FFmpeg", param)
	_ = f.runProcess(FFmpegExePath(f.preferInternal64Bit, f.GetFFmpegDir), param, true, f.handleActivation)
	return f.success
}

// Transcode runs the main conversion.
func (f *FFmpeg) Transcode(filenameOut string, modifiers ETranscode, actBytes string, from, to *time.Duration) bool {
	f.transcodingError = false
	f.success = false
	f.aborted = false
	f.matchedDuration = false
	f.audioMeta.Duration = 0

	param := tplTranscode
	if f.filenameMeta != "" {
		param = tplTranscode2
	}
	if actBytes == "" {
		param = strings.ReplaceAll(param, tokenActivation, "")
	} else {
		param = strings.ReplaceAll(param, tokenActivation, activationParam)
		param = strings.ReplaceAll(param, tokenActBytes, actBytes)
	}

	if from != nil {
		param = strings.ReplaceAll(param, tokenBegin, beginParam)
		param = strings.ReplaceAll(param, tokenTSFrom, fmt.Sprintf("%.3f", from.Seconds()))
		f.audioMeta.Begin = *from
	} else {
		param = strings.ReplaceAll(param, tokenBegin, "")
	}

	if to != nil {
		param = strings.ReplaceAll(param, tokenEnd, endParam)
		param = strings.ReplaceAll(param, tokenTSTo, fmt.Sprintf("%.3f", to.Seconds()))
		f.audioMeta.End = *to
	} else {
		param = strings.ReplaceAll(param, tokenEnd, "")
	}

	param = strings.ReplaceAll(param, tokenInput, f.filenameIn)
	param = strings.ReplaceAll(param, tokenOutput, filenameOut)

	if f.bitrateParam != "" {
		param = strings.ReplaceAll(param, tokenBitrate, f.bitrateParam)
	} else {
		param = strings.ReplaceAll(param, tokenBitrate, "")
	}

	if modifiers&ETranscodeCopy != 0 && f.bitrateParam == "" {
		param = strings.ReplaceAll(param, tokenCopy, copyParam)
	} else {
		param = strings.ReplaceAll(param, tokenCopy, "")
	}

	if f.filenameMeta != "" {
		param = strings.ReplaceAll(param, tokenInput2, f.filenameMeta)
	}

	logging.Log(4, "FFmpeg", param)
	result := f.runProcess(FFmpegExePath(f.preferInternal64Bit, f.GetFFmpegDir), param, true, f.handleTranscode)
	_ = result

	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.success && !f.aborted && !f.transcodingError
}

// ExtractMeta writes an ffmetadata file.
func (f *FFmpeg) ExtractMeta(filenameOut string) bool {
	f.success = true
	f.aborted = false
	f.audioMeta.Duration = 0

	param := tplExtractMeta
	param = strings.ReplaceAll(param, tokenInput, f.filenameIn)
	param = strings.ReplaceAll(param, tokenOutput, filenameOut)

	logging.Log(4, "FFmpeg", param)
	_ = f.runProcess(FFmpegExePath(f.preferInternal64Bit, f.GetFFmpegDir), param, true, f.handleMeta)
	return f.success && !f.aborted
}

// DetectSilence runs silencedetect over the whole input.
func (f *FFmpeg) DetectSilence(actBytes string) bool {
	f.success = false
	f.aborted = false
	f.matchedDuration = false
	f.audioMeta.Duration = 0
	f.silences = nil

	param := tplSilence
	if actBytes == "" {
		param = strings.ReplaceAll(param, tokenActivation, "")
	} else {
		param = strings.ReplaceAll(param, tokenActivation, activationParam)
		param = strings.ReplaceAll(param, tokenActBytes, actBytes)
	}
	param = strings.ReplaceAll(param, tokenInput, f.filenameIn)

	logging.Log(4, "FFmpeg", param)
	_ = f.runProcess(FFmpegExePath(f.preferInternal64Bit, f.GetFFmpegDir), param, true, f.handleSilence)
	return f.success && !f.aborted
}

// FFmpegExePath resolves the executable path.
func FFmpegExePath(preferInternal64Bit bool, getDir func() string) string {
	if preferInternal64Bit {
		if _, err := os.Stat("ffmpeg64.exe"); err == nil {
			return "ffmpeg64.exe"
		}
	}
	dir := ""
	if getDir != nil {
		dir = getDir()
	}
	if dir == "" || dir == "." {
		return "ffmpeg.exe"
	}
	if _, err := os.Stat(dir); err != nil {
		return "ffmpeg.exe"
	}
	return filepath.Join(dir, "ffmpeg.exe")
}

// runProcess runs the process, reading stdout or stderr concurrently.
func (f *FFmpeg) runProcess(exe, params string, stderr bool, handler func(string)) string {
	if _, err := os.Stat(exe); err != nil {
		return ""
	}

	args := splitArgs(params)

	cmd := exec.Command(exe, args...)
	cmd.StdinPipe()
	stdin, _ := cmd.StdinPipe()
	_ = stdin

	var pipe io.ReadCloser
	if stderr {
		p, _ := cmd.StderrPipe()
		pipe = p
	} else {
		p, _ := cmd.StdoutPipe()
		pipe = p
	}

	cmd.Start()
	if cmd.Process != nil {
		// best effort: below normal priority
		_ = cmd.Process
	}

	// read lines concurrently
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pipe)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if handler != nil {
				handler(line)
			}
			// cancellation check
			if f.cancel != nil && f.cancel() {
				f.mutex.Lock()
				f.aborted = true
				f.mutex.Unlock()
				if cmd.Process != nil && !isExited(cmd) {
					stdin.Write([]byte("q"))
				}
				break
			}
		}
	}()

	cmd.Wait()
	<-done

	return ""
}

// isExited reports whether the process has exited.
func isExited(cmd *exec.Cmd) bool {
	if cmd.Process == nil {
		return true
	}
	// ProcessState is set after Wait returns.
	return cmd.ProcessState != nil
}

// handleVersion parses the version line.
func (f *FFmpeg) handleVersion(line string) {
	if !f.relaxedVersionParsing {
		m := rgxVersion.FindStringSubmatch(line)
		if m != nil {
			vers := strings.ReplaceAll(m[1], "-", ".")
			if v, err := parseVersion(vers); err == nil {
				f.version = v
				f.success = true
			}
			return
		}
	} else {
		m := rgxVersionRel.FindStringSubmatch(line)
		if m != nil {
			vers := m[1]
			if v, err := parseVersion(strings.ReplaceAll(vers, "-", ".")); err == nil {
				f.version = v
			} else {
				f.version = &Version{Major: 9999, Minor: 9999}
			}
			f.success = true
			return
		}
	}
}

// handleAudioMeta parses the probe output.
func (f *FFmpeg) handleAudioMeta(line string) {
	m := rgxDurationEx.FindStringSubmatch(line)
	if m != nil {
		f.audioMeta.Duration = parseTimestamp(m[1])
		if n, err := strconv.ParseUint(m[2], 10, 32); err == nil {
			f.audioMeta.AvgBitRate = uint(n)
		}
		return
	}
	m = rgxAudioStreamEx.FindStringSubmatch(line)
	if m != nil {
		if n, err := strconv.ParseUint(m[2], 10, 32); err == nil {
			f.audioMeta.SampleRate = uint(n)
		}
		sc := strings.ToLower(m[3])
		if sc == "mono" {
			f.audioMeta.Channels = 1
		} else if sc == "stereo" {
			f.audioMeta.Channels = 2
		}
		// bit rate from stream, ignored in favour of the average
		f.success = true
		return
	}
	m = rgxAaFile.FindStringSubmatch(line)
	if m != nil {
		f.isAaFile = true
	}
}

// handleActivation parses the probe output with decryption.
func (f *FFmpeg) handleActivation(line string) {
	f.onCancel()

	if line == "" {
		return
	}
	logging.Log(4, "FFmpeg", line)

	if f.chapters != nil && !f.listComplete {
		m := rgxChapter.FindStringSubmatch(line)
		if m != nil {
			ch := domain.NewChapter()
			ch.Time.SetBegin(parseSeconds(m[1]))
			ch.Time.SetEnd(parseSeconds(m[2]))
			f.chapters = append(f.chapters, ch)
			return
		}
		m = rgxAnyStream.FindStringSubmatch(line)
		if m != nil {
			f.listComplete = true
		} else {
			var last *domain.Chapter
			if len(f.chapters) > 0 {
				last = f.chapters[len(f.chapters)-1]
			}
			if last != nil {
				m = rgxChapterMeta.FindStringSubmatch(line)
				if m != nil {
					last.Name = ""
					return
				}
				if last.Name == "" {
					m = rgxChapterTitle.FindStringSubmatch(line)
					if m != nil {
						last.Name = m[1]
					}
					return
				}
			}
		}
	}

	m := rgxErrorHeader.FindStringSubmatch(line)
	if m != nil {
		f.success = false
		return
	}
	m = rgxErrorActOpt.FindStringSubmatch(line)
	if m != nil {
		f.success = false
		return
	}
	m = rgxInvalid.FindStringSubmatch(line)
	if m != nil {
		f.success = false
		return
	}
	m = rgxNoActivOpt.FindStringSubmatch(line)
	if m != nil {
		f.hasNoActivation = true
		return
	}
	m = rgxAudioStream.FindStringSubmatch(line)
	if m != nil {
		format := strings.ToLower(m[1])
		f.isMp3Stream = format == "mp3"
		return
	}
	m = rgxAaFile.FindStringSubmatch(line)
	if m != nil {
		f.isAaFile = true
		return
	}
	m = rgxAaxFile.FindStringSubmatch(line)
	if m != nil {
		f.isAaxFile = true
		return
	}
}

// handleMeta parses the metadata extraction output.
func (f *FFmpeg) handleMeta(line string) {
	f.onCancel()
	logging.Log(4, "FFmpeg", line)
}

// handleSilence parses the silence detection output.
func (f *FFmpeg) handleSilence(line string) {
	f.onCancel()

	logging.Log(4, "FFmpeg", line)

	t := &f.audioMeta

	m := rgxDuration.FindStringSubmatch(line)
	if m != nil && !f.matchedDuration {
		f.audioMeta.Duration = parseTimestamp(m[1])
		f.matchedDuration = true
		return
	}

	if f.matchedDuration {
		m = rgxSilenceStart.FindStringSubmatch(line)
		if m != nil {
			if len(f.silences) > 0 {
				last := &f.silences[len(f.silences)-1]
				if last.End() == 0 {
					f.silences = f.silences[:len(f.silences)-1]
				}
			}
			start := parseSeconds(m[1])
			f.silences = append(f.silences, domain.NewTimeIntervalRange(start, 0))
			return
		}

		m = rgxSilenceEnd.FindStringSubmatch(line)
		if m != nil {
			if len(f.silences) == 0 {
				return
			}
			last := &f.silences[len(f.silences)-1]
			if last.End() != 0 {
				f.silences = f.silences[:len(f.silences)-1]
				return
			}
			end := parseSeconds(m[1])
			dur := parseSeconds(m[2])
			last.SetEnd(end)
			if mathAbs((last.Duration() - dur).Seconds()) > 0.01 {
				f.silences = f.silences[:len(f.silences)-1]
			}
			return
		}

		m = rgxTimestamp1.FindStringSubmatch(line)
		if m == nil {
			m = rgxMuxFinal.FindStringSubmatch(line)
			if m != nil {
				f.success = true
			}
			return
		}
	}

	if !f.matchedDuration {
		return
	}
	if t.Duration == 0 {
		return
	}

	ts := parseTimestamp(m[1])
	progress := ts.Seconds() / t.Duration.Seconds()
	if f.progress != nil {
		f.progress(progress)
	}
}

// handleTranscode parses the transcode output.
func (f *FFmpeg) handleTranscode(line string) {
	f.onCancel()

	logging.Log(4, "FFmpeg", line)

	t := &f.audioMeta

	m := rgxErrorVarious.FindStringSubmatch(line)
	if m != nil {
		f.transcodingError = true
	}

	m = rgxTimestamp1.FindStringSubmatch(line)
	if m == nil {
		m = rgxTimestamp2.FindStringSubmatch(line)
	}
	if m == nil {
		if !f.matchedDuration {
			m = rgxDuration.FindStringSubmatch(line)
			if m != nil {
				f.matchedDuration = true
				dur := parseTimestamp(m[1])
				if t.Begin == 0 {
					t.Duration = dur
				} else {
					t.Duration = dur - t.Begin
				}
				return
			}
		} else {
			m = rgxMuxFinal.FindStringSubmatch(line)
			if m != nil {
				f.success = true
			}
		}
		return
	}

	if t.Duration == 0 {
		return
	}

	ts := parseTimestamp(m[1])
	progress := ts.Seconds() / t.Duration.Seconds()
	if f.progress != nil {
		f.progress(progress)
	}
}

// onCancel checks the cancellation flag and writes 'q' to stdin.
func (f *FFmpeg) onCancel() {
	if f.cancel == nil {
		return
	}
	if !f.cancel() {
		return
	}
	f.mutex.Lock()
	f.aborted = true
	f.mutex.Unlock()

	if f.process != nil && f.process.Process != nil && f.process.ProcessState == nil {
		if stdin, err := f.process.StdinPipe(); err == nil {
			_, _ = stdin.Write([]byte("q"))
		}
	}
}

// parseVersion parses a version string.
func parseVersion(s string) (*Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty version")
	}
	v := &Version{}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, err
	}
	v.Major = n
	if len(parts) > 1 {
		n, err = strconv.Atoi(parts[1])
		if err == nil {
			v.Minor = n
		}
	}
	if len(parts) > 2 {
		n, err = strconv.Atoi(parts[2])
		if err == nil {
			v.Patch = n
		}
	}
	return v, nil
}

// Version is a parsed FFmpeg version.
type Version struct {
	Major int
	Minor int
	Patch int
}

func (v *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// parseTimestamp parses "HH:MM:SS.mmm" into a duration.
func parseTimestamp(s string) time.Duration {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	sec := 0.0
	fmt.Sscanf(parts[2], "%f", &sec)
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second))
}

// parseSeconds parses a seconds value into a duration.
func parseSeconds(s string) time.Duration {
	v, _ := strconv.ParseFloat(s, 64)
	return time.Duration(v * float64(time.Second))
}

// mathAbs returns the absolute value of a float64.
func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// SubstitActiv replaces the activation code in a command line with X's.
func SubstitActiv(s string) string {
	pos0 := strings.Index(s, activationBytes)
	if pos0 < 0 {
		return s
	}
	pos1 := strings.Index(s[pos0+1:], " ")
	if pos1 < 0 {
		return s
	}
	pos1 += pos0 + 1
	pos2 := strings.Index(s[pos1+1:], " ")
	if pos2 < 0 {
		return s
	}
	pos2 += pos1 + 1
	s = s[:pos1+1] + "XXXXXXXX" + s[pos2:]
	return s
}

// splitArgs splits a command line into arguments, respecting double quotes.
func splitArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
			continue
		}
		if (c == ' ' || c == '\t') && !inQuote {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteByte(c)
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}
