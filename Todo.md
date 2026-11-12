# Unbind — Implementation Plan

Living document. Update as work lands. Newest work goes at the top of its section.

Legend: `[ ]` todo · `[~]` in progress · `[x]` done · `[!]` blocked

---

## Overview

**Unbind** is a Go rewrite of *AAX Audio Converter* (`audiamus/AaxAudioConverter`,
v1.18.2), a Windows desktop application that converts Audible `.aax` / `.aa`
audiobook containers into ordinary `.mp3` / `.m4a` / `.m4b` files.

The full behavioural specification lives in [`design.md`](design.md) — the original
C# design document. This file tracks the Go implementation. The cross-application
handoff contract with **Unseal** lives in [`URIhandoff.md`](URIhandoff.md).

**Legal stance (normative, carried over from the original):** Unbind does not
break DRM. It applies the user's *own* activation code — the same bytes the
official Audible client uses — to FFmpeg's `-activation_bytes` option. It never
asks for, stores or transmits an account password. Output is for personal
archiving and conversion only and must not be uploaded to open servers,
torrents or any mass-distribution channel.

**The DRM is removed upstream.** Unseal (the Go rewrite of Book Lib Connect)
downloads and de-DRM's books and exports them as `.aax` plus Audible-shaped
sidecar JSON. Those files need no activation code, so Unbind's primary flow is
"convert what Unseal produced". The activation-code machinery survives as a
fallback for users who point Unbind at a still-encrypted file.

---

## Tech stack

| Layer | Technology | Why |
|-------|-----------|-----|
| Language | Go 1.26.4 | Matches the rest of the workspace; single static binary |
| **Constraint** | **`CGO_ENABLED=0` everywhere** | No cgo toolchain needed to build, cross-compilation is trivial, no gcc on the build machine |
| **Target** | **Windows x64** | The only supported platform for now, matching the original. Other platforms are not built, tested or shipped. |
| UI | `github.com/golang-gui/goui` v0.4.0 | Native desktop toolkit, no cgo — native APIs are loaded at runtime through purego. Ships the widget set this app needs: data table with sortable headers, tabs, split view, overlays, native file dialogs and native drag-and-drop. |
| MP3 tags | `github.com/bogem/id3v2/v2` v2.1.4 | ID3v2.3/2.4 read **and** write, cover art, custom `TXXX` frames, and native `CHAP` chapter frames. Replaces ATL for MP3. |
| MP4 tags | FFmpeg (`-metadata`, `-map_chapters`) | FFmpeg writes `ilst` metadata and a QuickTime chapter track in the same pass as the transcode. Avoids hand-rolling Nero `chpl` atoms and `stco` offset fixup. |
| MP4 read | `github.com/abema/go-mp4` v1.7.4 | Reads `moov/udta/meta/ilst` and `covr` from the source `.aax`, including the freeform `----` atoms that carry narrator, publisher and description. |
| Local IPC | `github.com/Microsoft/go-winio` v0.6.4 | Windows named pipes, used by the URI handoff receiver. |
| Parallelism | `golang.org/x/sync/errgroup` | Bounded concurrency with context cancellation. |
| OS integration | `golang.org/x/sys/windows` | Registry, named mutex, process times, long paths. |
| JSON | stdlib `encoding/json` | Replaces Newtonsoft.Json. |
| Charset | `golang.org/x/net/html/charset` | Replaces Ude.NetStandard for sidecar detection. |

**Build.** No make, no scripts. The Go toolchain is the build system:

```sh
go build ./...                                    # build everything
go build -o bin/unbind ./cmd/unbind               # build the binary
go test ./...                                    # run the tests
go test -race ./...                              # run the tests with the race detector
go vet ./...                                     # vet
gofmt -l .                                       # formatting check
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...   # release build
```

A release build stamps the version, commit and build date:

```sh
go build -trimpath -ldflags "-s -w \
  -X github.com/ngbcoder/Unbind/internal/version.Version=$(git describe --tags --always) \
  -X github.com/ngbcoder/Unbind/internal/version.Commit=$(git rev-parse --short HEAD) \
  -X github.com/ngbcoder/Unbind/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/unbind ./cmd/unbind
```

**Layers (enforced by import direction, never by a linter):**

| # | Layer | Packages | Rule |
|---|-------|----------|------|
| L0 | Foundation | `logging`, `platform`, `version` | No product knowledge. Pure utilities. |
| L1 | Configuration | `settings` | Enumerations and the settings interface hierarchy. Implemented by the app. |
| L2 | Domain | `domain`, `chapters`, `naming` | The book/part/chapter/track model and every pure algorithm. No I/O. |
| L3 | External | `ffmpeg`, `tags`, `metadata` | FFmpeg process control, tag read/write, Audible sidecar parsing. |
| L4 | Application | `engine` | The conversion pipeline. Depends on L2 and L3 through interfaces. |
| L5 | Presentation | `ui` | GOUI desktop widgets. Depends on L4 through interfaces only. |
| L6 | Composition root | `app`, `cmd/unbind` | Wiring, settings, startup sequence, CLI, single instance. |

---

## Identity

