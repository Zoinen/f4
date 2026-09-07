# vtui Multi-Language Bindings

`vtui` is a desktop-class terminal and graphical UI framework designed with a multi-language architecture. The core UI logic, layout engine, and renderers run in Go, while applications can be written in **Python**, **Node.js / TypeScript**, **C**, **C++**, or **WebAssembly**.

---

## Supported Languages & Guides

| Language / Ecosystem | Binding Model | Documentation |
|---|---|---|
| **Python** | Immediate-mode facade (`asyncio` ready) | [bindings/python/README.md](python/README.md) |
| **Node.js / TypeScript** | Immediate-mode & Typed definitions (`esbuild` style) | [bindings/node/README.md](node/README.md) |
| **PHP** | Immediate-mode facade via standard streams | [bindings/php/README.md](php/README.md) |
| **Lua** | Immediate-mode facade (Lua 5.1+ / LuaJIT) | [bindings/lua/README.md](lua/README.md) |
| **C** | Lightweight 6-function C ABI & immediate facade | [bindings/c/README.md](c/README.md) |
| **C++** | Modern C++17 RAII wrapper (`vtui.hpp`) | [bindings/cpp/README.md](cpp/README.md) |
| **WebAssembly (WASM)** | Universal `wasip1` module | [cmd/vtui-wasm](../cmd/vtui-wasm) |

---

## "Hello vtui" Across All Languages

Every language binding shares the exact same mental model, widget vocabulary, and layout engine:

### Python
```python
import vtui

def ui(u):
    with u.dialog(" Hello vtui ", w=40):
        name = u.edit("&Name:", "Type here...")
        if u.button("&Ok"):
            u.message(" Result ", f"You typed:\n{name}")

vtui.run(ui)
```

### Node.js / TypeScript
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
### PHP
```php
<?php
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
### Lua
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

### C++ (C++17)
```cpp
#include <vtui.hpp>

int main() {
    return vtui::run([](vtui::Ui& u) {
        auto d = u.dialog(" Hello vtui ", {.w = 40});
        auto name = u.edit("&Name:", "Type here...");
        if (u.button("&Ok")) {
            u.message(" Result ", "You typed:\n" + name);
        }
    });
}
```

### C (C11)
```c
#include <vtui.h>

static void ui(vtui_ui *u) {
    static char name[128] = "Type here...";
    vtui_dialog(u, " Hello vtui ", 40);
      vtui_edit(u, "&Name:", name, sizeof name);
      if (vtui_button(u, "&Ok"))
          vtui_message(u, " Result ", name);
    vtui_end(u);
}

int main(void) {
    return vtui_run(ui);
}
```

---

## Building All C/C++ Demos Together (from `bindings/`)

```bash
mkdir -p build && cd build
cmake ..
cmake --build .
./c/hello_c
./cpp/hello_cpp
```

---

## Architecture Highlights

1. **Zero Native Compiler Dependencies (Python & Node):**
   - Packaged following the `esbuild` distribution model. The pre-built `vtui-host` binary runs as a child process and communicates via bidirectional JSON Lines wire protocol over a private socket/pipe.
2. **GPU & Graphical Backends Out of the Box:**
   - Run any app with hardware-accelerated rendering by setting `VTUI_BACKEND=gogpu` or `VTUI_BACKEND=x11` / `wayland`.
3. **Session Recording & Deterministic Testing:**
   - Set `VTUI_RECORD=session.jsonl` to record UI sessions.
   - Replay and verify with `vtui-replay session.jsonl`.
   - Export to asciicast with `vtui-cast session.jsonl -o demo.cast`.
