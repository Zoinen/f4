# vtui-screen

`vtui.ScreenBuf.Dump` keeps the exact terminal text readable and stores cell
attributes as compact RLE. `vtui-screen` decodes that V1 format into a small
portable bitmap (PPM), with one tile per terminal cell. Empty cells show their
background colour; occupied cells show a small foreground mark. The text dump
remains the source of truth for the exact glyphs.

Usage:

```text
go run ./tools/vtui-screen vtui.screen.log screen.ppm
```

The input may be `-` for stdin and the output may be omitted or set to `-` for
stdout. Use `-cell-width 1 -cell-height 1` for a one-pixel-per-cell map, or
larger values when the bitmap will be viewed by a person. The decoder accepts
only `VTUI_SCREEN_DUMP_V1`, expands every RLE row to exactly the declared
screen width, and understands both indexed xterm colours and 24-bit RGB
attributes, including reverse video.