| Item | Value |
|------|-------|
| Program | Unbind |
| Binary | `unbind.exe` |
| Module | `github.com/ngbcoder/Unbind` |
| Repository | `github.com/ngbcoder/Unbind` (public) |
| License | GPL-3.0 — required, the upstream C# project is GPL-3.0 and this is a derivative |
| Attribution | README credits `audiamus/AaxAudioConverter` and its author |
| App id (GOUI) | `com.ngbcoder.unbind` |
| Config root | `%LOCALAPPDATA%\audiamus\Unbind\` |
| URI scheme | `unbind://` — see [`URIhandoff.md`](URIhandoff.md) |
| Named pipe | `\\.\pipe\Unbind.handoff` |
| Update manifest | `res/Setup.json` in this repository |

---

## Project layout

```
unbind/
├── cmd/unbind/            entry point, CLI, single-instance arbitration
├── internal/
│   ├── app/               composition root, startup sequence
│   ├── chapters/          cue sheet, chapter-mark adjustment, silence handling
│   ├── domain/            Book, Part, Chapter, Track, TimeInterval, AaxFileItem
│   ├── engine/            conversion pipeline, mode dispatch, parallelism
│   ├── ffmpeg/            process host, command templates, output parsing
│   ├── logging/           leveled logging facility
│   ├── metadata/          Audible sidecars, ASIN resolution, activation codes
│   ├── naming/            file and folder naming, tag values, counters
│   ├── platform/          registry, mutex, long paths, process times, IPC
│   ├── progress/          progress protocol, per-mille accumulator
│   ├── settings/          enumerations, settings interfaces, JSON store
│   ├── tags/              MP3 and MP4 tag read/write
│   ├── ui/                GOUI desktop interface
│   ├── util/              pure helpers
│   └── version/           build identity
├── docs/                  architecture, ADRs, user manual
├── res/                   update manifest, images
├── Todo.md
├── URIhandoff.md          the Unseal ⇄ Unbind handoff contract
├── design.md              the original C# design document (reference spec)
└── README.md
```

---

## Deviations from the design document

`design.md` is the specification of **product behaviour**, not a template for
internals. Where the Go implementation departs from it, the departure is
recorded here so it is a decision rather than an accident.

| # | Design document says | Unbind does | Why |
|---|---|---|---|
| 1 | .NET Framework 4.8, WinForms, x86+x64 | Go 1.26.4, GOUI, windows/amd64 | One static binary, no framework install, no cgo. GOUI replaces WinForms; it ships the table, tabs, split view, overlays and native file dialogs the app needs. |
| 2 | ATL (`z440.atl.core`) reads and writes tags | `bogem/id3v2/v2` for MP3; FFmpeg writes MP4 tags and chapters; `go-mp4` reads the source | ATL has no Go equivalent. Splitting by container keeps each side on a library that does it properly. The MP4 side avoids hand-rolling Nero `chpl` atoms and `stco` offset fixup. |
| 3 | `Parallel.ForEach` with a hand-rolled degree-of-parallelism helper | `errgroup` with `SetLimit` | Same semantics, less code. The mode-aware DOP rules are preserved verbatim in `engine`. |
| 4 | `IProgress<T>` and `IInteractionCallback` with a captured `SynchronizationContext` | `context.Context` plus a channel-based interactor posted to the GUI thread | The C# pattern exists only because .NET had no equivalent of GOUI's `app.Post`. |
| 5 | `ApplicationSettingsBase` with a custom `LocalFileSettingsProvider` for a hash-free path | A JSON file at a fixed path | The custom provider existed because the .NET default derives the path from a hash of the executable path and version. A JSON file has a stable path by construction, so the provider is unnecessary. |
| 6 | `AuxWin32Lib/Win32FileIO`, a pinned-buffer Win32 copy | `io.Copy` with a 64 KB buffer | The P/Invoke copy was a .NET allocation optimisation. Go's own copy is already efficient and portable. |
| 7 | `PerfLib` reads Windows performance counters by PID | `GetProcessTimes` on the child process handle | The perf-counter instance resolution in the original was the fiddliest part of that library, and it was only ever used for a diagnostic bar. |
| 8 | `PropGridLib` renders the naming settings as a property grid | A purpose-built GOUI widget | `PropGridLib` is a WinForms reflection grid with no Go counterpart, and the settings it rendered are a fixed set anyway. |
| 9 | `TreeDecomposition` dumps the settings object graph | A small reflect-based dumper in `logging` | ~80 lines replaces a whole project. |
| 10 | `AuxLib/Logging.cs` | `internal/logging` | Same levels, same file format, same rotation, same path redaction. |
| 11 | A named mutex on the assembly GUID, one instance only | A named mutex plus a named pipe, one instance only | The pipe is what lets a second launch hand its files to the running instance. See `URIhandoff.md`. |
| 12 | MP3 chapters written by ATL, or by FFmpeg behind the `MP3_CHAPTERS_BY_FFMPEG` switch | Always by `bogem/id3v2` `CHAP` frames | The library has native chapter support, so the compile-time switch and the FFmpeg fallback are both unnecessary. |
| 13 | `IntermedCopySingle`, a two-pass intermediate copy for iTunes compatibility | Kept as a setting, re-evaluated after the FFmpeg chapter path lands | FFmpeg now owns the `moov` layout. The two-pass copy may be a no-op; it must not be silently dropped. |
| 14 | `Logging.Log(uint, string)` resolves to the `Type` overload and throws | Not ported | A known C# defect (design §18.7). |
| 15 | `SettingsForm.btnFfmpegLoc_Click` compares the new value with itself | Not ported | A known C# defect (design §22.12). |
| 16 | `ContentMetaDataJson.cs`, an orphaned near-duplicate of the compiled model | Not ported | Dead weight in the original (design §22.13). |
| 17 | A hierarchy of settings interfaces, so the engine never sees the app's settings class | One concrete `settings.Settings` struct | Go has no designer-generated settings class, so there is nothing to hide. The app and the engine share one struct, the user interface edits its fields directly, and a conversion takes a frozen `Snapshot` so that a later edit cannot change a run in progress. Tests construct the struct directly instead of implementing an interface. |

