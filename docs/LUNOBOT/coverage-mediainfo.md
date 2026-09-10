# Coverage task: `plugins/mediainfo`

Origin: § 22.3 custom task. Codecov main was 61.06% overall; `util.go` in
`plugins/mediainfo` was 42.01% covered (119 lines) at task start.

The tests cover binary-width boundary helpers, FourCC and text cleanup,
duration/integer overflow guards, signed values, ISO-639 conversion, UTF-16
decoding, decimal parsing, canonical metadata tags, and pointer helpers. They
are entirely in-memory and do not depend on media files or external tools.
