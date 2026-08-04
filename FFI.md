# f4 FFI bridge

Sandboxed plugins cannot reach the host by themselves, and hand-writing a
wrapper for every function of every system library is not a plan. Instead f4
projects a small FFI broker into the sandbox: the plugin describes the ABI it
wants, and the broker performs the call.

The broker lives in `ffibridge` and is built on `pureffi`, so it needs no cgo.

## Signatures

A call is described by one string:

```
<return type>(<argument type>, <argument type>, ...)
```

Types are `void`, `bool`, `i8`, `u8`, `i16`, `u16`, `i32`, `u32`, `i64`, `u64`,
`f32`, `f64`, `ptr` and `str`. `ptr` is a raw address; `str` is a convenience
for `const char *` arguments and `char *` results, converted on the fly. An
empty argument list, or `void`, means no arguments. A trailing `...` marks a
C-variadic function.

```
i64(str)               size_t strlen(const char *)
ptr(ptr,ptr,i64)       void *memcpy(void *, const void *, size_t)
void(ptr,i64,i64,ptr)  void qsort(void *, size_t, size_t, int (*)(const void *, const void *))
i32(ptr,str,...)       int sprintf(char *, const char *, ...)
```

There is no C declaration parser on purpose. A `cdef`-style front end that
compiles C prototypes down to these signatures can be added later without
changing anything below it.

## Everything is an integer

Library handles, function pointers, memory blocks and callback trampolines are
all plain addresses. Nothing else crosses the boundary, which is what lets the
same broker serve a Lua sandbox and a wasm sandbox without a second design.

Memory allocated through the broker belongs to the broker and is released when
the plugin is torn down.

## Safety

FFI inside a sandbox is an escape hatch from that sandbox: a plugin holding it
can do anything f4 itself can do. Every operation passes through a single hook
so that the permission model can gate it per plugin, with the plugin's own
explanation shown to the user. Until that is wired up, an unset hook allows
everything, which is fine for local development and not for anything else.

Builds made with the `noffi` tag, and platforms `pureffi` does not cover, keep
the whole plugin system working and only report the bridge as unsupported.

## Reaching the bridge from a sandbox

An embedded Lua plugin gets the broker as the `f4ffi` module. A wasm guest
cannot load a library itself, so it gets the same broker over the plugin
protocol instead, as `Host.FFI.*` methods: `Open`, `OpenLibC`, `Sym`, `Call`,
`CallSym`, `Alloc`, `Free`, `CString`, `GoString`, `Read`, `Write`, `Peek`,
`Poke` and `Callback`.

This works only because the broker deals exclusively in integers and strings.
Handles, addresses and signatures survive MessagePack unchanged, so nothing has
to reconcile a guest's own memory offsets with host addresses.

`Host.FFI.Callback` registers a trampoline and returns its address. When native
code invokes it, the host calls `Plugin.OnFFICallback` with the id the plugin
chose. So a guest can hand a real C function pointer to a library it has never
been able to link against, and still never hold a host pointer.

A subprocess plugin is not given these methods. It is a native process and can
load libraries perfectly well on its own.