**Preserved without deviation** because they are behaviour, not structure: the
four conversion modes, the four-level parallelism and its mode-aware degree
rules, the chapter-mark adjustment heuristics and all their thresholds, the cue
sheet algorithm, the part-completeness arithmetic, the naming and tagging
matrix, the four conversion output modes, the output-folder layout, the
non-empty-folder conflict resolution, the rollback on cancellation, the
playlist format, the extra metadata files, the `.aax` archive, the activation
code sources, the Audible sidecar formats, the online update channel, and the
DRM position.

---

## M0 — Foundation · DONE

- [x] `go.mod` — module `github.com/ngbcoder/Unbind`, Go 1.26.4
- [x] `internal/logging` — leveled logger with the C# message format
- [x] `internal/logging` — level filter, off by default, `-Log=<n>` switches it on
- [x] `internal/logging` — file rotation on a date change and at 20 MB
- [x] `internal/logging` — 1000-files-per-day counter with wraparound
- [x] `internal/logging` — `SubstitUser` path redaction
- [x] `internal/logging` — lazy message form so expensive strings are not built
- [x] `internal/domain` — `TimeInterval` with the C# clamping rules
- [x] `internal/domain` — `Chapter` with the cue sheet and the adjustment algorithms
- [x] `internal/domain` — `Track` with the 1 ms minimum duration floor

## M1 — Domain model

- [x] `internal/settings` — `EConvFormat` (`mp3`, `mp4`)
- [x] `internal/settings` — `EConvMode` (`single`, `chapters`, `splitChapters`, `splitTime`)
- [x] `internal/settings` — `EFileNaming` (5 values)
- [x] `internal/settings` — `ETitleNaming` (7 values)
- [x] `internal/settings` — `ETrackNumbering` (3 values)
- [x] `internal/settings` — `ELongTitle` (4 values)
- [x] `internal/settings` — `EGeneralNaming` (3 values)
- [x] `internal/settings` — `EGeneralNamingEx` (4 values)
- [x] `internal/settings` — `EFlatFolderNaming` (2 values)
- [x] `internal/settings` — `EAaxCopyMode` flags enum with the 1/2/4 bit matrix
- [x] `internal/settings` — `ENamedChapters` (3 values)
- [x] `internal/settings` — `EOnlineUpdate` (3 values)
- [x] `internal/settings` — `EVerifyAdjustChapterMarks` (3 values)
- [x] `internal/settings` — `EFixAACEncoding` (3 values)
- [x] `internal/settings` — `EReducedBitRate` (8 values)
- [x] `internal/settings` — `EPreferEmbeddedChapterTimes` (3 values)
- [x] `internal/settings` — `ERoleTagAssignment` (5 values, 2 deprecated)
- [x] `internal/settings` — `EOutFolderConflict` (4 values)
- [x] `internal/settings` — `EProgressPhase` (5 values)
- [x] `internal/settings` — `ECallbackType` (10 values)
- [x] `internal/settings` — the `Settings` struct, the shared contract
- [x] `internal/settings` — the C# enum names, the localized rendering depends on them
- [x] `internal/settings` — the deprecated narrator role assignments
- [x] `internal/settings` — `TrackDuration` — the clamping range per mode
- [x] `internal/settings` — `ApplicableBitRate` — the source and the requested rate
- [x] `internal/settings` — `UseEmbeddedChapterTimes`
- [x] `internal/settings` — `Snapshot` — a frozen copy for a conversion run
- [x] `internal/domain` — `AaxFileItem` — the input file record
- [x] `internal/domain` — `AaxFileItem` — equality by file name, case-insensitive
- [x] `internal/domain` — `AaxFileItem` — `.aa` versus `.aax` selects the tag mapping
- [x] `internal/domain` — `Book` — `SortingTitle` identity
- [x] `internal/domain` — `Book` — `Parts` and the `PartNumber`
- [x] `internal/domain` — `Book` — `Caption` pair, one for files and one for tags
- [x] `internal/domain` — `Book` — `ExternalMeta` from the Audible sidecars
- [x] `internal/domain` — `Book` — `CheckParts` — the triangular-number completeness test
- [x] `internal/domain` — `Book` — `PartsType` `none` / `some` / `all`
- [x] `internal/domain` — `Book` — `InitAuthorTitle` — the runtime title regex
- [x] `internal/domain` — `Book` — `makeLongBookTitle` — the four long-title styles
- [x] `internal/domain` — `Book` — `PruneTitle` — colon and slash become semicolons
- [x] `internal/domain` — `Book` — `ExternalBookMeta.SeqString` — the `"1.5"` fraction
- [x] `internal/domain` — `Book` — `ChapterNumber` — local versus global numbering
- [x] `internal/domain` — `Book` — `Counts` — the chapter and track counters
- [x] `internal/domain` — `Part` — the chapter duality, `Chapters` and `Chapters2`
- [x] `internal/domain` — `Part` — `SwapChapters` and the `namedChaptersIn1` flag
- [x] `internal/domain` — `Part` — `NamedChapters` and `EmbeddedChapters` accessors
- [x] `internal/domain` — `Part` — `ComplementChapterNames` — unnamed chapters become `(n)`
- [x] `internal/domain` — `Part` — `ApplicableInFileNameForTranscode` — the input chain
- [x] `internal/domain` — `Part` — `IsMp3Stream` with the cached override
- [x] `internal/domain` — `Part` — `SetFFmpegPrefer64Bit` — the 32-hour threshold
- [x] `internal/domain` — `Part` — `IsTrancodeConversion` — copy versus re-encode
- [x] `internal/domain` — `Part` — `TrackId` and `ChapterId`
- [x] `internal/domain` — `Chapter` — `HasNamedChapters` — both lists non-empty
- [x] `internal/domain` — `Chapter` — `TimeAdjustments` — the pending delta
- [x] `internal/util` — `Prune` — the invalid and the doubtful characters
- [x] `internal/util` — `Shorten` — the C# elision with a single ellipsis
- [x] `internal/util` — `SplitTrim` and `Combine`

