# f4 Plugin Architecture (F4-RPC)

`f4` plugins speak **F4-RPC**: a small request/response protocol carried in **MessagePack**, with method names like `Plugin.Init`, `VFS.ReadDir` and `Host.Log`.

One protocol, several transports. The same plugin can run in its own process, communicating over `stdin`/`stdout`, or inside `f4` on an embedded interpreter. Nothing above the transport changes, so a plugin moves between them without being rewritten.

## The transports

**Subprocess.** `f4` launches an executable and talks to it over pipes. Write
the plugin in Go, Python, Rust, C++, Node.js or LuaJIT; anything that can read
and write MessagePack on stdio works. The plugin is a native OS process with
unrestricted access to the network, native libraries and the filesystem.

**Embedded Lua.** `f4` runs a `.lua` plugin on a built-in interpreter, with no
system Lua and no rocks to install. The plugin is a single file. `require('f4rpc')`
resolves to a preloaded module, so a script written for the subprocess
transport runs here unmodified.

**Embedded wasm.** `f4` runs a `.wasm` plugin on a built-in WebAssembly
runtime. The guest is a WASI command reading F4-RPC from stdin and writing it
to stdout, exactly as a subprocess plugin does, so the same source builds
either way. It is the only transport that is genuinely a sandbox: the guest
gets no filesystem, only stdio, a clock and a random source. Native calls
reach it through `Host.FFI.*`; see `FFI.md`.
## Getting started

```
f4 --new-plugin mydrive
```

That writes a working plugin: a virtual drive with a couple of files in it,
its manifest, and a README saying what to do next. It is a Lua plugin, so
there is nothing to build and nothing to install; edit `plugin.lua` and
restart f4.

Start reading the generated file at `Plugin.Init`. Everything else in it is
one `f4rpc.register` per method of the protocol described below.

An earlier revision of this document argued that embedded interpreters had been
abandoned for good, on three grounds. Each has an answer now:

1. **Binary bloat.** Build tags keep the interpreters out of builds that do not
   want them.
2. **No access to native APIs.** A sandbox cannot reach the host by design, so
   `f4` projects an FFI broker into it instead: the plugin describes the ABI it
   wants and the host performs the call. See `FFI.md`.
3. **Language lock-in.** The subprocess transport never went away, so no
   language is privileged.

`MessagePack` was chosen over JSON-RPC to keep serialization overhead and
latency low for large `ReadDir` chunks.

## How it works

1. `f4` scans the `plugins/` directory for executable files.
2. It launches each executable via `os/exec`, hooking into its `stdin`, `stdout`, and `stderr`.
3. `stderr` is immediately piped to `f4`'s internal `debug.log` for easy debugging.
4. `f4` sends a `Plugin.Init` request over `stdin`.
5. The plugin replies with its capabilities (e.g., registered Virtual File Systems).
6. When a user interacts with the plugin's VFS, `f4` transparently routes `ReadDir`, `Stat`, `Open`, etc., as RPC calls.

## Building a Plugin (Go SDK)

For Go developers, an SDK is provided in `sdk/f4plugin`. It handles all the multiplexing and binary protocol details, exposing a clean, synchronous interface. Check `plugins/dummy/main.go` for a reference implementation.
