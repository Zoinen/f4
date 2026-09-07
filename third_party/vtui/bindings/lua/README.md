# vtui for Lua

Desktop-class, declarative TUI and GUI framework for Lua (supports Lua 5.1+, LuaJIT, 5.2, 5.3, 5.4).

---

## Features

- **Zero External Dependencies:** Built-in embedded JSON codec and native POSIX socketpair via LuaJIT FFI or FIFO streaming on pure PUC-Rio Lua.
- **Immediate-Mode Facade:** Clean, linear functional API with automatic constraint layout calculation.
- **Hardware-Accelerated Backends:** Seamless GPU / X11 / Wayland rendering via `VTUI_BACKEND`.

---

## Quick Start

### `hello.lua`

```lua
local vtui = require("vtui")

vtui.run(function(u)
    u:dialog(" Hello vtui ", 40, function()
        local name = u:edit("&Name:", "Type here...")
        if u:button("&Ok") then
            u:message(" Result ", "You typed:\n" .. name)
        end
    end)
end)
```

Run directly:
```bash
lua hello.lua
# or
luajit hello.lua
```

---

## API Reference

### `vtui.run(ui_func, options)`
Starts the UI event loop and invokes `ui_func(u)` on each frame tick.

### `u:dialog(title, w, callback)`
Declares the root dialog window and nests children within its layout container.

### `u:edit(label, default_val, id): string`
Creates an input field with an associated buddy Label. Returns the entered text value.

### `u:button(text, id): boolean`
Declares an action button. Returns `true` if clicked during the event cycle.

### `u:checkbox(text, default_val, id): boolean`
Declares a checkbox. Returns its boolean state.

### `u:message(title, text, buttons)`
Opens a modal popup message dialog.

---

## Environment Variables

| Variable | Values | Description |
|---|---|---|
| `VTUI_BACKEND` | `ansi`, `gogpu`, `x11`, `wayland`, `ebiten` | Select rendering backend |
| `VTUI_HOST_BIN` | `/path/to/vtui-host` | Override path to `vtui-host` binary |
| `VTUI_TRACE` | `1` | Log protocol JSON Lines to `stderr` |
| `VTUI_RECORD` | `session.jsonl` | Record session events to a file |
