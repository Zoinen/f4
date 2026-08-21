# Portable build and distribution policy

This document is the mandatory source of truth for f4 build, packaging,
release, deployment, and CI work. `AGENTS.md` requires every AI agent to read
it before touching those workflows.

## Product contract

The downloadable unit for Linux and Windows is one Go executable per operating
system and CPU architecture. The Go executable contains the matching compressed
Qt frontend executable. On first Qt launch it materializes that frontend into a
content-addressed user cache; later launches reuse the same verified build path.
The Go process remains the parent and communicates with the Qt process through
the existing ExtUI protocol.

The two-process boundary is intentional. Do not merge the Go runtime and the Qt
event loop into one process merely to call the result a single binary.

The downloadable unit for macOS is one signed and notarized `.app` bundle. Its
Go executable, Qt helper, frameworks, plugins, and resources are nested code and
data inside that bundle and must be signed before distribution. Do not extract
an executable helper into a user cache on macOS.

User-installed plugins, user configuration, fonts, certificates, themes, GPU
drivers, display servers, and other host-owned data or services are outside the
single-file runtime contract.

## Linux contract

- Build the Go launcher with `CGO_ENABLED=0` and verify that it has no ELF
  interpreter or `DT_NEEDED` entries.
- Build Qt, QML modules, QWindowKit, ZoinGallery, C++ runtime, and application
  codecs into one `f4-qt-host` ELF.
- Embed every required QML module, Qt plugin, shader, image, and application
  resource. The host must not require a sibling Qt library, QML tree, or plugin
  directory.
- Dynamically use the system glibc and irreducible host graphics/device ABI.
  The selected compatibility baseline is glibc 2.27. Every native dependency,
  not only the final link, must be built against that baseline.
- The normal build must not bundle its own glibc. Alpine, pure NixOS, and other
  systems without the conventional glibc loader require a separately tested
  compatibility path and must not be claimed supported by the generic build.
- Audit the final host with `readelf`: maximum required `GLIBC_*` must be 2.27,
  no Qt, QWindowKit, ZoinGallery, codec, `libstdc++`, or `libgcc_s` shared
  dependency may remain, and RPATH/RUNPATH must not reference a build tree.
- Run the produced binary, not just the compiler, in the oldest supported
  environment and in current Linux environments. Exercise offscreen/software,
  X11, Wayland, and representative Intel/AMD/NVIDIA systems before broadening a
  release claim.

## Native dependency cache contract

- A binary-package cache is not evidence of the libc baseline: Conan package
  IDs do not encode the glibc version used while compiling. Linux cache keys
  must therefore include the complete baseline and compiler contract
  (`glibc-2.27-gcc11`); Windows keys must include its static MSVC contract.
- Persist only completed Conan package folders. Remove recipe sources, build
  trees, temporary files, and backup sources before saving a checkpoint. This
  keeps the cache small and prevents stale source trees from masking recipe or
  source-mirror changes.
- On every non-cancelled exit, including a failed `conan install`, save an
  explicit package-only checkpoint. Cache objects are immutable, so each
  interrupted checkpoint needs a fresh key; future jobs restore the newest
  compatible checkpoint before continuing the graph.

## Windows contract

- Build the Go launcher with `CGO_ENABLED=0`.
- Build Qt and the complete application-owned native dependency graph into one
  `f4-qt-host.exe`, using the static MSVC runtime.
- Embed the compressed host in `f4.exe`; do not ship Qt DLL, QML, or plugin
  directories beside it.
- Audit imports so only Windows system DLLs remain.

## macOS contract

- Use the repository deployment target consistently for Qt and every native
  dependency. Qt 6.11 currently makes that target macOS 13.0.
- Prefer bundled dynamic Qt frameworks and plugins. Static Qt is permitted only
  when it demonstrably simplifies the signed bundle; it is not a portability
  requirement because Apple system frameworks remain dynamic.
- Put the Go executable and Qt helper inside one normal `.app`, sign nested code
  in the required order, sign the outer bundle, notarize it, and verify it with
  Gatekeeper tooling.

## Embedded Qt-host lifecycle

The embedded payload is generated during the release build and is never
committed. The runtime lookup order is:

1. explicit `F4_EXT_UI_PATH`, for development and diagnostics;
2. a packaged sibling host, for distribution-maintainer builds;
3. the content-addressed embedded-host cache.

`F4_QT_HOST_CACHE_DIR` may override the cache root for tests and controlled
deployments. Otherwise use the operating system user cache directory. Extraction
must use a private directory, a temporary file, gzip integrity validation,
executable permissions, and atomic rename. Concurrent first launches must be
safe. A new payload hash must produce a new path, so upgrades cannot execute a
stale helper. Old cache generations may be garbage-collected only when they are
not running.

## CI and release gates

A portable artifact is complete only when CI proves all of the following:

1. the native dependency graph was built in the selected baseline environment;
2. the Qt host passes its C++/QML tests and an installed-tree-independent smoke
   test;
3. static-dependency, ABI, and Linux hardening audits (PIE, RELRO, BIND_NOW,
   non-executable stack) pass;
4. the generated compressed payload is embedded into a `CGO_ENABLED=0` Go
   launcher;
5. extraction tests prove first-run materialization, reuse, concurrency safety,
   and upgrade separation;
6. the published Linux/Windows artifact contains the Go launcher as its only
   application executable;
7. required license notices, corresponding sources, and LGPL relinking material
   are published alongside a release when the selected Qt license requires it.

Never infer portability from a successful link. Missing runtime, ABI, QML,
plugin, and platform tests mean the portable build is not complete.