## M2 — Chapter algorithms

- [ ] `internal/chapters` — `CreateCueSheet` — the number of tracks per chapter
- [ ] `internal/chapters` — `CreateCueSheet` — the search window, one third of the average length
- [ ] `internal/chapters` — `CreateCueSheet` — the 500 ms silence filter
- [ ] `internal/chapters` — `CreateCueSheet` — the silence midpoint cut
- [ ] `internal/chapters` — `CreateCueSheet` — the 10 ms epsilon at the chapter end
- [ ] `internal/chapters` — `CreateCueSheet` — the time-split variant
- [ ] `internal/chapters` — `DetermineTimeAdjustment` — the two strategies
- [ ] `internal/chapters` — `handleChapterMark` — the general case
- [ ] `internal/chapters` — `handlePartTitle` — the `-- Part I --` pattern
- [ ] `internal/chapters` — `closestToChapterMark` — the nearest-silence picker
- [ ] `internal/chapters` — `setAdjustment` — the 300 ms lead-in
- [ ] `internal/chapters` — `AdjustTime` — apply the pending delta
- [ ] `internal/chapters` — `AdjustTime` — migrate silences across the boundary
- [ ] `internal/chapters` — `AdjustTime` — drop zero-length silences
- [ ] `internal/chapters` — `SetNewEnd` — accept only inside a silence, unless `always`
- [ ] `internal/chapters` — `MergeSilences` — the `splitTime` aggregate chapter
- [ ] `internal/chapters` — `InsertPaddingChapters` — cover head and tail gaps
- [ ] `internal/chapters` — `RemovePaddingChapters`
- [ ] `internal/chapters` — `PreferEmbeddedChapterTimes` — match named to embedded by end time
- [ ] `internal/chapters` — `findMatchingChapters` — the 10-second match threshold
- [ ] `internal/chapters` — `findMatchingChapters` — the duplicate-match duration tiebreak
- [ ] `internal/chapters` — `SetMetaChapters` — one chapter per track
- [ ] `internal/chapters` — `SetMetaChapters` — rebased chapters for the time-split mode
- [ ] `internal/chapters` — `preProcessNamedChapters` — trim the embedded list to the window
- [ ] `internal/chapters` — `adjustNamedChapters` — truncate at the window edges
- [ ] `internal/chapters` — `preProcessLastChapterResidue` — drop the 1 ms stub
- [ ] `internal/chapters` — `combineChaptersSingleMode` — one chapter per part
- [ ] `internal/chapters` — `preprocessEmbeddedChapters` — drop implausibly short chapters
- [ ] `internal/chapters` — the 10 s / 25 s very-short and short thresholds

## M3 — Naming and tagging

