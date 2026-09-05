# vtui: Declarative UI and Multi-Language Bindings Specification

**Status:** Project Specification, Version 1.0
**Audience:** Implementer or AI agent completing roadmap milestones.

## Table of Contents

| Section | Topic |
|---|---|
| 0–5 | Working Rules, Glossary, Goals, Invariants, Architecture Overview |
| Part A (A1–A9) | Go Kernel Enhancements |
| Part B (6, 7) | Vocabulary, `.vui` Document Format, and Deterministic Layout Engine |
| Part C (8, 9) | Wire Protocol and Three Transports |
| Part D (10, 11) | Multi-Language Bindings (Python, Node/TS, C, C++, WASM) and Tooling |
| 12–15 | Milestones and Status Board, Risk Register, Open Questions, References |

---

## 0. How to Use This Document

1. **Follow milestones in Section 12 strictly in order.** A milestone is complete only when all readiness criteria pass.
2. **Do not invent names.** All widget types, properties, signals, and palette roles must strictly originate from `vocabulary.json` (Section 6.1).
3. **`vocabulary.json` is the Single Source of Truth.** Schemas, property access tables, TypeScript definitions, Python bindings, C headers, and markdown docs are generated from it.
4. **Never pass raw callbacks across FFI boundaries.** (See Invariant 4.1).
5. **No floats in layout coordinates.** All layout arithmetic is integer-based with strict deterministic rounding rules (Section 7.4).
6. **`go test ./...` must remain green at every step.**
7. **Backward compatibility is mandatory.** Existing Go vtui code (`NewDialog` + `AddItem`) continues working without changes.

---

## 1. Glossary

| Term | Meaning |
|---|---|
| **Kernel** | The `github.com/unxed/vtui` Go package. The single home of UI widgets and rendering logic. |
| **Host Application** | User program in Python / Node / C / C++ controlling the UI. |
| **Host Process** | Standalone `vtui-host` binary running the kernel over the JSON Lines wire protocol. |
| **Vocabulary** | `vocabulary.json` — machine-readable specification of all widgets, properties, and signals. |
| **`.vui`** | Declarative UI layout document in JSON format. |
| **Protocol** | `vtui wire protocol` — bidirectional stream of JSON Lines between host application and kernel. |
| **Patch** | Protocol message applying atomic mutations to a mounted widget tree. |
| **Transport** | Delivery channel: child process (pipes/socket), shared library (C ABI), or WASM module. |
| **Thin Layer** | 1:1 mapping with the wire protocol. |
| **Idiomatic Layer** | Immediate-mode facade (Dear ImGui style) built on top of the thin layer. |
| **Layout** | Deterministic integer geometry calculation from container constraints. |
| **GrowMode** | Turbo Vision style anchor resizing mechanism in vtui. |
| **Cell** | Single terminal character grid unit. All dimensions are integer cells. |

---

## 2. Goals

| # | Goal | Verification |
|---|---|---|
| G1 | UI described as data, not code | Readme dialog assembled from `.vui` without a single line of Go |
| G2 | Engine computes coordinates, not humans | No manual coordinate expressions like `dlg.X1 + 2` |
| G3 | Consistent mental model across 4 languages | "Hello vtui" in Python, JS/TS, C++, and C are structurally identical |
| G4 | Zero compiler requirement for end users | `pip install vtui` and `npm i vtui` work out of the box on clean systems |
| G5 | Single source of truth | Adding a property requires editing only `vocabulary.json` + `go generate` |
| G6 | GPU/X11/Wayland backends accessible from any language | `VTUI_BACKEND=gogpu python app.py` works seamlessly |
| G7 | Deterministic recording and replay | Session recordings replay bit-identically across platforms |

---

## 3. Non-Goals (Consciously Deferred)

| Deferred | Rationale |
|---|---|
| QML-style runtime expression engine (`width: parent.width - 4`) | Requires interpreter and dependency graph; Section 7 layout solves 95% of use cases. |
| Visual UI designer | Will be built on top after `.vui` format stabilization. |
| CSS-like cascading style sheets | Named palette roles (Section 6.7) solve styling cleanly without selector parsing. |
| Animations and transitions | Zero-allocation render loop policy takes precedence. |
| Binary protocol framing | JSON Lines traffic is measured in kilobytes; JSON readability simplifies debugging. |
| Custom host-defined widget types | Section 8.4 `Canvas` provides high-performance escape hatch. |

---

## 4. Invariants

### 4.1. Boundary is an Event Stream, Not a Call Graph
The Go kernel never invokes callbacks in foreign runtimes. Events are queued and drained asynchronously.

### 4.2. Kernel is the Single Owner of UI State
The host does not duplicate the widget tree. Cursor positions, focus, scroll, and text buffers live in Go.

### 4.3. Kernel Owns Terminal I/O
While a session is active, terminal stdin/stdout belongs to vtui. Wrappers redirect application stdout/stderr.

### 4.4. Terminal State is Always Restored
Panics, unhandled exceptions, child SIGKILL, or pipe closures trigger guaranteed `Shutdown()` terminal restoration.

