package domain

import (
	"runtime"
	"strconv"

	"github.com/ngbcoder/Unbind/internal/util"
)

// itoa is a short alias for strconv.Itoa, used throughout the domain.
func itoa(i int) string { return strconv.Itoa(i) }

// isNullOrWhiteSpace reports whether the string is empty or only whitespace.
func isNullOrWhiteSpace(s string) bool { return util.IsNullOrWhiteSpace(s) }

// is64BitOperatingSystem reports whether the process runs on a 64-bit system.
// The 64-bit FFmpeg is only selected on such a system.
var is64BitOperatingSystem = runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"
