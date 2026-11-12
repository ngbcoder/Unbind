// Package logging provides the process-wide, level-filtered file logger.
//
// The semantics are inherited from the C# original (AuxLib/Logging.cs):
//
//   - Levels are plain integers with inverted meaning. A message is written when
//     level <= Level. The default level is -1, i.e. logging is off unless the
//     user asks for it with -Log=<n>.
//   - Output format: "HH:mm:ss.fff {threadId:0000} [{context}] {message}".
//   - Files live in %LOCALAPPDATA%\<Company>\<AppName>\log and are named
//     "<AppName>_<yyyy-MM-dd>_<000>.log". Rotation happens on a date change or
//     at 20 MB; after 1000 files in one day the counter wraps.
//   - The user's profile path is replaced by the literal "USER" in every logged
//     path, so a shared log does not reveal the account name.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level thresholds. A message is emitted when its level is <= the configured
// level.
const (
	LevelOff    = -1
	LevelError  = 1
	LevelInfo   = 2
	LevelDetail = 3
	LevelTrace  = 4
)

const (
	maxFileSize    = 20 * 1024 * 1024
	maxFilesPerDay = 1000
	timeFormat     = "15:04:05.000"
	fileTimeFormat = "2006-01-02"
)

var (
	mu sync.Mutex

	level = LevelOff

	// appName and company determine the log directory and file name.
	appName = "AAXAudioConverter"
	company = "audiamus"

	// out is the current log file. It is nil until the first message is written.
	out *os.File

	// outDay is the day the current file belongs to.
	outDay string

	// outIndex is the counter within the current day.
	outIndex int

	// instantFlush writes every message straight through. Enabled by -Log.
	instantFlush = false

	// now is swappable for tests.
	now = time.Now
)

// SetLevel sets the active level. Values <= LevelOff disable logging.
func SetLevel(l int) {
	mu.Lock()
	defer mu.Unlock()
	level = l
	if l >= 0 {
		instantFlush = true
	}
}

// Level returns the active level.
func CurrentLevel() int {
	mu.Lock()
	defer mu.Unlock()
	return level
}

// SetAppName sets the application name used for the log directory and file name.
func SetAppName(name string) {
	mu.Lock()
	defer mu.Unlock()
	appName = name
}

// SetCompany sets the company name used for the log directory.
func SetCompany(name string) {
	mu.Lock()
	defer mu.Unlock()
	company = name
}

// Log writes a message if the level passes the filter. ctx is an arbitrary
// value whose string representation identifies the origin of the message; it is
// usually the caller's type or a short name.
func Log(l int, ctx any, msg string) {
	mu.Lock()
	defer mu.Unlock()
	if l > level {
		return
	}
	writeLocked(l, ctx, msg)
}

// Logf is the lazy form of Log: the message is built only when the level
// passes. Use it for expensive messages.
func Logf(l int, ctx any, fn func() string) {
	mu.Lock()
	if l > level {
		mu.Unlock()
		return
	}
	msg := fn()
	mu.Unlock()
	Log(l, ctx, msg)
}

// writeLocked writes one message. The caller holds mu.
func writeLocked(l int, ctx any, msg string) {
	f := ensureFileLocked()
	if f == nil {
		return
	}

	t := now()
	threadID := 0
	fmt.Fprintf(f, "%s %04d [%v] %s\n",
		t.Format(timeFormat),
		threadID,
		contextString(ctx),
		SubstitUser(msg),
	)
	if instantFlush {
		_ = f.Sync()
	}
}

// contextString renders the context of a message.
func contextString(ctx any) string {
	switch v := ctx.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ensureFileLocked opens the log file, rotating when the day changed or the
// file grew too large. It returns nil if the file cannot be opened; logging
// must never break the application.
func ensureFileLocked() *os.File {
	if out != nil {
		if day := now().Format(fileTimeFormat); day != outDay {
			_ = out.Close()
			out = nil
		}
	}
	if out != nil {
		if fi, err := out.Stat(); err == nil && fi.Size() < maxFileSize {
			return out
		}
		_ = out.Close()
		out = nil
	}

	dir := LogDirectory()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}

	day := now().Format(fileTimeFormat)
	if outDay != day {
		outDay = day
		outIndex = 0
	}

	name := filepath.Join(dir, fmt.Sprintf("%s_%s_%03d.log", appName, day, outIndex))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	out = f
	outIndex = (outIndex + 1) % maxFilesPerDay
	return out
}

// LogDirectory returns the directory holding the log files.
func LogDirectory() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, company, appName, "log")
}

// Close releases the current log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if out != nil {
		_ = out.Close()
		out = nil
	}
}

// SubstUser replaces the user's profile directory with the literal "USER" so
// that a log file can be shared without disclosing the account name.
func SubstitUser(s string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return s
	}
	if !strings.ContainsRune(s, os.PathSeparator) {
		return s
	}
	return strings.ReplaceAll(s, home, "USER")
}
