# Plugin system overhaul: plan and rationale

This document exists so the work can be resumed from the repository alone. It
records what is being built, which decisions were made and why, and where the
work currently stands.

## Why

f4 exists partly because plugins in Far3, far2l and far2m are painful:

- they are not portable (the Lua plugin interfaces of Far3 and far2m are not
  fully compatible, and the DLLs are bound to the Windows API);
- binary plugins on Linux are bound to a libc version, which turns
  distribution into a mess, against a single f4 binary;
- far2l has no Lua at all;
- writing C or C++ is a high barrier to entry.

Nobody writes f4 plugins yet either. The likely reasons: f4 already ships with
batteries included, the barrier to entry is high and there is no getting
started guide, portability is unsolved (write in Lua and the target system may
not have Lua), and the API differs from Far3 so porting is a chore.

## The architecture

**One protocol, three transports.**

f4 already had an out-of-process plugin protocol: F4-RPC, MessagePack over
stdin/stdout, with method names like `Plugin.Init`, `VFS.ReadDir` and
`Host.Log`. Rather than growing a second and third plugin API for embedded
Lua and embedded wasm, all three transports carry that same protocol:

| transport | where the plugin runs | why it exists |
| --- | --- | --- |
| subprocess | its own process | any language, full OS access |
| embedded Lua | in f4, on gopher-lua | zero install, ships as one file |
| embedded wasm | in f4, on wazero | zero install, real sandbox, any source language |

The wasm guest is a WASI command, not a module of exported functions: it reads
F4-RPC from stdin and writes it to stdout, exactly as a subprocess plugin does.
So the same plugin source builds either to a native binary or to a `.wasm`
with no changes, and the transport costs a pair of pipes rather than a third
plugin ABI.

The consequences are the point: one SDK surface, one set of documentation, one
set of host methods, and a plugin that moves between transports without being
rewritten. `PluginTransport` in `plughost.go` is the whole seam, a single
`Call(method, params, result)`; `newHostMethods` is the host side, shared by
every transport.

An earlier version of `PLUGINS.md` argued against embedded interpreters on
three grounds. They are answered rather than ignored: binary bloat is handled
with build tags, the sandbox's inability to reach native APIs is handled by the
FFI bridge, and language lock-in does not apply because the subprocess
transport never went away.

## Decisions and their rationale

1. **FFI is projected into the sandbox instead of wrapping host APIs.** Writing
   a wrapper for every Windows API function is not a plan. The plugin describes
   the ABI it wants and the broker performs the call. See `FFI.md`.

2. **A signature mini-language, not a C declaration parser.** `i64(str)` rather
   than `size_t strlen(const char *)`. A cdef-style front end can be layered on
   later without disturbing anything beneath it, and it is needed only for
   porting existing LuaJIT code.

3. **The Lua FFI module is named `f4ffi`, not `ffi`.** Ours is signature
   strings and raw addresses; LuaJIT's is `cdef` and `ffi.new`. Taking the name
   would make ported code fail late and confusingly rather than immediately and
   clearly. A future cdef front end can take the name honestly.

4. **The embedded runtime preloads `f4rpc`.** A plugin written against the
   subprocess SDK runs embedded without modification, because `require('f4rpc')`
   finds the preloaded module and never reaches the file that would drag in a
   MessagePack rock.

5. **Values cross the Lua boundary through MessagePack.** It costs a round trip
   but guarantees both transports agree on field naming, which is precisely
   where the older Far plugin APIs drifted apart.

6. **`print` is redirected into the host log, and `os`/`io` stay closed.** An
   in-process plugin writing to stdout would corrupt the screen f4 is drawing.

7. **One goroutine per Lua state, with an inline path for re-entry.** A native
   callback invoked on the runtime's own worker goroutine must not be queued or
   the worker deadlocks against itself.

8. **far2m's Lua API is the compatibility target, before Far3's.** The two
   share an ancestor in luafar, but far2m's is already free of Windows
   specifics. Implementing it gets most of Far3 for free; the remainder is a
   `winapi` shim later.

9. **No cgo, anywhere.** Portability is the reason f4 exists.

10. **The permission model is deliberately last.** This is alpha, and something
    working end to end is worth more than a model that is right the first time.
    Every dangerous operation already funnels through one hook, so adding it
    later is not invasive.

