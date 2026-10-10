# Platform support

Where the GUI backends are built, and which targets are known not to build.

## The FFI layer

The gogpu and Ebitengine backends reach the system through goffi's `ffi`
package. It has an implementation for:

- Windows, on any architecture;
- Linux, macOS and FreeBSD on `amd64` and `arm64`.

FreeBSD additionally needs `-gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std`
when built without cgo.

Everywhere else there is no FFI, and the X11 backend (or the Win32 backend on
Windows) is the one that runs. `gogpuFFIAvailable` reports this at run time as
well, because a static build (`-tags goffi_static`) compiles the FFI layer but
cannot load anything through it.

## Leaving backends out

Build tags drop a backend from the binary entirely, swapping it for the
stub the unsupported platforms already get:

- `vtui_noebiten` — the Ebitengine backend (`ebiten_*.go` → `ebiten_stub.go`);
- `vtui_nogogpu` — the gogpu backend (`gogpu_*.go` → `gogpu_stub.go`,
  `gogpu_ffi_stub.go`);
- `vtui_nococoa` — the Cocoa backend on macOS (`cocoa_gui_darwin.go`,
  `cocoa_gui_keys_darwin.go` → `cocoa_gui_stub.go`). It brings in no module
  of its own, purego being there already, so it saves little; what it leaves
  out is AppKit, and the `init` that pins the main goroutine to the main
  thread.

The first two together leave the X11, Wayland and Win32 backends and remove Ebitengine,
gogpu, wgpu, naga and gg from the build graph: about 9 MB of a linux/amd64
binary, which stays linked otherwise even when the program never selects
those backends, because their packages run `init` code. Asking for a dropped
backend returns an error, and `gogpu` falls back to X11 (Win32 on Windows)
the same way it does where there is no FFI. f4's lite build uses both.
`backend_tags_test.go` checks the build graph; CI builds and vets the
combination on Linux, Windows and macOS.

The FFI layer (goffi) stays: Wayland loads libxkbcommon through it.

## Known gaps

### NetBSD, and why the shim is not optional

NetBSD needs the same `fakecgo` `-gcflags` shim FreeBSD does, and this cannot
be fixed in goffi or pureffi. fakecgo supplies `environ`, `__progname` and
`__ps_strings` in place of the crt0 a cgo-free build does not link, and those
symbols have to reach the *dynamic* symbol table for rtld to resolve libc's
undefined references at startup. `//go:cgo_export_dynamic` is the only
mechanism for that, and the compiler accepts it only in a package built as
`std` -- hence the flag. FreeBSD has lived with it for the same reason.

Both CIs now pass it for NetBSD as well as FreeBSD, so `ffibridge` enables its
FFI path there.

`keytrans` deliberately does **not**. It supports building against vanilla
`ebitengine/purego`, not only the pureffi fork, and vanilla purego has no
NetBSD support (`syscall15Args` and `isAllSameFloat` come out undefined).
Turning NetBSD on in its constraints would gain the xkbcommon and XIM backends
for consumers that replace purego with pureffi, at the cost of breaking
everyone who does not. The trade is not worth it.

### FreeBSD: gogpu's platform manager, not the FFI layer

goffi's FFI layer has an implementation for FreeBSD (see above), which makes
it tempting to drop `freebsd` from `gogpu_stub.go`'s and `gogpu_ffi_stub.go`'s
build tags and let the real `gogpu_*.go` files select the backend there. That
does not build: `go vet`/`go build` for `freebsd/amd64` and `freebsd/arm64`
both fail with

    github.com/gogpu/gogpu@v0.53.0/internal/platform/platform.go:475:9: undefined: newPlatformManager

`gogpu` itself -- the windowing layer above goffi's FFI -- has no FreeBSD
`newPlatformManager`, independently of whether the FFI it would call can load
libraries. Enabling gogpu on FreeBSD needs that upstream gap closed first;
until then FreeBSD keeps using `gogpu_stub.go` and falls back to X11, same as
every other platform gogpu does not cover.

### plan9

Does not build yet, but the target is reachable and CI for it is not the
obstacle: GitHub has no Plan 9 runners, but nothing here needs one. A Plan 9
row would cross-compile on `ubuntu-latest` exactly as the illumos, solaris and
dragonfly rows already do, so it would be build-and-vet coverage with no test
execution -- the same deal those targets get.

Two blockers are gone. `plan9/amd64` used to match the gogpu constraint and
select a backend goffi cannot serve; it is excluded now. And `vtinput` grew a
Plan 9 reader: its Unix one is built on poll(2) plus a self-pipe, neither of
which Plan 9 has, so reads there run in a pump goroutine that reports over a
channel instead.

