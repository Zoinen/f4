# Base project rules — f4

> Auto-detected conventions from codebase analysis. Edit as needed.

## Naming Conventions

- Files: `snake_case.go`, named after the feature they hold
  (`action_registry.go`, `panels_frame.go`, `os_vfs_search.go`)
- Test files: the source file name plus `_test.go`, kept next to the source
- Platform files: build-tag suffixes carry the platform, not runtime branching —
  `*_windows.go`, `*_unix.go`, `*_other.go`, plus `//go:build` lines in 182 files
- Variables and functions: Go standard `camelCase` unexported, `PascalCase` exported
- Types: `PascalCase`; Far-derived structures keep the names of their C++ originals
  even when Go style would suggest otherwise
- Packages: single lowercase word (`vfs`, `wincon`, `ttyx`, `cloudfox`, `envman`)
- **Language: English.** Comments, identifiers and error strings in new or
  rewritten code are English, because the perimeter already is — the
  architecture document, the agent instructions, this file, commit messages —
  and because a warning is worthless to the contributor it was written for if
  they cannot read it. Roughly 55 of some 600 files carry Russian comments from
  the project's author; those stay as they are. Translating them is its own
  piece of work, not something a move commit smuggles in.
- **`config` vs `Settings`** — the two are not synonyms here, and keeping them
  apart is what stops one from swallowing the other:
  - `config` names the application's configuration: the package
    `internal/config`, the type `F4Config`, the global `config.App`, the file it
    loads. There is exactly one, so the word is taken — do not add a second
    package or a global under a name that means the same thing.
  - `Settings` names the parameters of a single subsystem, as a type inside that
    subsystem's own package: `netproxy.Settings`, `update.Settings`,
    `mediainfo.Settings`. Never a package of its own, never a global.
  - `settings` inside a filename is about the settings *dialog*, not storage —
    `proxy_settings_ui.go`, `settings_save.go`. The comment atop `settings_save.go`
    draws the line itself: the window geometry comes from the GUI backend, the
    settings file from `internal/config`.

## Module Structure

The reasoning behind the layers, and what may import what, is in
`.ai-factory/ARCHITECTURE.md`. This section is the rule; that document is the
argument.

- `cmd/f4/` — the composition root: `main.go`, the four module-wide auditors,
  and the Windows `.syso` resources the linker takes from the main package's own
  directory. Five files.
- `internal/` — the application, 40 packages in four layers. `internal/app` is
  layer 4 and the only package permitted to import every other one;
  `cmd/f4/architecture_test.go` enforces that in both directions.
- `vfs/` — the filesystem abstraction all panels and plugins go through
- `sdk/` — the plugin API third parties compile against
- `plugins/<name>/` — one package per plugin
- `internal/testutil/`, `internal/paneltest/` — test scaffolding shared across
  packages. Go will not let one package import another's tests, so these are
  ordinary packages; import them from `_test.go` files only. A helper only one
  package uses stays in that package
- `plugring/` — data, not a Go package: the community catalogue of installable
  plugins. It stays in the root: that is the surface an outside contributor is
  pointed at, and its URL is published
- `tools/` — developer tooling, not shipped in the binary
- UI and input live outside this repository, in the `vtui` and `vtinput` libraries

### File Placement

- **A file goes to the package that owns its subject.** If none does, create
  one. Never `internal/app`: it wires subsystems together and implements none of
  them, and a file parked there is a file whose home was not looked for.
- **Nothing new in `cmd/f4`.** It is `main.go`, the auditors and the `.syso`.
- **Inside a package, `<topic>.go` and `<topic>_<aspect>.go`.** The prefix names
  the topic, not the package: `panel/frame.go`, never `panel/panel_frame.go`. A
  platform suffix comes last: `panel/frame_procenv_windows.go`.
- **A test lives with its subject.** A test that spans packages is hosted by the
  highest one it needs; if that pulls it away from private members it must
  reach, split it or make it a `package X_test`.
- **Resources travel with the package that embeds them**, and a test that reads
  them from disk asserts it found some — a sweep over an empty directory passes.

## Code Navigation

- Navigation runs through the gopls MCP server (`.mcp.json` runs `gopls mcp`)
  when it is available. Check with `command -v gopls` before relying on it: the
  server is not part of a clone, and where gopls is missing it never starts and
  its tools are not offered. Then `grep`, `go doc` and `go list` carry the work,
  and the answer says navigation ran without gopls.
- With gopls present, use it for symbol questions instead of `grep`: `go_search`,
  `go_symbol_references`, `go_package_api`, `go_file_context`, `go_diagnostics`.
  The tree is 69 packages and a symbol's package is not always the one its name
  suggests, so grep over the module is slow and imprecise.
- Answers come from `go/types`, so they match what the compiler sees, including
  files behind another platform's build tag.
- Installing it is the developer's choice, not a step an agent takes on its own:
  `go install golang.org/x/tools/gopls@latest`, with `$(go env GOPATH)/bin` on
  PATH. Nothing is indexed into the repository and a fresh clone needs no setup.
- Grep remains correct for non-symbol text: comments, error strings, build tags,
  ini keys.

## Error Handling

- Wrap with context: `fmt.Errorf("doing X: %w", err)` (~395 call sites)
- Compare with `errors.Is` / `errors.As`, never string matching (~159 call sites)
- Sentinel errors via `errors.New` at package level (~141)
- Error strings frequently become dialog text the user reads, so they are written for
  a human — this is why `ST1005` is disabled in `.golangci.yml`
- Startup and fatal paths report to stderr with the `f4: ` prefix:
  `fmt.Fprintf(os.Stderr, "f4: cannot create %q: %v\n", path, err)`
- `errcheck` and `gosec` run in CI: deliberately ignored errors are written as
  `_ = f()`, not left bare

## Control Flow

- Prefer flat, readable control flow over deeply nested conditionals. Use guard
  clauses, early `return`/`continue`, small named helper methods, or explicit
  classification logic when they make the code easier to follow. Handle edge cases
  and irrelevant branches early so the main path stays visible.

## Logging

- No logging framework. Diagnostics go through the `VTUI_DEBUG` environment
  variable, which `vtui` writes to `<profile>/logs/debug.log`
  (`cmd/f4/debug_log.go`); `--debug` / `--log=1` set it up
- User-facing failures go to stderr with the `f4: ` prefix
- Do not add a logging dependency; do not log to stdout — it is the rendered UI

## Testing

- Standard library `testing` only, no testify (552 files import `"testing"`)
- Helpers call `t.Helper()` as the first statement (~398 call sites)
- Table-driven tests with `for _, tt := range` in ~94 files
- Tests are the review mechanism for this AI-only codebase: a change lands with a
  test, a bug fix lands with a regression test
- Tests are mostly sequential — `t.Parallel()` is the exception, not the default,
  because much of `cmd/f4` shares global terminal state
- Use the system Go build cache (`go env GOCACHE`); never redirect `GOCACHE`

## Build & Portability

- `CGO_ENABLED=0` is non-negotiable: the single static binary depends on it.
  FFI goes through `purego` / `ffibridge`
- Every change must keep the full cross-platform matrix building, including the
  exotic targets (mips, riscv64, loong64, ppc64, Illumos, Solaris)
- New dependencies are weighed against the ~110 MB binary
- Subsystem changes update the matching document in `docs/`