11. **A grant is remembered under the plugin's catalog id, not its path.** The
    id is what PlugRing installs and removes under, so it is the only key for
    which forgetting a removed plugin's answers actually works. A path is also
    the user's home directory written into the stored grants, and it changes
    when the configuration directory moves. A plugin registered by hand has no
    id and falls back to its path, which is the only stable thing it has.

## Distribution

The catalog carries source and wasm, not binaries.

The three transports differ in what has to reach the user. A Lua plugin is
text: portable, readable, nothing to build, identical on every platform. A
wasm plugin is one artifact for all of them, sandboxed and unbound to any
libc. A native plugin is a platform binary, and that is the only one that is a
problem: a distribution maintainer will not mirror third party binaries, a
reviewer cannot audit them, and they need a build per operating system and
architecture. So PlugRing distributes `.lua` and `.wasm`; a native plugin stays
a local affair, registered by the user or installed by the system package
manager.

Almost nothing gives up speed for this. The case that needs speed is served by
wasm, whose overhead is a factor rather than an order of magnitude. The case
that needs native APIs is served by the FFI bridge, which lets a plugin remain
portable text and call the target system's own libraries instead of carrying
its own build of them. What is left over is small enough to be worth the
trade.

The catalog dialog groups by category and greys out what this build cannot
run, so the constraint is visible before the install rather than discovered
after it. `plugring_meta.go` holds the vocabulary: categories following
plugring.farmanager.com so that somebody arriving from there recognises the
shelves, and a `runtimes` field so a plugin can say which interpreter it needs.
That last one exists because a plugin using LuaJIT's `cdef` genuinely has
nowhere else to go, and declaring the dependency is better than failing at
load: f4 can then say so before the install rather than after.

Two things in the existing manifest worked against this policy. `setup_cmd`
ran an arbitrary shell command with the user's privileges at install time and
is now ignored outright, with no confirmation offered, because no dialog makes
that acceptable. The `{os}`/`{arch}` placeholders in the download URL exist
only to ship per platform binaries; an entry using them is flagged before the
install and can still be installed on the user's say-so, since the catalog in
the wild predates the rule.

## Status

Done:

- **Step 1: `ffibridge`.** The FFI broker over pureffi. Signature parsing,
  dynamic dispatch through a runtime-built `reflect.FuncOf` type, a memory
  arena, callbacks, raw peek and poke, and a permission hook that is currently
  left open. Degrades to a stub under the `noffi` build tag.
- **Step 2: `luaplug`.** The embedded gopher-lua runtime: the `f4rpc` module,
  the `f4ffi` module, the sandbox, a worker goroutine per state and a deadline
  on every entry into the interpreter.
- **Step 2b: transports.** `PluginTransport` and shared `newHostMethods`;
  `LuaPlugin` mounts a Lua script as an in-process plugin. `plugins/dummy_lua`
  now runs without a system Lua.
- **Step 3: Far-compatible Lua macros.** `Macro{}`, `Keys()`, `akey()`,
  `Area`, `APanel`/`PPanel`, `CmdLine`, `Far`, `mf.*` and `bit.*`, reading
  `Macros/scripts` under the config directory. The dialect is far2m's. Macros
  run off the UI goroutine so that they can ask the UI for panel state without
  deadlocking it; `MacroHost` is the seam that enforces this in one place, and
  is what lets the engine be tested without a terminal. Recorded macros keep
  working and take precedence, as in Far.
  Documented for users in `MACROS.md`.
- **Step 4: embedded wasm on wazero.** The guest is a WASI command over stdio,
  so `startPluginSession` in `plughost.go` now holds everything a transport
  does once it has two byte streams, and the wasm transport is just the
  streams. The guest gets no filesystem, making this the first transport that
  is actually a sandbox. `Plugin.Init` gained a timeout along the way: a valid
  but silent module would otherwise hang startup forever, and so would a
  broken subprocess.
- **Step 4b: FFI over the protocol, as `Host.FFI.*`.** The earlier worry about
  guest offsets not being host addresses turned out to be the wrong frame: the
  broker deals only in integers and strings, so it projects onto the existing
  protocol directly. A wasm guest gets real native calls and real C callbacks
  without ever holding a host pointer. Subprocess plugins are not given these
  methods, having no need of them. Zero-copy over guest linear memory remains
  available as a later optimisation for heavy data rather than a precondition.
