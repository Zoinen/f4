# f4 — cross-platform TUI file manager in Go

## Overview

`f4` is a terminal file manager that reproduces the features, UX, data structures and
rendering logic of `far2l` / Far Manager, implemented entirely in Go. It runs either
inside a terminal or as a standalone graphical window, and ships as a single static
binary (`CGO_ENABLED=0`) for Linux, Windows, macOS, the BSDs, Illumos and Solaris
across amd64, arm64, arm, 386, mips*, riscv64, loong64 and ppc64*.

This working copy is a fork: `origin` is `github.com/ArtemYurov/f4`, `upstream` is
`github.com/unxed/f4`. The Go module path stays `github.com/unxed/f4`.

Two project conventions drive everything else:

- **AI-only codebase.** Every line is AI-generated; the test suite is the review
  mechanism. Changes are expected to arrive with tests.
- **Far heritage.** Internal structures keep the names of their C++ originals so that
  developers familiar with the Far API can navigate the code.

## Core Features

- Two-panel file manager with Far/far2l keymap, dialogs, viewer, editor and macros
- Virtual filesystem layer (`vfs/`): local FS, archives, FTP/SFTP, S3 and cloud
  backends, Android and iOS devices, SQLite browsing, FUSE mounting
- Plugin system with several transports: in-process Go plugins, RPC plugins, Lua
  plugins (`luaplug/`), and a WASM host (`wazero`)
- Syntax highlighting via `colorer4go` and `chroma`
- Media support: audio playback (mp3/flac/ogg), ID3 editing, media metadata
- Spreadsheet mode (`sheet/`) and a piece-table backed editor (`piecetable/`)
- Terminal integration: ConPTY on Windows, host console mode, ttyx sessions
- Self-update from the command line (`f4 --update nightly|stable`)

## Tech Stack

- **Programming language:** Go 1.26.6, `CGO_ENABLED=0`
- **Framework:** none (custom TUI); UI and input come from the external `vtui` and
  `vtinput` libraries
- **Rendering backends:** `--gui=win32|gogpu|x11|wayland|ebiten`,
  `--tty=ansi|win32`
- **Database:** none for the application itself; `plugins/sqlite` reads user SQLite
  files as a VFS
- **Notable dependencies:** `wazero` (WASM), `hanwen/go-fuse` (FUSE),
  `aws-sdk-go-v2` (S3), `pkg/sftp`, `jlaffaye/ftp`, `mholt/archives`,
  `alecthomas/chroma`, `ebitengine/purego`, `danielpaulus/go-ios`
- **Lint:** golangci-lint v2 — staticcheck, errcheck, ineffassign, unused, gosec
- **CI:** GitHub Actions (`.github/workflows/build.yml`), cross-platform build matrix
  plus nightly and tagged releases

## Architecture Notes

The tree is organised by subsystem rather than by layer:

- `cmd/f4/` — the application itself: 687 files in `package main`, covering panels,
  dialogs, editor, viewer, actions, macros and terminal handling. This is where most
  work happens, and it is deliberately one flat package.
- `vfs/` — the filesystem abstraction every panel and plugin goes through, with
  `hostfs/`, `hostmode/`, `hostpath/` subpackages and per-OS files
  (`*_unix.go`, `*_windows.go`).
- `plugins/` — one package per plugin (`archive`, `cloudfox`, `netfox`, `mediainfo`,
  `envman`, `ios`, `android`, `sqlite`, `visren`, `id3editor`, `chroma`) plus dummy
  plugins used as transport fixtures.
- `internal/` — non-exported platform helpers: `wincon`, `ttyx`, `netproxy`,
  `hideconsole`.
- `sdk/`, `plugring/`, `luaplug/` — plugin API, registry and Lua engine.
- `piecetable/`, `textlayout/`, `sheet/`, `colorer/`, `fusefs/`, `vtvibe/` —
  self-contained subsystems consumed by `cmd/f4`.
- `tools/` — developer tooling, including the `ttytest` terminal harness.
- `docs/` — 48 subsystem documents (VFS, PLUGINS, MACROS, KEYMAP, CONPTY, WINCON,
  TERMINAL, UX_GUIDELINES and others). Consult the relevant one before touching a
  subsystem.

Platform differences are handled with build-tag file suffixes, not runtime branching.
Anything touching the console, the filesystem or process spawning has a per-OS file.

## Architecture

See `.ai-factory/ARCHITECTURE.md` for the detailed architecture guidelines:
folder structure, dependency rules and module boundaries.
Pattern: Modular Monolith (module = Go package).

## Non-Functional Requirements

- **Portability:** every change must keep the full build matrix green, including the
  exotic targets. Prefer `os`/`syscall` wrappers already present over new per-OS code.
- **No cgo:** the static single-binary promise depends on it. FFI goes through
  `purego`/`ffibridge`, not cgo.
- **Tests as canaries:** 560 `_test.go` files. New behaviour lands with a test; a
  bug fix lands with a regression test.
- **Error handling:** wrap with `fmt.Errorf("...: %w", err)`, compare with
  `errors.Is`. Error strings often become dialog text the user reads.
- **Binary size:** ~110 MB today; new dependencies are weighed against it.
- **Documentation:** subsystem changes update the matching file in `docs/`.
