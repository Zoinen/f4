# Prepared file-panel artwork

These committed SVGs are separate from the original Lucide chrome icons.
`names.txt` covers file-type/folder variants and virtual-panel device/action rows.
Each numbered directory contains the same artwork with stroke width `number / 2`
and a precomputed tight viewBox including the complete stroked contour.
Original paths and aspect ratios are preserved. Menu/button/path-control icons
continue to use `icons/lucide` or `lucide-gallery`, unchanged.

To regenerate after updating original SVGs or the manifest, on a native Qt build:

```
cmake --build <build-dir> --target F4PrepareFileIcons
<build-dir>/F4PrepareFileIcons <path-to-qt-host>/icons
```

The offline tool measures at high resolution on a padded canvas so stroke caps
cannot be clipped. It is excluded from normal builds. Its outputs are committed:
release builds and cross-compilers only embed the prepared SVGs; the application
never scans alpha pixels, computes artwork bounds or crops icon rasters.