- **Step 5: onboarding.** `f4 --new-plugin <name>` writes a working Lua
  plugin, its manifest and a README, and a test loads the generated file
  through `LuaPlugin` and reads a file off the drive it mounts, so the
  template cannot rot unnoticed in front of the audience least able to debug
  it. Lua only, deliberately: a Go template needs a toolchain and a go.mod
  pointing at the SDK, a wasm one needs a target as well, and neither is a
  five minute hello world. `PLUGINS.md`, `LUA.md` and `PLUGRING.md` are
  rewritten for three transports, a submission walkthrough and the
  distribution policy; `LUA.md` settles the FFI naming, with `f4ffi` the Lua
face of the bridge and `Host.FFI.*` the protocol underneath it for guests
  that cannot have a module.
- **Step 6: the permission model (V1).** Manifest permissions with the author's
  own justification text, asked for on first real use (for FFI) or at load time
  (for `unsafe-stdlib`), and remembered persistently in `plugin_permissions.json`
  under the plugin's catalog ID (not its file path, so it survives updates and
  moves). `PermissionStore.Forget` is called when a plugin is uninstalled via
  PlugRing. A global permission management dialog is implemented
  (`actionPluginPermissions` / `btnPerms`) to review and revoke granted permissions.
- **Step 6.1: Enforce the `native` permission.** Enforced the `native` permission
  check inside `RPCPlugin.Init()`. It queries the permission gate before spawning
  any subprocess, gracefully refusing to load the plugin if denied. Headless test
  scenarios are auto-approved to prevent blocking runs.
- **Step 6.2: Complete API Documentation.** `PLUGINS.md` was completely rewritten to include
  comprehensive API documentation, object model comparisons with classic Far and modern editors,
  and a step-by-step PlugRing publication guide. Historical rationale was consolidated.
- **Step 4c: choosing where `Ctrl+.` records to.** Implemented a configuration
  option (`MacroRecordFormat`) with a UI selector in Panel Settings. Overwriting or
  deleting recorded macros clears them from both backends to prevent conflicts.
- **Step 4d: an action registry.** Implemented a global action registry mapping
  stable semantic names (e.g. `File.Copy`, `Panel.Rescan`, `Settings.Panel`) to Go
  functions. Exposed via `Actions.Run()` to Lua macros and `Host.RunAction` to
  external RPC plugins.

Next, in order:

- **Step 7: Far3/far2l API compatibility layer.** Map standard `far2m` and `luafar`
  namespace bindings to allow loading legacy Lua scripts directly without rewriting
  them. All namespacing choices (specifically `Actions` registry and decoupled FFI)
  were designed to make this step unblocked and non-invasive.

## Known issues

- The permission dialog is reachable only from the plugin management dialog,
  which is otherwise about plugins registered by hand. The list it opens is
  global, so it is the right list in a slightly wrong place until PlugRing
  grows an entry point of its own.
- `native` is named in the permission vocabulary but not yet enforced: gating
  a subprocess needs a decision about what happens to a plugin refused at
  launch.
- `unsafe-stdlib` is asked for at load rather than at first use, because
  gopher-lua builds a state's globals when the state is created and there is
  no later moment at which `os` and `io` could appear. A plugin that did not
  declare the permission is not asked and simply does without it.
- `Host.InputBox` and `Host.Menu` block until the UI answers. A plugin that
  calls them from inside a VFS request served on the UI goroutine will
  deadlock. This predates the embedded transports and affects the subprocess
  one identically.
- A runtime that has hit its call deadline was interrupted at an arbitrary
  instruction and should be discarded rather than reused.
- `Keys()` is not synchronous the way Far's is. Keys queued by a macro are
  injected as one batch once the macro returns, so a macro that inspects panel
  state between two `Keys()` calls sees the state from before either of them.
  Making it synchronous means re-entering the input loop from inside the
  interpreter.
- A macro's `condition` is evaluated after the key has already been consumed,
  because evaluating it on the UI goroutine could deadlock. When a condition
  declines, the original key is replayed, which costs one extra trip through
  the input queue.
- `Event{}`, `MenuItem{}` and `CommandLine{}` declarations are accepted and
  ignored, so that scripts using them still contribute their `Macro{}` entries.
