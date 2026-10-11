# Pinned ConPTY probe

`tools/conptyreconcile` is a standalone Windows probe for the pinned
`OpenConsole.exe` described in [CONPTY_NATIVE_TEST.md](CONPTY_NATIVE_TEST.md).
It owns its Go module and has no terminal-specific integration layer; it
records the native stream and validates logical lines whose boundaries come
only from explicit newline bytes. Non-Windows builds fail explicitly and
cannot close the native gate.

Run `go run ./tools/conptyreconcile -help` for the list of modes. The main
ones: `-probe` and `-probe-static` (marker-delimited workload, with and
without resize interleaving), `-command-probe` and `-command-compare`
(`dir /s /b` through the pinned host against a redirected file),
`-clear-probe`, `-scroll-probe`, `-command-suite`, `-tabs-probe`,
`-link-probe`, `-progress-probe`, `-unicode-probe`, `-reflow-probe`,
`-lifecycle-probe`, `-edge-probe`, `-quirk-probe` and `-gate`; `-seed N`
repeats one seed in a fresh session. Every session checks the live host's
path, product version and SHA-256 against the pinned executable before
attaching the child, and each report stores the host identity, the exact
child input, the session dimensions, the raw output and its SHA-256 (raw
bytes are also written to `<report>.sessions/<width>x<height>.raw`).

The full description of every mode and the captured evidence
(`native-openconsole-probe*.json` with their `.raw` sessions) live in
unxed/f4#1684; the findings drawn from them are in
[PINNED_HOST_FACTS.md](PINNED_HOST_FACTS.md) and
[CONPTY_NATIVE_AUDIT.md](CONPTY_NATIVE_AUDIT.md).
