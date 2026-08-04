# Application icon

`f4.svg` is the default editable source for the application icon. A file named
`f4-N.svg` overrides it for the corresponding `N`-pixel output. The generator
currently uses size-specific artwork for 16, 24, 30, 32, 36, and 42 pixels and
falls back to `f4.svg` at every other size. The files in `generated/` and the
`rsrc_windows_*.syso` files at the repository root are generated artifacts and
are intentionally committed so a normal source checkout can be packaged without
additional graphics software.

After changing the SVG, regenerate every platform asset with:

```sh
go generate
```

This command is cross-platform and only requires the Go toolchain. It creates:

- PNG files at 16, 24, 28, 30, 32, 36, 42, 48, 56, 64, 128, 256, 512, and 1024 pixels for Linux and packaging;
- a multi-resolution `f4.ico` for Windows;
- a multi-resolution `f4.icns` for macOS;
- Windows resource objects for amd64 and arm64 builds.

CI runs the same command and fails if the committed outputs are stale.

The generator vendors a small patch to `oksvg`: upstream scales path geometry
but not stroke metrics or user-space gradients when a target size is applied.
The patch applies the same transform to all three and is covered by regression
tests in `tools/icons/main_test.go`.
