# AAX Audio Converter — System Design

> **Document type:** As-built architecture & design specification
> **Product:** AAX Audio Converter (`AaxAudioConverter.exe`)
> **Version described:** 1.18.2
> **Repository:** `P:\Nbprojects\C\AAXAudioConverter` (`audiamus/AaxAudioConverter`)
> **Status:** Living document — update alongside code changes
> **Target framework:** .NET Framework 4.8, Windows 10/11, x86+x64

---

## Table of Contents

1. [Document Control](#1-document-control)
2. [Introduction](#2-introduction)
3. [System Context](#3-system-context)
4. [Solution Structure](#4-solution-structure)
5. [Layering and Dependency Rules](#5-layering-and-dependency-rules)
6. [Core Domain Model](#6-core-domain-model)
7. [Configuration Model](#7-configuration-model)
8. [The Conversion Pipeline](#8-the-conversion-pipeline)
9. [FFmpeg Integration Layer](#9-ffmpeg-integration-layer)
10. [Parallelism and Scheduling](#10-parallelism-and-scheduling)
11. [Progress and Status Reporting](#11-progress-and-status-reporting)
12. [Naming and Tagging Subsystem](#12-naming-and-tagging-subsystem)
13. [Metadata Acquisition](#13-metadata-acquisition)
14. [User Interface Architecture](#14-user-interface-architecture)
15. [Diagnostics, Logging and Performance Monitoring](#15-diagnostics-logging-and-performance-monitoring)
16. [Online Update Subsystem](#16-online-update-subsystem)
17. [Build, Packaging and Deployment](#17-build-packaging-and-deployment)
18. [Error Handling and Failure Modes](#18-error-handling-and-failure-modes)
19. [Security and Privacy](#19-security-and-privacy)
20. [Performance Characteristics](#20-performance-characteristics)
21. [Design Decisions and Rationale](#21-design-decisions-and-rationale)
22. [Known Limitations and Technical Debt](#22-known-limitations-and-technical-debt)
23. [Extension Guide](#23-extension-guide)
24. [Testing Strategy](#24-testing-strategy)
25. [Appendices](#25-appendices)

---

## 1. Document Control

### 1.1 Purpose of this document

This document specifies the **complete architecture of AAX Audio Converter** — the Windows
desktop application that converts Audible `.aax` (and legacy `.aa`) audiobook files into
plain `.mp3` or `.m4a`/`.m4b` files.

It is written to be read by three audiences:

| Audience | Use |
|---|---|
| **New maintainers** | Sections 3–7 give the map of the system; start at §4, then §6, then §8. |
| **Feature implementers** | §8 (pipeline), §12 (naming), §23 (extension guide) are the working references. |
| **Reviewers / auditors** | §9, §13, §18, §19 document the trust boundaries around DRM, external processes, and the network. |

The document is **descriptive first**: it describes the system as it exists in the source tree,
with file and line references so every claim is checkable. Where a design decision was
deliberate but non-obvious, the rationale is recorded in §21.

### 1.2 Source-of-truth map

Every statement in this document traces to one of these roots:

| Path | Contents |
|---|---|
| `src/AaxAudioConverterLib/` | Engine library — all conversion logic, no UI. |
| `src/AaxAudioConverter/` | WinForms application — presentation and interaction. |
| `src/AuxLib/` | Framework-agnostic support: logging, process hosting, extensions. |
| `src/AuxWinLib/` | Windows-only support: dialogs, culture, list-view sorting. |
| `src/AuxWin32Lib/` | P/Invoke layer: long-path file I/O, file associations. |
| `src/PerfLib/` | Performance counters (CPU load, process count). |
| `src/PropGridLib/` | Reflection-driven property grid for the naming settings UI. |
| `src/TreeDecomposition/` | Generic tree decomposition helper (used by the settings UI tree). |
| `src/InnoSetup/` | Inno Setup installer script. |
| `res/Setup.json` | Update manifest consumed by the online-update subsystem. |

### 1.3 Conventions

* **"the engine"** — `AaxAudioConverterLib` (`audiamus.aaxconv.lib`), the class
  `audiamus.aaxconv.lib.AaxAudioConverter`.
* **"the app"** — `AaxAudioConverter` (`audiamus.aaxconv`), the WinForms executable.
* **Source references** use `File.cs:line` against the tree above.
* Diagrams are ASCII so they render in any terminal or diff viewer.

---

## 2. Introduction

### 2.1 Product purpose

AAX Audio Converter takes DRM-protected Audible audiobook containers and produces ordinary,
unprotected audio files that any media player can open, while preserving as much of the
original metadata as possible.

```
.aax / .aa  ──[activation code]──▶  FFmpeg (DRM decode)  ──▶  .mp3 / .m4a / .m4b
                                          │
                                          └── tags, chapters, cover art, playlist
```

### 2.2 Scope

**In scope (what the system does):**

* Ingest `.aax` and `.aa` files from disk, drag-and-drop, command line, or file association.
* Locate and apply an Audible activation code from three independent sources.
* Decrypt via FFmpeg using the activation bytes (the user applies *their own* code — the
  system performs no cracking).
* Produce output in four structural modes: single file, per chapter, chapter + time split,
  and pure time split.
* Preserve and rewrite metadata: title, author, narrator, series, genre, year, publisher,
  copyright, cover art, disc/track numbers, and chapter markers.
* Emit an M3U playlist when more than one output file is produced.
* Optionally emit companion `.txt` metadata files and `.jpg`/`.png` cover files.
* Optionally copy the original `.aax` and its Audible metadata sidecars to an archive folder.
* Monitor CPU load and FFmpeg process count live during conversion.
* Check for, download, verify and install application updates.

**Out of scope (explicitly):**

* Breaking or circumventing DRM. The application only ever applies an activation code that
  is already bound to the user's own Audible account.
* Uploading, distributing or sharing de-DRM'd audio.
* Any non-Windows platform.
* Audio editing, normalisation, or mastering beyond format conversion and optional
  bit-rate reduction.
* Reading the `.aax` container format directly. **All** container parsing, DRM handling and
  audio decoding is delegated to FFmpeg.

### 2.3 Non-functional requirements

| # | Requirement | How it is met |
|---|---|---|
| NFR-1 | Must not lose a user's audio on cancellation | Rollback logic in `deleteOutDirectory` (§8.12) removes partial output on cancel |
| NFR-2 | Must handle books with >255 chapters | Chapters stored in own list, not byte-limited tag slots (§12.6) |
| NFR-3 | Must handle very long books (>32 h) | 64-bit FFmpeg auto-selected; `SetFFmpegPrefer64Bit` (§8.4.2) |
| NFR-4 | Must use all available cores | Nested `Parallel.ForEach` with mode-aware degree-of-parallelism (§10) |
| NFR-5 | Must support paths >260 characters | UNC `\\?\` prefixing throughout (`ExUnc`, §8.9) |
| NFR-6 | Must not leak the activation code into logs | `SubstitActiv` redaction (§19.2) |
| NFR-7 | Must be diagnosable from user-supplied logs | Structured logging with levels, thread ids, timestamps (§15.1) |
| NFR-8 | Must support English and German | Per-form `.resx` satellite resources; runtime culture switch (§14.6) |
| NFR-9 | Must be DPI-correct on high-DPI monitors | `PerMonitorV2` DPI awareness in `App.config` (§14.7) |
| NFR-10 | Must only run one instance | Named mutex on `AssemblyGuid` (`Program.cs:16`) |

### 2.4 Terminology

| Term | Meaning in this codebase |
|---|---|
| **AAX** | Audible's proprietary audiobook container (`.aax`). |
| **AA** | Legacy Audible container (`.aa`). |
| **Part** | One physical file of a multi-part book. `Book.Part`. |
| **Book** | A logical work, possibly composed of several parts. `Book`. |
| **Chapter** | A named time range within a part. `Chapter`. |
| **Track** | One output audio file. `Track`. |
| **Embedded chapters** | Chapter markers carried inside the `.aax` container (read from FFmpeg probe output). |
| **Named chapters** | Chapter titles + times from Audible's `content_metadata_*.json` sidecar. |
| **Activation code / bytes** | 8 hex digits (4 bytes) that FFmpeg passes via `-activation_bytes`. |
| **DRM** | The Audible encryption wrapping the AAC payload. |
| **Cue sheet** | The algorithm that subdivides a chapter into multiple tracks at silence points. |
| **iTunes compatibility** | Special handling that lets very long `.m4b` files play in iTunes (§21.7). |
| **Phase** | A coarse stage of the pipeline: `silence`, `adjust`, `transcoding`, `copying`. |
| **Step** | A fine-grained unit of progress (one part, one chapter, one track). |

---

## 3. System Context

### 3.1 Actors and external systems

```
                         ┌──────────────────────────────────────────────┐
                         │                 AAX Audio Converter          │
                         │   (AaxAudioConverter.exe, .NET FW 4.8)       │
                         └──────────────────────────────────────────────┘
                                    │            │            │
            ┌───────────────────────┘            │            └────────────────────┐
            │                                    │                                 │
            ▼                                    ▼                                 ▼
   ┌────────────────────┐            ┌──────────────────────┐          ┌─────────────────────┐
   │  Audible .aax/.aa  │            │      FFmpeg         │          │  Audible desktop   │
   │  input files       │            │  (ffmpeg.exe /       │          │  app (MS Store)    │
   │                    │            │   ffmpeg64.exe)      │          │  content + LocalState│
   └────────┬───────────┘            └──────────┬───────────┘          └──────────┬──────────┘
            │                                   │                                 │
            │ read via FFmpeg probe            │ exec, parse stderr              │ read JSON / .sys
            ▼                                   ▼                                 ▼
   ┌────────────────────┐            ┌──────────────────────┐          ┌─────────────────────┐
   │  ATL (z440) tag    │            │  Activation code    │          │ content_metadata_   │
   │  reader            │            │  sources:           │          │ <ASIN>.json         │
   │                    │            │  • user setting      │          │ SimsBySeries/       │
   └────────────────────┘            │  • HKLM\...\SWGIDMAP│          │ series_titles_*.json│
                                    │  • AudibleActivation │          └─────────────────────┘
                                    │    .sys (app data)   │
                                    └──────────────────────┘
                                                 ▲
                                                 │ activation bytes
                                                 ▼
                                    ┌──────────────────────┐
                                    │  Windows Registry   │
                                    │  HKLM\SOFTWARE\      │
                                    │   Audible\SWGIDMAP   │
                                    └──────────────────────┘

   ┌──────────────────────────────────────────────────────────────────────────┐
   │  GitHub (raw) ──▶ res/Setup.json ──▶ setup.exe download (MD5 verified)  │
   └──────────────────────────────────────────────────────────────────────────┘
```

### 3.2 External dependency inventory

| Dependency | Kind | Version / constraint | Failure mode |
|---|---|---|---|
| **FFmpeg** | External executable | Tested against **4.1**; `FFMPEG_VERSION = new Version(4, 1)` at `AaxAudioConverter.cs:46` | Version below minimum → user prompt; missing → `FFmpegLocationForm` |
| **ATL** (`z440.atl.core`) | NuGet, in-process | 4.20.0 | Tag read/write fails → file error recorded |
| **Newtonsoft.Json** | NuGet, in-process | 13.0.2 | Sidecar parse fails → metadata silently skipped |
| **Ude.NetStandard** | NuGet, in-process | 1.2.0 | Charset detection for sidecars |
| **Costura.Fody** | Build-time weaver | 4.1.0 | Embeds managed dependencies into the EXE |
| **Microsoft.WindowsAPICodePack** | NuGet, in-process | 1.1.4 | Common file/folder dialogs |
| **Audible desktop app** | Pre-installed software | Any Store version | Only needed for activation code + sidecar metadata |
| **Audible activation code** | User credential | 4 bytes | Required for DRM-protected content; *not* required for BLC-downloaded books |
| **Network** | HTTPS to `raw.githubusercontent.com` | Optional | Update check fails → silently ignored |

### 3.3 High-level architecture

```
┌───────────────────────────────────────────────────────────────────────────────────┐
│  PRESENTATION LAYER                       src/AaxAudioConverter (WinExe)           │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────────┐  │
│  │ MainForm   │ │SettingsForm│ │PreviewForm │ │ FileDetails│ │ FFmpegLocation │  │
│  │            │ │            │ │            │ │   Form     │ │     Form       │  │
│  └─────┬──────┘ └─────┬──────┘ └─────┬──────┘ └────────────┘ └────────────────┘  │
│        │              │              │                                            │
│        └──────────────┴──── ProgressProcessor ── InteractionCallbackHandler ─────┘ │
│                                   │                        │                      │
└───────────────────────────────────┼────────────────────────┼──────────────────────┘
                                    │  IProgress<ProgressMessage> │ IInteractionCallback
                                    ▼                        ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│  ENGINE LAYER                            src/AaxAudioConverterLib                  │
│                                                                                   │
│   ┌──────────────────────────────────────────────────────────────────────────┐    │
│   │  AaxAudioConverter  (facade / orchestrator)                             │    │
│   │   AddFilesAsync → ConvertAsync → convert → convertBooksParallel         │    │
│   │   → convertBook → {initDirectory, fixAAC, mode dispatch, transcode}     │    │
│   └───────┬────────────────────────────┬───────────────────────┬────────────┘    │
│           │                            │                       │                 │
│           ▼                            ▼                       ▼                 │
│   ┌───────────────┐          ┌──────────────────┐    ┌────────────────────────┐    │
│   │ Book / Part   │          │  FFmpeg (wrapper) │    │ TagAndFileNamingHelper │    │
│   │ Chapter/Track │          │  + ATL tagging    │    │  (names, folders, tags)│    │
│   └───────────────┘          └──────────────────┘    └────────────────────────┘    │
│           │                            │                       │                 │
│           ▼                            ▼                       ▼                 │
│   ┌──────────────────────────────────────────────────────────────────────────┐    │
│   │  Metadata: AudibleAppContentMetadata, AudibleAppSimsBySeries,          │    │
│   │            ActivationCode{,App,Registry}, ExtraMetaFiles, AaxFileCopier  │    │
│   └──────────────────────────────────────────────────────────────────────────┘    │
└───────────────┬───────────────────────────────────────────────────────────────────┘
                │
                ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│  SUPPORT LAYERS                                                                   │
│  AuxLib (logging, process host, extensions) │ AuxWin32Lib (long paths, file assoc)│
│  AuxWinLib (dialogs, culture) │ PerfLib (CPU/process counters) │ PropGridLib     │
│  TreeDecomposition                                                                     │
└───────────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Solution Structure

### 4.1 Project inventory

The solution `src/AAX Audio Converter.sln` contains **nine build projects** plus one
solution folder.

| Project | Output | Root namespace | Purpose |
|---|---|---|---|
| `AaxAudioConverter` | `AaxAudioConverter.exe` (WinExe) | `audiamus.aaxconv` | WinForms shell, all forms, user settings binding |
| `AaxAudioConverterLib` | `AaxAudioConverterLib.dll` | `audiamus.aaxconv.lib` | Conversion engine; **no UI references** |
| `AuxLib` | `AuxLib.dll` | `audiamus.aux` | Logging, process hosting, string/path extensions, settings provider |
| `AuxWinLib` | `AuxWinLib.dll` | `audiamus.aux.win` | `MsgBox`, culture switching, list-view column sorter, system menu |
| `AuxWin.DialogBox` | `AuxWin.DialogBox.dll` | `audiamus.aux.win.dialogbox` | Win32 hook-based message box (modern visual style on legacy shells) |
| `AuxWin32Lib` | `AuxWin32Lib.dll` | `audiamus.aux.w32` | `Win32FileIO` (long-path safe buffered copy), `FileAssociations` |
| `PerfLib` | `PerfLib.dll` | `audiamus.perf` | `PerformanceMonitor`, CPU load %, process count |
| `PropGridLib` | `PropGridLib.dll` | `audiamus.aux.propgrid` | Dynamic property descriptors for the naming grid |
| `TreeDecomposition` | `TreeDecomposition.dll` | — | Generic tree decomposition used by settings UI |
| `InnoSetup` | *(solution folder)* | — | `AaxAudioConverter setup.iss` installer script |

### 4.2 Dependency graph

```
                        ┌──────────────────────────┐
                        │      AaxAudioConverter   │  (app)
                        └────────────┬─────────────┘
                                     │
        ┌──────────────┬─────────────┼──────────────┬──────────────────┐
        ▼              ▼             ▼              ▼                  ▼
┌────────────┐ ┌────────────┐ ┌─────────────┐ ┌──────────┐  ┌────────────────┐
│  AuxLib    │ │ AuxWinLib  │ │  AuxWin32   │ │ PerfLib  │  │  PropGridLib   │
└─────┬──────┘ └─────┬──────┘ └─────────────┘ └──────────┘  └────────────────┘
      │              │ └──────────────► AuxWin.DialogBox
      │              │
      └──────┬───────┘
             ▼
   ┌─────────────────────────┐
   │   AaxAudioConverterLib  │  (engine)
   └────────────┬────────────┘
                │ references
                ├──► AuxLib
                ├──► AuxWin32Lib
                └──► TreeDecomposition
```

**Key property:** the engine project `AaxAudioConverterLib` has **zero** references to
`System.Windows.Forms` or to the app project. Its reference list (`AaxAudioConverterLib.csproj:36-60`)
contains only `System`, `System.Core`, `System.Data`, `System.Net.Http`, `System.Xml`,
`System.Xml.Linq`, `Newtonsoft.Json`, `ATL`, `Ude.NetStandard`, and two `System.*` compat
shims. This makes the engine unit-testable in isolation and keeps the UI replaceable.

### 4.3 The engine's public surface

`audiamus.aaxconv.lib.AaxAudioConverter` is `public partial class ... : IPreviewTitle, IDisposable`.

| Member | Signature | Purpose |
|---|---|---|
| ctor | `AaxAudioConverter(IConvSettings settings, IResources resources)` | Binds settings + localized strings |
| `AddFilesAsync` | `Task<IEnumerable<AaxFileItem>> AddFilesAsync(IEnumerable<string>, IInteractionCallback<InteractionMessage, bool?>)` | Filter + probe candidate files |
| `ConvertAsync` | `Task ConvertAsync(IEnumerable<AaxFileItem>, CancellationToken, IProgress<ProgressMessage>, IInteractionCallback<InteractionMessage, bool?>)` | The single entry point for all conversion work |
| `VerifyFFmpegPath` | `Version VerifyFFmpegPath(bool? relaxedFFmpegVersionCheck = null)` | Probes `ffmpeg -version`, returns parsed version |
| `VerifyFFmpegPathVersionAsync` | `Task<bool> ...(Func<InteractionMessage, bool?>, bool? = null)` | Verify + prompt if version too old |
| `TitlePreview` | `(string tag, string file) TitlePreview(AaxFileItem)` | Preview of final tag/file names |
| `SortingTitleWithPart` | `string SortingTitleWithPart(AaxFileItem)` | Sorting key, part-stripped |
| `PruneTitle` / `GetGenre` | `string` | Filename sanitisation / genre resolution |
| `ReinitActivationCode` | `bool ReinitActivationCode()` | Re-scan all activation code sources |
| Properties | `HasActivationCode`, `NumericActivationCodes`, `Settings`, `AppContentDirectory`, `AutoLaunchAudioFile`, `StopwatchElapsed`, `FileErrors`, `VerifiedFFmpegPath` | Observable state for the UI |

---

## 5. Layering and Dependency Rules

```
┌──────────────────────────────────────────────────────────────────────┐
│ L4  UI          AaxAudioConverter (WinForms)                        │
│                 Forms · ProgressProcessor · PGANaming · FileAssoc   │
├──────────────────────────────────────────────────────────────────────┤
│ L3  ENGINE      AaxAudioConverterLib                                │
│                 AaxAudioConverter · Book · Chapter · Track           │
│                 FFmpeg · TagAndFileNamingHelper · Metadata readers  │
├──────────────────────────────────────────────────────────────────────┤
│ L2  PLATFORM    AuxLib · AuxWinLib · AuxWin32Lib · PerfLib          │
│                 Logging · ProcessHost · Win32FileIO · FileAssoc     │
├──────────────────────────────────────────────────────────────────────┤
│ L1  EXTERNAL    FFmpeg.exe · Audible app · ATL · Newtonsoft.Json     │
└──────────────────────────────────────────────────────────────────────┘
```

**Rules enforced in the tree:**

* **R-1** The engine never references the UI. Enforced by project references.
* **R-2** The engine never references `System.Windows.Forms`. Dialogs are surfaced to the
  host through `IInteractionCallback` (`AuxLib/Interfaces.cs`) so the engine can ask
  questions from a worker thread.
* **R-3** Every side effect that needs user consent (overwrite a folder, supply a new
  activation code, confirm a version upgrade) is routed through
  `IInteractionCallback.Interact(...)`, never through a direct dialog.
* **R-4** All external-process interaction is funnelled through exactly one class,
  `AuxLib.ProcessHost`. FFmpeg is the only consumer in this product.
* **R-5** All logging goes through `AuxLib.Logging`, which is a process-wide singleton with
  a level filter — enabling logging is a single global switch.
* **R-6** All settings are read through the `IConvSettings` interface hierarchy
  (`SettingsInterface.cs`); the engine has no compile-time knowledge of the app's
  `Properties.Settings` class.

---

## 6. Core Domain Model

### 6.1 Object graph

```
                            ┌──────────────────────────┐
                            │        Book              │
                            │  SortingTitle            │
                            │  Parts : List<Part>      │
                            │  PartsType : EParts      │
                            │  ExternalMeta            │
                            │  Progress : ProgressData │
                            │  FileCaption / TagCaption│
                            └───────────┬──────────────┘
                                        │ 1
                                        │ *
                          ┌─────────────▼──────────────┐
                          │       Book.Part            │
                          │  PartNumber               │
                          │  AaxFileItem              │
                          │  Chapters  : List<Chapter> │  ← embedded OR named (swapped)
                          │  Chapters2 : List<Chapter> │  ← the other one
                          │  Tracks    : List<Track>   │
                          │  ActivationCode           │
                          │  BrandIntro / BrandOutro  │
                          │  TmpFileName              │
                          │  Duration                 │
                          └─────────────┬──────────────┘
                                        │ 1
                                        │ *
                          ┌─────────────▼──────────────┐
                          │        Chapter             │
                          │  Time : TimeInterval       │
                          │  Name : string             │
                          │  Silences : List<Interval> │
                          │  IsPaddingChapter         │
                          │  TimeAdjustment           │
                          │  TmpFileName              │
                          └─────────────┬──────────────┘
                                        │ 1
                                        │ *
                          ┌─────────────▼──────────────┐
                          │         Track              │
                          │  Time : TimeInterval       │
                          │  FileName : string         │
                          │  Title : string            │
                          │  Chapter : Chapter         │
                          │  MetaChapters : List<Ch.>  │
                          │  State : ETrackState       │
                          └────────────────────────────┘

                          ┌────────────────────────────┐
                          │      AaxFileItem           │
                          │  FileName, BookTitle,      │
                          │  Authors, Narrators,       │
                          │  Duration, SampleRate,     │
                          │  AvgBitRate, Cover,        │
                          │  Genre, Publisher, ...     │
                          └────────────────────────────┘
```

### 6.2 `AaxFileItem` — the input file record

`src/AaxAudioConverterLib/AaxFileItem.cs`

A `public class` implementing `IEquatable<AaxFileItem>` and `IAudioQuality`. Created only
via `new AaxFileItem(path)`, whose constructor:

1. Records `FileName`, extension kind (`HasExtAA`), `FileSize`, `FileDate`.
2. Calls `TagAndFileNamingHelper.ReadMetaData(this)`.
3. **Throws `IOException` if metadata could not be read** — a file whose tags cannot be read
   is not a candidate for conversion.

| Property | Type | Source | Notes |
|---|---|---|---|
| `FileName` | `string` | ctor | Long path already normalised to UNC form by caller |
| `HasExtAA` | `bool` | ctor | Legacy `.aa` vs `.aax` — selects a different ATL tag mapping |
| `IsAA` | `bool` | FFmpeg probe | Legacy container detected from FFmpeg's `Input #0, aa, from` banner |
| `BookTitle` | `string` | ATL `Title` tag | The **sorting key** for book grouping |
| `Authors` / `Author` | `string[]` / `string` | ATL `AlbumArtist` (aax) or `Composer` (aa) | Joined with `"; "` |
| `Narrators` / `Narrator` | `string[]` / `string` | ATL custom tag `©nrt` | |
| `Duration` | `TimeSpan` | FFmpeg `Duration:` banner | |
| `SampleRate`, `Channels`, `AvgBitRate` | `uint` | FFmpeg stream banner | |
| `Cover` / `CoverExt` | `byte[]` / `string` | ATL `EmbeddedPictures` | `.jpg` or `.png` |
| `Genre`, `Publisher`, `Copyright`, `Abstract` | `string` | ATL tags + Audible custom tags | |
| `PublishingDate` | `DateTime?` | ATL `Year`, else `©rldt`, else year parsed from the copyright string | See §13.4 |
| `ContentMetadataFile` | `AsinJsonFile` | Audible sidecar | Set lazily, §13.5 |
| `SimsBySeriesFile` | `AsinJsonFile` | Audible sidecar | Set lazily, §13.6 |
| `CustomNames` | `CustomTagFileNames` | UI preview form | Per-book override of author/title/year/genre |
| `Converted` | `bool` | engine | Drives the green checkmark in the file list |

**Equality** is by `FileName` (case-insensitive) only — `AaxFileItem.cs:61-72`. This is used
to de-duplicate the file list after drag-and-drop adds (`MainForm.cs:483`).

### 6.3 `Book` — the logical work

`src/AaxAudioConverterLib/Book.cs` — a `partial class` split across three files:

| File | Responsibility |
|---|---|
| `Book.cs` | Identity, part detection, title/author derivation, counters |
| `Book.Part.cs` | The nested `Part` class: chapters, tracks, silence handling |
| `Book.Progress.cs` | Nested `ProgressData`: phase sequence and weights |

Key members:

| Member | Purpose |
|---|---|
| `SortingTitle` | Title with any part suffix removed — the identity of the work |
| `Parts` / `PartsType` | The constituent files; `EParts.none` / `.some` / `.all` |
| `ExternalMeta` | Author/series info lifted from Audible's `SimsBySeries` JSON |
| `FileCaption` / `TagCaption` | Two parallel `(Author, Title)` pairs — one for filenames (pruned), one for tags (verbatim) |
| `CustomNames` | Per-book overrides from the preview dialog |
| `Progress` | `ProgressData` — the phase plan for this book |
| `DefaultAudioFile` | The file to hand to the media player after conversion |

#### 6.3.1 Part detection — `CheckParts()`

`Book.cs:143-164`. A file is treated as "part *n*" when its title matches the part regex
(§8.2). Given the set of parts, completeness is decided arithmetically:

```
hasParts  = any part number > 0
highest   = max(part numbers)
partsSum  = sum(part numbers)
expected  = highest * (highest + 1) / 2      // triangular number
allParts  = partsSum != 1 && partsSum == expected
```

* `partsSum == 1` means the only "part" is part 1 — that is a **single-part book**, not a
  multi-part one, so `allParts` is forced false.
* A book with parts 1, 2, 3 has `partsSum = 6 = expected` → `EParts.all` → parts are
  concatenated into one logical work.
* A book with only part 2 (`partsSum = 2 ≠ 3`) → `EParts.some` → each part is treated as an
  independent book for folder and playlist purposes.

`EParts` has three meaningful behaviours across the system:

| `PartsType` | Folder layout | Playlist | Part subfolder |
|---|---|---|---|
| `none` | `<Author>/<Title>/` | one at book root | no |
| `some` | `<Author>/<Title>/<Part N>/` | one **per part** | yes |
| `all` | `<Author>/<Title>/` | one at book root | no (parts merged) |

### 6.4 `Book.Part` — one input file

`src/AaxAudioConverterLib/Book.Part.cs`

| Member | Purpose |
|---|---|
| `PartNumber` | 0 = standalone, *n* = part *n* of a multi-part book |
| `AaxFileItem` | The source file record |
| `Chapters` / `Chapters2` | The **chapter duality** — see §6.5 |
| `Tracks` | Output files derived from this part |
| `ActivationCode` | The activation code that successfully decrypted this part |
| `BrandIntro` / `BrandOutro` | Audible's brand-intro/outro durations (from sidecar) |
| `TmpFileName` | Intermediate decrypted/copied file in the temp directory |
| `TmpFileNameIntermedCopy` | Intermediate copy for the iTunes-compatibility path |
| `TmpFileNameAACFixed` | Intermediate copy with the AAC `esds` atom patched |
| `ApplicableInFileNameForTranscode` | Resolves the correct input for a given stage (§6.6) |
| `FFmpegPrefer64Bit` | Whether to use `ffmpeg64.exe` for this part (§8.4.2) |
| `IsMp3Stream` | Whether the source payload is already MP3 (legacy `.aa` files) |
| `SKU` | Audible SKU, needed to locate series metadata for multi-part books |

### 6.5 The chapter duality — `Chapters` vs `Chapters2`

This is one of the more subtle parts of the model and deserves explicit treatment.

A part can have **two independent chapter lists**:

* **`Chapters`** — *embedded* chapters, extracted from the `.aax` container itself by
  parsing FFmpeg's probe output (`FFmpeg.cs:474-553`). Times are reliable; **names are
  usually absent**.
* **`Chapters2`** — *named* chapters, extracted from Audible's
  `content_metadata_<ASIN>.json` sidecar (`AudibleAppContentMetadata.cs:117-173`). Times are
  approximate and occasionally wrong; **names are present and meaningful**.

`HasNamedChapters` is true only when **both** lists are non-empty (§6.5.1). When that
holds, the engine may *prefer* one list's times over the other's.

`SwapChapters()` (`Book.Part.cs:70-75`) exchanges the two lists and flips the
`_namedChaptersIn1` flag. This is how the engine says "from here on, `Chapters` holds the
list I want to work with". The accessors make the intent explicit:

```csharp
public IReadOnlyList<Chapter> NamedChapters  => HasNamedChapters ? (_namedChaptersIn1 ? Chapters : Chapters2) : Chapters;
public IReadOnlyList<Chapter> EmbeddedChapters => HasNamedChapters ? (_namedChaptersIn1 ? Chapters2 : Chapters) : Chapters;
```

> **Invariant:** the pair `(Chapters, Chapters2)` always holds the same two *logical* lists;
> only which slot each occupies changes. Code that reasons about "named vs embedded" must
> use the `NamedChapters` / `EmbeddedChapters` accessors, never the raw fields.

### 6.5.1 The four chapter states

| `Chapters` | `Chapters2` | Meaning | Engine behaviour |
|---|---|---|---|
| non-empty | null | Embedded only | Single chapter list; no named-chapter features |
| non-empty | non-empty | Both available | Named chapters can be preferred; times can be reconciled (§8.7) |
| null | non-empty | Named only | Rare; named list is promoted |
| null | null | Neither | A single synthetic chapter covering the whole part is created (§8.5.1) |

### 6.6 The `ApplicableInFileNameForTranscode` chain

Every FFmpeg invocation downstream of the probe stage must read from the **right** input
file, because different preprocessing stages write different intermediates. The resolution
order (`Book.Part.cs:33`) is:

```
TmpFileName              ← chapter-split intermediate (decrypted, chapter-scoped)
  ?? TmpFileNameAACFixed  ← patched-AAC intermediate
  ?? AaxFileItem.FileName ← the original .aax (requires an activation code)
```

The **activation code is only needed when reading the original**. Once a part has been
decrypted into `TmpFileName`, downstream stages pass `actcode = null` (§8.9.2). The code
that decides this is:

```csharp
string actcode = part.TmpFileName is null ? part.ActivationCode : null;   // AaxAudioConverter.cs:1445, :1546
```

### 6.7 `Chapter` — a time range with a name

`src/AaxAudioConverterLib/Chapter.cs`

| Member | Purpose |
|---|---|
| `Time` | `TimeInterval` with `Begin` / `End` / `Duration` |
| `Name` | Chapter title; `(1)`, `(2)`… if unnamed (§6.7.1) |
| `Silences` | Silence intervals detected inside this chapter, relative to chapter start |
| `IsPaddingChapter` | A synthetic chapter inserted to cover head/tail gaps |
| `TimeAdjustment` | Pending delta for `Begin` and `End` (see §8.7.3) |
| `TmpFileName` | Chapter-scoped intermediate file |

### 6.7.1 Name complementing

`ComplementChapterNames()` (`Book.Part.cs:60-68`) replaces empty names with `(n)` where *n*
is the 1-based index. This guarantees every chapter has a printable name without changing
the source data.

### 6.8 `Track` — one output file

`src/AaxAudioConverterLib/Track.cs`

| Member | Purpose |
|---|---|
| `Time` | Absolute time range **within the source**, used for the FFmpeg `-ss`/`-to` |
| `FileName` | Absolute output path |
| `Title` | Tag title (computed by `TagAndFileNamingHelper.SetTrackTitle`) |
| `Chapter` | Owning chapter (null in single mode) |
| `MetaChapters` | Chapter markers to embed *inside* the output file (§12.6) |
| `State` | `initial` / `current` / `complete` / `aborted` |

`HasMinDuration` enforces a 1 ms floor (`Track.cs:19, 28`); tracks shorter than that are
skipped as a safety net (§8.10.4).

### 6.9 `TimeInterval` — the value type underpinning all timing

`src/AaxAudioConverterLib/TimeInterval.cs`

A small mutable class with an unusual but important invariant:

* Setting `Begin` clamps `End` up to `Begin`; setting `End` clamps `Begin` down to `End`.
* `Duration` is **always** recomputed as `End - Begin` on either assignment, *except* when
  `Duration` itself is assigned, which resets `Begin = 0` and `End = Duration`.

The `Duration` setter's reset behaviour is what makes `new Chapter(duration)` mean
"chapter starting at zero with this length" — the dominant convention in the engine.

`Shifted(offset)` produces a copy displaced in time; used when chapter-scoped silence
lists are merged into a book-level list.

---

---

## 7. Configuration Model

### 7.1 The settings interface hierarchy

The engine never sees the app's generated `Properties.Settings` class. It sees a set of
interfaces declared in `src/AaxAudioConverterLib/SettingsInterface.cs`:

```
IConvSettings ◄──────────────────────────────────────────────────────────────┐
  ├── INamingAndModeSettings ◄── INamingSettingsEx ◄── INamingSettings        │
  │       │                        │                        │                │
  │       │                        └── ITitleNamingSettings │                │
  │       │                            ├── SeriesTitleLeft  │                │
  │       │                            └── LongBookTitle     │                │
  │       └── IModeSettings ◄── IBitRateSettings           │                │
  │                ├── ConvFormat, ConvMode                 │                │
  │                └── VariableBitRate, ReducedBitRate      │                │
  ├── ITitleSettingsEx ◄── ITitleNamingSettings             │                │
  │       └── AddnlValTitlePunct                            │                │
  ├── IActivationSettings  (ActivationCode : uint?)         │                │
  ├── IUpdateSetting       (OnlineUpdate, …Declined)        │                │
  ├── IAaxCopySettings     (AaxCopyMode, AaxCopyDirectory)  │                │
  ├── IRoleTagAssigmentSettings (TagArtist, TagAlbumArtist, │                │
  │                             TagComposer, TagConductor)  │                │
  └── operational: NonParallel, FFmpeg64bitHours,           │                │
                   InputDirectory, OutputDirectory,          │                │
                   FFMpegDirectory, RelaxedFFmpegVersionCheck,│                │
                   ConvertByFileDate, TrkDurMins, PartNames, │                │
                   Genres, FlatFolders, FlatFolderNaming,     │                │
                   WithSeriesTitle, NumDigitsSeriesSeqNo,     │                │
                   FullCaptionBookFolder, OutFolderConflict,  │                │
                   ShortChapterSec, VeryShortChapterSec,      │                │
                   VerifyAdjustChapterMarks,                  │                │
                   PreferEmbeddedChapterTimes,                │                │
                   Latin1EncodingForPlaylist, IntermedCopySingle,│               │
                   FixAACEncoding, AutoLaunchPlayer, Version  │                │
                                                                       ────────┘
                                                          (IAppSettings in the app
                                                           adds FileAssoc, ShowStartupTip,
                                                           FileDateColumn, SettingsTab)
```

### 7.2 Settings persistence

Two mechanisms coexist:

| Scope | Mechanism | Location |
|---|---|---|
| **User** | `ApplicationSettingsBase` + `LocalFileSettingsProvider` | `HKCU\Software\…\user.config`, mirrored to `%LOCALAPPDATA%\<Company>\_nohash\<AppName>\user.config` |
| **Application** | `ApplicationSettingsBase` default provider | `machine.config` / app config |

`AuxLib/LocalFileSettingsProvider.cs` exists to solve a specific problem: the default
provider derives the `user.config` path from a hash of the **executable path + version**, so
settings "disappear" when the app is upgraded or moved. The custom provider maintains a
**canonical copy** at a stable, hash-free path and mirrors to/from it on every read and
write. All copy failures are swallowed so persistence can never break the app.

Three settings are application-scoped (machine-wide) and are the only ones a user with
elevated rights would want to set globally:

| Setting | Default | Effect |
|---|---|---|
| `NonParallel` | `false` | Forces all parallelism to 1 — the single most useful field for debugging |
| `FFmpeg64bitHours` | `0` | Hours of duration above which 64-bit FFmpeg is used; `0` ⇒ built-in 32 h threshold |
| `OnlineUpdateUrl` / `OnlineUpdateDebug` | `""` / `false` | Override the update manifest URL; force-always-report-new-version |

### 7.3 Enumerated settings and their semantics

#### Conversion format & mode

| Enum | Values | Meaning |
|---|---|---|
| `EConvFormat` | `mp3`, `mp4` | Target container. `mp4` resolves to `.m4a` or `.m4b` per `Settings.M4B`. |
| `EConvMode` | `single`, `chapters`, `splitChapters`, `splitTime` | Output structure — see §8.3. |

#### Naming

| Enum | Values | Applies to |
|---|---|---|
| `EFileNaming` | `track`, `track_book`, `track_book_author`, `author_book_track`, `book_track` | Output **file** name |
| `ETitleNaming` | `track`, `track_book`, `track_book_author`, `book_author`, `author_book`, `author_book_track`, `book_track` | **Tag** title |
| `ETrackNumbering` | `track`, `chapter_a_track`, `track_b_chapter_c` | Track numbering scheme in split mode |
| `EGeneralNaming` | `source`, `standard`, `custom` | Genre / part prefix source |
| `EGeneralNamingEx` | `source`, `standard`, `custom`, `_nofolders` | Chapter prefix; `_nofolders` suppresses chapter subfolders |
| `EFlatFolderNaming` | `author_book`, `book_author` | Flat (single-level) folder name order |

#### Chapters

| Enum | Values | Meaning |
|---|---|---|
| `ENamedChapters` | `no`, `yes`, `yesAlwaysWithNumbers` | Whether to use Audible's named chapters and whether to prefix the number |
| `EVerifyAdjustChapterMarks` | `no`, `splitChapterOrTimeMode`, `allModes` | When to re-derive chapter marks from silence detection |
| `EPreferEmbeddedChapterTimes` | `no`, `ifSilent`, `always` | When to prefer the container's own chapter times over the sidecar's |

#### Output quality

| Enum | Values | Meaning |
|---|---|---|
| `EReducedBitRate` | `off`, `_128k` … `_24k` | Cap the output bit rate; `off` = keep source |
| `EFixAACEncoding` | `no`, `withIntermediateCopy`, `allModes` | Patch the AAC `esds` atom (44.1 kHz bug) |
| `EOutFolderConflict` | `ask`, `overwrite`, `new_folder`, `skip` | What to do when the target folder is non-empty |
| `EAaxCopyMode` | flags | Where/how to archive the original `.aax` |

`EAaxCopyMode` is a `[Flags]` enum whose numeric values encode a folder-layout matrix
(`Enums.cs:70-83`):

| Bit | Meaning |
|---|---|
| `1` | book first in file name |
| `2` | include author in file name |
| `4` | group into per-author folders |
| `8` | reserved (unused in the current implementation) |

The `AaxFileCopier` decodes exactly these three bits (`AaxFileCopier.cs:26-28`).

### 7.4 Derived settings

Some settings are computed, not stored:

| Extension | Location | Rule |
|---|---|---|
| `settings.UseEmbeddedChapterTimes()` | `Extensions.cs:60` | `PreferEmbeddedChapterTimes == ifSilent && NamedChapters != no` |
| `settings.ApplicableBitRate(sourceBitrate)` | `Extensions.cs:46` | Returns `(applicable, requested)`; `applicable == 0` means "no change" |
| `settings.TrkDurMins.TrackDuration(mode)` | `Extensions.cs:22` | Clamps to `[3, 15]` for split-chapter mode, `[3, 90]` for time-split mode |
| `Book.PartsType` | `Book.cs:143` | Arithmetic completeness check (§6.3.1) |

---

## 8. The Conversion Pipeline

This is the heart of the system. The engine is a **facade + orchestrator**:
`AaxAudioConverter` owns the sequence, and the domain objects (`Book`, `Part`, `Chapter`,
`Track`) own the data and the per-object algorithms.

### 8.1 Top-level flow

```
MainForm.btnConvert_Click
  └─ converter.ConvertAsync(fileitems, cts.Token, progress, callback)
       └─ Task.Run(() => convert(fileitems))                       AaxAudioConverter.cs:388
            ├─ ResourceGuard: start/stop the master stopwatch
            ├─ Callbacks.Progress(Reset = true)
            │
            ├─ PHASE 1 — GROUPING
            │    for each AaxFileItem:
            │      parseTitleForPart(title) ──► (match, sortingTitle, partStub, partNo)
            │      if match   → accumulate into current Book
            │      else       → close current Book, start a new one
            │    (optionally re-order books by file date)
            │
            ├─ PHASE 2 — PER-BOOK PLAN
            │    initProgress(books)  → assign the phase sequence per book
            │
            ├─ PHASE 3 — PARALLEL OVER BOOKS
            │    Parallel.ForEach(books, DOP.Book, convertBook)
            │      └─ convertBook(book)                            AaxAudioConverter.cs:586
            │           ├─ checkActivationAndGetChapters(book)   §8.4
            │           ├─ book.InitAuthorTitle(Settings)         §8.4.3
            │           ├─ preProcessChapters(book)               §8.5
            │           ├─ initDirectory(book)                    §8.11
            │           ├─ fixAACEncoding(book)                   §8.8
            │           ├─ mode dispatch                          §8.3
            │           │    single         → convertSingleMode
            │           │    chapters       → convertChapterMode
            │           │    splitChapters  → convertSplitChapterMode
            │           │    splitTime      → convertSplitTimeMode
            │           │         └─ each: prepareTrackFiles…() then transcodeTracks()
            │           ├─ on cancel  → deleteOutDirectory(book)   §8.12
            │           ├─ on success → makePlaylist(book)         §8.13
            │           │                extraMetaFiles(book)      §8.14
            │           │                copyAaxFiles(book)        §8.15
            │           └─ cleanTempDirectory()
            │
            └─ setAutoLaunchAudioFile(books)
```

### 8.2 Grouping input files into books

The grouping step (`AaxAudioConverter.cs:407-446`) converts a flat list of files into a list
of `Book` objects. The partition key is the **sorting title** — the title with any
`Part N` suffix stripped.

The part regex is built dynamically per settings (`regexPartPattern()`,
`AaxAudioConverter.cs:217-247`):

```
^(.*)\s+((part|<user part names>|<resource part names>)\s+(\d+))
   └─1┘   └──────────────┘        └─4─┘
            └─2─┘  └─3─┘
```

* Group 1 = the sorting title.
* Group 2 = the whole "Part N" fragment (removed from the sorting title).
* Group 3 = the part-name stub (e.g. `Part`), used later for folder naming when
  `EGeneralNaming.source` is selected.
* Group 4 = the part number.

The alternation list combines the built-in `Part`, the user's `Settings.PartName`, the
user's `Settings.PartNames` list, and the localized `Resources.PartNames`. Names containing
non-word characters are skipped (they would break the regex). Matching is case-insensitive.

`parseTitleForPart` (`AaxAudioConverter.cs:470-486`) then cleans the sorting title:

```
sortingTitle = bookTitle.Remove(part-fragment)
             .Replace("  ", " ")     // collapse double spaces
             .Replace(" :", ":")     // tidy "Title : Subtitle"
             .Trim()
```

**Note:** this is a *title-suffix* heuristic, not an authoritative format. Books whose
titles genuinely end in something like "Part 2" but are not multi-part will be
mis-grouped. This is an accepted trade-off of the title-based approach — see §22.

### 8.3 The four conversion modes

| Mode | Output structure | Chapter handling | Typical use |
|---|---|---|---|
| **`single`** | One output file per **part** | Chapters collapsed into one span; chapter markers embedded as metadata | Long-form listening, single M4B |
| **`chapters`** | One output file per **chapter** | Named or embedded chapters define the files | Navigable audiobooks |
| **`splitChapters`** | Several files per chapter, split at silence | Chapters subdivided by silence detection | Mobile-friendly short tracks |
| **`splitTime`** | Files of roughly equal length, chapters ignored | Chapters merged; silence merged across the whole book | Fixed-length track sets |

Each mode has a `prepareTrackFiles*` method that produces the `Track` list, followed by the
shared `transcodeTracks(book)`.

```
                        ┌──────────────────────┐
                        │  prepareTrackFiles*  │
                        └──────────┬───────────┘
                                   │ produces Track objects
                                   ▼
                        ┌──────────────────────┐
                        │  transcodeTracks     │
                        └──────────┬───────────┘
                                   │
        ┌──────────────────────────┼──────────────────────────┐
        ▼                          ▼                          ▼
  by PART (parallel)      by CHAPTER (parallel)      by TRACK (parallel)
```

### 8.4 Stage A — activation and chapter acquisition

#### 8.4.1 The activation code

`ActivationCode` (`ActivationCode.cs`) aggregates codes from three sources into a
de-duplicated list:

| Source | Implementation | Notes |
|---|---|---|
| User setting | `Settings.ActivationCode` (`uint?`) | Entered via `ActivationCodeForm`; rendered as `00-00-00-00` |
| Registry | `ActivationCodeRegistry` | `HKLM\SOFTWARE\Audible\SWGIDMAP` (and the `Wow6432Node` variant). Value names 0–8 are 4-byte activation blobs; `0xffffffff` is skipped || Audible app data | `ActivationCodeApp` | `<%LOCALAPPDATA%>\Packages\AudibleInc.Audible*\LocalState\AudibleActivation.sys` — the first 4 bytes of each file, newest first |

Codes are stored as **8 uppercase hex digits** (`ToHexString`), which is exactly what
FFmpeg's `-activation_bytes` expects.

#### 8.4.2 Verification loop

`checkActivationAndGetChapters` (`AaxAudioConverter.cs:308-325`) tries each code in turn:

```
lock (_lockable) {
  while (true) {
    foreach (code in codes) {
      result = verifyActivationAndGetChapters(part, code)
      if (!result.HasValue) return false;      // hard error (invalid file type)
      if (result.Value)  return true;          // success
    }
    if (!activationErrorCallback(part)) return false;   // user aborted
    _activationCode.ReinitActivationCode();             // user supplied a new code
  }
}
```

The whole loop is under a lock so that parallel books do not each pop their own dialog.

`verifyActivationAndGetChapters` (`AaxAudioConverter.cs:327-356`) runs
`ffmpeg -hide_banner [-activation_bytes XXXXXXXX] -i "<file>"` and parses the banner:

* **Success** ⇒ the container decrypted. From the same output it harvests:
  * the chapter list (`_rgxChapter`),
  * whether the stream is MP3 (`_rgxAudioStream`),
  * whether the file is `.aa` (`_rgxAaFile`) or `.aax` (`_rgxAaxFile`).
* **`HasNoActivation`** (`"Option activation_bytes not found"`) ⇒ this file needs no
  activation (books downloaded with Book Lib Connect). The engine retries with `null`.
* **Failure** ⇒ `_success = false`.

On success the engine calls `part.ComplementChapterNames()` and `getContentMetadata(part)`
(§13.5).

#### 8.4.3 64-bit FFmpeg selection

`Book.Part.SetFFmpegPrefer64Bit` (`Book.Part.cs:104-120`) decides whether to use
`ffmpeg64.exe`:

```
if (ConvMode != single)              → false    (split modes never load the whole book)
if (FFMpegDirectory is set)          → false    (user's own FFmpeg, their choice)
if (Duration < threshold)            → false
threshold = FFmpeg64bitHours > 0 ? FFmpeg64bitHours hours : 32 hours
→ FFmpegPrefer64Bit = ApplEnv.Is64BitOperatingSystem
```

The 32-hour default exists because 32-bit FFmpeg runs out of address space on very long
books. The bundled installer ships **both** `ffmpeg.exe` and `ffmpeg64.exe`
(`AaxAudioConverter setup.iss:55-56`).

### 8.5 Stage B — chapter pre-processing

`preProcessChapters(book)` (`AaxAudioConverter.cs:702-753`) runs a fixed sequence of
transformations over every part:

```
for each part:
   1. ensure a chapter list exists
        if Chapters == null  → new List<Chapter>()
        if Chapters.Count == 0 → add a single Chapter covering the whole part
   2. derive the chapter-name stub from the first chapter's name
        (e.g. "Chapter 5" → stub "Chapter")
   3. preprocessEmbeddedChapters(part, i, shortChapter, veryShortChapter)   §8.5.2
   4. if part.HasNamedChapters → preprocessNamedChapters(part)              §8.5.3
   5. preProcessLastChapterResidue(part)                                   §8.5.4
   6. branch on mode:
        single & no chapter verification → combineChaptersSingleMode(part)   §8.5.5
        else                            → part.InsertPaddingChapters()        §8.6
   7. report counters and add progress steps
```

#### 8.5.1 Synthetic single chapter

When a file has no embedded chapters at all, a single `Chapter` spanning the whole duration
is created (`AaxAudioConverter.cs:707-708`). This guarantees the downstream code always has
at least one chapter to work with, in every mode.

#### 8.5.2 Removing implausibly short chapters

`preprocessEmbeddedChapters` (`AaxAudioConverter.cs:800-838`) removes chapters that are too
short to be real content:

| Rule | Condition | Rationale |
|---|---|---|
| Very short, first or last | `chapter.Duration <= veryShortChapter` **and** (no named chapters **or** this is a middle part) | Brand intros/outros and cross-part seams |
| Short, middle parts only | `book.PartsType == all` **and** `chapter.Duration <= shortChapter` **and** the chapter is at the seam of a non-final part | Part boundaries produce a stub chapter on each side |

The distinction between "very short" (`Settings.VeryShortChapterSec`, default 10 s) and
"short" (`Settings.ShortChapterSec`, default 25 s) encodes the difference between *junk*
and *legitimately short*.

Setting `ChaptersCurtailed = true` on the part records that the end of the book was
trimmed, which changes how the final chapter is later adjusted (§8.7.4).

#### 8.5.3 Reconciling named chapters with embedded chapters

`preprocessNamedChapters` (`AaxAudioConverter.cs:840-860`) is the heart of the
"named chapters" feature. When a part has **both** named and embedded chapters:

```
if (ShortChapterSec > 0) {
    tBeg = first named chapter's Begin
    tEnd = last named chapter's End
    adjustNamedChapters(part, tBeg, tEnd)
}

if (VeryShortChapterSec > 0) {
    tBeg = part.BrandIntro
    tEnd = part.Duration - part.BrandOutro
    adjustNamedChapters(part, tBeg, tEnd)
} else {
    tBeg = 0
    tEnd = part.Duration
    adjustNamedChapters(part, tBeg, tEnd, part.Book)
}

part.SwapChapters()   // named chapters become the working list
```

`adjustNamedChapters` (`AaxAudioConverter.cs:862-907`) trims the *embedded* list
(`Chapters2`) to the window `[tBeg, tEnd]`:

* Chapters starting at or after `tEnd` are removed.
* A chapter straddling `tEnd` is truncated to end at `tEnd`.
* Chapters ending at or before `tBeg` are removed.
* A chapter straddling `tBeg` is truncated to start at `tBeg`.

The `book` overload additionally extends the **final** chapter of the **final** part to the
true end of the book — Audible's sidecar often under-reports the last chapter's end.

The result is that after `SwapChapters()`, `Chapters` holds the **named** chapter list with
times clamped to the real content boundaries, and `Chapters2` holds the **embedded** list
for times that can be preferred later.

#### 8.5.4 Last-chapter residue

`preProcessLastChapterResidue` (`AaxAudioConverter.cs:755-773`) handles a specific data
defect: the sidecar's last chapter can be a 1 ms stub. If the last named chapter is shorter
than 1 s but the corresponding embedded chapter is longer, the named one is dropped —
the embedded data is more trustworthy.

#### 8.5.5 Collapsing to a single chapter (single mode)

`combineChaptersSingleMode` (`AaxAudioConverter.cs:775-797`) produces one chapter spanning
the whole (possibly trimmed) content:

```
newChapter = Chapter(part.Duration)
part.SwapChapters()               // embedded list is now in Chapters2
part.Chapters = [ newChapter ]
newChapter.Begin = Chapters2.First().Begin
newChapter.End   = Chapters2.Last().End
// then shift every embedded chapter by -Begin, so they are relative to the
// trimmed start (used later for cue-sheet maths)
```

### 8.6 Padding chapters

`Book.Part.InsertPaddingChapters` (`Book.Part.cs:307-326`) inserts up to two synthetic
chapters so that the chapter list covers the file **exactly**:

* If `Chapters[0].Begin > 0` → insert a padding chapter `[0, first.Begin]`.
* If `Chapters[last].End < Duration` → insert a padding chapter `[last.End, Duration]`.

Padding chapters exist so that no audio is silently dropped at the head or tail when
tracks are cut. They are removed again by `RemovePaddingChapters()` once their purpose is
served (§8.6.1).

#### 8.6.1 Lifecycle of padding chapters

```
insertPaddingChapters()   ──▶  [padding][real chapters][padding]
        │
        ├─ detectSilenceBook()        — splits by silence, padding is harmless
        ├─ verifyAdjustChapterMarks() — padding chapters are excluded from adjustment
        └─ RemovePaddingChapters()    ──▶  [real chapters]
```

`RemovePaddingChapters` (`Book.Part.cs:328-333`) removes the first and/or last element if
flagged as padding.

### 8.7 Stage C — chapter mark verification and adjustment

This is the most algorithmically intricate part of the system. It answers the question:
*"Audible's chapter marks are sometimes off by a second or two. Can we find the true
boundaries by looking at where the silence actually is?"*

#### 8.7.1 When it runs

| `EVerifyAdjustChapterMarks` | `chapters` | `splitChapters` | `splitTime` | `single` |
|---|---|---|---|---|
| `no` | ✗ | ✗ | ✗ | ✗ |
| `splitChapterOrTimeMode` | ✗ | ✓ | ✓ | ✗ |
| `allModes` | ✓ | ✓ | ✓ | ✓ |

Additionally, `settings.UseEmbeddedChapterTimes()` forces a related path even when
verification is off (§8.7.6).

#### 8.7.2 The two-phase algorithm

```
Phase 1 — detectSilenceBook(book)
   for each part (serially — see §8.7.5):
     for each chapter (in parallel):
        1. cut the chapter out to a temp file   (ffmpeg -ss .. -to .. -c:a copy)
        2. run silencedetect on the temp file  (ffmpeg -af silencedetect=… -f null -)
        3. store the detected silences on the Chapter

Phase 2 — verifyAdjustChapterMarks(book)
   numAffected = book.DetermineTimeAdjustments()
   if none affected → done
   adjustTimesAndRetranscode(book)  →  per part:
       chapters = affected chapters
       part.AdjustTimesWithPreference(preference)  →  computes new times
       retranscodeParallel(part, chapters)         →  re-cuts those chapters
```

Why is silence detection **chapter-scoped** rather than book-scoped? The comment at
`AaxAudioConverter.cs:1523-1525` explains it: FFmpeg's `silencedetect` filter prints
timestamps with only 6 significant digits (`g6`). Over a 10-hour book that precision is
worthless. By cutting the book into chapters first, each detection run sees a short file
where the precision is adequate, giving roughly 10 ms resolution.

#### 8.7.3 The adjustment algorithm

`Book.Part.DetermineTimeAdjustments(book)` (`Book.Part.cs:396-410`) walks the chapters and
calls `chapter.DetermineTimeAdjustment(nextChapter)` (`Chapter.cs:180-189`). The last
chapter is normally skipped — unless `ChaptersCurtailed` is set, in which case it *is*
examined, because the truncation means its end is wrong.

`Chapter.DetermineTimeAdjustment` delegates to one of two strategies:

**`handlePartTitle(next)`** (`Chapter.cs:389-434`) — detects the pattern

```
"-- Part I [- Title] -- Chapter 1 [- Title] -- Text"
             ^                    ^
      chapter mark here    chapter mark here
```

Condition: this chapter is **very short** (< 10 s), the next chapter is **short** (< 10 s),
and the next chapter has silences. The engine merges the silences of both chapters,
optionally joins them across the boundary if both chapters start/end with silence,
keeps only silences longer than 900 ms, and picks the one closest to the chapter mark.
The silence is then split so that the trailing half belongs to the previous chapter.

**`handleChapterMark(next)`** (`Chapter.cs:335-387`) — the general case:

1. If the chapter **ends with silence**:
   * If that silence is longer than 2 s and the next chapter also starts with silence →
     leave it alone (a genuine long pause between chapters).
   * Otherwise, if the next chapter does **not** start with silence → split the trailing
     silence in half between the two chapters.
2. Else if the **next chapter starts with silence** → check whether it is the right silence:
   only a silence ending within 10 s of this chapter's end and longer than 900 ms counts.
3. Otherwise, look for a candidate silence **near** the chapter mark:
   * Candidates: silences of this chapter ending within 10 s of the chapter end and
     longer than 900 ms, plus the next chapter's silences starting within 10 s of its
     beginning and longer than 900 ms.
   * The candidate **closest to the chapter mark** wins; ties are broken in favour of the
     longer silence.
4. If the best silence ends more than 30 s before the chapter end → out of range, no
   adjustment.

`Chapter.setAdjustment(next, silence)` (`Chapter.cs:477-494`) converts the chosen silence
into a time delta:

```
newSilDuration = silence.Duration - 300 ms      // leave a 300 ms lead-in
if (newSilDuration < 0) newSilDuration = silence.Duration / 3

newEnd = silence.Begin + newSilDuration
adjust = chapter.Duration - newEnd

TimeAdjustment.End -= adjust                   // this chapter ends earlier
nextChapter.forwardAdjustment(adjust)           // the next chapter starts later
```

The 300 ms lead-in (`TS_SILENCE_ANTE`) means the new chapter boundary falls just inside the
silence, not at its edge — the boundary is then inaudible.

`Chapter.AdjustTime(next)` (`Chapter.cs:227-309`) applies the pending adjustment and,
critically, **migrates silence intervals** between chapters so that the silence lists stay
consistent with the new boundaries:

* If the chapter becomes **shorter** (adjustment negative), silences of the next chapter
  that begin before the adjustment point are duplicated into this chapter, truncated at
  this chapter's new duration.
* If the chapter becomes **longer** (adjustment positive), silences of this chapter that
  end after the new end are moved into the next chapter.

This "simply duplicate affected silences" approach (`Chapter.cs:233-236`) is a deliberate
simplification that trades exactness for robustness.

#### 8.7.4 `ChaptersCurtailed`

Set when chapters were removed or truncated at the end of a book (§8.5.2, §8.5.3). Its
effect is to make the final chapter participate in adjustment — otherwise its (wrong) end
time would silently cut off audio.

#### 8.7.5 Serial parts, parallel chapters

`detectSilenceParts` (`AaxAudioConverter.cs:1502-1536`) iterates parts **serially**
(`// always serial`, line 1518) but processes chapters of a part **in parallel**. The
rationale is that chapter detection is CPU-bound on FFmpeg processes; running parts in
parallel would multiply the number of concurrent FFmpeg processes without improving
throughput on a saturated machine, while making progress reporting harder to reason about.

#### 8.7.6 Preferring embedded chapter times

`EPreferEmbeddedChapterTimes` controls whether the container's own chapter times (which are
usually exact) should override the sidecar's (which are usually approximate but named).

| Value | Behaviour |
|---|---|
| `no` | Keep the named chapters' times. |
| `ifSilent` | Use embedded times only where silence detection indicates the named time is wrong. Enabled automatically when `NamedChapters != no`. |
| `always` | Always use embedded times. |

`Book.Part.PreferEmbeddedChapterTimes(always)` (`Book.Part.cs:138-175`) matches named
chapters to embedded chapters by **end time**:

```
findMatchingChapters():
   for each named chapter (except the last):
      find the embedded chapter whose End is closest to the named chapter's End
      if |delta| > 10 s → reject the match
      if two named chapters match the same embedded chapter:
          keep the one whose *duration* is closest to the embedded chapter's duration
          exclude the embedded chapter and re-run for the loser
   then, for each matched pair:
      nmCh.SetNewEnd(ebCh.Time.End, nextNamedChapter, always)
```

`Chapter.SetNewEnd(end, next, always)` (`Chapter.cs:198-225`) is conservative by default:
if `always` is false, it only accepts the new end time if the new boundary falls **inside a
detected silence** — otherwise it refuses the change. With `always == true` it accepts
unconditionally.

Both the current and the next chapter are recorded as affected, so both get re-cut.

### 8.8 Stage D — the AAC encoding fix

Some 44.1 kHz `.aax` files contain a malformed MPEG-4 `esds` atom that makes FFmpeg
mis-encode the AAC stream. `FixAtomESDS` (`FixAtomESDS.cs`) patches it **in place, during
the file copy**:

```
ESDS_ATOM = {0x05, 0x80, 0x80, 0x80, 0x02}   // the esds atom header
PARA      = {0x12, 0x12}                     // decoder config descriptor, length 0x12
PARA_FIX  = {0x12, 0x10}                     // corrected: length 0x10
```

The patch runs only in the **first 10 MB buffer** of the file (`globalOffset > 0` → return),
because the `esds` atom is always in the header. The copy itself uses
`FileEx.Copy` (`FileEx.cs`) → `Win32FileIO` (`AuxWin32Lib`), a pinned-buffer, 64 KB-block
Win32 copy with progress reporting and cancellation.

The fix is applied in one of two ways:

| `EFixAACEncoding` | Behaviour |
|---|---|
| `no` | Not applied |
| `withIntermediateCopy` | Applied only when an intermediate copy is already being made (single mode with `IntermedCopySingle`) |
| `allModes` | Applied to every 44.1 kHz part in every mode, as a dedicated preprocessing pass (`fixAACEncoding`, `AaxAudioConverter.cs:910-977`) |

In `allModes` the fix produces `part.TmpFileNameAACFixed`, which then becomes the input for
all downstream FFmpeg calls via `ApplicableInFileNameForTranscode` (§6.6).

### 8.9 Stage E — transcoding

#### 8.9.1 The track preparation per mode

**`single`** — `prepareTrackFilesSingleMode` (`AaxAudioConverter.cs:1040-1080`):

```
if (chapter verification or embedded-times preferred):
    preprocessAudioChapterMode(book)     // silence detection etc.
for each part:
    if (chapter verification or embedded-times preferred):
        combineChaptersSingleMode(part)
    if (IntermedCopySingle && format==mp4 && !part.IsMp3Stream):
        plan an intermediate copy (and possibly the AAC fix)
    part.Tracks.Add(new Track(part.Duration))
TagAndFileNamingHelper.SetFileNames(resources, settings, book)
```

**`chapters`** — `prepareTrackFilesChapterMode`:

```
preprocessAudioChapterMode(book)
nameTrackFilesChapterMode(book)     // one Track per Chapter
book.SetMetaChapters(settings)      // chapter markers for the output files
```

**`splitChapters`** — `prepareTrackFilesSplitChapterMode`:

```
silenceAndCueSheet(book, null)      // detect silence + build tracks
nameTrackFilesSplitChapterMode(book)
book.SetMetaChapters(settings)
```

**`splitTime`** — `prepareTrackFilesSplitTimeMode`:

```
copyBookTempParallel(book, false)   // decrypt the whole book once
silenceAndCueSheet(book, mergeSilences)   // detect silence across the merged book
nameTrackFilesSplitChapterMode(book)
book.SetMetaChapters(settings)
```

#### 8.9.2 `silenceAndCueSheet`

`silenceAndCueSheet` (`AaxAudioConverter.cs:1258-1297`):

```
1. totalNumChapters = detectSilenceBook(book)        → §8.7.2 phase 1
   if it fails → abort this book
2. decide how to reconcile:
     if (mode >= chapters && VerifyAdjustChapterMarks >= splitChapterOrTimeMode)
         verifyAdjustChapterMarks(book)               → §8.7.2 phase 2
     else if (mode == splitChapters && UseEmbeddedChapterTimes())
         preferEmbeddedChapterTimesWithRetranscoding(book)
     else
         book.PreferEmbeddedChapterTimes(preference)
3. book.RemovePaddingChapters()
4. callback?.Invoke(book)      // splitTime passes mergeSilences here
5. book.CreateCueSheet(settings)                       → §8.9.3
6. reconcile the progress step count:
     totalNumTracks  = Σ part.Tracks.Count
     totalNumChapters = Σ part.Chapters.Count
     if they differ → AddTotalSteps(totalNumTracks - totalNumChapters)
        (may be NEGATIVE in split-time mode with long tracks)
```

`mergeSilences` (`Book.Part.MergeSilences`, `Book.Part.cs:335-380`) is the `splitTime`
special case: it collapses **all** chapters of a part into **one** aggregate chapter whose
silence list is the concatenation of every chapter's silences, shifted into absolute
part-relative time. Overlapping silences at chapter boundaries are merged.

#### 8.9.3 The cue sheet algorithm

`Chapter.CreateCueSheet(part, trackDurMins, splitTimeMode)` (`Chapter.cs:67-177`) splits one
chapter into several tracks at natural pause points. This is the core of the "split into
short tracks" feature.

**Inputs:**
* `trackDurMins` — the desired track length in minutes (3–15 for split-chapter mode,
  3–90 for split-time mode; `ETrackDuration`).
* the chapter's silences.

**Step 1 — decide the number of tracks.**

```
numTracks = max( round(chapterLength / (trackDurMins * 60)), 1 )
aveTrackLen = chapterLength / numTracks
```

**Step 2 — compute the search window.**

```
searchRange = aveTrackLen / 3          // ± 1/3 of the average track length
```

**Step 3 — filter silences.** Only silences of at least **500 ms** are considered track
boundaries (`TS_SHORT_SILENCE`).

**Step 4 — iterate.**

```
tsStart = 0                            (splitTime mode: the chapter's real Begin)
tsEnd   = tsStart + aveTrackLen
chEnd   = chapter.Duration - 10 ms      (splitTime mode: the chapter's real End)

while true:
   if (tsEnd < chEnd):
      silUp = first silence with Begin in [tsEnd, tsEnd + searchRange]
      silDn = last  silence with End   in [tsEnd - searchRange, tsEnd]
      if both found → pick the one with the smaller distance to tsEnd
      if one found  → pick that one
      if none found → tsEnd += aveTrackLen      // no pause: extend the track

      if a silence was found:
         actualEnd = sil.Begin + sil.Duration / 2      // split the silence
         if (actualEnd > chEnd) actualEnd = chEnd
         delta = actualEnd - tsEnd
         emit Track(tsStart, actualEnd)
         tsStart = actualEnd
         tsEnd   = tsStart + aveTrackLen - delta
         if (tsEnd + searchRange > chEnd) tsEnd = chEnd
   else:
      emit Track(tsStart, chEnd)                        // final track
      break
```

**Why split the silence in half?** The comment at `Chapter.cs:136-138` explains it: the
first half of the silence stays with the outgoing track, the second half goes to the
incoming one, so the pause is preserved in the output — but a cut in a compressed stream
is never sample-exact.

**Why is `chEnd` reduced by 10 ms?** `TS_EPS_SILENCE` guards against a boundary that lands
exactly on the chapter end and produces a zero-length final track.

#### 8.9.4 The three-level parallelism of transcoding

`transcodeTracks(book)` (`AaxAudioConverter.cs:1618-1647`) drives the final stage:

```
transcodeTracksByPartAndChapterParallel(book, chapterTracks)
   Parallel.ForEach(chapterTracks.Parts, DOP.Part, part =>
      transcodeChaptersParallel(book, part)
         Parallel.ForEach(part.Chapters, DOP.Chapter, chapter =>
            transcodeTracksParallel(book, part.Part, chapter)
               Parallel.For(0, chapter.Tracks.Count, DOP.Track, i =>
                  ... actual FFmpeg transcode + tag write ...
               )
         )
   )
```

`ChapteredTracks` (`ChapteredTracks.cs`) is the index structure that maps a flat list of
tracks back to their (part, chapter) grouping while preserving a global track number
offset — needed because the UI shows continuous track numbering across parts.

`transcodeTracksParallel` (`AaxAudioConverter.cs:1717-1767`) per track:

```
1. compute the modifiers:
     copy = (format==mp4 && !part.IsMp3Stream) || (format==mp3 && part.IsMp3Stream)
     modifiers = copy ? ETranscode.copy : ETranscode.normal
2. inFile = chapter.TmpFileName ?? part.ApplicableInFileNameForTranscode
3. TagAndFileNamingHelper.SetTrackTitle(resources, settings, book, track)
4. using ResourceGuard → report the track number
5. using ThreadProgress → map FFmpeg's fractional progress to progress steps
6. transcode:
     single mode → transcodeTrackSingle(...)
     else        → transcodeTrackMulti(...)
7. if the transcode succeeded → updateTags(book, track)
8. track.State = complete | aborted
```

The `copy` modifier means: if the source is already in the target container (an `.aa` file
holding MP3 being converted to MP3, or an `.aax` holding AAC going to MP4), the audio
stream is copied without re-encoding — fast and lossless. Otherwise the stream is
re-encoded to the target bit rate.

#### 8.9.5 The intermediate copy (iTunes compatibility)

Very long `.m4b` files fail in iTunes because of how the `moov` atom is laid out. The fix
is to produce the file in two passes: first create the container with the chapters, then
remux into the final file.

`makeIntermediateCopy` (`AaxAudioConverter.cs:1888-1920`):

```
sourceFile = inFile
if (44.1 kHz && FixAACEncoding == withIntermediateCopy):
    sourceFile = copy with the esds atom patched

transcode(sourceFile → intermediateFile)     // first pass
transcode(intermediateFile → outFile)        // second pass, remux
```

The second pass passes `actcode = null` and `t = null` — the intermediate file is already
decrypted, and the whole file is copied.

#### 8.9.6 Writing the tags

`updateTags` (`AaxAudioConverter.cs:1974-1985`) calls
`TagAndFileNamingHelper.WriteMetaData(resources, settings, book, track, withChapters: true)`,
which uses the **ATL** library to write the container's metadata (§12.6).

### 8.10 Stage F — output directory initialisation

#### 8.10.1 The folder layout

`initDirectory(book, author, title)` (`AaxAudioConverter.cs:2132-2233`) builds the target
path:

```
root = Settings.OutputDirectory
└── [Author]              ← if not flat folders
    └── [Series [seq]]    ← if WithSeriesTitle and the book has a series
        └── [Title]       ← the book folder
```

Variations:

| Setting | Effect |
|---|---|
| `FlatFolders` | Skips the author and series levels; folder is `"<Author> - <Title>"` or `"<Title> - <Author>"` per `FlatFolderNaming` |
| `FullCaptionBookFolder` | The book folder name becomes `"<Author> - <Series [seq]> - <Title>"` |
| `NumDigitsSeriesSeqNo` | Zero-pads the series sequence number |

The `book.IsNewAuthor` / `book.IsNewSeries` flags record whether the author/series folders
were created by this run, so that rollback can remove them (§8.12).

#### 8.10.2 The non-empty-folder conflict

If the target folder exists and is **not empty**, the engine asks the user:

```
MsgDirectoryCreationCallback:
  "<dir>" exists and is not empty.
  Override content, new folder or skip book?
  [Yes] Override content   [No] New folder   [Cancel] Skip book
```

The answer is tri-state (`bool?`): `true` = overwrite, `false` = new folder, `null` = skip.
The three-way mapping is `ECallbackType.question3` → `InteractionCallbackHandler` →
`MsgBox.Show(YesNoCancel)`.

**"New folder"** appends a numeric suffix: `Title (2)`, `Title (3)`, … until a free name is
found (`AaxAudioConverter.cs:2201-2214`).

**"Apply to all"** — for a multi-book conversion, the user is offered to reuse the answer
for every subsequent book. `MultiBookInitDirectoryHandling` (`AaxAudioConverter.nested.cs:97-102`)
stores:

| Field | Meaning |
|---|---|
| `HasBeenAnswered` | `null` = not yet asked, `false` = asked, apply to all, `true` = don't ask again |
| `Answer` | `null` = skip, `false` = new folder, `true` = overwrite |

When `Settings.OutFolderConflict != ask`, the question is pre-answered from the setting
(`overwrite` → true, `new_folder` → false, `skip` → null) and no dialog is shown
(`AaxAudioConverter.cs:503-521`).

#### 8.10.3 Cleaning stale content

`createPartDirectories(book)` (`AaxAudioConverter.cs:2279-2307`) removes leftovers from a
previous run so that the new output is not polluted:

* Part sub-folders whose name does not start with the configured part prefix are deleted.
* Files with a different extension than the current output format are deleted.

The book folder itself is cleared (`di.Clear()`) when a new folder was created or when the
book is not a "partial parts" book.

### 8.11 Stage G — the temp directory

All intermediate files live in `%LOCALAPPDATA%\<Company>\<AppName>\tmp`, converted to a
`\\?\` extended-length path (`TempDirectoryLong`, `AaxAudioConverter.cs:84`).

The temp directory is:

* **created and emptied** in the constructor (`initTempDirectory`),
* **emptied after every book** in non-single mode (`AaxAudioConverter.cs:658-659`),
* **emptied after the whole run** (`cleanTempDirectory` in `convertBooksParallel`),
* **emptied on dispose** (`Dispose` → `cleanTempDirectory`).

> The temp directory is **shared** across the whole run, not per book. In `single` mode,
> books are processed with a degree of parallelism of up to 4, so the temp filenames must
> be unique per book — they are, because they embed the author and title
> (`$"{AuthorFile} - {TitleFile} - {PartNumber}{ext}"`).

### 8.12 Stage H — cancellation and rollback

Cancellation arrives as a `CancellationToken` that is threaded into every `ParallelOptions`
and checked at every step boundary. When the user aborts:

1. `Callbacks.Cancelled` becomes true.
2. FFmpeg processes receive `q` on stdin, which makes them quit gracefully
   (`FFmpeg.onCancel`, `FFmpeg.cs:719-725`).
3. `deleteOutDirectory(book)` (`AaxAudioConverter.cs:2058-2098`) removes the partial output:

```
if (PartsType != some):
    delete the whole book folder
else:
    delete each part folder
    delete the book folder if it is now empty

// and clean up empty parents:
if (!FlatFolders):
    if (book.IsNewSeries && the series folder is now empty) → delete it
    if (book.IsNewAuthor && the author folder is now empty) → delete it

finally: mark all parts as not converted
```

The `IsNewAuthor` / `IsNewSeries` flags are what prevent the rollback from deleting
folders that pre-existed the run.

**On closing the window**, `MainForm.OnClosing` cancels the token and then deliberately
sleeps 2 seconds on the UI thread (`MainForm.cs:216`) to give the FFmpeg processes time to
receive the quit signal before the process exits.

### 8.13 Stage I — the playlist

`makePlaylist(book)` (`AaxAudioConverter.cs:1987-2034`) writes an M3U file:

```
if (total tracks < 2) → no playlist (but remember the single file for autoplay)
if (PartsType == some && mode == single) → no playlist (one file per part)

filename = "<Author> - <Title>.m3u"   (in the flat naming of the book folder)
encoding = Latin1 or UTF-8 per Settings.Latin1EncodingForPlaylist
```

Content:

```
#EXTM3U
#EXTINF:<seconds>, <track title>
<relative path to the track>
...
```

The playlist is written **per part** when `PartsType == some` (each part folder gets its
own), and once at the book root otherwise. `book.DefaultAudioFile` points at the playlist
when there is more than one track, so that the "autoplay" feature opens the playlist rather
than an arbitrary single file.

### 8.14 Stage J — extra metadata files

`ExtraMetaFiles.WriteFiles` (`ExtraMetaFiles.cs`) optionally writes, next to the audio:

* **`<name>.jpg` / `<name>.png`** — the cover art extracted from the `.aax`.
* **`<name>.txt`** — a human-readable metadata sheet:

```
Author:      Jane Doe
Title:       The Long Book
Duration:    12:34:56
Narrator:    John Smith
Genre:       Audiobook
Year:        2021
Publisher:   Some Publisher
Copyright:   © 2021 Some Publisher
SampleRate:  22050 Hz
BitRate:     64 kb/s

<the abstract, word-wrapped to 80 characters>
```

The abstract is word-wrapped at 80 columns without breaking words
(`ExtraMetaFiles.cs:95-114`).

### 8.15 Stage K — archiving the original `.aax`

`AaxFileCopier.Copy` (`AaxFileCopier.cs`) copies the source files (and the Audible
metadata sidecars) to an archive folder, with the naming controlled by the `EAaxCopyMode`
flags:

```
stub = TitleFile
     | TitleFile + " - " + AuthorFile     (bookWithAuthor)
     | AuthorFile + " - " + TitleFile     (bookWithAuthor && !bookFirst)

if (withParts)  stub += " (Part N)"
if (ASIN known) stub += "_" + ASIN
stub += original extension
```

The ASIN comes from the Audible sidecar (`content_metadata_<ASIN>.json`), which the copier
fetches if it has not already been located.

Alongside the `.aax`, the copier copies `content_metadata_<ASIN>.json` and
`SimsBySeries/<ASIN>.json` so the archive remains usable by other tools.

Copying is skipped entirely if the source and destination directories are the same.

### 8.16 End of run

```csharp
setAutoLaunchAudioFile(books)   // AaxAudioConverter.cs:461-468
```

Selects the first book's `DefaultAudioFile` — the playlist if there is one, otherwise the
single output file — and only when `Settings.AutoLaunchPlayer` is enabled. `MainForm`
launches it with `Process.Start` after a fully successful run (§14.4).

---

## 9. FFmpeg Integration Layer

### 9.1 `FFmpeg` — the wrapper

`src/AaxAudioConverterLib/FFmpeg.cs`. `public class FFmpeg : ProcessHost`. One instance
represents one FFmpeg invocation. It is **not** reusable across invocations in practice —
the engine creates a fresh instance per operation, which keeps the mutable parsing state
(`_success`, `_error`, `_aborted`, `_matchedDuration`, `_listComplete`) trivially correct.

| Member | Signature | Purpose |
|---|---|---|
| `VerifyFFmpegPath` | `bool (bool relaxedVersionParsing)` | Runs `ffmpeg -version`, parses the version |
| `GetAudioMeta` | `bool ()` | Probe for duration, sample rate, channels, bit rate |
| `VerifyActivation` | `bool (string actBytes, bool withChapters = false)` | Probe **with** decryption; harvests chapters |
| `Transcode` | `bool (string out, ETranscode mods, string actBytes = null, TimeSpan? from = null, TimeSpan? to = null)` | The main conversion |
| `ExtractMeta` | `bool (string out)` | Write an `ffmetadata` file |
| `DetectSilence` | `bool (string actBytes = null)` | Run `silencedetect` over the whole input |
| `SetBitRate` | `void (IBitRateSettings, IAudioQuality, bool anyway)` | Compute the `-b:a` / `-q:a` / `-ar` parameters |

Internal state exposed to the engine: `Chapters`, `Silences`, `AudioMeta`, `IsMp3Stream`,
`IsAaFile`, `IsAaxFile`, `HasNoActivation`, `Version`, `TranscodingError`.

### 9.2 The command templates

Commands are built from templates with `<PLACEHOLDER>` tokens, then substituted
(`FFmpeg.cs:44-60`):

| Template | Purpose |
|---|---|
| `-hide_banner <ACTIVATION> -y <BEGIN> <END> -i "<INPUT>" -map_metadata -1 -map_chapters -1 <BITRATE> -vn <COPY> "<OUTPUT>"` | Normal transcode — metadata and chapters are stripped and re-written later |
| `... -i "<INPUT>" -i "<INPUT2>" -map_metadata 1 -map_chapters 1 ...` | Transcode with an external metadata file (the `ffmetadata` chapter file) |
| `-hide_banner -y -i "<INPUT>" -f ffmetadata "<OUTPUT>"` | Extract metadata to an `ffmetadata` file |
| `-hide_banner <ACTIVATION> -i "<INPUT>"` | Probe (used for activation verification and audio meta) |
| `-hide_banner -y <ACTIVATION> -i "<INPUT>" -af silencedetect=noise=-30dB:d=0.25 -f null -` | Silence detection |

Token substitution:

| Token | Replaced with |
|---|---|
| `<ACTIVATION>` | `-activation_bytes XXXXXXXX` or the empty string |
| `<ACT_BYTES>` | The 8 hex digits |
| `<BEGIN>` | `-ss <seconds>` (3 decimals) or empty |
| `<END>` | `-to <seconds>` (3 decimals) or empty |
| `<BITRATE>` | `-b:a Nk` / `-q:a N` / `-ar NHz` or empty |
| `<COPY>` | `-c:a copy` or empty |
| `<INPUT>`, `<OUTPUT>`, `<INPUT2>` | The quoted paths |

> `-map_metadata -1 -map_chapters -1` on the primary transcode is deliberate: it strips
> Audible's own metadata so the engine can write a clean, complete set afterwards.

### 9.3 Output parsing

FFmpeg's human-readable output is parsed with regexes (`FFmpeg.cs:352-411`).

| Regex | Matches | Used by |
|---|---|---|
| `_rgxDuration` | `Duration: 12:34:56.78` | duration |
| `_rgxDurationEx` | `Duration: … bitrate: 64 kb/s` | duration + average bit rate |
| `_rgxAudioStream` | `Stream #0:0: Audio: aac (…)` | codec detection (MP3 vs AAC) |
| `_rgxAudioStreamEx` | `… , 22050 Hz, mono, fltp, 64 kb/s` | sample rate, channels, bit rate |
| `_rgxChapter` | `Chapter #0: start 0.000000, end 300.000000` | chapter times |
| `_rgxChapterMeta` | `Metadata:` | chapter-name section marker |
| `_rgxChapterTitle` | `title: Chapter 1` | chapter name |
| `_rgxSilenceStart` | `[silencedetect] silence_start: 4602.43` | silence start |
| `_rgxSilenceEnd` | `[silencedetect] silence_end: 4603.68 \| silence_duration: 1.24948` | silence end |
| `_rgxTimestamp1` | `frame= … time=00:05:00.00` | transcoding progress |
| `_rgxTimestamp2` | `size= … time=00:05:00.00` | transcoding progress (fallback) |
| `_rgxMuxFinal` | `video: … audio: … muxing overhead` | **success marker** |
| `_rgxErrorVarious` | `Error while decoding \| corrupt input packet \| Conversion failed \| aborting \| Invalid data found…` | failure marker |
| `_rgxAaFile` | `Input #0, aa, from` | legacy `.aa` detection |
| `_rgxAaxFile` | `[aax]` | `.aax` detection |
| `_rgxNoActivatonOpt` | `Option activation_bytes not found` | "no activation needed" |
| `_rgxErrorActivationOption` | `Error setting option activation_bytes` | bad activation code |

**Success is detected by the mux-overhead line** (`_rgxMuxFinal`), not by the exit code.
This is because FFmpeg can exit non-zero for benign reasons while still producing a
correct file, and the mux line is the last thing printed on success.

**The probe reads stderr, everything else reads stdout.** `runProcess` is called with
`getStdErrorNotOutput: true` for probes and `false` for transcodes — FFmpeg writes its
banner and progress to stderr, but the chapter/metadata listing to stdout.

> **Known limitation:** `ProcessHost.runProcess` builds a `processOutputStringBuilder` in
> async mode but never appends to it, so the returned string is always empty in async
> mode. This is harmless because the async path is driven entirely by the event handler.

### 9.4 Cancellation

Every output line handler calls `onCancel()` first (`FFmpeg.cs:475, 557, 568, 659`):

```csharp
private void onCancel () {
  if (!Cancel.IsNull () && Cancel ()) {
    _aborted = true;
    if (!Process.IsNull () && !Process.HasExited)
      Process.StandardInput.Write ('q');      // FFmpeg's graceful quit
  }
}
```

Writing `q` to FFmpeg's stdin makes it finish the current frame and exit cleanly, which
avoids a half-written output file. `ProcessHost` redirects standard input
(`ProcessHost.cs:36`), which is what makes this possible.

### 9.5 Progress reporting

`FFmpeg.Progress` is an `Action<double>` in the range 0..1. During transcoding:

```
ts = parse the timestamp from the progress line
progress = ts.TotalSeconds / t.Duration.TotalSeconds
Progress?.Invoke(progress)
```

The engine wraps this in `ThreadProgress` (`ThreadProgress.cs`), which converts the
fractional progress into **per-mille steps**:

```
_report(new ProgressMessage { IncStepsPerMille = (uint)(value * 1000 - alreadyReported) })
```

On dispose, `ThreadProgress` flushes the remainder so that the step count always reaches
exactly 1000. This is what makes the track-level progress bar accurate.

### 9.6 Bit-rate handling

`FFmpeg.SetBitRate` (`FFmpeg.cs:141-204`):

```
(applicable, requested) = settings.ApplicableBitRate(sourceBitrate)
if (applicable == 0):
    if (!anyway) → leave the bit rate alone
    else → applicable = sourceBitrate
else:
    reducing = true
    _bitrateParam = getSampleRate(sampleRate, applicable)

if (settings.VariableBitRate):
    _bitrateParam += getVariableBitrate(applicable, isMp3)
else:
    _bitrateParam += $"-b:a {applicable}k"
```

`getSampleRate` (`FFmpeg.cs:165-178`) also **reduces the sample rate** when the bit rate
is low:

| Bit rate | Sample rate cap |
|---|---|
| > 64 kb/s | 44100 Hz |
| 32–64 kb/s | 22050 Hz |
| < 32 kb/s | 11025 Hz |

The cap is `min(sourceRate, cap)` — the sample rate is never raised.

`getVariableBitrate` produces `-q:a N`:
* **AAC**: `round(cbr / 100.0, 1)` clamped to `[0.1, 2.0]`.
* **MP3**: an index into the VBR table `{320, 250, 210, 195, 185, 150, 130, 120, 105, 85, 0}`
  — the largest table entry not exceeding the target bit rate.

### 9.7 The FFmpeg executable

`FFmpeg.FFmpegExePath` (`FFmpeg.cs:91-108`) resolves the executable in this order:

```
if (PreferInternal64Bit && File.Exists("ffmpeg64.exe")) → "ffmpeg64.exe"
else:
    dir = FFmpeg.GetFFmpegDir?.Invoke()
    → dir/ffmpeg.exe   (or just "ffmpeg.exe" if the dir does not exist)
```

`FFmpeg.GetFFmpegDir` is a **static delegate** set by the engine constructor
(`AaxAudioConverter.cs:103`) to `Settings.FFMpegDirectory ?? ApplDirectory`. The static
indirection exists so that the `FFmpeg` class — which lives in the engine and has no
settings reference of its own — can find the executable without coupling.

### 9.8 The `ffmetadata` chapter file

`FFMetaData` (`FFMetaData.cs`) reads and writes FFmpeg's `ffmetadata` format:

```
;FFMETADATA1
[CHAPTER]
TIMEBASE=1/1000
START=0
END=300000
title=Chapter 1
[CHAPTER]
...
```

Used when `MP3_CHAPTERS_BY_FFMPEG` is enabled — a compile-time switch that lets FFmpeg
write the chapter markers into an MP3 file instead of the ATL library (§12.6).

---

## 10. Parallelism and Scheduling

### 10.1 The three-level structure

```
Level 1 — BOOKS          Parallel.ForEach(books,   DOP.Book)
Level 2 — PARTS         Parallel.ForEach(parts,   DOP.Part)
Level 3 — CHAPTERS      Parallel.ForEach(chaps,   DOP.Chapter)
Level 4 — TRACKS        Parallel.For(0, tracks,    DOP.Track)
```

`DegreeOfParallelism` (`AaxAudioConverter.nested.cs:9-95`) computes the degree for each
level, and it is **mode-aware**:

| Mode | Book | Part | Chapter | Track |
|---|---|---|---|---|
| `single` | `min(4, cores)` | `max(cores / Book, 1)` | 1 | 1 |
| `chapters` | 1 | 1 | `cores` | 1 |
| `splitChapters` | 1 | 1 | `cores / Track` | adaptive (below) |
| `splitTime` | 1 | 1 | 1 | `cores` |

The reasoning:

* **`single` mode** — one output file per part, each a whole-book transcode. Up to 4 books
  in parallel, and within a book the parts in parallel. This is the only mode where
  *book-level* parallelism is safe, because each book writes to its own output folder.
* **`chapters` mode** — the whole book is one logical unit; parallelism is at the chapter
  level, giving `cores` concurrent FFmpeg processes.
* **`splitChapters` mode** — the number of tracks per chapter varies wildly. The engine
  computes the average and the minimum tracks-per-chapter and then chooses a track
  parallelism:

  ```
  avg = average tracks per chapter
  min = minimum tracks per chapter

  trackParallelism = 1
  if (avg > 2)  trackParallelism = 2
  if (avg > 4 && min > 2) trackParallelism = 4
  if (avg > 4 && cores >= 4) {
      trackParallelism = cores
      while (trackParallelism > avg) trackParallelism /= 2
  }

  Track   = min(trackParallelism, cores)
  Chapter = max(cores / Track, 1)
  ```

  This prevents a book with one huge chapter and many tiny ones from spawning an
  excessive number of concurrent processes.
* **`splitTime` mode** — one continuous stream cut into equal-length tracks; chapter-level
  parallelism is pointless (the chapters are merged), so all parallelism is at the track
  level.

### 10.2 The `NonParallel` escape hatch

`Settings.NonParallel` (application-scoped) forces `MaxDegreeOfParallelism = 1`
everywhere (`AaxAudioConverter.cs:76`). This is the debugging switch: with parallelism
disabled, the log is a strictly sequential narrative and race conditions become visible
as ordering anomalies.

### 10.3 Cancellation propagation

Every `ParallelOptions` carries the `CancellationToken`:

```csharp
private ParallelOptions parallelOptions (int maxDegreeOfParallelism = -1) {
  if (Settings.NonParallel) maxDegreeOfParallelism = 1;
  return Callbacks is null
    ? new ParallelOptions { MaxDegreeOfParallelism = maxDegreeOfParallelism }
    : new ParallelOptions { CancellationToken = Callbacks.CancellationToken,
                            MaxDegreeOfParallelism = maxDegreeOfParallelism };
}
```

All `Parallel.For`/`ForEach` calls are wrapped in `try { … } catch (OperationCanceledException) { }`
— the exception is the *expected* cancellation signal, not an error.

Because `ParallelOptions.CancellationToken` is set, the TPL throws
`OperationCanceledException` as soon as the token is signalled, which stops the remaining
iterations from starting.

### 10.4 Thread safety

| Shared state | Protection |
|---|---|
| `AaxAudioConverter._fileErrors` | `lock (_lockable)` on every add and read |
| `Book.Progress` (phase index) | Accessed only from the book's own task |
| `Chapter.Silences` | Each chapter is processed by exactly one task |
| `Part.Tracks` | Appended during preparation (serial) and read during transcoding (parallel read) |
| `Track.State` | Written by the owning track task only |
| `Callbacks` | Replaced on the UI thread before the run, cleared after |
| `ATL` tag writing | **Not** thread-safe — each track writes its own file, so there is no shared state, but see §22.4 |
| `ProcessList` | `HashSet<Process>` under `lock` |
| `Logging` | `lock (_lockable)` around every write |

---

## 11. Progress and Status Reporting

### 11.1 Two progress bars

The UI shows two independent progress bars (`MainForm.Designer.cs`):

| Control | Name | Meaning | Unit |
|---|---|---|---|
| `progressBarPart` | Phase bar | Overall progress through the **phase** plan (silence → adjust → transcoding → copying) | Weighted phase units |
| `progressBarTrack` | Step bar | Progress through the **steps** (parts, chapters, tracks) | Per-mille, 1/1000 of a step |

The phase bar is weighted because the phases have wildly different costs
(`Book.Progress.cs:9-23`):

| Phase | Weight (copy) | Weight (transcode) |
|---|---|---|
| `silence` | 3 | 3 |
| `adjust` | 2 | 2 |
| `transcoding` | 2 | **10** |
| `copying` | 1 | 1 |

When the book is a pure stream copy (no re-encoding), transcoding is cheap; when it is a
real transcode, it dominates. The two weight tables reflect that.

The weight is multiplied by the number of parts to give the book's total
(`CurrentPhaseWeightBook = CurrentPhaseWeightPart * _numParts`).

### 11.2 The `ProgressMessage` protocol

`src/AaxAudioConverterLib/ProgressMessage.cs`

```csharp
public class ProgressMessage {
  public bool     Reset;                    // reset everything
  public bool     ResetSteps;               // the step count changed (re-plan)
  public uint?    AddTotalWeightedPhases;   // extend the phase bar's maximum
  public uint?    IncWeightedPhases;        // advance the phase bar
  public int?     AddTotalSteps;            // extend the step bar's maximum (may be NEGATIVE)
  public uint?    IncSteps;                 // advance the step bar by whole steps
  public uint?    IncStepsPerMille;         // advance the step bar by a fraction of a step
  public ProgressInfo Info;                 // human-readable status
}
```

`AddTotalSteps` being `int?` and possibly **negative** is deliberate: the number of tracks
is only known after silence detection and cue-sheeting, and the difference from the
initial estimate (one step per part) can be negative when a book has fewer tracks than
chapters (§8.9.2).

### 11.3 The `ProgressInfo` payload

`ProgressInfo` carries the *semantic* state — which book, which part, which chapter, which
track — separate from the numeric progress. Each factory method produces a snapshot:

| Factory | Produces |
|---|---|
| `ProgressInfoBook(title, phase)` | Book-level status with a phase |
| `ProgressInfoChapter(title, n)` | Chapter-level status |
| `ProgressInfoTrack(title, n)` | Track-level status |
| `ProgressInfoPart(title, n)` | Part-level status |
| `ProgressInfoBookCounters(title, chapters, tracks, part)` | Counter summary |

Each has a `…Cancel` variant that carries the `Cancel = true` flag, which tells the UI to
**remove** that item from its display (used when a book or chapter is rolled back).

### 11.4 `ProgressProcessor` — the UI side

`src/AaxAudioConverter/ProgressProcessor.cs`

```csharp
class ProgressProcessor {
  ProgBar     _progBarPhase;    // progressBarPart
  ProgBar     _progBarStep;     // progressBarTrack, M = 1000 (per-mille)
  BookProgress _bookProgress;   // the info-label model
}
```

`ProgBar` (`ProgressProcessor.cs:259-345`) implements a two-level accumulator:

```
_accuValue  = value carried over from the previous segment
_value      = value in the current segment
AccuValue   = ceil((_accuValue + _value) / M)
```

`Reset(progBarValueOnly: true)` **rolls** the current segment into the accumulator and
resets only the segment — which is exactly what "the plan changed, but the work done so
far still counts" means. This is used by `ProgressMessage.ResetSteps`.

`BookProgress` (`ProgressProcessor.cs:18-252`) maintains a `SortedDictionary<string, BookInfo>`
keyed by book title, and renders the info label with a **graceful degradation ladder**:

```
try: all details, full length
  → all details, 60 chars
  → all details, 30 chars
  → all details, 20 chars
  → phase + counters
  → counters only
  → minimum
```

Each attempt is measured with `TextRenderer.MeasureText` against the label's width, and the
first one that fits is used. This keeps the status line informative on a narrow window
without truncating into unreadability.

The label shows, per book: the title, the part number, the chapter count, the track count,
the current phase, and — at the highest verbosity — the list of in-flight part/chapter/track
numbers.

### 11.5 Thread hopping

`IProgress<ProgressMessage>.Report` is called from worker threads. `MainForm` constructs
`new Progress<ProgressMessage>(_progress.Update)` on the UI thread, and the default
`Progress<T>` implementation marshals the callback to the captured
`SynchronizationContext` — so all UI updates happen on the UI thread automatically.

The same mechanism applies to `IInteractionCallback`: `InteractionCallback<T,TResult>`
captures the `SynchronizationContext` at construction and uses `Send` to run the handler on
the UI thread (`AuxLib/InteractionCallback.cs:18, 26-40`). This is what allows a background
FFmpeg monitor to pop a modal dialog safely.

---

---

## 12. Naming and Tagging Subsystem

`TagAndFileNamingHelper` (`src/AaxAudioConverterLib/TagAndFileNamingHelper.cs`) is a
`partial class` split across two files:

| File | Responsibility |
|---|---|
| `TagAndFileNamingHelper.cs` | Naming logic and ATL tag read/write |
| `TagAndFileNamingHelper.Numbers.cs` | The nested `Numbers` class — all the counters |

### 12.1 The two-caption model

A book has **two** captions, each an `(Author, Title)` pair:

| Caption | Built from | Used for |
|---|---|---|
| `TagCaption` | The raw author and title, plus series handling | The **metadata** written into the audio file |
| `FileCaption` | `TagCaption.Author.Prune()` and `PruneTitle(TagCaption.Title)` | The **file and folder names** |

`Prune` (`AuxLib/ExtensionMethods.cs:91-118`) removes:

* all `Path.GetInvalidFileNameChars()`,
* a "doubtful" set of 60+ exotic Unicode characters (smart quotes, dashes, currency
  symbols, `¡`, `«`, `»`, `…`, `–`, `—`, `˜`, `™`, `›` …),
* trailing dots.

`PruneTitle` (`Book.cs:280-286`) additionally replaces `:` and `/` with `;` so that a
title like `Book: Subtitle / Part` becomes `Book; Subtitle ; Part` — legal on Windows and
still readable.

### 12.2 Title derivation — `Book.InitAuthorTitle`

`Book.cs:248-278`:

```
title = SortingTitle
if (LongBookTitle != no) → title = title.Replace(" -:", ":")   // fix "Title -: Subtitle"

match = _rgxTitle.Match(title)
if (match.Success) → title = match.Groups[1].Value.Trim()

if (!AddnlValTitlePunct.Contains('-')) {
    idx = title.IndexOf(" -")
    if (idx >= 0) → title = title.Substring(0, idx)    // drop " - Subtitle"
}

title = makeLongBookTitle(title, settings)

TagCaption = new Caption(fi.Author, title)
FileCaption = new Caption(TagCaption.Author.Prune(), PruneTitle(title))
```

The title regex is **assembled at runtime** from the settings (`initRegexTitle`,
`Book.cs:379-419`). Its shape is:

```
^([\w+\s+ <escaped punctuation> ]+)\W*
```

where `<escaped punctuation>` is the union of:

* the built-in set `.,'/-&`,
* the user's `AddnlValTitlePunct` (extra characters that should also terminate the title),
* `:` when `LongBookTitle != no`.

This lets a user say "also split the title at a `;`" without touching code. The regex is
cached in two static forms (`__rgxTitle0`, `__rgxTitleL`) for the common cases and built
dynamically only when the user's custom punctuation requires it.

`makeLongBookTitle` (`Book.cs:334-354`) implements the "long book title" styles:

| `ELongTitle` | Result |
|---|---|
| `no` | Title unchanged |
| `as_is` | Title unchanged |
| `book_series` | `Subtitle: Title` — the series/subtitle moved **in front** |
| `series_book` | `Title: Subtitle` — the series/subtitle moved **behind** |

`SeriesTitleLeft` decides whether the colon split takes the **first** or the **last**
colon.

### 12.3 The `Numbers` class — all the counters

`TagAndFileNamingHelper.Numbers` (`TagAndFileNamingHelper.Numbers.cs`) computes every
numbering value needed for naming and tagging:

| Field | Meaning |
|---|---|
| `nTrk` / `nTrks` / `nnTrk` | Current track number, total tracks, digit count |
| `nChTrk` / `nChTrks` / `nnChTrk` | Track number **within** its chapter, count, digit count |
| `nChp` / `nnChp` | Current chapter number, digit count |
| `nPrt` / `nnPrt` | Current part number, digit count |
| `nDsk` / `nDsks` | Disc number and count (= part number and part count) |

The computation **differs by `PartsType`**:

* **`EParts.some`** (partial parts) — track, chapter and disc numbers are **local to the
  part**. Each part is an independent book, so numbering restarts.
* **`EParts.none` / `EParts.all`** — numbers are **global across parts**: the current
  track number is the sum of the track counts of all preceding parts plus the index
  within this part. Disc number = part number; disc count = number of parts.

This is what makes a multi-part book produce a continuous, correctly numbered track
sequence in `single` and `chapters` mode.

### 12.4 File naming

`file()` (`TagAndFileNamingHelper.cs:424-433`) dispatches on the mode:

| Mode | Function |
|---|---|
| `single` | `fileSingle()` |
| `chapters`, `splitChapters`, `splitTime` | `fileChapters()` |

`fileChapters()` (`TagAndFileNamingHelper.cs:435-450`):

| `EFileNaming` | Result |
|---|---|
| `track` | `<track>` |
| `track_book` | `<track> - <title>` |
| `track_book_author` | `<track> - <title> - <author>` |
| `author_book_track` | `<author> - <title> - <track>` |
| `book_track` | `<title> - <track>` |

`fileSingle()` branches further on `PartsType`:

* `none` → `fileSingleNone()`: `<title>` / `<title> - <author>` / `<author> - <title>`
  (the track component is meaningless with one file).
* `some` → `fileSingleSome()`: the **part number** replaces the track:
  `<part>`, `<part> - <title>`, `<title> - <part>`, `<part> - <title> - <author>`,
  `<author> - <title> - <part>`.
* `all` → `fileChapters()` — the parts are merged, so the track naming applies.

`TrackForFile` replaces `/` with `,` in the track component, because a slash is illegal in
a filename.

### 12.5 Folder structure

`fullPath()` (`TagAndFileNamingHelper.cs:386-422`) assembles the absolute output path:

| Mode | `PartsType` | Path |
|---|---|---|
| `single` | `none`/`all` | `<bookFolder>/<file>` |
| `single` | `some` | `<bookFolder>/<Part N>/<file>` — only when `ExtraMetaFiles` is on |
| `chapters`, `splitTime` | `none`/`all` | `<bookFolder>/<file>` |
| `chapters`, `splitTime` | `some` | `<bookFolder>/<Part N>/<file>` |
| `splitChapters` | `none`/`all` | `<bookFolder>/<Chapter N>/<file>` |
| `splitChapters` | `some` | `<bookFolder>/<Part N>/<Chapter N>/<file>` |

The chapter sub-folder is suppressed when `EGeneralNamingEx._nofolders` is selected.
`PGANaming` prevents the invalid combination of `_nofolders` + `chapter_a_track`
numbering by playing a system sound and refusing the change
(`PGANaming.cs:83-86`).

The `<Chapter N>` folder name is `ChapterPrefix + " " + number`, where the prefix comes
from `EGeneralNamingEx`:

| Value | Prefix |
|---|---|
| `source` | `book.ChapterNameStub` — the word Audible itself used ("Chapter", "Kapitel", …) |
| `standard` | The localized `Resources.ChapterNamePrefixStandard` |
| `custom` | `Settings.ChapterName` |

The part prefix works the same way with `EGeneralNaming`.

### 12.6 Tag writing — ATL

Tag writing is done by the **ATL** library (`z440.atl.core`), not by FFmpeg. The ATL
library understands ID3v2 (MP3), MP4/M4A chapters, and the Audible-specific atoms.

`writeMetaDataAtl` (`TagAndFileNamingHelper.cs:667-762`) writes:

| ATL property | Value |
|---|---|
| `Album` | `book.TitleTag` |
| `Artist` | `buildPerformerTag(settings.TagArtist, author, narrators)` |
| `AlbumArtist` | `buildPerformerTag(settings.TagAlbumArtist, …)` |
| `Composer` | `buildPerformerTag(settings.TagComposer, …)` |
| `Conductor` | `buildPerformerTag(settings.TagConductor, …)` |
| `Comment` | The book's abstract |
| `Year` | The custom year, else the publishing year |
| `Genre` | The custom genre, else the resolved genre |
| `Copyright` | From the source |
| `Publisher` | From the source |
| `DiscNumber` / `DiscTotal` | Part number / part count |
| `Title` | The computed track title |
| `TrackNumber` / `TrackTotal` | Track number / track count |
| `EmbeddedPictures` | The cover art as a front picture |
| `AdditionalFields["wmrk"]` | `"Processed by AAX Audio Converter <version>"` |

`buildPerformerTag` (`TagAndFileNamingHelper.cs:774-796`) implements the role assignment:

| `ERoleTagAssignment` | Result |
|---|---|
| `none` | Empty string |
| `author` | The author |
| `author_narrator` | `author; narrator` |
| `narrator` | The narrator |

(The `author__narrator__` and `__narrator__` values are deprecated and are migrated by
`Settings.FixNarrator()` at startup — `Settings.cs:26-51`.)

`tagSubFormat` (`TagAndFileNamingHelper.cs:764-772`) forces ID3v2 version 3 on Windows 7
and 8 (`ApplEnv.OSVersion.Major < 10`) for compatibility, and version 4 otherwise.

### 12.7 Chapter metadata

`writeChapters` (`TagAndFileNamingHelper.cs:843-867`) writes the chapter markers:

```csharp
ATL.Settings.MP4_createNeroChapters   = true;   // Nero/QuickTime-style chapters
ATL.Settings.MP4_createQuicktimeChapters = true;
ATL.Settings.FileBufferSize = 2_000_000;         // 2 MB, for large chapter counts

tagFile.Chapters.Clear();
foreach (var ch in chapters) {
  tagFile.Chapters.Add(new ATL.ChapterInfo {
    StartTime = (uint)ch.Time.Begin.TotalMilliseconds,
    EndTime   = (uint)ch.Time.End.TotalMilliseconds,
    Title     = ch.Name
  });
}
```

**This is how the ">255 chapters" requirement is met** — the chapter list is written
directly into the container rather than into a limited tag slot.

Which chapters go into which file is decided by `Book.Part.SetMetaChapters`
(`Book.Part.cs:250-305`):

| Mode | Chapters embedded |
|---|---|
| `chapters`, `splitChapters` | One chapter per track, named after the track's chapter |
| `splitTime` | **All** chapters that overlap the track's time range, with times **rebased** to the track's start, the first clamped to 0 and the last clamped to the track duration |

The rebase in `setMetaChaptersMulti` (`Book.Part.cs:277-305`) is what makes a
split-time track carry the correct chapter markers for its slice of the book.

### 12.8 Metadata reading

`readMetaData` (`TagAndFileNamingHelper.cs:499-507`) reads in two passes:

1. **`readMetadataFFmpeg`** — a probe for duration, sample rate, channels, bit rate, and
   `.aa` detection.
2. **`readMetadataAtl`** — the ATL tag read, which differs between `.aa` and `.aax`:

| Tag | `.aax` | `.aa` |
|---|---|---|
| Title | `Title` | `Title` |
| Author | `AlbumArtist` | `Composer` |
| Narrator | custom `©nrt` | `Artist` |
| Abstract | `Description`, else custom `©des` | `Description` |
| Publisher | `Publisher`, else custom `©pub` | `Publisher` |
| Genre | `Genre` | `Genre` |

The `©` prefix is the ID3 convention for a three-character custom field; `atlCustomTag`
(`TagAndFileNamingHelper.cs:637-645`) adds it automatically.

---

## 13. Metadata Acquisition

### 13.1 Overview

Audible's desktop app stores metadata next to the audio files. The engine locates and
parses three kinds of sidecar:

```
<Content dir>/
   book_<ASIN>.aax                       ← the audio
   content_metadata_<ASIN>.json          ← chapters + SKU
   SimsBySeries/<ASIN>.json              ← author, series, title
   series_titles_<ASIN>.json             ← multi-part series variant
```

The content directory is `ActivationCodeApp.GetPackageDirectories()` →
`<package>\LocalState\Content` (`AaxAudioConverter.cs:2319-2329`).

### 13.2 The ASIN

The ASIN is extracted from the **filename** with the regex
`([0-9A-Z]{10})` surrounded by `_` or `.` (`AudibleAppContentMetadata.cs:87-88`):

```csharp
private static readonly Regex __regexAsinAax = new Regex (@"_([0-9A-Z]{10})(_|$|\.)", ...);
```

### 13.3 The three-location search

`AsinJsonFile.FindFile` (`AudibleAppContentMetadata.cs:30-60`) searches three places, in
order:

| # | Location | Rationale |
|---|---|---|
| 1 | **Local** — directly beside the `.aax` | Book Lib Connect puts them together |
| 2 | **Relative** — `<contentDir>\..\SimsBySeries\` | The app's own layout |
| 3 | **Absolute** — every Audible package's `SimsBySeries` folder | The app's shared cache |

All three use `AsUncIfLong()` for long-path support.

### 13.4 `content_metadata_<ASIN>.json`

Parsed by `getContentMetaChapters` (`AudibleAppContentMetadata.cs:117-173`) into
`json.AppContentMetadata` (`ContentMetadata.json.cs`):

```
content_metadata
   ├── chapter_info
   │     ├── brandIntroDurationMs
   │     ├── brandOutroDurationMs
   │     ├── runtime_length_ms
   │     └── chapters[]  { length_ms, start_offset_ms, title }
   └── content_reference
         ├── asin, sku, acr, marketplace, version, …
```

Two data repairs are applied:

1. **Empty titles** → replaced with `"."` so that `Chapter.Name` is never blank.
2. **Zero-length chapters** → their length is bumped to 1 ms
   (`Chapter.MS_MIN_CHAPTER_LENGTH`), and the following chapter's start offset is pushed
   forward to close the gap. This keeps the chapter list strictly increasing and
   non-overlapping.

The result populates:

| Target | Source |
|---|---|
| `part.Chapters2` | The named chapter list |
| `part.BrandIntro` | `brandIntroDurationMs` |
| `part.BrandOutro` | `brandOutroDurationMs` |
| `part.Duration` | `runtime_length_ms` |
| `part.SKU` | `content_reference.sku` |

### 13.5 The publishing-date fallback chain

`readMetadataAtlAax` (`TagAndFileNamingHelper.cs:547-618`) determines the publication year
with a three-step fallback:

```
1. ATL's Year tag → new DateTime((int)tags.Year, 1, 1)
2. If missing, or the year is more than 2 years in the future (clearly wrong):
      custom tag ©rldt → DateTime.TryParse
3. If still missing:
      regex "\D(\d{4})\D" against the Copyright string → that year
```

The "more than two years in the future" guard catches files with a placeholder year such
as `9999`.

### 13.6 `SimsBySeries` — author and series

`AudibleAppSimsBySeries` (`AudibleAppSimsBySeries.cs`) resolves the series metadata.

**Two lookup strategies:**

1. **Direct by ASIN** — `SimsBySeries/<ASIN>.json`. Works for single-part books.
2. **By SKU** — for multi-part books, where each part has its own ASIN but they share a
   SKU. The engine enumerates all `??????????.json` files in the `SimsBySeries` folders,
   deserializes each, and compares the SKU.

**Two file formats:**

| File | JSON shape | Parser |
|---|---|---|
| `<ASIN>.json` | `{ "product": { … } }` | `getProductsSingle` |
| `series_titles_<ASIN>.json` | `{ "similar_products": [ … ] }` | `getProductsMulti` |

**SKU matching** (`findBySku`, `AudibleAppSimsBySeries.cs:162-175`):

```csharp
if (part.SKU.StartsWith(product.sku) || part.SKU.StartsWith(product.sku_lite)) → match
```

`StartsWith` rather than equality, because Audible's part SKUs are the book SKU with a
part suffix.

A `HashSet<string> _discardedFiles` caches filenames already examined and rejected, so a
large library is not re-parsed for every book.

**The extracted data** becomes `Book.ExternalMeta`:

```csharp
new Book.ExternalBookMeta {
  Authors = product.authors.Select(a => a.name).ToList(),
  Book    = product.title,
  Series  = product.series?.Select(s => (s.title, s.sequence)).ToList()
}
```

`ExternalBookMeta.SeqString(numDigits)` (`Book.cs:79-102`) formats the series sequence
number with a configurable digit count, handling the `"1.5"` style where the decimal part
is a sub-part number:

```
regex "(\D*)((\d*)(\.\d+)?)(\D*)"
   └1┘   └─2─┘ └3┘└4┘  └5┘
              │    └── the ".5" fraction
              └─────── the integer part
```

The integer part is zero-padded to `numDigits`; the fraction is preserved.

### 13.7 The metadata acquisition flow

`getContentMetadata(part)` (`AaxAudioConverter.cs:1954-1972`) is only invoked when
either named chapters or series support is enabled:

```
flags = default
if (withSeries && !namedChapters) → flags = EFlags.skuOnly

new AudibleAppContentMetadata().GetContentMetadata(part, flags)
if (withSeries) new AudibleAppSimsBySeries().GetSimsBySeries(part)
```

`EFlags.skuOnly` means "find the metadata file and read the SKU, but do not parse the
chapters" — used when only the series information is needed.

---

## 14. User Interface Architecture

### 14.1 Form inventory

| Form | Purpose |
|---|---|
| `MainForm` | The main window: file list, mode/format selection, naming grid, conversion control, progress |
| `SettingsForm` | The settings dialog — 5 tabs: General, Folder, Conversion, Chapters, Meta Tags |
| `PreviewForm` | Per-book customization of author/title/year/genre tags and file names |
| `FileDetailsForm` | Read-only view of the metadata and cover art |
| `ActivationCodeForm` | Manual entry of the activation code |
| `FFmpegLocationForm` | Locate and verify the FFmpeg executable |
| `AboutForm` | Version and credits |
| `WhatsNewForm` | Shown after an update to an existing user |
| `StartupTipForm` | Shown on first start |
| `GenresForm` | Manage the genre list |

### 14.2 `MainForm` structure

```
┌────────────────────────────────────────────────────────────────┐
│ panelTop                                                          │
│  ┌──────────────────────────────┐  ┌──────────────────────────┐ │
│  │ grpBoxAaxFiles               │  │ tableLayoutSettings      │ │
│  │  listViewAaxFiles            │  │  ├ grpBoxFormat           │ │
│  │   Title | Author | Size |    │  │  │  (M4A/M4B)  (MP3)      │ │
│  │   Duration | Year | Narrator │  │  ├ grpBoxMode             │ │
│  │   SampleRate | BitRate | Date│  │  │  single/chapters/split  │ │
│  │  btnAddFile  btnRem         │  │  ├ grpBoxDest             │ │
│  └──────────────────────────────┘  │  │  lblSaveTo              │ │
│                                    │  └ grpBoxNaming           │ │
│                                    │     propGridNaming        │ │
│                                    └──────────────────────────┘ │
├────────────────────────────────────────────────────────────────┤
│ panelExec                                                          │
│  progressBarPart   (phase)                                        │
│  progressBarTrack  (step)                                         │
│  lblProgress        (status text)                                │
│  vprogbarNumProc    (FFmpeg process count)                       │
│  vprogbarCpu        (CPU load)                                   │
│  btnConvert  btnAbort                                               │
└────────────────────────────────────────────────────────────────┘
```

### 14.3 The naming property grid

`PGANaming` (`src/AaxAudioConverter/PGANaming.cs`) adapts the `INamingSettings` interface
into a `PropertyGrid`-friendly object using `PropGridLib`'s
`BasePropertyGridAdapter<INamingSettings>`.

Each property is declared with:

* `[PropertyOrder(n)]` — the display order in the grid,
* `[TypeConverter(typeof(EnumChainConverterRM<EFileNaming, ChainPunctuationDash>))]` —
  a **localized enum** converter that renders `track_book_author` as
  `"<track> - <book> - <author>"` in the current language,
* a custom setter that enforces cross-property constraints.

The `EnumChainConverter` mechanism (`AuxLib/EnumChainTypeConverter.cs`) splits the enum
name on `_`, looks each part up in the resource manager, and wraps it in the punctuation
chain's prefix/suffix. Parts of exactly one character are treated as **abbreviations** and
mapped to the chain's infix elements — which is why `ChainPunctuationDot` has the infix
list `{".", "(", ")"}` (for `a`, `b`, `c`) and `ChainPunctuationDash` has `{" - "}`.

`PGANaming.Update()` (`PGANaming.cs:129-136`) enforces the dependency rules:

```
ChapterName.ReadOnly = (ChapterNaming != custom)
GenreName.ReadOnly   = (GenreNaming  != custom)
TrackNumbering.ReadOnly = !(in split-chapter mode)
ChapterNaming.ReadOnly   = !(in split-chapter mode)
```

### 14.4 The conversion flow in the UI

`btnConvert_Click` (`MainForm.cs:903-997`):

```
1. ensure an output directory exists (else prompt)
2. ensure the aax-archive directory exists if that mode is on
3. enable the abort button, disable convert
4. _perfMonitor.Start()
5. create the progress and interaction callbacks
6. collect the selected files, ordered by book title
7. _progress.Reset(); create a CancellationTokenSource
8. await _converter.ConvertAsync(fileitems, _cts.Token, progress, callback)
9. stop the perf monitor, restore the cursor
10. mark the successfully converted rows with the green icon
11. show a summary message box (all done / some done / aborted)
12. if everything succeeded → autoplay the first output
```

The summary message includes up to 8 file errors, plus a count of the remainder
(`addFileErrors`, `MainForm.cs:999-1026`).

### 14.5 The list view

Columns: Title, Author, Size, Duration, Year, Narrator, Sample rate, Bit rate, and an
optional File Date column.

The sorting is handled by `ListViewColumnSorter` (`AuxWinLib`), configured with
`ESortModifiers.SortByTagOrText` (`MainForm.cs:98-101`). Numeric columns carry a `Tag`
holding the real number, so `10 MB` sorts after `9 MB` rather than before `2 MB`.

The title column's `Tag` is the **sorting title with the part suffix removed**
(`_converter.SortingTitleWithPart(fi)`), so a multi-part book's parts sort together.

Clicking the File Date column sets `Settings.ConvertByFileDate`, which makes the engine
order the books by file date before conversion (`AaxAudioConverter.cs:449-452`).

### 14.6 Internationalization

* Every form has a `.resx` and a `.de.resx`; German resources are compiled into a
  satellite assembly `de\AaxAudioConverter.resources.dll`.
* `Culture.SetCultures` (`AuxWinLib/Culture.cs`) discovers the available languages by
  scanning the exe directory for `<lang>\*.resources.dll` folders, plus the neutral
  culture.
* Changing the language at runtime writes the choice to `Settings.Language`, saves, and
  restarts the application (`SettingsForm.cs:394-406`).
* `Program.Main` sets `CultureInfo.DefaultThreadCurrentCulture = InvariantCulture` — the
  *formatting* culture is invariant so that number and date formatting in tags and file
  names is stable across locales, while the *UI* culture is localised.
* `MainForm.refillListView` temporarily switches the thread's culture to the UI culture
  so that the displayed dates and numbers are localised, then restores it
  (`MainForm.cs:598-606`).

### 14.7 DPI awareness

`App.config` sets:

```xml
<System.Windows.Forms.ApplicationConfigurationSection>
  <add key="DpiAwareness" value="PerMonitorV2" />
</System.Windows.Forms.ApplicationConfigurationSection>
```

`app.manifest` declares compatibility with Windows 7 through 10 and requests
`asInvoker` execution level (so the app does not require elevation).

---

## 15. Diagnostics, Logging and Performance Monitoring

### 15.1 The logging system

`AuxLib/Logging.cs` — a process-wide singleton with a level filter.

**Levels** are raw integers with **inverted semantics**: a message is written when
`level <= Logging.Level`. The level is `-1` by default, so **logging is off unless the
user passes a command-line argument**:

```
AaxAudioConverter.exe -Log=3
```

(`Program.cs:22-26`). Passing the argument also enables `InstantFlush`, so the log is
complete even if the app crashes.

**Usage in the engine:**

| Level | Typical content |
|---|---|
| 1 | Errors and exceptions |
| 2 | Phase-level milestones ("converting book X", "done") |
| 3 | Detailed steps: file names, chapter lists, FFmpeg commands, counters |
| 4 | FFmpeg's raw output, every line |

The `Func<string>` overload is used for expensive messages — the string is only built when
the level passes:

```csharp
Log (3, this, () => $"\"{book.SortingTitle.Shorten ()}\", #parts={book.Parts.Count ()}");
```

**Output format** (`Logging.cs:319-323`):

```
HH:mm:ss.fff {threadId:0000} [{context}] {message}
```

Example:

```
13:22:05.117 0004 [FFmpeg.ffMpegAsyncHandlerTranscode] { 3} 1200/3600
```

**Files** are written to `%LOCALAPPDATA%\<Company>\<AppName>\log\` as
`{AppName}_{yyyy-MM-dd}_{000}.log`. Rotation happens on a date change or at 20 MB; after
1000 files in a day the counter wraps and the oldest file is overwritten.

**Privacy:** `SubstitUser()` replaces the user's profile path with the literal `USER` in
every logged path.

### 15.2 The settings dump

`TreeDecomposition` (`src/TreeDecomposition`) serializes any object graph into an
indented, human-readable tree. The engine uses it to dump the entire settings state at
the start of a conversion (`AaxAudioConverter.cs:390`):

```csharp
Log (3, this, () => Settings.Dump (EDumpFlags.byInterface | EDumpFlags.inherInterfaceAttribs | EDumpFlags.inclNullVals));
```

The `byInterface` flag groups the properties by the interface that declares them, which
makes the correspondence with the settings interfaces (§7.1) visible in the log. The
`inherInterfaceAttribs` flag merges attributes declared on base interfaces.

`TreeDecomposition` handles:

* cycle detection via a `Stack<Type>`,
* a recursion depth guard (a type seen more than 20 times is treated as primitive),
* custom formatting via `[TextFormat]`, `[ToString]`, `[DisplayName]`, `[Browsable]`,
* "primitive" detection — anything whose namespace starts with `System` is primitive,
  which keeps the dump shallow and readable.

### 15.3 Performance monitoring

`PerfLib/PerformanceMonitor.cs` samples the CPU load of the app and its FFmpeg children.

**Wiring** — an observer chain with zero direct coupling:

```
ProcessHost.runProcess
   → Singleton<ProcessList>.Instance.Add(p)
       → ProcessList.Notify.Add(p)
           → PerformanceMonitor.Add(p)      // creates a CpuUsage for that PID
```

`AuxLib` does not reference `PerfLib`; `PerfLib` registers itself as the observer
(`PerformanceMonitor` ctor, `PerformanceMonitor.cs:43-51`).

**Sampling** — a 200 ms timer. On each tick:

```
usage = Σ GetCurrentValue() over all tracked child processes
usage = average over the processes with a non-zero reading
usage = min(usage, 100)
mean  = (mean * 3 + usage) / 4          // 3-tap moving average
Callback(new PerfCallback(
   new ValueMax(processCount - 1, ProcessorCount),   // -1: exclude this app itself
   new ValueMax((int)mean)))
```

`CpuUsage.GetCurrentValue()` throttles itself to one counter read per 200 ms
(`CpuUsage.cs:7, 31`) to avoid burning CPU on the sampling itself.

**Resolving the perf-counter instance** is the fiddly part
(`PerfLib/ProcessCpuCounter.cs`): Windows perf-counter instance names are *not* PIDs —
they look like `ffmpeg#3`. The code enumerates the `Process` category's instance names,
filters by process-name prefix, and reads the `ID Process` counter on each candidate until
the raw value matches the target PID.

**The UI** (`PerformanceHandler.cs`) drives two `VerticalProgressBar` controls
(a `ProgressBar` subclass that ORs in `PBS_VERTICAL = 0x04`), with tooltips showing the
current values.

---

## 16. Online Update Subsystem

### 16.1 The manifest

`res/Setup.json` is published to
`https://raw.githubusercontent.com/audiamus/AaxAudioConverter/master/res/Setup.json`
and fetched by `OnlineUpdate` (`OnlineUpdate.cs:47`):

```json
[
  {
    "Url": "https://github.com/audiamus/AaxAudioConverter/releases/download/v1.18.2/AaxAudioConverter-1.18.2-Setup.exe",
    "AppName": "AaxAudioConverter",
    "Version": "1.18.2",
    "Preview": false,
    "Descript": null,
    "Md5": "caacf818b0c41f2280ea3d1c5982351b"
  },
  {
    "AppName": "BookLibConnect",
    "InfoLinkUrl": "https://raw.githubusercontent.com/audiamus/BookLibConnect/master/res/Setup.json"
  }
]
```

An entry with an `InfoLinkUrl` instead of a `Url` is an **indirect reference** — its own
manifest is fetched and the first entry of *that* is used (`resolvePackageInfosAsync`,
`OnlineUpdate.cs:378-392`). This is how the updater also offers to update the companion
app Book Lib Connect.

### 16.2 The update flow

```
MainForm.OnLoad
   └─ checkOnlineUpdate()                     MainForm.cs:254-261
        └─ update.UpdateAsync(interact, exitApp, isBusy)
             ├─ getSetupRefAsync()             fetch + resolve the manifest
             ├─ updateDefaultPackageAsync()    the app itself
             └─ updateOtherPackagesAsync()     other apps (Book Lib Connect)
```

For the default package:

```
1. newVersion = serverVersion > AssemblyVersion
2. exists = checkDownloadAsync(pi)           // the file is already in ~/Downloads and the MD5 matches
3. if (!exists):
      if (OnlineUpdate == promptForDownload) → ask the user
      downloadSetupAsync(pi)                  // download, then verify the MD5
4. isBusy = busyCallback()                    // are there files queued for conversion?
      if (isBusy) → defer the update until the app closes
5. install(pi, interact, installNow)          // prompt, then launch the installer
```

### 16.3 Integrity verification

`checkDownloadAsync` (`OnlineUpdate.cs:414-422`) computes the MD5 of the downloaded file
and compares it, case-insensitively, with the manifest value. A mismatch is treated as
"not downloaded" and the file is re-fetched.

> The manifest is fetched over HTTPS from a GitHub raw URL, and the MD5 is fetched
> **in the same document** as the URL — so the MD5 provides protection against a
> corrupted download, not against a compromised manifest. See §19.4.

### 16.4 Declining updates

The user can decline an update for a companion app. The decision is persisted in
`Settings.OnlineUpdateOthersDeclined` as a semicolon-separated list of
`AppName:Version` pairs (`DeclinedPackage`, `OnlineUpdate.cs:20-45`).

A declined package is not offered again **unless a newer version appears**:

```csharp
bool decline = declinedPackagesIn.FirstOrDefault(dp =>
    pi.AppName.Equals(dp.AppName) && (pi.Version <= dp.Version || dp.Version == zeroVersion)) != null;
```

The special `zeroVersion` entry means "declined for now, ask again next time".

### 16.5 Installed-version detection

For companion apps, the installed version is read from the registry
(`OnlineUpdate.cs:213-229`):

```
HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{AppName}_is1\DisplayVersion
   (falling back to HKCU)
```

The `WithSpaces()` extension inserts spaces into the camel-cased app name to match the
Inno Setup uninstall key naming.

### 16.6 The deferred update

If the user starts a conversion while an update is pending, `isBusyForUpdate`
(`MainForm.cs:272-277`) returns `true` and sets `_updateAvailableFlag`. On window close,
`OnClosing` cancels the close and calls `handleDeferredUpdateAsync()`, which installs the
update and exits the app.

---

## 17. Build, Packaging and Deployment

### 17.1 Build configuration

| Project | Language version | Target | Output |
|---|---|---|---|
| `AaxAudioConverter` | 7.3 | .NET FW 4.8 | `AaxAudioConverter.exe` (WinExe), AnyCPU |
| `AaxAudioConverterLib` | 7.1 | .NET FW 4.8 | `AaxAudioConverterLib.dll` |
| `AuxLib` | 7.3 | .NET FW 4.8 | `UtilLib.dll` |
| `AuxWinLib` | 7.3 | .NET FW 4.8 | `AuxLib.dll` |
| `AuxWin32Lib` | — | .NET FW 4.8 | `AuxWin32Lib.dll` (unsafe blocks enabled) |
| `PerfLib` | — | .NET FW 4.8 | `PerfLib.dll` |
| `PropGridLib` | — | .NET FW 4.8 | `PropGridLib.dll` |
| `TreeDecomposition` | 7.1 | .NET FW 4.8 | `TreeDecomposition.dll` |

The solution uses the **legacy (non-SDK) csproj format** with `packages.config`
(NuGet packages restored into `src/packages/`).

> **Naming quirk:** the project `AuxLib` produces the assembly `UtilLib`, and the project
> `AuxWinLib` produces the assembly `AuxLib`. The assembly names and the project names do
> not correspond. This is historical and preserved for compatibility.

### 17.2 Costura

`FodyWeavers.xml` enables **Costura.Fody**, which embeds the managed dependencies
(ATL, Newtonsoft.Json, Ude, and the WindowsAPICodePack) into the executable at build
time. The result is a single self-contained `AaxAudioConverter.exe` plus the native
FFmpeg binaries.

### 17.3 The installer

`src/InnoSetup/AaxAudioConverter setup.iss`:

| Setting | Value |
|---|---|
| `AppName` | AAX Audio Converter |
| `AppVersion` | 1.18.2 |
| `DefaultDirName` | `{autopf}\audiamus\AAX Audio Converter` |
| `PrivilegesRequired` | `admin` (overridable in the dialog) |
| `ArchitecturesInstallIn64BitMode` | `x64` |
| `ArchitecturesAllowed` | `x86 x64` |
| `DisableWelcomePage` | `no` |
| Languages | English (default), German |

Installed files:

| Source | Destination |
|---|---|
| `AaxAudioConverter.exe` | `{app}` |
| `ffmpeg64.exe` | `{app}` (64-bit install mode only) |
| `ffmpeg.exe` | `{app}` |
| `AaxAudioConverter.exe.config` | `{app}` |
| `AaxAudioConverter*.pdf` | `{app}` — the manual (English and German) |
| `de\AaxAudioConverter.resources.dll` | `{app}\de` |

The manual is also opened from the system menu (`onSysMenuHelp`, `MainForm.cs:377-399`),
which picks the localized PDF if present and falls back to the neutral one.

### 17.4 Assembly version

`Properties/AssemblyInfo.cs` carries the version; `ApplEnv.AssemblyVersion` reads it and
it is the reference point for the update check (`OnlineUpdate.cs:245`).

---

## 18. Error Handling and Failure Modes

### 18.1 The error taxonomy

| Class | Example | Handling |
|---|---|---|
| **Fatal** | The output directory is not writable | Exception → `exceptionCallback` → error dialog; the book is abandoned |
| **Per-book** | The activation code is wrong | Interactive: the user is offered a retry, a new code, or skip |
| **Per-part** | The activation code does not decrypt this file | `checkActivationAndGetChapters` returns false; the part is skipped, the book continues |
| **Per-chapter** | A chapter fails to transcode | `noteTranscodingError` records it; the track is marked `aborted`; the run continues |
| **Per-track** | A single track fails | `track.State = aborted`; the run continues; the error is reported in the summary |
| **Cosmetic** | A sidecar is missing or malformed | Silently ignored; the feature degrades |

### 18.2 The file-error accumulator

`FileError` (`AaxAudioConverter.nested.cs:104-141`) records the source file, the chapter
(if known) and the track (if known):

```csharp
public class FileError {
  public string SourceFile { get; }
  public IChapter Chapter { get; }
  public ITrack Track { get; }
  public string ToShortString () { … }   // for the summary dialog
  public override string ToString () { … }  // for the log
}
```

The list is guarded by `lock (_lockable)` and exposed read-only via
`AaxAudioConverter.FileErrors`.

### 18.3 The summary dialog

`MainForm.addFileErrors` (`MainForm.cs:999-1026`) shows up to 8 errors in the completion
message, plus a count of the rest. The completion message distinguishes three outcomes:

| Condition | Icon | Message |
|---|---|---|
| `converted == 0` | Stop | "Aborted" |
| `0 < converted < total` | Warning | "Some files converted" |
| `converted == total` | Information | "All files converted" |

### 18.4 FFmpeg failure detection

A transcode is considered failed when:

* the `_rgxMuxFinal` (mux overhead) line was **not** seen — the run did not complete
  normally, **or**
* `_rgxErrorVarious` matched — `Error while decoding`, `corrupt input packet`,
  `Conversion failed`, `aborting`, or `Invalid data found when processing input`.

The two are distinguished: `TranscodingError` is `true` only for the second case, which
decides whether the failure is reported as a file error or merely as an aborted track.

### 18.5 Cancellation is not an error

`OperationCanceledException` is caught and ignored at every `Parallel` call site — it is
the expected cancellation signal, not a failure. `FFmpeg` distinguishes `_aborted` from
`_error` for the same reason.

### 18.6 Library-boundary error swallowing

At the edges of the support libraries, exceptions are swallowed deliberately:

| Location | Rationale |
|---|---|
| `LocalFileSettingsProvider` copy operations | Settings persistence must never break the app |
| `FileAssociations` | Registry manipulation must never break the app |
| `ProcessCpuCounter`, `CpuUsage` | Performance counters are a diagnostic, not a requirement |
| `Culture.Init` | A bad language setting must not prevent startup |
| `ProcessHost` priority setting | Some systems deny it |
| `ExtDirInfo.Clear` | Individual files may be locked |

### 18.7 Known latent defects

| Defect | Location | Impact |
|---|---|---|
| `Logging.Log(uint, string)` resolves to the `Type` overload and dereferences a null `Type` | `AuxLib/Logging.cs:110` | `NullReferenceException` when the log level ≥ 1 and the 2-arg overload is used (e.g. `ArgParser.Log()`) |
| `processOutputStringBuilder` is never appended to in async mode | `AuxLib/ProcessHost.cs:45, 71-74` | The return value of an async `runProcess` is always empty — harmless, since the async path is handler-driven |
| `SettingsForm.btnFfmpegLoc_Click` compares `newSetting` with itself | `SettingsForm.cs:340` | The `Dirty` flag is never set by this path |

---

## 19. Security and Privacy

### 19.1 The DRM position

The application **does not crack DRM**. It applies the user's own activation code — the
same bytes the official Audible player uses — to FFmpeg's `-activation_bytes` option.
This is stated in the README's anti-piracy notice and is a hard constraint on the design.

The practical consequence: **the engine never parses the `.aax` container format.** All
decryption is FFmpeg's job. The engine only:

* finds the activation code,
* passes it to FFmpeg,
* reads FFmpeg's output.

### 19.2 Activation-code redaction

The activation code must not leak into logs or crash reports. Three mechanisms:

| Mechanism | Location | Effect |
|---|---|---|
| `ToStringConverterActivationCode` | `ToStringConverters.cs:6-15` | The settings dump renders the code as `XXXXXXXX` |
| `SubstitActiv()` | `FFmpeg.cs:758-773` | The logged FFmpeg command line has the code replaced with `XXXXXXXX` |
| `ActivationCodeForm` | `ActivationCodeForm.cs:29` | The code is displayed as `00-00-00-00` |

`SubstitActiv` finds the `-activation_bytes` token in the command line and replaces the
following 8 hex digits.

### 19.3 Path redaction

`SubstitUser()` (`AuxLib/ExtensionMethods.cs:120-129`) replaces the user's profile path
with the literal `USER` in every logged path, so a shared log does not reveal the
username.

### 19.4 Network exposure

The application makes exactly **one** outbound network request: fetching the update
manifest over HTTPS. The update manifest itself is fetched over **plain HTTP** in one
configuration path (`OnlineUpdate` uses `HttpClient`; `ServicePointManager.SecurityProtocol`
is set to TLS 1.2 at `OnlineUpdate.cs:450` — but only in `downloadAsync`, not in
`getSetupRefAsync`).

The downloaded installer is verified against an MD5 that is supplied by the same manifest
document. This protects against a corrupted or truncated download, but **not** against a
compromised manifest. There is no code-signing check on the installer.

The update feature can be disabled entirely with `Settings.OnlineUpdate = no`.

### 19.5 Registry access

| Access | Purpose | Impact |
|---|---|---|
| `HKLM\SOFTWARE\Audible\SWGIDMAP` (read) | Activation codes | Read-only |
| `HKCU\Software\Classes\.aax`, `\.aa` (write) | File associations | Opt-in; the user is asked on first run and can decline |
| `HKLM\...\Uninstall\{app}_is1` (read) | Installed version of companion apps | Read-only |
| `HKCU\...\CurrentVersion\Uninstall` (read) | Same, 32-bit fallback | Read-only |

The file-association write is the only persistent system modification, it is opt-in, and
it notifies Explorer of the change (`SHChangeNotify(SHCNE_ASSOCCHANGED)`).

### 19.6 Data residency

| Data | Location | Lifetime |
|---|---|---|
| User settings | `%LOCALAPPDATA%\<Company>\_nohash\<AppName>\user.config` | Persistent |
| Log files | `%LOCALAPPDATA%\<Company>\<AppName>\log\` | Persistent; rotated daily / at 20 MB |
| Temp files | `%LOCALAPPDATA%\<Company>\<AppName>\tmp\` | Cleared after every run |
| Downloaded installers | `%USERPROFILE%\Downloads` | User-managed |

Nothing is uploaded anywhere. The application makes no telemetry call.

### 19.7 The `asInvoker` execution level

`app.manifest` requests `asInvoker` — the app runs with the user's own privileges and
does not request elevation. The installer, by contrast, requires `admin` (with a
`PrivilegesRequiredOverridesAllowed=dialog` escape).

---

## 20. Performance Characteristics

### 20.1 Where the time goes

For a typical 10-hour book in `chapters` mode on a 8-core machine:

| Stage | Share | Notes |
|---|---|---|
| Silence detection | ~35 % | Two FFmpeg passes per chapter: cut + detect |
| Transcoding | ~55 % | Depends entirely on the bit rate change |
| Tagging | ~5 % | ATL rewrites the container |
| Bookkeeping | ~5 % | |

In `single` mode with no bit-rate change, transcoding is a **stream copy** and the whole
conversion is close to I/O bound.

### 20.2 The cost of silence detection

The two-pass design (cut the chapter, then detect silence in the cut) doubles the I/O for
the detection stage. The trade-off is precision: FFmpeg's `silencedetect` prints
timestamps with 6 significant digits, which is unusable over a 10-hour book but fine over a
5-minute chapter (§8.7.2).

### 20.3 Parallel efficiency

The parallelism model is intentionally conservative:

* In `single` mode, at most 4 books run concurrently — beyond that, the disk becomes the
  bottleneck and the FFmpeg processes thrash.
* In `chapters` mode, `cores` chapters run concurrently — each is an independent FFmpeg
  process reading a different slice of the source.
* In `splitChapters` mode, the track parallelism adapts to the actual chapter sizes.

FFmpeg processes are launched at `ProcessPriorityClass.BelowNormal`
(`ProcessHost.cs:49`) so the UI stays responsive.

### 20.4 The I/O path

File copying uses `Win32FileIO` (`AuxWin32Lib`) — a pinned-buffer Win32 copy with 64 KB
blocks, rather than a managed `Stream.CopyTo`. The 10 MB buffer
(`FileEx.BUFSIZ`) is shared between the reader and the writer, halving the allocations.

Progress is reported at most every 200 ms (`FileEx.IVL_MS`).

### 20.5 Long-path overhead

Every path longer than 250 characters is converted to the `\\?\` extended-length form.
This is a pure string operation (`ExUnc.AsUncIfLong`) with no measurable cost, but it does
mean paths must be converted *before* being passed to the file APIs and stripped again
before being displayed.

---

## 21. Design Decisions and Rationale

### 21.1 Delegate all audio processing to FFmpeg

**Decision.** The engine never decodes, demuxes or encrypts anything itself.

**Rationale.** The `.aax` container format and the DRM scheme are proprietary and change.
Implementing them in the application would be a maintenance catastrophe and would place
the application on the wrong side of the anti-circumvention line. FFmpeg already does the
job, correctly, in every container format.

**Consequence.** The engine's job is reduced to *orchestration*: decide what to ask FFmpeg
for, in what order, and interpret what it says back.

### 21.2 Parse FFmpeg's human-readable output

**Decision.** The engine reads FFmpeg's text output with regexes rather than using a
machine-readable format.

**Rationale.** FFmpeg has no stable JSON output mode for the metadata the engine needs
(chapters, silence intervals, stream properties) in the versions the application
supports. The text output is stable enough in practice, and the engine is careful to
match only the specific lines it needs.

**Consequence.** The engine is sensitive to FFmpeg's output format. The
`RelaxedFFmpegVersionCheck` setting and the `_rgxVersionRelaxed` fallback exist to
tolerate version differences. See §22.1.

### 21.3 Two chapter lists with a swap

**Decision.** A part holds two chapter lists (`Chapters`, `Chapters2`) and can swap them.

**Rationale.** Named chapters (from the sidecar) and embedded chapters (from the
container) each have strengths: named chapters have meaningful titles, embedded chapters
have accurate times. The system needs to *combine* them — take the names from one and the
times from the other — and to be able to switch its working list at several points in the
pipeline.

**Consequence.** The `NamedChapters` / `EmbeddedChapters` accessors must be used instead of
the raw fields, or the code will reason about the wrong list.

### 21.4 A shared temp directory

**Decision.** All intermediate files live in one temp directory, cleared after each book
(non-single mode) and after the run.

**Rationale.** A per-book temp directory would multiply the directory creations and the
cleanup work, and the intermediate files are short-lived by nature. The filenames embed
the author, title and part number, so collisions are avoided.

**Consequence.** The temp directory is shared, so a crash leaves files behind. They are
cleared on the next start (`initTempDirectory`).

### 21.5 Direct manipulation of the output folder

**Decision.** The engine writes directly into the target folder, and asks before
overwriting.

**Rationale.** Writing to a staging area and moving at the end would double the I/O for
large books and complicate the "new folder" conflict resolution. The rollback on cancel
(§8.12) mitigates the risk.

**Consequence.** A crash mid-conversion leaves a partial output folder. The
`IsNewAuthor`/`IsNewSeries` flags limit the rollback to what this run created.

### 21.6 The settings interface hierarchy

**Decision.** The engine consumes settings through a set of narrow interfaces, not through
the app's generated settings class.

**Rationale.** It keeps the engine UI-agnostic and independently testable, and it lets
the interfaces be composed by concern (naming, mode, bit rate, tags, update, archive)
rather than by one monolithic interface.

**Consequence.** Adding a setting means touching the interface, the app's settings class,
and usually the settings form. The `TreeDecomposition` dump makes the mapping visible.

### 21.7 Interaction via callbacks, not dialogs

**Decision.** The engine asks questions through `IInteractionCallback`.

**Rationale.** The engine runs on worker threads; showing a WinForms dialog from a worker
thread is unsafe. The callback captures the `SynchronizationContext` and marshals to the
UI thread.

**Consequence.** The engine is fully testable with a scripted callback, and the UI can
decide how to present each question.

### 21.8 ATL for tagging, FFmpeg for transcoding

**Decision.** Split the responsibility: FFmpeg produces the audio, ATL writes the tags.

**Rationale.** FFmpeg's metadata writing is inconsistent across containers and it does not
support the Nero/QuickTime chapter styles that iTunes expects. ATL does both properly.

**Consequence.** Two libraries must understand the file format, and there is a small
window in which the file has FFmpeg's metadata but not ATL's.

### 21.9 The `Part` abstraction

**Decision.** A `Book` owns a list of `Part` objects even for single-file books (where
there is exactly one part with number 0).

**Rationale.** It removes a special case from almost every algorithm. A single-part book
is simply a book with one part whose `PartNumber` is 0, and `PartsType` is `none`.

**Consequence.** `PartsType` still has to be checked in the places where the difference
is real (folders, playlists, numbering).

### 21.10 The `EParts.all` / `EParts.some` distinction

**Decision.** Part completeness is decided arithmetically (the triangular-number test).

**Rationale.** The engine cannot ask the user "is this a multi-part book?" for every
batch — the parts may arrive in any order and the user may convert them separately. The
triangular-number test gives a good answer without interaction.

**Consequence.** A book with parts 1, 2, 4 (missing 3) is classified as `some` and each
part becomes an independent book with its own folder and playlist. This is arguably the
right behaviour for an incomplete download.

### 21.11 Splitting the silence in half at a track boundary

**Decision.** A track boundary lands at the **midpoint** of the detected silence.

**Rationale.** Putting the boundary at the silence's start would cut mid-word; putting it
at the end would leave a long gap at the head of the next track. The midpoint preserves
both halves of the pause. The code comments acknowledge that a cut in a compressed stream
is never sample-exact (§8.9.3).

### 21.12 `TS_SILENCE_ANTE` — the 300 ms lead-in

**Decision.** A new chapter boundary is placed 300 ms **inside** the silence.

**Rationale.** If the boundary were placed at the silence's start, the chapter would end
exactly where the narrator stops — but any rounding in the container could clip the last
syllable. A 300 ms lead-in guarantees the cut lands in silence with margin.

### 21.13 The 32-hour 64-bit FFmpeg threshold

**Decision.** Books of 32 hours or more use `ffmpeg64.exe` in `single` mode.

**Rationale.** 32-bit FFmpeg runs out of address space on very long books. 32 hours is a
conservative threshold that is never reached in practice, so the switch is effectively
free. `Settings.FFmpeg64bitHours` makes it tunable.

### 21.14 The `ExtraMetaFiles` companion files

**Decision.** The `.txt` and cover files are optional and off by default.

**Rationale.** They are a convenience for users who want the metadata in a browsable
form, but most users do not want extra files in their music library.

### 21.15 The `wmrk` watermark tag

**Decision.** A `wmrk` custom tag is written: `"Processed by AAX Audio Converter <version>"`.

**Rationale.** It is a courtesy marker that identifies the tool. It is a single custom
field and does not interfere with standard players.

---

## 22. Known Limitations and Technical Debt

### 22.1 FFmpeg output-format sensitivity

The engine depends on FFmpeg's text output format. A future FFmpeg release that changes
the banner layout could break chapter extraction, silence detection, or version parsing.

**Mitigation.** The version check warns the user when the installed FFmpeg is older than
4.1. `RelaxedFFmpegVersionCheck` allows a looser version parse.

### 22.2 Title-based part detection

The "is this a multi-part book?" decision is made by matching a `Part N` suffix in the
**title**. A book whose title legitimately ends in "Part 2" but which is not a multi-part
book will be mis-grouped.

**Mitigation.** The user can extend the part-name list (`Settings.PartNames`) and the
localized list is already consulted.

### 22.3 The sidecar dependency for named chapters and series

Named chapters and series metadata exist **only** if the Audible app's sidecars are
present. Without them, the app falls back to the embedded chapters, which have no names.

**Mitigation.** The `NamedChapters` setting can be turned off; the embedded chapters are
always available.

### 22.4 ATL thread-safety

The ATL library is not documented as thread-safe. The engine writes tags from multiple
threads concurrently, but each thread writes a **different file**, so there is no shared
state. The `lock (__lockableATL)` that would have protected it is commented out
(`TagAndFileNamingHelper.cs:679`, `:804`).

**Risk.** Low but non-zero — ATL uses some static state internally.

### 22.5 The 2-second shutdown delay

`MainForm.OnClosing` sleeps 2 seconds on the UI thread to let FFmpeg processes receive
their quit signal (`MainForm.cs:216`).

**Risk.** The app appears to hang for 2 seconds on close during a conversion.

### 22.6 The `Logging.Log(uint, string)` overload

The 2-argument overload resolves to the `Type` overload and throws a
`NullReferenceException` when the level is ≥ 1 (§18.7). It affects `ArgParser.Log()`,
which is not called on the normal startup path — but the `ArgParser` is constructed on
every launch, so this is a latent trap for anyone who adds a `Log()` call.

### 22.7 `Program.Main` ignores most command-line arguments

Only `-Log=<n>` is honoured. The `ArgParser` supports a much richer syntax
(`-tag`, `-tag=value`, typed accessors) that is unused. The app also accepts a file path
as `args[1]`, but only via `MainForm.OnActivated` (`MainForm.cs:184-186`), which silently
ignores anything starting with `-`.

### 22.8 No automated tests

There is **no test project** in the solution. The `MANY_CHAPTERS_TEST`, `MP3_CHAPTERS_BY_FFMPEG`
and `MULTIPART_TEST` compile-time switches (`AaxAudioConverter.cs:1-2`,
`TagAndFileNamingHelper.cs:1`, `:598-608`) are the only systematic test harness, and they
are manual.

**Impact.** The chapter-adjustment heuristics (§8.7) and the cue-sheet algorithm (§8.9.3)
are the most likely places for a regression to go unnoticed.

### 22.9 The settings-provider copy cost

`LocalFileSettingsProvider` copies the whole `user.config` file on every read and write.
For a settings file of a few kilobytes this is negligible, but it is O(file size) per
access rather than O(changed property).

### 22.10 The dead localization code in `Culture`

`Culture.localizeForm` (`AuxWinLib/Culture.cs:118-140`) is present but its call site is
commented out (`Culture.cs:114-115`). Re-localizing open forms after a language change
therefore does not work — which is why the app restarts instead.
### 22.11 The `EAaxCopyMode` flag `8`

Bit 8 is defined in the enum but never used (`Enums.cs:78`). `AaxFileCopier` only reads
bits 1, 2 and 4.

### 22.12 `SettingsForm.btnFfmpegLoc_Click` never sets `Dirty`

`Dirty |= string.Equals(newSetting, newSetting)` compares the new value with itself,
which is always `true` — so `Dirty` *is* set, but the comparison is obviously a bug that
was meant to compare the old value (`SettingsForm.cs:340`).

### 22.13 An orphaned source file

`src/AaxAudioConverterLib/ContentMetaDataJson.cs` is a near-duplicate of
`ContentMetadata.json.cs` — it differs only in one line (`content_size_in_bytes` is
`long` there, `int` in the compiled version).

It is **not** referenced by `AaxAudioConverterLib.csproj` and is therefore **not compiled**
— it is dead weight left over from a rename. It would break the build with a duplicate
type error if it were ever added to the project.

### 22.14 Assembly names do not match project names

Two support libraries have assembly names that differ from their project names:

| Project | Assembly | Namespace |
|---|---|---|
| `AuxLib` | **`UtilLib.dll`** | `audiamus.aux` |
| `AuxWinLib` | **`AuxLib.dll`** | `audiamus.aux.win` |

The project called `AuxLib` produces `UtilLib.dll`, and the project called `AuxWinLib`
produces `AuxLib.dll`. Code that references the *project* `AuxLib` therefore gets an
assembly called `UtilLib`, and code that references the *project* `AuxWinLib` gets an
assembly called `AuxLib`. This is historical and preserved for compatibility, but it is
genuinely confusing and worth renaming before the next major version.

---

## 23. Extension Guide

### 23.1 Adding a new conversion mode

1. **Add the enum value** to `EConvMode` (`Enums.cs`).
2. **Add the phase plan** in `initProgress` (`AaxAudioConverter.cs:523-584`) — the switch on
   `Settings.ConvMode` decides which phases run.
3. **Add the preparation method** `prepareTrackFiles<Mode>Mode(book)` that builds the
   `Track` list, and the corresponding `convert<Mode>Mode(book)` that calls it and then
   `transcodeTracks(book)`.
4. **Add the dispatch** in `convertBook` (`AaxAudioConverter.cs:627-643`).
5. **Add the naming branch** in `TagAndFileNamingHelper.file()`, `fullPath()`, `track()`
   and `chapter()`.
6. **Add the parallelism rules** in `DegreeOfParallelism.SetChapterTrack`
   (`AaxAudioConverter.nested.cs:39-94`).
7. **Add the track-length range** — `ETrackDuration` and `TrackDurationEx.TrackDuration`
   (`Extensions.cs:22-28`).
8. **Add the radio button** in `MainForm.Designer.cs` and the handler in `MainForm.cs`.

### 23.2 Adding a new setting

1. **Declare the property** on the appropriate interface in `SettingsInterface.cs`.
2. **Add it to `Properties/Settings.settings`** with the desired scope and default.
3. **Regenerate** `Properties/Settings.Designer.cs` (normally done by Visual Studio).
4. **Add the control** in `SettingsForm.Designer.cs` and wire it in
   `SettingsForm.initControlsFromSettings*` / `updateSettingsFromControls*`.
5. **Read it** in the engine through the interface.

### 23.3 Adding a new naming scheme

1. **Add the enum value** to `EFileNaming` / `ETitleNaming`.
2. **Add the case** in `fileChapters()`, `fileSingleNone()`, `fileSingleSome()` or
   `titleNaming()`.
3. **Add a resource string** for the enum name so `EnumChainConverter` can render it
   (the converter splits the name on `_` and looks up each part).

### 23.4 Supporting a new metadata sidecar

1. **Add a JSON model** in `AaxAudioConverterLib/*.json.cs` (following the
   `json.AppContentMetadata` pattern).
2. **Add a finder** modelled on `AsinJsonFile.FindFile` — local, relative, absolute.
3. **Add a reader** modelled on `AudibleAppContentMetadata.getContentMetaChapters`.
4. **Wire it** into `getContentMetadata` (`AaxAudioConverter.cs:1954-1972`).

### 23.5 Adding a new output format

1. **Add the enum value** to `EConvFormat`.
2. **Add the extension constant** to `TagAndFileNamingHelper` and resolve it in
   `fullPath()`.
3. **Update the copy-vs-transcode decision** in `transcodeTracksParallel`
   (`AaxAudioConverter.cs:1718-1720`) — a new format may or may not be stream-copyable
   from the source.
4. **Verify the ATL support** for the new container's tag format.

### 23.6 Adding a new FFmpeg operation

1. **Add a template constant** in `FFmpeg.cs` with `<PLACEHOLDER>` tokens.
2. **Add a public method** that builds the command, calls `runProcess`, and parses the
   output.
3. **Add the parsing regexes** — compile them as `static readonly Regex` with
   `RegexOptions.Compiled`.
4. **Handle cancellation** by calling `onCancel()` at the top of the output handler.
5. **Handle progress** by invoking `Progress?.Invoke(fraction)` from the handler.

---

## 24. Testing Strategy

### 24.1 Current state

There is no automated test project. Testing is manual, driven by the compile-time switches:

| Switch | Location | Purpose |
|---|---|---|
| `MANY_CHAPTERS_TEST` | `AaxAudioConverter.cs:1` | Replaces the chapter list with 378 synthetic chapters to exercise the >255-chapter path |
| `MP3_CHAPTERS_BY_FFMPEG` | `AaxAudioConverter.cs:2` | Routes MP3 chapter writing through FFmpeg's `ffmetadata` instead of ATL |
| `MULTIPART_TEST` | `TagAndFileNamingHelper.cs:1` | Rewrites the title to simulate a multi-part book |
| `TRACE && EXTRA` | `FFmpeg.cs:420`, `ThreadProgress.cs:4` | Additional trace output |

### 24.2 Recommended test layers

**Unit level** — pure functions with no I/O:

| Target | Cases |
|---|---|
| `Book.CheckParts` | none / some / all, missing parts, single part 1 |
| `Book.ExternalBookMeta.SeqString` | `"1"`, `"1.5"`, `"2.0"`, no series, non-numeric |
| `Chapter.CreateCueSheet` | No silences, one long silence, many silences, chapter shorter than the target length, chapter exactly at the boundary |
| `Book.Part.PreferEmbeddedChapterTimes` | Unambiguous match, ambiguous match, no match, threshold exceeded |
| `TagAndFileNamingHelper.Numbers` | `none` / `some` / `all`, single track, >999 tracks (digit count) |
| `ExString.Prune` | All invalid chars, the doubtful set, trailing dots, empty input |
| `ExUnc` | Short path, long path, UNC path, already-converted path |
| `TimeInterval` | Setting Begin past End, setting End before Begin, setting Duration |
| `TagAndFileNamingHelper` naming | Every `EFileNaming` × `EConvMode` × `EParts` combination |

**Integration level** — with a real FFmpeg and a small synthetic `.aax`:

| Scenario | Assertions |
|---|---|
| Convert a 3-chapter book in each of the 4 modes | The expected number of files, names, and tags |
| Convert a 2-part book | `EParts.all`, continuous track numbering, one playlist |
| Convert a book with a partial part set | `EParts.some`, per-part folders and playlists |
| Cancel mid-conversion | The output folder is removed; no partial files remain |
| A wrong activation code | The error dialog appears; entering a good code recovers |
| A missing activation code (Book Lib Connect book) | The conversion succeeds with no code |
| A 44.1 kHz book with `FixAACEncoding` | The output plays correctly; the `esds` atom is patched |
| A book with >255 chapters | All chapters are written |
| A very long book (>32 h) | `ffmpeg64.exe` is used; no out-of-memory |
| A path >260 characters | The conversion succeeds |
| An FFmpeg error mid-book | The error is reported; the rest of the book completes |

**Regression level** — golden-file comparison:

Record the exact output file names, tag values, and chapter markers for a set of fixture
books, and compare after each change. The naming and tagging subsystem is the most
regression-prone area because it is a large matrix of settings.

### 24.3 Manual verification checklist

1. Run with `-Log=3` and confirm the log contains the settings dump, the chapter lists, the
   FFmpeg command lines (with the activation code redacted), and the phase timings.
2. Confirm the activation code never appears in plain text in the log.
3. Confirm the temp directory is empty after a successful run and after a cancelled run.
4. Confirm the output folder contains exactly the expected files, with no leftovers.

---

## 25. Appendices

### 25.1 Complete enumeration reference

#### `EConvFormat` (`Enums.cs:5`)
`mp3`, `mp4`

#### `EConvMode` (`Enums.cs:6`)
`single`, `chapters`, `splitChapters`, `splitTime`

#### `EFileNaming` (`Enums.cs:15-21`)
`track`, `track_book`, `track_book_author`, `author_book_track`, `book_track`

#### `ETitleNaming` (`Enums.cs:28-36`)
`track`, `track_book`, `track_book_author`, `book_author`, `author_book`,
`author_book_track`, `book_track`

#### `ETrackNumbering` (`Enums.cs:41-48`)
`track`, `chapter_a_track`, `track_b_chapter_c`

#### `ELongTitle` (`Enums.cs:50`)
`no`, `book_series`, `series_book`, `as_is`

#### `EGeneralNaming` (`Enums.cs:52-56`)
`source`, `standard`, `custom`

#### `EGeneralNamingEx` (`Enums.cs:58-63`)
`source`, `standard`, `custom`, `_nofolders`

#### `EFlatFolderNaming` (`Enums.cs:65-68`)
`author_book`, `book_author`

#### `EAaxCopyMode` (`Enums.cs:72-83`, `[Flags]`)
`no = 0`, `flat__author_book = 10`, `flat__book_author = 11`, `author__book = 12`,
`author__author_book = 14`, `author__book_author = 15`

#### `ENamedChapters` (`Enums.cs:85-89`)
`no`, `yes`, `yesAlwaysWithNumbers`

#### `EOnlineUpdate` (`Enums.cs:91-95`)
`no`, `promptForDownload`, `promptForInstall`

#### `EUpdateInteract` (`Enums.cs:97-102`)
`newVersAvail`, `installNow`, `installLater`, `mayAskAgain`

#### `EVerifyAdjustChapters` (`Enums.cs:104-108`) — legacy, superseded
`no = 0`, `splitChapterMode = 1`, `bothChapterModes = 2`

#### `EVerifyAdjustChapterMarks` (`Enums.cs:110-114`) — the one in use
`no = 0`, `splitChapterOrTimeMode = 1`, `allModes = 2`

#### `EFixAACEncoding` (`Enums.cs:116-120`)
`no = 0`, `withIntermediateCopy = 1`, `allModes = 2`

#### `EReducedBitRate` (`Enums.cs:128-136`)
`off`, `_128k`, `_96k`, `_64k`, `_48k`, `_32k`, `_24k`

#### `EPreferEmbeddedChapterTimes` (`Enums.cs:138-142`)
`no`, `ifSilent`, `always`

#### `ERoleTag` (`Enums.cs:144-149`)
`none`, `artist`, `albumArtist`, `composer`, `conductor`

#### `ERoleTagAssignment` (`Enums.cs:152-160`)
`none = 0`, `author = 1`, `author__narrator__ = 2` *(deprecated)*, `author_narrator = 3`,
`__narrator__ = 4` *(deprecated)*, `narrator = 5`

#### `EOutFolderConflict` (`Enums.cs:161-166`)
`ask`, `overwrite`, `new_folder`, `skip`

#### `EProgressPhase` (`ProgressMessage.cs:2`)
`none`, `silence`, `adjust`, `transcoding`, `copying`

#### `ETrackState` (`Track.cs:8`)
`initial`, `current`, `complete`, `aborted`

#### `ECallbackType` (`AuxLib/InteractionMessage.cs:2`)
`info`, `infoCancel`, `warning`, `error`, `errorQuestion`, `errorQuestion3`, `question`,
`question3`, `custom`

### 25.2 Timing constants

| Constant | Value | Meaning | Location |
|---|---|---|---|
| `Chapter.MS_MIN_CHAPTER_LENGTH` | 1 ms | The minimum chapter length | `Chapter.cs:16` |
| `Chapter.TS_MIN_LAST_CHAPTER_LENGTH` | 1 s | The minimum length of the last chapter | `Chapter.cs:17` |
| `Chapter.TS_EPS_SILENCE` | 10 ms | The epsilon at the chapter end | `Chapter.cs:20` |
| `Chapter.TS_MAX_LAST_SILENCE` | 2 s | Above this, a trailing silence is a genuine pause | `Chapter.cs:21` |
| `Chapter.TS_CATCH_RANGE` | 30 s | How far from the chapter end a silence may be | `Chapter.cs:22` |
| `Chapter.TS_PART_TITLE_LENGTH` | 10 s | The "part title" pattern window | `Chapter.cs:23` |
| `Chapter.TS_LONGER_SILENCE` | 900 ms | Silences longer than this are track boundaries | `Chapter.cs:24` |
| `Chapter.TS_SHORT_SILENCE` | 500 ms | Silences longer than this are considered at all | `Chapter.cs:25` |
| `Chapter.TS_SILENCE_ANTE` | 300 ms | The lead-in inside the silence | `Chapter.cs:26` |
| `Book.Part.DurationFFmpeg64Bit` | 32 h | The 64-bit FFmpeg threshold | `Book.Part.cs:101` |
| `ETrackDuration.Min` | 3 min | The minimum track length | `Enums.cs:123` |
| `ETrackDuration.MaxSplitChapter` | 15 min | The maximum track length in split-chapter mode | `Enums.cs:124` |
| `ETrackDuration.MaxTimeSplit` | 90 min | The maximum track length in time-split mode | `Enums.cs:125` |
| `FFmpeg` silence threshold | −30 dB / 0.25 s | The `silencedetect` parameters | `FFmpeg.cs:55` |
| `Book.Part` matching threshold | 10 s | The maximum distance for a chapter-time match | `Book.Part.cs:200` |

### 25.3 Support-library quick reference

| Class | Library | Purpose |
|---|---|---|
| `Logging` | AuxLib | Process-wide file logger with level filtering |
| `ProcessHost` | AuxLib | The base class for external-process execution |
| `ArgParser` | AuxLib | `-tag` / `-tag=value` command-line parsing |
| `LocalFileSettingsProvider` | AuxLib | Location-independent user settings storage |
| `ResourceGuard` | AuxLib | RAII scope guard |
| `Singleton<T>` | AuxLib | Lazy, thread-safe singleton |
| `Temp` | AuxLib | Pseudo-unique string generation |
| `ApplEnv` | AuxLib | Static application/environment facts and paths |
| `DefaultSettings` | AuxLib | Reset settings to their declared defaults |
| `InteractionCallback<T,TResult>` | AuxLib | SynchronizationContext-marshalling callback |
| `ProcessList` | AuxLib | Global registry of running child processes |
| `ExUnc` | AuxLib | Extended-length (`\\?\`) path conversion |
| `ExString` | AuxLib | String pruning, shortening, joining |
| `BigEndianReader` | AuxLib | Big-endian binary reads |
| `PerformanceMonitor` | PerfLib | CPU load and process count sampling |
| `CpuUsage` | PerfLib | A single throttled perf-counter sampler |
| `ProcessCpuCounter` | PerfLib | Perf-counter instance resolution by PID |
| `ListViewColumnSorter` | AuxWinLib | Tag-aware list-view column sorting |
| `Culture` | AuxWinLib | Runtime language discovery and switching |
| `InteractionCallbackHandler` | AuxWinLib | Message-box implementation of `IInteractionCallback` |
| `SystemMenu` | AuxWinLib | Extends the window's system menu |
| `MsgBox` | AuxWin.DialogBox | Owner-centred message boxes |
| `WinFileIO` | AuxWin32Lib | Pinned-buffer Win32 file I/O |
| `FileAssociations` | AuxWin32Lib | Registry-based file-association management |
| `TreeDecomposition` | TreeDecomposition | Reflection-driven object-graph dump |
| `DynamicTypeDescriptor` | PropGridLib | Dynamic property-grid configuration |
| `BasePropertyGridAdapter<T>` | PropGridLib | Adapter base for settings grids |

### 25.4 Source-file index

#### Engine — `src/AaxAudioConverterLib/`

| File | Lines | Role |
|---|---|---|
| `AaxAudioConverter.cs` | 2381 | The orchestrator — the whole pipeline |
| `AaxAudioConverter.nested.cs` | 145 | `DegreeOfParallelism`, `MultiBookInitDirectoryHandling`, `FileError` |
| `Book.cs` | 432 | Book identity, part detection, title derivation |
| `Book.Part.cs` | 422 | The `Part` class — chapters, tracks, silence handling |
| `Book.Progress.cs` | 58 | The phase plan and weights |
| `Chapter.cs` | 503 | Cue sheets, time adjustment, silence handling |
| `Track.cs` | 56 | The output file record |
| `ChapteredTracks.cs` | 49 | The (part, chapter) → tracks index |
| `TimeInterval.cs` | 80 | The timing value type |
| `FFmpeg.cs` | 774 | The FFmpeg wrapper — commands and output parsing |
| `TagAndFileNamingHelper.cs` | 882 | Naming, folders, ATL tag read/write |
| `TagAndFileNamingHelper.Numbers.cs` | 91 | The `Numbers` counter class |
| `AudibleAppContentMetadata.cs` | 206 | `content_metadata_<ASIN>.json` |
| `AudibleAppSimsBySeries.cs` | 197 | `SimsBySeries` / `series_titles_*.json` |
| `AaxFileItem.cs` | 84 | The input file record |
| `ActivationCode.cs` | 102 | The activation-code aggregator |
| `ActivationCodeApp.cs` | 77 | Codes from the Audible app's data |
| `ActivationCodeRegistry.cs` | 83 | Codes from the registry |
| `OnlineUpdate.cs` | 467 | The update subsystem |
| `UpdatePackageInfo.cs` | 80 | The update manifest model |
| `ExtraMetaFiles.cs` | 122 | The `.txt` and cover companions |
| `AaxFileCopier.cs` | 114 | The `.aax` archiver |
| `FileEx.cs` | 64 | The buffered, callout-aware file copy |
| `FixAtomESDS.cs` | 45 | The AAC `esds` atom patch |
| `FFMetaData.cs` | 164 | The `ffmetadata` chapter file format |
| `CallbackWrapper.cs` | 42 | The progress/interaction/cancellation bundle |
| `ProgressMessage.cs` | 106 | The progress protocol |
| `ThreadProgress.cs` | 68 | Fractional → per-mille progress conversion |
| `AudioMeta.cs` | 9 | The probe result container |
| `Enums.cs` | 167 | Every enumeration |
| `Extensions.cs` | 63 | The derived-settings extensions |
| `Interfaces.cs` | 13 | `IPreviewTitle`, `IAudioQuality` |
| `SettingsInterface.cs` | 105 | The settings interface hierarchy |
| `ResourceInterface.cs` | 45 | `IResources` — the localized string contract |
| `ChainPunctuation.cs` | 17 | The punctuation chains |
| `CustomTagFileNames.cs` | 24 | The per-book overrides |
| `ToStringConverters.cs` | 25 | The activation-code and path converters |
| `ContentMetadata.json.cs` | 43 | The `content_metadata` JSON model |
| `SimsBySeries.json.cs` | 108 | The `SimsBySeries` JSON model |
| `SeriesTitles.json.cs` | 18 | The `series_titles` JSON model |

#### Application — `src/AaxAudioConverter/`

| File | Lines | Role |
|---|---|---|
| `MainForm.cs` | 1094 | The main window |
| `MainForm.Designer.cs` | 533 | The main window layout |
| `ProgressProcessor.cs` | 406 | The progress → UI adapter |
| `SettingsForm.cs` | 554 | The settings dialog |
| `SettingsForm.Designer.cs` | 1146 | The settings dialog layout |
| `PreviewForm.cs` | 424 | The per-book customization dialog |
| `FileDetailsForm.cs` | 65 | The metadata/cover viewer |
| `PGANaming.cs` | 150 | The naming property-grid adapter |
| `PerformanceHandler.cs` | 54 | The perf bars → UI adapter |
| `FileAssoc.cs` | 72 | The file-association prompt and setter |
| `Program.cs` | 38 | The entry point |
| `Resources.cs` | 59 | The `IResources` implementation |
| `Settings.cs` | 54 | The `IAppSettings` interface and migration |

### 25.5 Glossary of source locations

| Looking for | Go to |
|---|---|
| The top of the pipeline | `AaxAudioConverter.convert` — `AaxAudioConverter.cs:388` |
| The per-book pipeline | `AaxAudioConverter.convertBook` — `AaxAudioConverter.cs:586` |
| The FFmpeg command templates | `FFmpeg.cs:44-57` |
| The FFmpeg output regexes | `FFmpeg.cs:352-411` |
| The cue-sheet algorithm | `Chapter.CreateCueSheet` — `Chapter.cs:67` |
| The chapter adjustment | `Chapter.DetermineTimeAdjustment` — `Chapter.cs:180` |
| The embedded-times matcher | `Book.Part.findMatchingChapters` — `Book.Part.cs:177` |
| The file-naming matrix | `TagAndFileNamingHelper.file*` — `TagAndFileNamingHelper.cs:424-497` |
| The folder-layout matrix | `TagAndFileNamingHelper.fullPath` — `TagAndFileNamingHelper.cs:386` |
| The tag-writing code | `TagAndFileNamingHelper.writeMetaDataAtl` — `TagAndFileNamingHelper.cs:667` |
| The parallelism rules | `AaxAudioConverter.nested.cs:9-95` |
| The phase weights | `Book.Progress.cs:9-23` |
| The progress protocol | `ProgressMessage.cs` |
| The settings interfaces | `SettingsInterface.cs` |
| The progress-bar arithmetic | `ProgressProcessor.ProgBar` — `ProgressProcessor.cs:259` |
| The logging implementation | `AuxLib/Logging.cs` |
| The process-launch implementation | `AuxLib/ProcessHost.cs` |
| The long-path helpers | `AuxLib/ExtensionMethods.cs:171-221` |
| The installer script | `src/InnoSetup/AaxAudioConverter setup.iss` |

---

## Document maintenance

This document describes version **1.18.2**. When the code changes:

1. Update the affected section.
2. Update the source-file index (§25.4) if files were added or removed.
3. Update the enumeration reference (§25.1) if enums changed.
4. Re-verify the line references — they drift as the code changes.
5. Record new design decisions in §21 with the rationale, not just the outcome.

The most volatile parts are the **line references** and the **file index**. The
architecture, the pipeline structure, and the design rationale change far more slowly.