- [x] `internal/naming` — `Prune` — strip the invalid file name characters
- [x] `internal/naming` — `Prune` — strip the 60+ doubtful typographic characters
- [x] `internal/naming` — `Prune` — trim trailing dots
- [x] `internal/naming` — `fileChapters` — the five `EFileNaming` schemes
- [x] `internal/naming` — `fileSingleNone` — the single-file schemes
- [x] `internal/naming` — `fileSingleSome` — the part-number schemes
- [x] `internal/naming` — `fullPath` — the folder layout matrix
- [x] `internal/naming` — the part and chapter sub-folder prefixes
- [x] `internal/naming` — `Numbers` — the track, chapter, part and disc counters
- [x] `internal/naming` — `Numbers` — local numbering for `EParts.some`
- [x] `internal/naming` — `Numbers` — global numbering across parts
- [x] `internal/naming` — `Numbers` — the digit counts for zero padding
- [x] `internal/naming` — `SetTrackTitle` — the tag title schemes
- [x] `internal/naming` — `TrackForFile` — replace the slash in the track component
- [x] `internal/naming` — `GetGenre` — the genre resolution and the fallback
- [ ] `internal/naming` — `buildPerformerTag` — the role assignments
- [ ] `internal/naming` — the `wmrk` watermark tag

## M4 — Settings, progress and diagnostics

- [ ] `internal/settings` — the concrete settings struct
- [ ] `internal/settings` — the JSON store with an atomic write
- [ ] `internal/settings` — user-scoped versus application-scoped settings
- [ ] `internal/settings` — the settings dump for the log
- [x] `internal/progress` — the progress message protocol
- [x] `internal/progress` — `AddTotalSteps` may be negative
- [x] `internal/progress` — the per-mille accumulator
- [x] `internal/progress` — `ResetSteps` rolls the segment into the accumulator
- [x] `internal/progress` — the progress info factories
- [ ] `internal/progress` — the `…Cancel` variants that remove an item
- [ ] `internal/progress` — the graceful degradation ladder for the status line
- [ ] `internal/logging` — the settings dump via reflection
- [ ] `internal/logging` — the cycle detection and the depth guard

## M5 — FFmpeg integration · DONE

- [x] `internal/ffmpeg` — the process host, stdout and stderr read concurrently
- [x] `internal/ffmpeg` — the command templates with `<PLACEHOLDER>` tokens
- [x] `internal/ffmpeg` — the token substitution table
- [x] `internal/ffmpeg` — `VerifyFFmpegPath` — parse `ffmpeg -version`
- [x] `internal/ffmpeg` — the relaxed version parse fallback
- [x] `internal/ffmpeg` — `GetAudioMeta` — the probe
- [x] `internal/ffmpeg` — `VerifyActivation` — the probe with `-activation_bytes`
- [x] `internal/ffmpeg` — `Transcode` — the main conversion
- [x] `internal/ffmpeg` — `ExtractMeta` — write an `ffmetadata` file
- [x] `internal/ffmpeg` — `DetectSilence` — the `silencedetect` filter
- [x] `internal/ffmpeg` — the duration and duration-plus-bitrate regexes
- [x] `internal/ffmpeg` — the audio stream regexes
- [x] `internal/ffmpeg` — the chapter and chapter-title regexes
- [x] `internal/ffmpeg` — the silence start and end regexes
- [x] `internal/ffmpeg` — the timestamp regexes for the progress
- [x] `internal/ffmpeg` — the mux-overhead success marker
- [x] `internal/ffmpeg` — the error regexes
- [x] `internal/ffmpeg` — the `.aa` and `.aax` detection regexes
- [x] `internal/ffmpeg` — the no-activation-needed and bad-activation regexes
- [x] `internal/ffmpeg` — the cancellation handler writes `q` to stdin
- [x] `internal/ffmpeg` — the fractional progress callback
- [x] `internal/ffmpeg` — `SetBitRate` — the `-b:a` / `-q:a` computation
- [x] `internal/ffmpeg` — the sample-rate cap that follows the bit rate
- [x] `internal/ffmpeg` — the VBR quality table for MP3
- [x] `internal/ffmpeg` — the executable resolution, 64-bit and the user directory
- [x] `internal/ffmpeg` — the `ffmetadata` chapter file format
- [x] `internal/ffmpeg` — the tests for the regexes, the parsing and the helpers

## M6 — Tag read and write · DONE

- [x] `internal/tags` — the `Reader` and `Writer` interfaces
- [x] `internal/tags` — the MP3 reader over `bogem/id3v2`
- [x] `internal/tags` — the MP3 writer over `bogem/id3v2`
- [x] `internal/tags` — the MP3 cover art as an `APIC` frame
- [x] `internal/tags` — the MP3 custom tags as `TXXX` frames
- [x] `internal/tags` — the MP3 chapters as `CHAP` frames
- [x] `internal/tags` — the MP3 ID3v2 version 4 by default
- [x] `internal/tags` — the MP4 reader over `go-mp4`
- [x] `internal/tags` — the MP4 `ilst` walk
- [x] `internal/tags` — the MP4 freeform `----` atoms for narrator, publisher, description, release date
- [x] `internal/tags` — the MP4 cover art from `covr`
- [x] `internal/tags` — the publishing-date fallback chain
- [x] `internal/tags` — the copyright-string year fallback
- [x] `internal/tags` — the two-year future guard on the year
- [ ] `internal/tags` — the MP4 writer — FFmpeg writes the metadata (M8)
- [ ] `internal/tags` — the MP4 writer — FFmpeg writes the chapters via `ffmetadata` (M8)
- [ ] `internal/tags` — the MP4 writer — the cover via an attached picture input (M8)
- [ ] `internal/tags` — the `©rldt` custom release date (needs a real .aax to verify)

