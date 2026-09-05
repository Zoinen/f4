# Architectural Proposals and Modern Best Practices for vtui

This document compiles battle-tested, industry-standard architectural approaches (inspired by Flutter, Qt/QML, React, SolidJS, Language Server Protocol, and Wayland) that enhance `vtui`'s modularity, performance, and multi-language ergonomics.

---

## 1. Fine-Grained Signal Reactivity

### Context
The project already features the `vreactive` package (`Property[T]`, `Computed[T]`, `Effect`, `StateMachine`).

### Proposal
Integrate `vreactive` with generated property access and UI facades:
- Widgets can optionally bind properties directly to signals: `edit.BindText(prop)`.
- Signal mutations batch at the end of each frame tick (Microtask Queue) and update only dirty nodes.
- **Benefits:** Eliminates unnecessary `Redraw()` passes and avoids race conditions when updating UI from concurrent background workers.

---

## 2. Declarative Virtual-Tree Diffing in Immediate-Mode Facades

### Context
Currently, the `Ui` facade in Python and Node.js constructs the initial tree on mount and tracks user interactions.

### Proposal
Add lightweight virtual tree diffing in language client libraries:
- The `ui(u)` function executes on each event step and generates an in-memory virtual tree.
- The facade diffs the previous virtual tree with the new one by `(type, id, key)`.
- The diff produces a minimal JSON Lines patch:
  `[{"kind": "set", "id": "statusLabel", "props": {"text": "Updated"}}]`.
- **Benefits:** Users write completely linear declarative code while only lightweight property deltas cross the IPC boundary.

---

## 3. Runtime Introspection & Protocol Schema Discovery (LSP / OpenAPI Style)

### Context
Dynamic language bindings (Python, Node.js, Ruby, Lua) can interact with different versions of `vtui-host`.

### Proposal
- The `{"op": "describe"}` operation returns the active `vocabulary.json` directly from the running kernel.
- Client libraries can dynamically validate or generate methods in memory without requiring binary recompilation for minor kernel updates.

---

## 4. Shared Memory (SHM) for High-Frequency Canvas Escape-Hatch

### Context
Section 8.4 provides the `Canvas` element for custom pixel/cell rendering (graphs, image viewers, hex editors).

### Proposal
- For local IPC transports (`vtui-host` over Unix Domain Socket or SHM):
  - Canvas cell/pixel buffers write directly to an `mmap` shared memory segment (similar to Wayland SHM / X11 MIT-SHM).
  - The protocol only transmits damage notifications: `{"op": "damage", "rect": [x, y, w, h]}`.
- **Benefits:** 60+ FPS performance for heavy custom canvas widgets with zero memory copying and no JSON serialization overhead.

---

## 5. Hierarchical State Preservation during Hot Reload

### Context
`vui_loader.go` provides `VTUI_WATCH=1` hot reload with state preservation by element ID.

### Proposal
- Extend state identification from flat `id`s to hierarchical structural paths `(ParentID / ChildType / Index)` for unnamed controls:
  - If a control lacks an explicit `id`, its state maps to its tree structural position.
- **Benefits:** Developers can edit `.vui` templates live without losing focus, input text, or scroll offsets, even without explicitly naming every element.

---

## 6. Declarative Session Recordings and Golden Headless Testing

### Context
`vtui-record` and `vtui-replay` capture and replay JSON Lines protocol sessions.

### Proposal
- Use recorded `.jsonl` files as standard UI regression tests:
  - Tests spawn the host, replay event scripts, and verify deterministic screen state within milliseconds in headless mode.
- **Benefits:** 100% end-to-end UI regression testing without browser drivers or physical TTY requirements.
