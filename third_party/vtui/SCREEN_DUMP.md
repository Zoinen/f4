# Text screen dumps

`ScreenBuf.Dump` writes a compact, human-readable snapshot for debugging and
automated UI inspection. `DecodeScreenDump` reads the same format back into a
`ScreenDump` bitmap:

```go
dump, err := vtui.DecodeScreenDump(reader)
if err != nil {
	return err
}
for _, row := range dump.Rows {
	// row.Text is the readable preview; row.Attributes has dump.Width entries.
}
```

The current format is `VTUI_SCREEN_DUMP_V1`. Its invariants are:

* the header declares a non-negative `width x height`;
* the text section contains exactly `height` UTF-8 rows;
* the attribute section contains exactly one `R<row>` line per row;
* each RLE row expands to exactly `width` 64-bit attributes;
* malformed headers, row numbers, tokens, counts, or trailing non-empty data
  are rejected by the decoder.

The text preview is deliberately kept as a string rather than split into
one-rune cells. A grapheme cluster can contain several runes, and the second
cell of a wide character is emitted as a filler with no text. The attribute
map is the exact per-cell bitmap; callers that need rendered pixels can use
the text rows and this map without having to reverse-engineer RLE first.

This separation is an invariant of V1: the decoder validates the exact colour
map but does not guess cell boundaries from Unicode display width. A future
format can add an explicit per-cell character layer without changing the V1
decoder.
