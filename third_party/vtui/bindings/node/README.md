# vtui for Node.js and TypeScript

Desktop-class, declarative TUI and GUI framework for Node.js and TypeScript.

---

## Features

- **No Native Addons / No node-gyp:** Follows the `esbuild` architecture pattern. Communicates over a lightweight IPC stream to the `vtui-host` engine.
- **Full TypeScript Support:** Includes auto-generated type definitions in `vtui.d.ts`.
- **Immediate-Mode Facade:** Clean, linear functional API with automatic layout.
- **GPU Backends:** Switch between ANSI terminal, GPU, X11, and Wayland via environment variables.

---

## Quick Start

### JavaScript (`hello.js`)

```javascript
const { run } = require("vtui");

run(u => {
  u.dialog(" Hello vtui ", 40, () => {
    const name = u.edit("&Name:", "Type here...");
    if (u.button("&Ok")) {
      u.message(" Result ", `You typed:\n${name}`);
    }
  });
});
```

### TypeScript (`hello.ts`)

```typescript
import { run, Ui } from "vtui";

run((u: Ui) => {
  u.dialog(" Hello vtui ", 40, () => {
    const name = u.edit("&Name:", "Type here...");
    if (u.button("&Ok")) {
      u.message(" Result ", `You typed:\n${name}`);
    }
  });
});
```

---

## API Reference

### `u.dialog(title: string, w?: number, callback?: () => void)`
Defines the root dialog window and nests children within its layout container.

### `u.edit(label: string, defaultValue?: string, id?: string): string`
Creates an Edit input field with an associated buddy Label. Returns the entered text value.

### `u.button(text: string, id?: string): boolean`
Declares an action button. Returns `true` if clicked during the event cycle.

### `u.checkbox(text: string, defaultValue?: boolean, id?: string): boolean`
Declares a checkbox. Returns its boolean state.

### `u.message(title: string, text: string, buttons?: string[])`
Opens a modal popup message dialog.

---

## Environment Variables

| Variable | Values | Description |
|---|---|---|
| `VTUI_BACKEND` | `ansi`, `gogpu`, `x11`, `wayland`, `ebiten` | Select rendering backend |
| `VTUI_HOST_BIN` | `/path/to/vtui-host` | Override path to `vtui-host` binary |
| `VTUI_TRACE` | `1` | Log protocol JSON Lines to `stderr` |
| `VTUI_RECORD` | `session.jsonl` | Record session events to a file |
