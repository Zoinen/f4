# vtui for PHP

Desktop-class, declarative TUI and GUI framework for PHP.

---

## Features

- **No Native PHP Extensions Required:** Communicates with the `vtui-host` engine using standard PHP streams and Unix domain sockets (`stream_socket_pair`).
- **Immediate-Mode Facade:** Clean, declarative functional UI code with automatic layout calculation.
- **Full Support for GPU / Graphical Backends:** Run applications with hardware acceleration by setting `VTUI_BACKEND=gogpu` or `x11` / `wayland`.

---

## Quick Start

### `hello.php`

```php
<?php

require_once __DIR__ . '/src/Vtui.php';

use function Vtui\run;
use Vtui\Ui;

run(function(Ui $u) {
    $u->dialog(" Hello vtui ", 40, function() use ($u) {
        $name = $u->edit("&Name:", "Type here...");
        if ($u->button("&Ok")) {
            $u->message(" Result ", "You typed:\n" . $name);
        }
    });
});
```

Run directly with:
```bash
php hello.php
```

---

## API Reference

### `Vtui\run(callable $uiFunc, array $options = [])`
Starts the UI event loop and invokes `$uiFunc(Ui $u)` on each frame tick.

### `$u->dialog(string $title, int $w = 40, ?callable $callback = null)`
Declares the root dialog window and nests children within its layout container.

### `$u->edit(string $label, string $default = "", ?string $id = null): string`
Creates an input field with an associated buddy Label. Returns the entered text value.

### `$u->button(string $text, ?string $id = null): bool`
Declares an action button. Returns `true` if clicked during the event cycle.

### `$u->checkbox(string $text, bool $default = false, ?string $id = null): bool`
Declares a checkbox. Returns its boolean state.

### `$u->message(string $title, string $text, array $buttons = ['&Ok'])`
Opens a modal popup message dialog.

---

## Environment Variables

| Variable | Values | Description |
|---|---|---|
| `VTUI_BACKEND` | `ansi`, `gogpu`, `x11`, `wayland`, `ebiten` | Select rendering backend |
| `VTUI_HOST_BIN` | `/path/to/vtui-host` | Override path to `vtui-host` binary |
| `VTUI_TRACE` | `1` | Log protocol JSON Lines to `stderr` |
| `VTUI_RECORD` | `session.jsonl` | Record session events to a file |
