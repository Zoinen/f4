# vtui for Python

Desktop-class, stateful TUI and GUI framework for Python with Far Manager / Turbo Vision ergonomics.

---

## Features

- **Linear Immediate-Mode API:** Declare UI dialogs, forms, and buttons with clean Python context managers.
- **Asyncio Ready:** Native integration with `asyncio.run` and non-blocking event pumps.
- **Zero Native Build Toolchain Required:** Communicates with `vtui-host` via standard library sockets without `cffi` or C compiler requirements.
- **Hardware-Accelerated Backends:** Seamless GPU / X11 / Wayland rendering via `VTUI_BACKEND`.

---

## Quick Start

### 1. Basic Example (`hello.py`)

```python
import vtui

def ui(u):
    with u.dialog(" Hello vtui ", w=40):
        name = u.edit("&Name:", "Type here...")
        if u.button("&Ok"):
            u.message(" Result ", f"You typed:\n{name}")

if __name__ == "__main__":
    vtui.run(ui)
```

### 2. Asyncio Example (`async_demo.py`)

```python
import asyncio
import vtui

def ui(u):
    with u.dialog(" Async Demo ", w=40):
        name = u.edit("&User:", "Alice")
        if u.button("&Submit"):
            u.message(" Welcome ", f"Hello async user: {name}")

async def main():
    await vtui.run_async(ui)

if __name__ == "__main__":
    asyncio.run(main())
```

---

## API Reference

### `u.dialog(title: str, w: int = 40, h: int = 10)`
Context manager declaring the root window/dialog container.

### `u.edit(label: str, default: str = "", id: Optional[str] = None) -> str`
Creates an input field with an automatic hotkey label (`&Name:` binds `Alt+N`). Returns the current text string.

### `u.button(text: str, id: Optional[str] = None) -> bool`
Declares a push button. Returns `True` if clicked in the last event step.

### `u.checkbox(text: str, value: bool = False, id: Optional[str] = None) -> bool`
Declares a checkbox. Returns the current checked boolean state.

### `u.message(title: str, text: str, buttons: Optional[List[str]] = None)`
Displays a modal message box.

---

## Environment Variables

| Variable | Values | Description |
|---|---|---|
| `VTUI_BACKEND` | `ansi`, `gogpu`, `x11`, `wayland`, `ebiten` | Select rendering backend |
| `VTUI_HOST_BIN` | `/path/to/vtui-host` | Override path to `vtui-host` binary |
| `VTUI_TRACE` | `1` | Log protocol JSON Lines to `stderr` |
| `VTUI_RECORD` | `session.jsonl` | Record all session events to a file |
