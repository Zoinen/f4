# F4-RPC for Lua Developers

If you are coming from the **Far3** or **far2m** plugin ecosystems, you are likely used to writing Lua plugins that run *in-process*. In those environments, your Lua scripts directly manipulate memory, call internal C/C++ API structs via FFI or bindings, and pause the main application thread while executing.

`f4` uses a radically different architecture: **Out-of-Process RPC**.

This document will help you bridge the gap and understand how to build powerful Lua plugins for `f4`.

## How Lua talks to f4

Instead of calling C functions, your Lua plugin acts as a standalone console application.
1. `f4` launches your `plugin.lua` script as a subprocess.
2. `f4` sends binary **MessagePack** requests to your script's `stdin`.
3. Your script processes the request and writes a MessagePack response to `stdout`.

Because `f4` takes care of all the UI rendering, multi-threading, and caching, your Lua script only needs to focus on the data layer (Virtual File Systems, searching, executing logic).

## Getting Started

To write Lua plugins, you need an environment capable of reading and writing MessagePack.

### 1. Requirements
* Any standard Lua interpreter (`lua` or `luajit`).
* A MessagePack library. We highly recommend François Perrad's lightweight library:
  ```bash
  luarocks install lua-MessagePack
  ```

### 2. The Lua SDK (Standalone vs. Development)
To keep your plugin independent of the `f4` source tree, it should bundle its own copy of the SDK.

1.  **In development:** You can reference the SDK from the `f4` repository using relative paths (e.g., `../../sdk/lua/`).
2.  **In distribution:** Copy `sdk/lua/f4rpc.lua` directly into your plugin's folder.

The `f4rpc.lua` file is self-contained and handles request multiplexing, stdio streaming, and binary mode adjustments for Windows.

### 3. A Basic VFS Plugin
Here is how you initialize a plugin and register a Virtual File System:

```lua
#!/usr/bin/env lua
local f4rpc = require('f4rpc')

-- 1. Create a Host wrapper for outgoing calls
local host = {}
function host.Log(msg) f4rpc.call("Host.Log", msg) end
function host.Message(msg) f4rpc.call("Host.Message", msg) end

-- 2. Register the initialization handler
f4rpc.register("Plugin.Init", function()
    host.Log("Lua plugin starting up...")
    return { Drives = { "My Lua Drive" } }
end)

-- 3. Handle VFS Directory reads
f4rpc.register("VFS.ReadDir", function(req)
    -- req is a table: { Drive = "My Lua Drive", Path = "/" }
    if req.Path == "/" or req.Path == "" then
        return {
            { Name = "hello.txt", Size = 12, IsDir = false },
            { Name = "data", Size = 0, IsDir = true }
        }
    end
    return {}
end)

-- 4. Start the blocking event loop
f4rpc.serve()
```

## Advanced Operations: Callbacks to Host

Because F4-RPC uses full duplex multiplexing, your Lua plugin can call back into `f4` *while* processing a request.

For example, if your plugin handles a `RunProgressTask`, it can send progress updates back to the UI:

```lua
f4rpc.register("Plugin.OnProgressTask", function()
    for i=1, 10 do
        -- Check if user clicked "Cancel" in the f4 UI
        local cancelled = f4rpc.call("Host.IsProgressCancelled")
        if cancelled then break end

        -- Send update to the UI
        f4rpc.call("Host.UpdateProgress", { Msg = "Working...", Percent = i*10 })
        os.execute("sleep 0.5") -- Simulate heavy work
    end
    return nil
end)
```

## A Note on Windows, `stdin`, and Lua

By default, the Windows C runtime translates `\n` to `\r\n` on `stdout`, and stops reading `stdin` if it encounters a `Ctrl+Z` (0x1A) character. This utterly destroys binary protocols like MessagePack.

If you are writing Lua plugins meant to run on Windows, you must ensure `stdin/stdout` are set to `O_BINARY`. The provided `f4rpc.lua` SDK attempts to do this automatically if it detects it is running under **LuaJIT** (using the FFI library to call `_setmode`).

If you are using standard `lua.exe` on Windows, you may need a binary I/O C-extension or a custom launcher executable. On Linux and macOS, this is not an issue.