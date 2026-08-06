# FISH+ shell test lab

`fishclient.py` speaks the FISH+ wire protocol to a shell started as a
subprocess. It is not a replacement for `go test`, which covers the same
ground with the real client; it exists for the things the Go tests cannot
reach.

## Why it exists

Two of the worst defects found in this protocol were invisible to `go test`
on the machine they were written on.

The first was a race: the helper used to arrive on the same stream the
requests do, and `dash` reads ahead of its parser, so a request that had
already arrived was executed as a shell command. Over ssh the latency almost
always hides it. Here there is no latency, so it happened nearly every time.

The second was `: > "$file"` killing the shell outright when the redirection
failed, because `:` is a special builtin and POSIX says a non-interactive
shell shall exit. The Go client reported a closed session and nothing else;
this lab shows the remote `stderr`.

## Running it

    python3 test_patch.py                 # whatever /bin/sh is here
    python3 test_patch.py /bin/dash
    python3 test_patch.py /bin/bash

Run it under `dash` at least once. `bash` is forgiving in ways the shells on
the hosts people connect to are not.

## Pretending to be another host

The second argument is prepended to `PATH`, which is how a host we do not own
gets simulated. Put a script named after a tool in a directory and it shadows
the real one: a `find` that refuses `-printf`, a `stat` that speaks the BSD
`-f` dialect, a `dd` without `iflag`, a `head -c` that drains its input the
way macOS does. That is how the macOS code paths in the helper are exercised
on a GNU build machine, from the probe output in issue #316 rather than from
guesswork.

## Writing another test

Use `Remote.exec` for anything whose request is one line plus path lines, and
`Remote.send` for a command that carries a payload. End every test that
exercises a failure with `r.exec("noop")`: only what the session answers
*after* a refused request shows that the payload really left the wire.