What is left is in vtui, and it is five files rather than one idea:

| File | Missing on Plan 9 |
| --- | --- |
| `gui_grid_raster.go` | `glyphKey`, `drawBoxGlyph` |
| `crash_report_pid_unix.go` | `syscall.Kill` |
| `sys_unix.go` | `unix.Dup2` |
| `terminal_env_unix.go` | `syscall.SIGWINCH` |
| `gui_api_fallback.go` | `runInX11Window` |

The first row is worth a look on its own account. It is the CPU raster the
Win32 and Cocoa renderers share, compiled everywhere -- as is each renderer,
with a stub host where its platform is missing -- so that their tests run on
every CI runner; a Plan 9 build pulls it in without needing it. It wants the
constraint of the X11 raster helpers it calls, and the two renderers with it.
The others are the usual Unix-isms that need a Plan 9 variant or a constraint
that excludes it.

`gui_api_fallback.go` is the one that is not merely mechanical. Plan 9 has no
GUI backend at all -- rio is not X11, and vtui has no rio renderer -- so there
is nothing for the fallback to fall back *to*. The realistic goal on Plan 9 is
the terminal path with no GUI, which makes the question "is the ANSI path
self-sufficient here" rather than "which backend do we pick". That is a design
decision, not a build tag.

## Fixed

`android/arm64` builds. Ebitengine is now excluded there: `GOOS=android` also
satisfies the `linux` build tag, so the Ebitengine backend was being selected
on Android, where its own `internal/ui` package does not compile without the
gomobile/cgo path (`dipToNativePixels` and `graphicsDriverCreatorImpl` come out
undefined). That is an Ebitengine limitation, reported upstream; vtui simply
does not select the backend there. The gogpu backend still is.

`windows/386` used to fail with `undefined: isSpecialOrModifiedKey`: the helper
lived in `gogpu_host.go`, which is limited to `amd64`/`arm64`, while its caller
in the Win32 backend is built for every Windows architecture. It now lives in
`keys_special.go` with no build tag.

## macOS: the Cocoa backend

`--gui=cocoa` (`RunInGUIWindow(..., "cocoa", ...)`) opens an AppKit window
without cgo and without a GPU: purego registers an `NSView` subclass with the
Objective-C runtime, the grid is rasterised by `gridRaster` (the Win32
backend's raster, now shared), and each frame becomes a CoreGraphics image
set as the view layer's contents. It is built for darwin on amd64 and arm64;
elsewhere, and with `-tags vtui_nococoa`, `cocoa_gui_stub.go` takes its
place and asking for it returns an error. It is never chosen automatically:
an empty backend name keeps looking for `WAYLAND_DISPLAY` and `DISPLAY`.

AppKit belongs to the main thread, so the backend has to be started from the
main goroutine; `cocoa_gui_darwin.go` locks it to the main thread in `init`,
as gogpu and Ebitengine do in theirs. FrameManager renders on its own
goroutine as with every backend, and whatever it needs from the window --
a display pass, a title, a size -- is handed over with
`performSelectorOnMainThread:`.

Two properties of the FFI layer (pureffi, over goffi) shape the code:

- Callbacks take no structures on arm64 and return none on any
  architecture. `NSTextInputClient` passes and returns `NSRange` and
  `NSRect` by value, so the view does not adopt it. Typed text comes from
  the keyboard layout through `UCKeyTranslate`, which also combines dead
  keys; input methods that compose in a window of their own (Chinese,
  Japanese, Korean) do not work.
- On arm64, arguments that spill to the stack each take an 8-byte slot in
  goffi's calls, where Apple's ABI instead packs the small ones tightly.
  Since that mismatch would misalign any call whose arguments spill, the
  backend only makes calls whose arguments stay in registers, and never
  lets one spill onto the stack. `cmd/cocoa-smoke` builds its key events
  with Quartz event services instead of `+[NSEvent keyEventWithType:...]`,
  whose `BOOL` and `unsigned short` would otherwise land on the stack.

Keys follow the gogpu backend on macOS: Command is the left Ctrl channel,
Control the right one, Option is Alt, and an Option chord carries the key's
own character, not the one Option composes. Clipboard is goclip's, as for
every backend. Not done yet: drag and drop, and moving the window to a
display of another scale -- the font stays rasterised for the display the
window opened on, and Core Animation scales the frames.

CI runs `cmd/cocoa-smoke` on an arm64 and an Intel macOS runner (the `cocoa`
job). It drives the window from outside with real `NSEvent`s and checks both
that each event reached the application and the colours of known cells in
the frame the backend gave Core Animation; `report.txt`, those frames and
screenshots of the window are uploaded as the `cocoa-smoke-darwin-*`
artifacts.
