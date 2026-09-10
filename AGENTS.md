# Local workspace instructions

## Mandatory portable-build policy

- Before changing, running, debugging, or reviewing any build, packaging,
  release, deployment, or CI workflow, read
  `docs/PORTABLE_BUILD_POLICY.md` completely.
- The platform contracts and verification gates in that document are required;
  do not replace them with a directory bundle on Linux or Windows, and do not
  replace the signed application bundle with runtime extraction on macOS.

## Build and launch rules

- After making changes in the main checkout, normally replace the running
  canonical build: terminate the process tree for
  `D:\Code\f4\f4.exe`, build the new binary, and launch its Qt frontend
  with `--gui=qt`.
- When working in a Git worktree, build and launch the worktree's own binary
  inside that worktree. Its filename must include a sanitized worktree or
  branch identifier (for example, `f4-<worktree-name>.exe`) so binaries from
  multiple worktrees can run in parallel. Do not overwrite
  `D:\Code\f4\f4.exe` from a worktree.
- When restarting a worktree build, terminate only the process tree whose
  executable path resolves to that worktree-specific binary. Never terminate
  a same-named or canonical F4 process belonging to another worktree or
  checkout.
- This workspace develops the Qt frontend. Always launch F4 directly with
  `--gui=qt`, unless the user explicitly requests another frontend. Do not
  launch it through GoGPU, Windows Terminal, or another wrapper or terminal
  host. Use the current worktree's freshly built Qt host.
- If a build fails, keep the last working application instance alive, fix the
  build, and repeat the cycle before handing the task back to the user.

## Worktree identity in the UI

- The top header line must show the current Git worktree branch name centered
  horizontally. Resolve it from the active checkout at runtime; do not
  hard-code a branch name or reuse a value from another worktree.
- If the current checkout is not a worktree with a distinct branch identity,
  preserve the existing header content and centering behavior.

## Mandatory static physical-pixel alignment

- Every non-animated visual item must resolve to the physical pixel grid at
  rest. Validate scene-space positions, layout edges, dimensions, and effective
  ancestor translations with the actual window DPR; an integer logical
  coordinate is not sufficient. Treat 175% (`devicePixelRatio == 1.75`) as a
  mandatory fractional-scale case.
- Snap the result of layout arithmetic, including centering, margins, spacing,
  and container placement. Snapping only a leaf item's local `x`/`y` is not
  sufficient when a parent contributes a fractional scene transform.
- Native-rendered text and raster images must never inherit a fractional static
  translation or scale. When exact mathematical centering falls between two
  physical pixels, choose a neighboring whole physical pixel instead.
- Animation frames are the only exception. Animation start, end, and every
  non-animating/resting state must return to the physical pixel grid.
- Every new or modified static UI layout must have a regression check at 175%
  scale that maps the affected items into scene coordinates and verifies whole
  physical-pixel positions. Include rendered-image verification when text,
  icons, one-pixel strokes, or clipping could regress without a geometry change.
- A pixel-grid check of a wrapper (`Rectangle`, `RowLayout`, `ColumnLayout`,
  control background, and so on) does **not** cover its visual descendants.
  Give every affected static `Text`, `TextInput`, `TextEdit`, `IconLabel`, and
  raster-image leaf a stable `objectName`, then test each leaf's scene-space
  origin after the window is shown and the layout has settled. For a compound
  control, enumerate every line/style, including small captions and secondary
  labels; wrapper-only coverage is forbidden.
- Treat automatic centering by `RowLayout`/`ColumnLayout` as unsafe at a
  fractional DPR. Even when both extents are whole physical pixels, an odd
  physical-pixel difference produces a half-pixel centering offset (for
  example, centering 37 px inside 74 px yields 18.5 px). Choose compatible
  extents or apply a scene-space pixel correction to the shared visual group
  after layout; never accept the layout's implicit centering unverified.
- `snapPx(localValue)` proves only that one local value is rounded. It does not
  prove final alignment when any ancestor, anchor, implicit-size calculation,
  layout distribution, transform, or window offset is fractional. Tests and
  corrective bindings must map the leaf through its complete ancestor chain to
  `QQuickWindow::contentItem` and round that final scene coordinate.
- Static native text and raster icons must map local unit vectors to scene unit
  vectors (no scale, rotation, shear, or layer resampling) as well as land on
  whole physical-pixel origins. A 175% regression must assert both translation
  and effective transform for the actual leaves and grab the rendered window
  when the change affects text or icon sharpness. Inspect the smallest/secondary
  text in that capture; checking only a large bold label is insufficient.
- When a pixel-grid regression is found, first add or extend a test that fails
  on the offending leaf coordinate, record the measured physical coordinate
  that caused it, and only then apply the fix. A test that would have passed the
  broken hierarchy is not an acceptable regression test.


# AGENTS.md

