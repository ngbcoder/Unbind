# URI Handoff — Unseal → Unbind

> **Document type:** Interface specification
> **Consumers:** `Unseal` (sender) · `Unbind` (receiver)
> **Status:** Draft — awaiting sign-off before either side implements
> **Owner of this copy:** `BookLibConnect` (the sender), because the button lives here
> **Intended location:** `P:\Nbprojects\Go\BookLibConnect\URIhandoff.md`
> **Related:** `design.md` §10.3 (export stage) · `Todo.md` M4, M7

---

## Table of contents

1. [Purpose](#1-purpose)
2. [The contract in one picture](#2-the-contract-in-one-picture)
3. [Roles and responsibilities](#3-roles-and-responsibilities)
4. [Wire format — the URI](#4-wire-format--the-uri)
5. [Wire format — the handoff document](#5-wire-format--the-handoff-document)
6. [Transport — the local endpoint](#6-transport--the-local-endpoint)
7. [Receiver behaviour — Unbind](#7-receiver-behaviour--aax-audio-converter)
8. [Sender behaviour — Unseal](#8-sender-behaviour--unseal)
9. [The button](#9-the-button)
10. [Error handling](#10-error-handling)
11. [Security](#11-security)
12. [Versioning and compatibility](#12-versioning-and-compatibility)
13. [Platform differences](#13-platform-differences)
14. [Test vectors](#14-test-vectors)
15. [Open decisions](#15-open-decisions)
16. [Appendix A — registry registration (optional)](#appendix-a--registry-registration-optional)
17. [Appendix B — message framing](#appendix-b--message-framing)

---

## 1. Purpose

Unseal downloads and de-DRM's audiobooks, then exports them as `.aax` files plus
Audible-shaped sidecar JSON. Unbind turns those `.aax` files into
`.mp3` / `.m4a` / `.m4b`.

Today the user has to leave Unseal, open Unbind, and drag the exported
files across. This document specifies a **one-click handoff**: a button in Unseal
that loads the selected exported `.aax` files into a running (or not yet running)
Unbind.

### 1.1 What this document is

A **wire format** and a **behavioural contract** that both sides implement
independently. Neither side imports the other's code. The contract is:

- a **URI** (`unbind://…`) that names the operation,
- an optional **JSON document** that carries the payload,
- a **local endpoint** that delivers the request to an already-running receiver.

### 1.2 What this document is not

- It is not a plugin API. There is no shared library, no shared Go module, no
  in-process call.
- It does not describe the conversion itself. Unbind owns that
  entirely.
- It does not describe the reverse direction. Unbind does **not**
  report completion back to Unseal in v1 (see [§15](#15-open-decisions)).

---

## 2. The contract in one picture

```
┌──────────────────────┐                          ┌───────────────────────────┐
│  Unseal (sender)     │                          │  Unbind      │
│                      │                          │  (receiver)               │
│  [Convert…] button   │                          │                           │
│         │            │                          │                           │
│         ▼            │                          │                           │
│  build HandoffDoc ───┼──► writes %TEMP%         │                           │
│         │            │    handoff-<token>.json  │                           │
│         ▼            │                          │                           │
│  spawn converter ────┼──► argv[1] =             │                           │
│         with URI     │    "unbind://add?doc=…"  │                           │
│                      │                          │                           │
└──────────────────────┘                          └───────────┬───────────────┘
                                                              │
                                     ┌────────────────────────┴──────────────┐
                                     │                                       │
                              not running?                             running?
                                     │                                       │
                                     ▼                                       ▼
                              primary instance                    secondary instance
                              reads URI, reads doc,                 dials local endpoint,
                              adds files, shows window             forwards URI, exits
                                     │                                       │
                                     │◄──────── ack ────────────────────────┘
```

The URI is the **contract**. The endpoint is an **implementation detail of the
receiver** — Unseal never speaks it. Unseal only ever spawns a process.

---

## 3. Roles and responsibilities

| | Unseal (sender) | Unbind (receiver) |
|---|---|---|
| Owns the `.aax` files and sidecars | ✅ | consumes them |
| Knows the ASIN, series, cover cache | ✅ | — |
| Registers the URI handler | ❌ | ✅ (optional, Windows) |
| Spawns the receiver | ✅ | — |
| Defines the URI grammar and the document schema | ✅ (owner) | implements it |
| Implements the local endpoint | ❌ | ✅ |
| Deduplicates file paths | — | ✅ |
| Converts the files | — | ✅ (user-triggered) |

**The receiver is authoritative for validation.** The sender may send anything; the
receiver validates and drops what it cannot use.

---

## 4. Wire format — the URI

### 4.1 Grammar

```
unbind://<verb>[?<param>=<value>[&<param>=<value>]*]
```

The scheme is `unbind`, lowercase. Windows treats the scheme case-insensitively,
so the receiver must compare case-insensitively.

### 4.2 Verbs

| Verb | Meaning |
|---|---|
| `add` | Add one or more files to the receiver's file list. |
| `show` | Bring the receiver's window to the front. Takes no files. |

Unknown verbs are ignored (logged, not fatal).

### 4.3 Parameters — `add`

| Param | Multiplicity | Meaning |
|---|---|---|
| `p` | repeatable | Absolute path of one file to add. |
| `doc` | 0 or 1 | Absolute path of a handoff JSON document. |
| `activate` | 0 or 1 | `1` (default) → raise and focus the window. `0` → load silently. |
| `src` | 0 or 1 | Source identifier, e.g. `unseal`. Informational only. |

**Precedence.** If `doc` is present it is authoritative: its `files` array is used
and any `p` parameters are **ignored** (the receiver logs a warning). A document
may not be combined with inline paths in v1.

### 4.4 Parameters — `show`

| Param | Multiplicity | Meaning |
|---|---|---|
| `activate` | 0 or 1 | `1` (default) → raise and focus. |
| `src` | 0 or 1 | Source identifier. |

### 4.5 Encoding

- Parameters are percent-encoded per RFC 3986. Go's `url.Values.Encode()` produces
  a compliant encoding; `url.Parse` + `u.Query()` reverses it.
- Windows paths contain backslashes and a drive colon (`C:\…`). Both are
  percent-encoded (`%5C`, `%3A`). This is expected — **do not hand-build URIs by
  string concatenation.**
- The receiver must reject relative paths and paths containing `..` segments
  after normalisation.
- A URI must stay under the Windows command-line limit (32 767 characters). With
  `doc=` the URI is a few hundred bytes regardless of how many files travel.

### 4.6 Examples

```
unbind://add?p=C%3A%5CBooks%5Cbook1.aax&activate=1&src=unseal
unbind://add?doc=C%3A%5CUsers%5Cx%5CAppData%5CLocal%5Caudiamus%5CUnseal%5Ctmp%5Chandoff-8f3a1c.json
unbind://show
```

---

## 5. Wire format — the handoff document

### 5.1 Purpose

The URI carries the *instruction*. The document carries the *data*. This split
keeps the URI short, sidesteps percent-encoding of large payloads, and gives
room for metadata that will not fit in a query string.

### 5.2 Location and ownership

| | |
|---|---|
| Written by | Unseal |
| Path | `%LOCALAPPDATA%\audiamus\Unseal\tmp\handoff-<token>.json` |
| Read by | Unbind (read-only) |
| Deleted by | **Unseal**, via its temp-directory housekeeping (M8) |

The receiver **must not** delete the document — the sender owns its temp
directory. The receiver must open it read-only.

`<token>` is 8 lowercase hex characters, unique per handoff.

### 5.3 Schema

```jsonc
{
  "protocol": 1,                          // required, integer, see §12
  "source": "unseal",                     // required, string, informational
  "created": "2026-11-11T20:15:00Z",      // optional, RFC 3339 UTC
  "files": [                              // required, array (may be empty)
    {
      "path": "C:\\Users\\x\\Music\\Audible\\Export\\Title_B0XXXXXXXXXX_LC.aax",
                                           // required, absolute path, must exist
      "asin": "B0XXXXXXXXXX",             // optional
      "title": "The Long Book",           // optional
      "authors": ["Jane Doe"],            // optional, array of strings
      "narrators": ["John Smith"],        // optional, array of strings
      "series": "The Series",             // optional
      "seriesSequence": "1",              // optional, string (may be "1.5")
      "coverPath": "C:\\…\\img\\B0XXXXXXXXXX.jpg",
                                           // optional, fallback only (§5.5)
      "durationMs": 3600000               // optional, integer
    }
  ],
  "options": {                            // optional
    "format": "mp4",                      // "mp3" | "mp4"
    "mode": "chapters"                    // "single" | "chapters" | "splitChapters" | "splitTime"
  }
}
```

### 5.4 Field rules

| Rule | Detail |
|---|---|
| Unknown fields | Ignored. A receiver must not fail on them. |
| Missing `files` | Treated as an empty array → the receiver logs "handoff contained no files". |
| `path` | Must be absolute, must exist, must be a regular file. |
| `coverPath` | Absolute path to an image file. Used **only** if the `.aax` has no embedded cover. |
| `options.format` | Maps to Unbind's `EConvFormat`: `mp3`, `mp4`. |
| `options.mode` | Maps to `EConvMode`: `single`, `chapters`, `splitChapters`, `splitTime`. |
| `options` absent | The receiver uses **its own current settings**; the handoff does not change them. |

### 5.5 Metadata precedence

The `.aax` file itself is the primary source of truth. The document is a
**supplement**:

| Field | Precedence |
|---|---|
| Title, authors, narrators, cover | The file's embedded tags win. Document values fill gaps. |
| ASIN, series, series sequence | The sidecar JSON next to the file wins. Document values fill gaps. |

Rationale: the exported `.aax` retains Audible's original `moov`, so it already
carries `aART`, `covr` and the `©nrt` freeform tag. The document is belt-and-braces.

### 5.6 Inline form

A `p=` parameter carries no metadata. The receiver synthesises the equivalent of
`{ "path": "<p>" }` and nothing else.

---

## 6. Transport — the local endpoint

The endpoint exists for one reason: **the receiver may already be running.**
A second launch of the receiver must reach the first instance rather than opening
a duplicate window.

### 6.1 Abstraction

The receiver implements one interface with two backends:

```
LocalEndpoint
  ├── pipe_windows.go   — Windows named pipe
  └── socket_unix.go    — macOS / Linux Unix domain socket
```

### 6.2 Endpoint names

| Platform | Name |
|---|---|
| Windows | `\\.\pipe\Unbind.handoff` |
| macOS | `$XDG_RUNTIME_DIR/unbind.sock` (fallback `os.UserCacheDir()/unbind.sock`) |
| Linux | `$XDG_RUNTIME_DIR/unbind.sock` (fallback `os.UserCacheDir()/unbind.sock`) |

The Windows named pipe is **session-local** by default (no `Global\` prefix), which
matches the session-local semantics of the POSIX socket. Two users on the same
machine do not collide.

### 6.3 Message set

| Direction | Message | Payload |
|---|---|---|
| client → server | `handoff` | `{"type":"handoff","uri":"unbind://add?…"}` |
| client → server | `ping` | `{"type":"ping"}` |
| server → client | `ack` | `{"type":"ack","ok":true,"added":3,"skipped":0,"error":""}` |
| server → client | `pong` | `{"type":"pong"}` |

The client is deliberately **dumb**: it relays the URI it was given and does not
interpret it. All parsing happens in the receiver.

### 6.4 Framing

See [Appendix B](#appendix-b--message-framing): 4-byte little-endian length prefix
followed by that many bytes of UTF-8 JSON. Maximum message size 1 MiB.

### 6.5 Lifecycle

- The **primary** instance creates the endpoint *before* it shows the window, so a
  secondary instance that starts a moment later always finds it.
- The **primary** removes the endpoint on shutdown (delete the socket file on
  POSIX).
- A stale socket file (primary crashed) is detected by a failed connect and
  removed by the next primary.

---

## 7. Receiver behaviour — Unbind

### 7.1 Startup sequence

```
1. Parse argv.
   - argv[1] == "unbind://…"  → URI request
   - any other argv element     → a file path (file-association behaviour)
   - both may be present.

2. Acquire the single-instance mutex (named, per user).
   ┌─ acquired ──► WE ARE THE PRIMARY
   │                 2a. Start the LocalEndpoint server (accept loop).
   │                 2b. If a URI was given → handle it here (see §7.2).
   │                 2c. Else if file paths were given → add them.
   │                 2d. Show the main window, focused.
   │
   └─ not acquired ─► WE ARE A SECONDARY
                     2e. Dial the LocalEndpoint. Retry with backoff for up to 5 s.
                     2f. Send {"type":"handoff","uri":…}  (or {"type":"ping"} for `show`).
                     2g. Await the ack (timeout 5 s).
                     2h. Exit 0 on ack, exit non-zero on failure.
                        Never show a window.
```

**Startup order matters.** The primary must start the endpoint server *before*
displaying the window, otherwise a secondary launched in that gap will fail to
connect. The mutex is acquired first, so a secondary knows a primary exists before
the endpoint is guaranteed to be listening — hence the retry in step 2e.

### 7.2 Handling a request (primary)

All of this happens on the GUI thread, via the framework's post-to-GUI-thread
mechanism:

```
1. Parse the URI.
   - unknown scheme        → log, ignore
   - unknown verb          → log, ignore
   - malformed percent-encoding → log, ignore
2. Resolve the payload:
   - doc= present → open read-only, parse JSON, extract files[] and options[]
   - p= present   → synthesise one file entry per p
3. Validate every file:
   - absolute path, exists, is a regular file
   - extension ∈ {.aax, .aa, .m4b, .m4a, .mp4}
   - reject anything else, count it as skipped
4. Dedupe against the current list:
   - Windows  → case-insensitive path comparison
   - macOS/Linux → case-sensitive path comparison
   - already present → counted as skipped, not re-added
5. Add the surviving files through the same code path as drag-and-drop:
   filter → probe → read metadata → add to the list → select.
6. Apply options.format / options.mode ONLY if the document supplied them.
   Never overwrite the user's settings with the document's defaults.
7. If activate=1 → raise and focus the window.
8. Reply on the endpoint with the ack: {ok, added, skipped, error}.
```

### 7.3 Do not auto-convert

The receiver **loads and selects** the files. It does **not** start a conversion.
The user still chooses the output format, the mode, and presses Convert.

This is a deliberate safety property: see [§11](#11-security).

### 7.4 Behaviour during a running conversion

If a conversion is in progress when a handoff arrives, the files are still added
to the list and selected, and a transient status message reports how many were
added. The conversion in flight is **not** interrupted.

### 7.5 Sidecar resolution

The receiver already resolves Audible sidecars relative to the `.aax` file:

```
<dir of the .aax>/content_metadata_<ASIN>.json
<dir of the .aax>/<ASIN>.json
<dir of the .aax>/series_titles_<ASIN>.json
```

Unseal writes all three flat into the export directory beside the `.aax`, so the
**local** lookup hits and no Audible-app directory search is needed.

The ASIN is derived from the file name with the pattern `_([0-9A-Z]{10})(_|$|\.)`.
Unseal names exports `<title20>_<ASIN>_LC[_<bitrate>_<samplerate>][_Mono|_Stereo].aax`,
which matches. **The receiver should take the last match in the name, not the
first** — a title containing `_XXXXXXXXXX_` would otherwise produce a false
positive.

---

## 8. Sender behaviour — Unseal

### 8.1 Sequence

```
1. The user selects rows in the download-queue table and presses [Convert…].
2. Build the payload:
     files[] ← one entry per selected row, path = Conversion.ExportedFilePath
     options ← absent (the receiver keeps its own settings)
3. Write the handoff document:
     %LOCALAPPDATA%\audiamus\Unseal\tmp\handoff-<token>.json
     — write to a .tmp sibling first, then rename, so the receiver never sees a
       partially written document.
4. Build the URI:
     unbind://add?doc=<percent-encoded path>&activate=1&src=unseal
5. Spawn the receiver:
     exec.Command(convExePath, uri)
     - HideWindow on Windows (no console flash)
     - Do not wait for exit; but detect an immediate spawn failure.
6. On spawn failure → error dialog offering to locate the converter executable.
```

### 8.2 Spawning

The sender spawns the **executable directly** with the URI as `argv[1]`. It does
**not** go through `ShellExecuteW`, `xdg-open`, or `open`. This keeps the sender
free of registry, XDG and Apple Events dependencies, and it works identically on
all three platforms.

The URI is still the contract, and the receiver still parses it — the protocol
handler in [Appendix A](#appendix-a--registry-registration-optional) is a
convenience for humans and for the shell, not a requirement for the sender.

### 8.3 Locating the receiver

| Platform | How |
|---|---|
| Windows | Auto-detect from `HKLM`/`HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{AppName}_is1\InstallLocation`; otherwise a user setting. |
| macOS / Linux | A user setting pointing at the executable. |

The sender stores the resolved path in its own settings (`UnbindPath`)
and lets the user browse for it.

---

## 9. The button

### 9.1 Placement

Main window, download-queue toolbar, next to the existing `[Download…]` and
`[Remove]` buttons. Label: **"Convert with Unbind"**.

### 9.2 Enablement

The button is enabled **only** when all of the following hold:

1. At least one row is selected.
2. Every selected row's state is `exported` or `converted`
   (Unseal's `EConversionState`; ordinals 11 and 13).
3. The converter executable path is configured and the file exists.

When disabled, the tooltip states which condition failed.

### 9.3 Behaviour

- Selected rows may belong to **different books**. All of their exported files
  are handed over in one request.
- For a multi-part book, one file per part is sent. The receiver groups them back
  into one book using its own part-detection logic — the sender does not need to
  say anything about parts.
- The button does **not** wait for the conversion. It returns immediately; the
  receiver works in its own window.
- The sender does **not** change its own state machine. A book stays `exported`
  after the handoff.

---

## 10. Error handling

| Situation | Behaviour |
|---|---|
| Converter executable not found | Sender: error dialog, "Locate…" offers a file picker. |
| Spawn fails (permissions, not an executable) | Sender: error dialog with the OS error. |
| Receiver not running | Normal spawn; it becomes the primary and shows its window. |
| Receiver already running | Secondary forwards over the endpoint and exits; the primary raises its window. |
| Endpoint unreachable for >5 s | Secondary exits with code 3 and logs it. Sender sees a normal exit (the process started), so it shows no error — the receiver simply did not respond. **Improvement candidate:** have the secondary write a one-byte marker so the sender can distinguish "started" from "responded". |
| Malformed URI | Receiver logs it, ignores it, shows its window normally. Never fatal. |
| `doc` missing or unreadable | Receiver logs an error, shows "Handoff document not found", continues with an empty list. |
| `doc` is not valid JSON | Same as above, with the parse error in the log. |
| A listed file does not exist | Skipped, counted in the ack, the rest are still added. |
| A listed file is already in the list | Skipped as "already present". |
| `protocol` newer than the receiver supports | Receiver refuses the document with a clear message. Files are not added. |

---

## 11. Security

The URI scheme is **invocable by any local process**. The receiver must be
defensive.

| Control | Rule |
|---|---|
| No implicit execution | A handoff **only adds files to a list**. It never starts a conversion, never writes to disk, never spawns anything. The user must still press Convert. |
| Path validation | Absolute only. No `..` segments after normalisation. Must exist and be a regular file. Extension must be in the allowed set. |
| Document validation | Absolute path. Must exist. Maximum 8 MiB. Parsed as **data only** — no interpolation into commands, no path extraction from fields other than `path`/`coverPath`. |
| No elevation | The receiver runs `asInvoker`, like the C# original. |
| Endpoint hardening (Windows) | Create the pipe with an SDDL that restricts access to the owner: `D:P(A;;GA;;;OW)`. |
| Endpoint hardening (POSIX) | Create the socket with mode `0600` in a user-private directory. |
| No network | Nothing in this path touches the network. |

---

## 12. Versioning and compatibility

- The document carries `"protocol": 1`. The receiver rejects documents with
  `protocol` greater than its own maximum, with a readable message.
- The receiver **ignores unknown fields** at every level. The sender must
  therefore never remove a field that a deployed receiver might still read.
- The URI has no version field. The verbs and parameters in §4 are additive: a
  future receiver may add parameters, and an old receiver ignores them.
- Both sides should ship a **golden fixture** of a minimal and a rich document,
  and a test that the fixture parses into the expected structure. That way a
  change to either side that breaks the contract fails a test rather than a user.

---

## 13. Platform differences

| Concern | Windows | macOS | Linux |
|---|---|---|---|
| Endpoint | named pipe | Unix socket | Unix socket |
| Path comparison | case-insensitive | case-sensitive | case-sensitive |
| Long paths | `\\?\` prefix beyond 260 chars | n/a | n/a |
| Spawn | `exec.Command`, `HideWindow` | `exec.Command` | `exec.Command` |
| Shell integration | `.aax`/`.aa` file association + optional `unbind://` handler | `.app` bundle `CFBundleURLTypes` (optional) | `.desktop` + XDG MIME (optional) |
| Receiver path discovery | uninstall registry key | user setting | user setting |
| Runtime prerequisites | — | `.app` bundle, signing for distribution | X11, Pango/Cairo, GTK |

**Scope.** The sender and the receiver are both developed and shipped for
Windows. The transport and the path helpers are factored so that a POSIX build
is a matter of adding `socket_unix.go` and dropping the registry-dependent
features — not a rewrite.

---

## 14. Test vectors

Golden fixtures, committed to **both** repositories:

| File | Content |
|---|---|
| `testdata/handoff/minimal.json` | One file, no metadata, no options. |
| `testdata/handoff/rich.json` | Two files of one multi-part book, full metadata, options. |
| `testdata/handoff/empty.json` | `files: []` — the degenerate case. |
| `testdata/handoff/uri_minimal.txt` | `unbind://add?p=C%3A%5CBooks%5Ca.aax&activate=1&src=unseal` |
| `testdata/handoff/uri_doc.txt` | `unbind://add?doc=…&activate=1&src=unseal` |

Each side must have a test that:

- **Sender:** builds a document from a set of conversions and asserts it matches
  the golden fixture byte-for-byte (after normalising the token and timestamp).
- **Receiver:** parses the golden fixture into its internal structures and
  asserts the expected file list, metadata and options.
- **Both:** parse the golden URIs and assert the expected verb and parameters.

---

## 15. Open decisions

| # | Question | Recommendation |
|---|---|---|
| 1 | Should the handoff start the conversion automatically? | **No.** Load and select only. The user presses Convert. A receiver setting `AutoStartOnHandoff` may enable it later. |
| 2 | Should the receiver report completion back to the sender? | **Not in v1.** The sender's state stays `exported`. A reverse channel is a v2 item. |
| 3 | One shared Go module for the schema, or duplicated structs? | **Duplicated structs plus a shared golden fixture.** No cross-repository dependency, no version skew; the `protocol` field plus ignored-unknown-fields makes divergence safe. |
| 4 | Where does this document live? | This copy is in the BookLibConnect repository. The converter repository should hold a pointer to it, or an identical copy under the same name. |
| 5 | Should `doc` be replaced by inline `p` parameters entirely? | No — the document exists to carry metadata, and to keep the URI short. |
| 6 | Should the URI scheme be `unbind` or something namespaced further? | `unbind` is short and matches the product. Risk of collision on a shared machine is negligible for a desktop utility. |

---

## Appendix A — registry registration (optional)

The protocol handler is a **convenience**, not a requirement. Without it the
sender still works, because it spawns the executable directly.

Registered by the Unbind installer (Inno Setup) into the **per-user**
registry, so no elevation is needed:

```
HKEY_CURRENT_USER\Software\Classes\unbind
    (Default)      = "URL:Unbind Protocol"
    "URL Protocol" = ""

HKEY_CURRENT_USER\Software\Classes\unbind\shell\open\command
    (Default)      = "\"C:\Program Files\audiamus\Unbind\Unbind.exe\" \"%1\""
```

The same installer removes the key on uninstall.

The C# original already registers `.aax` and `.aa` file associations into
`HKCU\Software\Classes` and notifies Explorer with `SHChangeNotify(
SHCNE_ASSOCCHANGED)`. The Go port reuses that machinery for the protocol
handler.

---

## Appendix B — message framing

Every endpoint message is:

```
┌────────────────────────┬──────────────────────────────┐
│ length (4 bytes, LE)   │ payload (length bytes, UTF-8)│
└────────────────────────┴──────────────────────────────┘
```

- The length is the payload size in bytes, excluding the 4-byte prefix.
- Maximum payload: 1 MiB. A larger length is a protocol error → close the
  connection.
- The reader must read exactly `length` bytes; a short read is an error.
- The payload is a single JSON object. No streaming, no chunking.

Go implementation sketch (Windows named pipe via `go-winio`, POSIX socket via
`net`):

```go
func writeMessage(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > maxMessage {
		return errMessageTooLarge
	}
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
	if _, err := w.Write(n[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readMessage(r io.Reader, v any) error {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return err
	}
	size := binary.LittleEndian.Uint32(n[:])
	if size > maxMessage {
		return errMessageTooLarge
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
```
