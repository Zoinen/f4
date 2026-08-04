# f4 PlugRing

A community catalog for f4 plugins, macros and colour schemes. The name is a
nod to Far Manager's PlugRing; the shape is built for a world where the same
plugin has to run on Linux, Windows, macOS and the BSDs without anybody
building it three times.

## What the catalog carries

**Source and WebAssembly. Not binaries.**

A Lua plugin is text: portable, readable, nothing to build, byte for byte the
same everywhere. A wasm plugin is one artifact for every platform, sandboxed
and not bound to any libc version. A native plugin is a binary per operating
system and architecture, which no distribution will mirror, no reviewer can
audit and no user can check.

So a catalog entry's entrypoint is a bare `.lua` file or a bare `.wasm` module,
and nothing else.

This costs almost nothing in speed. Work that needs to be fast goes in wasm.
Work that needs the system's own libraries uses f4's FFI bridge, which lets a
plugin stay portable text and call those libraries directly instead of carrying
its own build of them. See `FFI.md`.

Native plugins are not forbidden, they are simply not distributed here. A user
can register one by path in the plugin manager, and a distribution can package
one the way it packages anything else.

### What gets turned away

- **`setup_cmd`.** It ran an arbitrary shell command with the user's
  privileges at install time, which is worse than shipping a binary because
  nobody reads it. f4 no longer runs it at all: the field is ignored and the
  fact is logged. A plugin that needs a build step does not belong here.
- **`{os}` and `{arch}` in the download URL.** They exist for one purpose:
  serving a different binary per platform.
- **An entrypoint that is not a bare `.lua` or `.wasm` file**, including one
  that names an interpreter, such as `luajit main.lua`.
An entry breaking any of these can still be installed, after a dialog saying
what is wrong: the catalog predates the rule and hiding half of it would be a
worse first impression. The one exception is `setup_cmd`, which is never run,
with or without confirmation.

## Hosting and submission

The catalog lives in the `f4` repository. There is no backend and no account to
create. One plugin is one Markdown file in `plugring/`: YAML frontmatter for
the client, prose below it for people. GitHub renders the prose; a workflow
compiles the frontmatter into the index f4 downloads.

To submit or update a plugin:

1. Write the plugin. `f4 --new-plugin <name>` gives you a working one and a
   `manifest.json` with the fields already filled in.
2. Publish it somewhere f4 can download it from: a release asset, a tag
   archive, anything reachable over HTTPS.
3. Add `plugring/<your-id>.md` to a fork of the f4 repository, with the
   frontmatter below and a description underneath it. Screenshots welcome.
4. Open a pull request.

The policy is open: anything that is not malware, subject to GitHub's terms and
the obvious legal limits. Tests are strongly encouraged and not demanded.

## The format

```yaml
---
id: "notes"
name: "Notes"
version: "1.0.2"
author: "unxed"
description: "A scratchpad drive that keeps your notes one Alt+F1 away."
url: "https://github.com/unxed/f4-notes/releases/download/v1.0.2/notes.zip"
entrypoint: "plugin.lua"
category: "filesystem"
runtimes: ["embedded"]
---
```

`id` is the directory the plugin is installed into and has to be unique in the
catalog. `entrypoint` is the file inside the archive that f4 runs.

### `category`

Where the plugin sits on the shelf. The catalog groups by it, and somebody
arriving from plugring.farmanager.com should recognise the arrangement:

`archive`, `editor`, `filesystem`, `network`, `panel`, `service`, `tools`,
`viewer`, `other`.

Common spellings are accepted, so `file system` and `vfs` both mean
`filesystem`. Leaving it out means `other`.

### `runtimes`

Which interpreters or runtimes the plugin works with, so f4 can say "this needs
something you do not have" before the install rather than after it.

| value | meaning |
| --- | --- |
| `embedded` | f4's own Lua interpreter. Nothing to install. |
| `wasm` | f4's own WebAssembly runtime. Nothing to install. |
| `luajit`, `lua51`, `lua54` | an interpreter the user must already have |
| `native` | a platform binary |

Leave it out and f4 infers it from the entrypoint, which is right for almost
everything. Declare it when your plugin genuinely needs something else: a
plugin using LuaJIT's `cdef` has nowhere else to go, and saying so is better
than failing at load. An entry that names only runtimes f4 cannot provide is
shown greyed out with the reason.