> Structural map of the repository for AI agents and new contributors. Keep it
> factual — describe only what exists. Update it when the structure changes.

## Project Overview

`f4` is a cross-platform TUI file manager written entirely in Go that reproduces the
features, UX and internal structures of `far2l` / Far Manager. It ships as a single
static binary and runs either in a terminal or as a standalone graphical window.

## Tech Stack

- **Programming language:** Go 1.26.6, `CGO_ENABLED=0`
- **Framework:** none — custom TUI; UI and input come from the external `vtui` and
  `vtinput` libraries
- **Database:** none for the application; `plugins/sqlite` browses user SQLite files
- **Lint:** golangci-lint v2 (staticcheck, errcheck, ineffassign, unused, gosec)

## Project Structure

```
cmd/f4/          # the composition root: main.go, four module-wide auditors,
                 # and the Windows .syso resources the linker takes from here
internal/        # everything the application is, in layers
  app/           #   layer 4: the action table, the event loop, the bootstrap;
                 #   the only package allowed to import every other one
  panel/         #   layer 3: the panels frame and the file panel
  editor/        #   layer 3: the editor view
  viewer/        #   layer 3: the viewer
  terminal/      #   layer 3: the terminal view, PTY sessions, ConPTY
  cmdline/       #   layer 3: the command line and apply-command
  dialog/        #   layer 3: dialogs and help
  settings/      #   layer 3: settings catalog, transactions and Settings Center
  media/         #   layer 3: images, audio, video
  nativeui/      #   layer 3: typed Qt scenes and incremental presentation adapter
  macro/         #   layer 3: the macro engine
  plughost/      #   layer 2: the plugin host and its registries
  gui/           #   layer 2: the GUI backends
  fileops/       #   layer 1: file operations and the operation queue
  update/        #   layer 1: the updater
  fusefs/        #   layer 1: FUSE mounting
  textlayout/    #   layer 1: text layout and wrapping
  vtvibe/        #   layer 1: the vtvibe session/provider layer
                 #   layer 0 leaves, imported by anything above them:
  action/        #     the action registry
  appcmd/        #     frame command constants
  colorer/       #     the colour scheme f4 installs for colorer4go
  config/        #     configuration and the profile directory
  history/       #     command, folder and view/edit history
  i18n/          #     the message catalogue and the .lng files
  ini/           #     the INI parser
  keymap/        #     key names, remapping and the hotkey manager
  luaplug/       #     the Lua plugin engine
  netproxy/      #     network proxy
  navtrace/      #     navigation trace correlation
  mediatiming/   #     media timing contexts and events
  winshell/      #     Windows shell property dialogs
  numeric/       #     numeric helpers
  piecetable/    #     the piece table backing the editor
  semantic/      #     the GUI semantic protocol's shared fields
  sheet/         #     spreadsheet mode
  sysinfo/       #     drives, CPU, memory
  textsearch/    #     text search
  theme/         #     colours, styles, file highlighting
  toast/         #     transient notifications
  ttyx/          #     tty extensions
  unpack/        #     archive extraction
  wincon/        #     Windows console
  hideconsole/   #     a vendored fork, console hiding on Windows
  testutil/      #   test scaffolding shared across packages; _test.go use only
  paneltest/     #   the same, for helpers that need a panels frame
vfs/             # filesystem abstraction used by every panel and plugin
  hostfs/        #   host filesystem access
  hostmode/      #   host console mode
  hostpath/      #   path translation
plugring/        # community catalogue of installable plugins: data, not a
                 # package; a contributor adds an entry here and opens a PR
plugins/         # one package per plugin: archive, cloudfox, netfox, mediainfo,
                 # envman, ios, android, sqlite, visren, id3editor, chroma
                 # dummy_internal / dummy_rpc / dummy_lua are transport fixtures
sdk/             # plugin API: f4plugin, f4rpc, f4settings, lua, extui
tools/           # developer tooling, incl. the ttytest terminal harness
docs/            # 48 subsystem documents — read the relevant one before editing
packaging/       # distribution packaging
artifacts/       # build artifacts
.ai-factory/     # AI Factory context: config, description, rules, plans
```

## Key Entry Points

| File | Purpose |
| --- | --- |
| `cmd/f4/main.go` | The entry point: one call to `app.Main` |
| `internal/app/bootstrap.go` | CLI flags, startup mode selection, and the wiring of every subsystem's seam |
| `internal/app/api.go` | `coreAPI`, the host surface plugins are given |
| `internal/app/actions_table.go`, `internal/action/registry.go` | Action definitions and dispatch |
| `embedded.go` | Assets embedded into the binary |
| `go.mod` | Module `github.com/unxed/f4`, Go 1.26.6, dependency set |
| `f4.example.ini` | Reference configuration file |
| `highlight.ini` | Syntax highlighting configuration |
| `.golangci.yml`, `.golangci-strict.yml` | Lint configuration |
| `.github/workflows/build.yml` | CI: cross-platform build matrix, tests, releases |