## M7 — Audible metadata · DONE

- [x] `internal/metadata` — the ASIN regex
- [x] `internal/metadata` — `AsinJsonFile` — the local, relative and absolute search
- [x] `internal/metadata` — `content_metadata_<ASIN>.json` — the chapter tree
- [x] `internal/metadata` — the empty-title repair, a single dot
- [x] `internal/metadata` — the zero-length chapter repair, the 1 ms floor
- [x] `internal/metadata` — the brand intro and outro durations
- [x] `internal/metadata` — `SimsBySeries/<ASIN>.json` — the product record
- [x] `internal/metadata` — `series_titles_<ASIN>.json` — the series list
- [x] `internal/metadata` — the SKU lookup for multi-part books
- [x] `internal/metadata` — the `StartsWith` SKU comparison
- [x] `internal/metadata` — the activation code from the user setting
- [x] `internal/metadata` — the activation code from `HKLM\SOFTWARE\Audible\SWGIDMAP`
- [x] `internal/metadata` — the activation code from `AudibleActivation.sys`
- [x] `internal/metadata` — the eight-hex-digit code format
- [x] `internal/metadata` — the activation code redaction in the log
- [x] `internal/metadata` — the tests for the ASIN, the sidecars and the codes
- [ ] `internal/metadata` — the discarded-file cache, the sidecar scan is cheap enough for now
- [ ] `internal/metadata` — the absolute search in the Audible package directories

## M8 — Conversion pipeline

- [ ] `internal/engine` — group the input files into books by sorting title
- [ ] `internal/engine` — the part regex, built from the settings
- [ ] `internal/engine` — the sorting title cleanup
- [ ] `internal/engine` — the phase plan and the weighted phases
- [ ] `internal/engine` — the activation verify loop, locked
- [ ] `internal/engine` — the `HasNoActivation` retry
- [ ] `internal/engine` — the per-book plan
- [ ] `internal/engine` — `convertSingleMode`
- [ ] `internal/engine` — `convertChapterMode`
- [ ] `internal/engine` — `convertSplitChapterMode`
- [ ] `internal/engine` — `convertSplitTimeMode`
- [ ] `internal/engine` — the chapter-scoped silence detection
- [ ] `internal/engine` — the serial parts, parallel chapters rule
- [ ] `internal/engine` — `verifyAdjustChapterMarks` — the re-transcode of affected chapters
- [ ] `internal/engine` — the four-level parallelism and the mode-aware degree rules
- [ ] `internal/engine` — the `NonParallel` escape hatch
- [ ] `internal/engine` — the output directory layout
- [ ] `internal/engine` — the flat-folder variants
- [ ] `internal/engine` — the non-empty-folder conflict, the three-way answer
- [ ] `internal/engine` — the "new folder" numeric suffix
- [ ] `internal/engine` — the "apply to all" handling
- [ ] `internal/engine` — the stale content cleanup
- [ ] `internal/engine` — the AAC `esds` atom patch
- [ ] `internal/engine` — the transcode, the copy-versus-re-encode decision
- [ ] `internal/engine` — the intermediate copy for iTunes compatibility
- [ ] `internal/engine` — the rollback on cancellation
- [ ] `internal/engine` — the `IsNewAuthor` and `IsNewSeries` rollback flags
- [ ] `internal/engine` — the playlist writer, Latin-1 or UTF-8
- [ ] `internal/engine` — the extra metadata files, the text sheet and the cover
- [ ] `internal/engine` — the `.aax` archive with the `EAaxCopyMode` flags
- [ ] `internal/engine` — the temp directory, shared across the run
- [ ] `internal/engine` — the file error accumulator

## M9 — URI handoff receiver

- [x] `internal/app` — the `unbind://` URI parser
- [x] `internal/app` — the URI verbs, `add` and `show`
- [x] `internal/app` — the URI parameters, `p`, `doc`, `activate`, `src`
- [x] `internal/app` — the handoff document schema and the `protocol` field
- [x] `internal/app` — the path validation rules
- [x] `internal/app` — the deduplication, case-insensitive on Windows
- [ ] `internal/app` — the CLI parser, the positional files and `--handoff`
- [ ] `internal/app` — the named mutex, the primary and secondary arbitration
- [ ] `internal/platform` — the named pipe server
- [ ] `internal/platform` — the named pipe client
- [ ] `internal/platform` — the length-prefixed message framing
- [ ] `internal/platform` — the pipe security descriptor, owner rights only
- [ ] `internal/app` — the secondary forwards the URI and exits
- [ ] `internal/app` — the primary handles the request on the GUI thread
- [ ] `internal/app` — the handoff does not auto-convert
- [ ] `internal/app` — the handoff during a running conversion

## M10 — Desktop interface

