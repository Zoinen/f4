# Pinned ConPTY probe

`tools/conptyreconcile` is a standalone Windows probe for the pinned
`OpenConsole.exe` described in [CONPTY_NATIVE_TEST.md](CONPTY_NATIVE_TEST.md).
It owns its Go module and has no terminal-specific integration layer. The
probe records the native stream and validates logical lines whose boundaries
come only from explicit newline bytes.

## Run

```text
go run ./tools/conptyreconcile -probe -report artifacts/pinned-conpty-probe.json
go run ./tools/conptyreconcile -probe-static -report artifacts/pinned-conpty-probe-static.json
go run ./tools/conptyreconcile -command-probe -report artifacts/pinned-conpty-command.json
go run ./tools/conptyreconcile -command-compare -command-compare-width 512 -report artifacts/pinned-conpty-command-compare-512.json
go run ./tools/conptyreconcile -clear-probe -report artifacts/pinned-conpty-clear.json
go run ./tools/conptyreconcile -scroll-probe -report artifacts/pinned-conpty-scroll.json
go run ./tools/conptyreconcile -command-suite -report artifacts/pinned-conpty-command-suite.json
go run ./tools/conptyreconcile -tabs-probe -report artifacts/pinned-conpty-tabs.json
go run ./tools/conptyreconcile -link-probe -report artifacts/pinned-conpty-link.json
go run ./tools/conptyreconcile -progress-probe -report artifacts/pinned-conpty-progress.json
go run ./tools/conptyreconcile -unicode-probe -report artifacts/pinned-conpty-unicode.json
go run ./tools/conptyreconcile -reflow-probe -report artifacts/pinned-conpty-reflow.json
go run ./tools/conptyreconcile -lifecycle-probe -report artifacts/pinned-conpty-lifecycle.json
go run ./tools/conptyreconcile -edge-probe -report artifacts/pinned-conpty-edge.json
go run ./tools/conptyreconcile -quirk-probe -report artifacts/pinned-conpty-quirk.json
go run ./tools/conptyreconcile -gate -report artifacts/pinned-conpty-gate.json
```

Проблемный seed можно повторить отдельной свежей native-сессией без повторной
загрузки уже проверенного host:

```text
go run ./tools/conptyreconcile -seed 21 -report artifacts/pinned-conpty-seed-21.json
PINNED_CONPTY_SEED=21 go run ./tools/conptyreconcile -report artifacts/pinned-conpty-seed-21.json
```

Параметр предназначен для воспроизводимого разбора конкретного seed; он не
заменяет полный прогон `-seeds` и не превращает единичный успех в результат
гейта.

The probe downloads the pinned Windows Terminal bundle into
`%LOCALAPPDATA%\pinned-conpty\1.12.10983.0\`, verifies the nested x64 package
and `OpenConsole.exe` version/SHA-256, and reuses the verified cache. An
explicit `-probe-host C:\path\OpenConsole.exe` is accepted only after the same
identity checks.

Before attaching the child, every session resolves the live host image with
`QueryFullProcessImageNameW` and fails closed unless path, product version and
SHA-256 match the pinned executable. The native ConDrv server/client path and
`PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` are used directly.

The workload uses the markers `__PINNED_CONPTY_PROBE_BEGIN__` and
`__PINNED_CONPTY_PROBE_END__`, and OSC title `pinned-conpty-probe`. It includes
the exact-width boundaries, repeated and blank records, Unicode/Bidi,
controls, alternate-screen transitions, and a 257-character line. Static mode
keeps the viewport at `80x25`; normal mode interleaves resize operations while
the child writes.

`-command-probe` is a diagnostic native run of `cmd.exe` with recursive
`dir /s /b` at normal and narrow widths. It reports observed absolute-CUP plus
`CRLF` frequency and lifecycle status; those counts are measurements of the
pinned host, never a rule for reconstructing history.

`-command-compare` repeats `dir /s /b` once through a redirected file and once
through the pinned host, then compares the host's rendered logical lines with
the file after only CRLF normalization. `-command-compare-width` selects the
fixed capture width (the consumer reflow path does not resize the host). It is
an independent diagnostic and fails closed on any mismatch; it is not yet a
gate pass.

`-clear-probe` runs a PowerShell `Clear-Host` sequence and checks that the
pinned host emits exactly one `ESC[3J`, removing the pre-clear marker from
history while retaining the post-clear marker.

`-scroll-probe` validates consumer-only scrollback: complete logical lines from
a static pinned-host session are spilled to a bounded piece-table model, then
scrolled and reflowed at several display sizes without changing history.

`-command-suite` runs bounded `echo`, `type`, `findstr`, and PowerShell cases
through the pinned host with exact marker-delimited rendered output and exit /
lifecycle checks.

`-tabs-probe` and `-link-probe` isolate tab-stop and OSC 8 rendering at a wide
static host size so their rendered text can be compared without unrelated
control-phase cursor movement.

`-progress-probe` checks that intermediate carriage-return updates are not
mistaken for history records while the final state is retained.

`-unicode-probe` checks byte-exact CJK, combining mark, emoji, ZWJ and bidi
text at a wide static host size; display widths are not used as an expectation.

`-reflow-probe` keeps the pinned host at its original size and applies the
display-width matrix only to the consumer's complete logical lines. The
host-resize interleaving behind `-probe` remains an inactive future alternative
and is not part of the gate.

Each report stores the verified host identity, exact child input, session
dimensions, resize events, process identity, raw output and SHA-256. The raw
stream is written byte-for-byte beside the report as
`<report>.sessions/<width>x<height>.raw`; no text-mode conversion is allowed.
The gate compares the decoded logical payload and on-disk bytes directly.

Non-Windows builds fail explicitly and cannot close the native gate.