### 4.5. Deterministic Integer Layout
The same `.vui` document and terminal dimensions produce bit-identical cell coordinates across all architectures.

### 4.6. Strict Additive Compatibility
All core changes are additive. No existing public signatures are broken.

---

## 5. Architecture: Five Layers

```
  Host Application         Python / Node / C / C++
        ↓
  Idiomatic Layer          Immediate-mode facade generated from vocabulary
        ↓
  Thin Layer               1:1 mapping with protocol operations
        ↓
  Protocol                 JSON Lines: patches down, events up
        ↓
  Transport                Child Process | C-Shared Library | WebAssembly (wasip1)
        ↓
  Kernel (Go)              .vui Loader + Layout Engine + Widgets + Rendering
```

---

# 12. Milestones and Execution Status

### Current Status Board

| Milestone / Task | Description | Status |
|---|---|---|
| **M0** | `vocabulary.json`, `vocabulary.schema.json`, `cmd/vtui-gen`, `docs/widgets.md` | **Complete** |
| **M1 / A1** | Stable string identifiers (`SetID`, `ID`), auto-ID generation, `Lookup` | **Complete** |
| **M1 / A2** | `PropValue`, `PropertyAccess` interface and code-generated `properties_gen.go` | **Complete** |
| **M1 / A3** | Type factory `NewByType` and registry `RegisterType` | **Complete** |
| **M1 / A4** | Non-blocking event loop step `Step(timeout)` and `PostEvent` | **Complete** |
| **M1 / A5** | I/O decoupling `SessionConfig`, `SetOutput`, and `Resize(w, h)` | **Complete** |
| **M1 / A6** | Unified outbound event sink `UIEvent` and `SetEventSink` | **Complete** |
| **M1 / A7** | Virtualized `RowProvider` for `ScrollView`/`ListBox`/`Table` | **Complete** |
| **M1 / A8** | Guaranteed terminal restoration contract `Shutdown` | **Complete** |
| **M2** | Layout Engine (Section 7), `LoadDialog`, `vuic`, `vtui-lint`, golden tests | **Complete** |
| **M3** | JSON Lines protocol and `vtui-host` process | **Complete** |
| **M4** | Python bindings (`vtui` package, immediate-mode facade, asyncio) | **Complete** |
| **M5** | Node.js and TypeScript bindings (`vtui` package, `vtui.d.ts`) | **Complete** |
| **M6** | C, C++, and WebAssembly WASI bindings (`vtui.h`, `vtui.hpp`, `cmd/vtui-wasm`) | **Complete** |
| **M7** | Session recording, deterministic replay (`vtui-replay`), and cast export (`vtui-cast`) | **Complete** |
| **Acceptance** | Automated multi-language integration tests (`bindings_integration_test.go`) | **Complete** |

---

## M0 — Vocabulary [COMPLETE]
- `vocabulary.json` and `vocabulary.schema.json` created.
- `cmd/vtui-gen` tool generates `docs/widgets.md` and code bindings.

## M1 — Kernel Enhancements [COMPLETE]
- Tasks A1–A8 implemented in core Go packages with full backward compatibility.

## M2 — Layout Engine & .vui Document Format [COMPLETE]
- Deterministic 1D integer distribution math (Section 7.4) and container size hints.
- `.vui` loader (`LoadDialog`, `LoadDialogFile`) with autoSize, centering, and buddy links.
- Schema `vui.schema.json`, compiler `cmd/vuic`, validator `cmd/vtui-lint`, and hot reload (`VTUI_WATCH=1`).

## M3 — Wire Protocol & vtui-host Process [COMPLETE]
- JSON Lines protocol session (`ProtocolSession`) supporting down/up operations and patch mutations.
- Standalone `cmd/vtui-host` executable with `--protocol-fd`, `--socket`, `--backend`, and `VTUI_TRACE=1`.

## M4 — Python Bindings [COMPLETE]
- Thin `Session` and immediate-mode `Ui` facade in `bindings/python/vtui`.
- `asyncio` integration (`run_async`), examples (`hello.py`, `async_demo.py`), and test suite.

## M5 — Node.js & TypeScript Bindings [COMPLETE]
- Package `bindings/node` without native build dependencies.
- TypeScript definitions `vtui.d.ts` generated from vocabulary.
- Immediate-mode `Ui` facade, examples (`hello.js`, `hello.ts`), and unit tests.

## M6 — C, C++, and WASM Bindings [COMPLETE]
- C ABI shared library (`bindings/c/cabi/main.go`) exporting 6 core functions.
- C header `vtui.h`, C++ header-only wrapper `vtui.hpp`, and CMake build files.
- WebAssembly kernel in `cmd/vtui-wasm`.

## M7 — Tooling [COMPLETE]
- Session recording via `VTUI_RECORD=session.jsonl`.
- Deterministic playback and response verification tool `cmd/vtui-replay`.
- Asciicast v2 exporter `cmd/vtui-cast`.

---

## Final Acceptance

The test suite (`go test ./...`) automatically validates kernel features, layout math, golden coordinates, protocol lifecycle, and multi-language bindings integration.