- [x] `internal/ui` — the GOUI entry point, `ui.Run` from `main` only
- [x] `internal/ui` — the Modern theme via `modern.FromSettings`
- [x] `internal/ui` — the main window shell
- [x] `internal/ui` — the Add files button, the native file dialog
- [x] `internal/ui` — the onboarding wizard, the first-run detection
- [x] `internal/ui` — the onboarding wizard, the step navigation
- [x] `internal/ui` — the onboarding wizard, the skip and the finish
- [ ] `internal/ui` — the file table, nine columns
- [ ] `internal/ui` — the file table, sortable headers
- [ ] `internal/ui` — the file table, the numeric columns sort by their real value
- [ ] `internal/ui` — the file table, the stable row identifiers
- [ ] `internal/ui` — the file table, the green converted marker
- [ ] `internal/ui` — the format selector
- [ ] `internal/ui` — the mode selector
- [ ] `internal/ui` — the output folder picker, a native dialog
- [ ] `internal/ui` — the naming property grid, a custom widget
- [ ] `internal/ui` — the two progress bars, the phase bar and the step bar
- [ ] `internal/ui` — the status line, the graceful degradation ladder
- [ ] `internal/ui` — the CPU load and the process count indicators
- [ ] `internal/ui` — the convert and abort buttons
- [ ] `internal/ui` — the completion summary, up to eight errors
- [ ] `internal/ui` — the settings dialog, five tabs
- [ ] `internal/ui` — the settings dialog, the General tab
- [ ] `internal/ui` — the settings dialog, the Folder tab
- [ ] `internal/ui` — the settings dialog, the Conversion tab
- [ ] `internal/ui` — the settings dialog, the Chapters tab
- [ ] `internal/ui` — the settings dialog, the Meta Tags tab
- [ ] `internal/ui` — the per-book preview dialog
- [ ] `internal/ui` — the file details dialog with the cover
- [ ] `internal/ui` — the activation code dialog
- [ ] `internal/ui` — the FFmpeg location dialog
- [ ] `internal/ui` — the genres dialog
- [ ] `internal/ui` — the about dialog
- [ ] `internal/ui` — the whats-new dialog
- [ ] `internal/ui` — the first-run tip
- [ ] `internal/ui` — drag-and-drop of `.aax` files onto the window
- [ ] `internal/ui` — the background work marshalled back with `app.Post`
- [ ] `internal/ui` — plain field changes trigger `app.RequestUpdate`
- [ ] `internal/ui` — the timers created once and disposed by their owner

## M11 — Operations

- [ ] `internal/update` — fetch the manifest from `res/Setup.json`
- [ ] `internal/update` — compare the published version against the running version
- [ ] `internal/update` — the checksum verification
- [ ] `internal/update` — the deferred install when the app is busy
- [ ] `internal/update` — the declined-package list
- [ ] `internal/platform` — the `.aax` and `.aa` file associations
- [ ] `internal/platform` — the `unbind://` protocol handler registration
- [ ] `internal/platform` — the `SHChangeNotify` after a registration change
- [ ] `internal/platform` — the single-instance mutex
- [ ] `internal/platform` — the process CPU usage via `GetProcessTimes`
- [ ] `internal/platform` — the long-path `\\?\` prefixing
- [ ] `internal/logging` — the temp and log housekeeping
- [ ] `internal/i18n` — the resource strings for English and German
- [ ] `internal/i18n` — the runtime culture switch, the app restarts

## M12 — Packaging and release

- [ ] `goui.toml` — the app id, the icon and the version resources
- [ ] `cmd/unbind` — the `cmd/goui` build produces the Windows resources
- [ ] `res/Setup.json` — the update manifest
- [ ] `src/InnoSetup` — the installer script
- [ ] `src/InnoSetup` — ship `ffmpeg.exe` and `ffmpeg64.exe`
- [ ] `src/InnoSetup` — the per-user `unbind://` registration
- [ ] `src/InnoSetup` — the uninstall removes the protocol handler
- [ ] `docs/` — the user manual
- [ ] `docs/` — the architecture document
- [ ] `.github/workflows/ci.yml` — the format check, the vet, the race tests
- [ ] `.github/workflows/ci.yml` — the build matrix, windows/amd64
- [ ] `.github/workflows/release.yml` — build, checksum, publish a GitHub release
- [ ] `README.md` — the screenshots, the usage, the legal notice, the upstream attribution
- [ ] `LICENSE` — GPL-3.0

## M13 — Unseal integration

Lives in the **Unseal** repository, `github.com/ngbcoder/Unseal`.

- [ ] `internal/db` — a new `ExportedFilePath` column on the conversion entity
- [ ] `internal/export` — record the exported `.aax` path on success
- [ ] `internal/export` — the sidecars match the shapes the converter parses
- [ ] `internal/handoff` — build the handoff document
- [ ] `internal/handoff` — write it atomically to the temp directory
- [ ] `internal/handoff` — build the `unbind://add?doc=…` URI
- [ ] `internal/handoff` — spawn the converter executable
- [ ] `internal/handoff` — detect an immediate spawn failure
- [ ] `internal/config` — the `UnbindPath` setting
- [ ] `internal/config` — auto-detect from the uninstall registry key
- [ ] `internal/ui` — the "Convert with Unbind" button in the download queue
- [ ] `internal/ui` — the button is enabled only for `exported` and `converted` rows
- [ ] `internal/ui` — the tooltip explains why the button is disabled
- [ ] `internal/ui` — multiple books can be handed over in one request
- [ ] `internal/ui` — one file per part for a multi-part book
- [ ] `internal/ui` — the sender's state machine does not change
- [ ] `testdata/handoff/` — the golden fixtures, identical in both repositories

