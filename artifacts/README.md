# Native OpenConsole probe evidence

`native-openconsole-probe.json` and the `.sessions/*.raw` files are a sanitized
runtime capture from the pinned Windows Terminal/OpenConsole build. Absolute
workspace, temp, and cache paths are replaced with placeholders; ephemeral
handle values are redacted; only an allow-listed terminal environment is kept.

The report covers the `80x25`, `1x1`, and `121x40` sessions and records the
live host identity checks, resize events, exit codes, output hashes, and raw
ConPTY bytes.