## Documentation

| Document | Path | Description |
| --- | --- | --- |
| README | `README.md` | Project overview, downloads, backends, philosophy |
| Subsystem docs | `docs/*.md` | 48 documents: VFS, PLUGINS, MACROS, KEYMAP, TERMINAL, CONPTY, WINCON, UX_GUIDELINES and others |
| Issue reviews | `docs/ISSUES/` | Per-issue solution reviews |
| Spreadsheet | `docs/SPREADSHEET.md` | Spreadsheet mode specification |

## AI Context Files

| File | Purpose |
| --- | --- |
| `AGENTS.md` | This structural map of the repository |
| `.ai-factory/DESCRIPTION.md` | Project specification: stack, features, architecture notes |
| `.ai-factory/ARCHITECTURE.md` | Architecture pattern, boundaries and dependency rules |
| `.ai-factory/rules/base.md` | Detected code conventions: naming, errors, logging, tests |
| `.ai-factory/config.yaml` | AI Factory configuration: paths, language, git workflow |
| `.mcp.json` | MCP servers for this project: the gopls Go language server |

## Agent Rules

### Code navigation (gopls MCP)

- The MCP server is the Go language server itself: `.mcp.json` runs `gopls mcp`.
  Nothing is indexed into the repository — gopls reuses the same build cache as
  `go build`.
- **It is optional, so check before you reach for it:** `command -v gopls`. When
  it is absent the server never starts and its tools are simply not offered —
  fall back to `grep`, `go doc` and `go list`, and say that navigation ran
  without it. Do not install it as a side effect of another task; the one-time
  setup is `go install golang.org/x/tools/gopls@latest` with
  `$(go env GOPATH)/bin` on PATH.
- Answers come from `go/types`, so they are the compiler's view of the code
  rather than a text match: a symbol resolves inside its own package even where
  the name repeats, and files behind another platform's build tag still resolve.
- Use it instead of `grep` for symbol questions. The tree is 69 packages and a
  symbol's package is not always the one its name suggests, so grep over the
  whole module is both slow and imprecise:
  - `go_search` — find a symbol by name across the workspace
  - `go_symbol_references` — every reference to one symbol
  - `go_package_api` — the exported surface of a package
  - `go_file_context` — what a file declares and what it depends on
  - `go_diagnostics` — build and vet errors for the files you just changed
- The server follows edits on its own; there is nothing to rebuild after a rebase.
- Grep stays the right tool for text that is not a symbol: comments, error
  strings, build tags, config keys.

### Where new code goes

- Put a new file in the package that owns its subject. If no package owns it,
  create one — do not widen a neighbouring package because it is close enough,
  and never park the file in `internal/app`.
- `internal/app` is the composition root: it wires packages together and does not
  implement features. Code that lands there for lack of a better place is code
  whose owner was not decided.
- Do not add to `cmd/f4`. It holds `main.go`, the wiring tests, the module-wide
  auditors and the Windows `.syso` files, and nothing else.
- Inside a package, name files `<topic>.go` and `<topic>_<aspect>.go`, where the
  prefix is the topic inside the package, not the package name — `panel/frame.go`,
  never `panel/panel_frame.go`. Platform suffixes go on the end:
  `frame_dragdrop_windows.go`.
- Need something from a higher layer? Declare an interface in your package and
  let the caller supply the implementation. Never import upward, and never reach
  across a boundary through a shared mutable global.
- Logic belongs here but the type belongs elsewhere? Write a function taking the
  type, not a method — a method would drag the whole file into the type's
  package.
- `cmd/f4/architecture_test.go` enforces the layer rules. If a change needs an
  exemption there, the architecture document is what changes first, not the test.
- The full rules, with the reasoning, are in `.ai-factory/ARCHITECTURE.md`.

### Go build cache

- Use the system Go build cache reported by `go env GOCACHE` for all Go builds and tests.
- Do not redirect `GOCACHE` to `/tmp`, the repository, or another task-local directory unless the user explicitly asks for it.
- If the system cache is unavailable or not writable, report that constraint instead of silently creating a substitute cache.

### Shell commands

- Run shell commands one step at a time instead of chaining them, so a failing step is visible.
  - Wrong: `git checkout main && git pull`
  - Right: first `git checkout main`, then `git pull origin main`

### Portability

- `CGO_ENABLED=0` must stay: the single static binary depends on it. FFI goes through `purego` / `ffibridge`.
- Platform differences belong in build-tag files (`*_windows.go`, `*_unix.go`), not runtime branching.
- Changes must keep the full CI matrix building, exotic targets included.

### Tests

- This is an AI-only codebase; the test suite is the review mechanism. New behaviour lands with a test, a bug fix lands with a regression test.