---

## Testing

The C# original has **no automated tests**. The chapter-adjustment heuristics and
the cue sheet are the most likely places for a regression to go unnoticed, so
they are the first thing to cover.

- [x] `internal/domain` — `TimeInterval` — begin past end, end before begin, duration reset
- [x] `internal/domain` — `Book.CheckParts` — none, some, all, a missing part, a single part 1
- [x] `internal/domain` — `ExternalBookMeta.SeqString` — `"1"`, `"1.5"`, `"2.0"`, no series
- [x] `internal/chapters` — `CreateCueSheet` — no silences, one long silence
- [x] `internal/chapters` — `CreateCueSheet` — the split-time variant
- [x] `internal/chapters` — `CreateCueSheet` — a silence shorter than 500 ms is ignored
- [x] `internal/chapters` — `DetermineTimeAdjustment` — the general chapter mark
- [x] `internal/chapters` — `DetermineTimeAdjustment` — a short trailing silence is split
- [x] `internal/chapters` — `DetermineTimeAdjustment` — a long trailing silence is left alone
- [x] `internal/chapters` — `AdjustTime` — the end moves earlier and the silence is clipped
- [x] `internal/chapters` — `AdjustTime` — the silence migrates to the next chapter
- [x] `internal/chapters` — `SetNewEnd` — accepted only inside a silence, unless `always`
- [x] `internal/chapters` — the chapter duality — `SwapChapters` and the accessors
- [x] `internal/chapters` — the padding chapters — insert and remove
- [x] `internal/util` — `Prune` — every invalid character, the doubtful set, trailing dots, empty input
- [x] `internal/util` — `Shorten` — the C# elision with a single ellipsis
- [x] `internal/settings` — the enum names round-trip, the localised rendering depends on them
- [x] `internal/settings` — the role assignment ordinals, they are explicit in C#
- [x] `internal/settings` — the bit rate clamp, the track duration clamp
- [x] `internal/settings` — `ApplicableBitRate`, `UseEmbeddedChapterTimes`, `FixNarrator`
- [x] `internal/domain` — the title regex — a non-Latin title must not be truncated
- [ ] `internal/chapters` — `CreateCueSheet` — a chapter shorter than the target length
- [ ] `internal/chapters` — `CreateCueSheet` — a chapter exactly at the boundary
- [ ] `internal/chapters` — `DetermineTimeAdjustment` — the part-title pattern
- [ ] `internal/chapters` — `PreferEmbeddedChapterTimes` — an unambiguous match, an ambiguous match, no match
- [x] `internal/naming` — `Numbers` — none, some, all, a single track, more than 999 tracks
- [x] `internal/naming` — the naming matrix, every `EFileNaming` × `EConvMode` × `EParts`
- [ ] `internal/platform` — `ExUnc` — a short path, a long path, a UNC path, an already-converted path
- [ ] `internal/ffmpeg` — the output parsing, one case per regex
- [x] `internal/app` — the URI parsing, the golden fixtures
- [x] `internal/app` — the handoff document, the golden fixtures
- [x] `internal/progress` — the accumulator and the per-mille conversion
- [ ] integration — convert a three-chapter book in each of the four modes
- [ ] integration — convert a two-part book, `EParts.all`, one playlist
- [ ] integration — convert a partial part set, `EParts.some`
- [ ] integration — cancel mid-conversion, the output folder is removed
- [ ] integration — a wrong activation code, the dialog appears, a good code recovers
- [ ] integration — a file with no activation needed, the conversion succeeds
- [ ] integration — a 44.1 kHz book with the AAC fix, the `esds` atom is patched
- [ ] integration — a book with more than 255 chapters
- [ ] integration — a path longer than 260 characters
- [ ] regression — golden-file comparison of the names, the tags and the chapter markers

---

## Known gaps

- [ ] **ffmpeg is an external runtime dependency.** The C# original shipped it in
      the installer; the Go port must do the same or bootstrap it.
- [ ] **Only Windows is supported.** The code avoids OS-specific calls outside
      `internal/platform` and `internal/ui`, so other platforms could be added
      later, but they are not built, tested or shipped yet.
- [ ] **GOUI v0.4.0 is young.** It was published on 2026-10-26 and its API is
      still moving, so the interface layer will need occasional adjustment.
- [ ] **The user interface can only be tested by hand.** The pipeline and the
      pure algorithms get real unit tests; the GOUI layer gets model tests and
      the semantic snapshots that `gui/dev` produces.

---

## Backlog (post-v1)

- [ ] A reverse handoff channel, so Unseal learns when the conversion finished
- [ ] Nero `chpl` chapters in the MP4 output, alongside the QuickTime track
- [ ] An `AutoStartOnHandoff` setting
- [ ] A portable build, with the named pipe replaced by a Unix domain socket
- [ ] A property-grid-free settings screen, one screen per concern
- [ ] Cover art cache with a bounded size
- [ ] A library index exported as CSV or JSON
