package metadata

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// ActivationCodes aggregates activation codes from three sources.
type ActivationCodes struct {
	// Codes is the de-duplicated list of codes, as 8 uppercase hex digits.
	Codes []string
}

// NewActivationCodes collects codes from all three sources.
func NewActivationCodes(userCode *uint32) *ActivationCodes {
	ac := &ActivationCodes{}
	seen := map[string]bool{}

	add := func(code string) {
		code = strings.ToUpper(strings.TrimSpace(code))
		if len(code) != 8 {
			return
		}
		if seen[code] {
			return
		}
		seen[code] = true
		ac.Codes = append(ac.Codes, code)
	}

	// 1. User setting.
	if userCode != nil {
		add(fmt.Sprintf("%08X", *userCode))
	}

	// 2. Registry.
	for _, code := range registryCodes() {
		add(code)
	}

	// 3. Audible app data.
	for _, code := range appDataCodes() {
		add(code)
	}

	return ac
}

// HasCode reports whether any code was found.
func (ac *ActivationCodes) HasCode() bool {
	return len(ac.Codes) > 0
}

// registryCodes reads activation codes from the Windows registry.
//
// The C# original reads HKLM\SOFTWARE\Audible\SWGIDMAP (and the Wow6432Node
// variant). Value names 0-8 are 4-byte activation blobs; 0xffffffff is
// skipped.
func registryCodes() []string {
	var codes []string

	keys := []string{
		`SOFTWARE\Audible\SWGIDMAP`,
		`SOFTWARE\Wow6432Node\Audible\SWGIDMAP`,
	}

	for _, keyPath := range keys {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		defer key.Close()

		names, err := key.ReadValueNames(0)
		if err != nil {
			continue
		}

		for _, name := range names {
			// Only value names 0-8.
			if len(name) != 1 || name[0] < '0' || name[0] > '8' {
				continue
			}

			// Read the value as binary.
			val, _, err := key.GetBinaryValue(name)
			if err != nil || len(val) < 4 {
				continue
			}

			code := binary.LittleEndian.Uint32(val[:4])
			if code == 0xFFFFFFFF {
				continue
			}
			codes = append(codes, fmt.Sprintf("%08X", code))
		}
	}

	return codes
}

// appDataCodes reads activation codes from the Audible app's data directory.
//
// The C# original reads the first 4 bytes of each AudibleActivation.sys file
// in %LOCALAPPDATA%\Packages\AudibleInc.Audible*\LocalState, newest first.
func appDataCodes() []string {
	var codes []string

	// Find Audible package directories.
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return codes
	}

	pattern := filepath.Join(localAppData, "Packages", "AudibleInc.Audible*", "LocalState")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return codes
	}

	type fileEntry struct {
		path    string
		modTime int64
	}
	var files []fileEntry

	for _, dir := range matches {
		// Find AudibleActivation.sys files.
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !strings.HasPrefix(entry.Name(), "AudibleActivation") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, fileEntry{
				path:    filepath.Join(dir, entry.Name()),
				modTime: info.ModTime().Unix(),
			})
		}
	}

	// Sort by modification time, newest first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime > files[j].modTime
	})

	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil || len(data) < 4 {
			continue
		}
		code := binary.LittleEndian.Uint32(data[:4])
		if code == 0xFFFFFFFF {
			continue
		}
		codes = append(codes, fmt.Sprintf("%08X", code))
	}

	return codes
}
