"""Does ConPTY invent line breaks? Same output, four write sizes.

A child writes 200 'A' into an 80-column pseudoconsole, in chunks of 200,
15, 16 and 1 characters. Same text, same screen, four different ways of
handing it to WriteConsoleW. The payload has no CR, no LF and no escape
sequence, so every line break in the ConPTY output was put there by ConPTY.

Since microsoft/terminal#17510, WriteCharsLegacy appends a `\\r\\n` when the
last character of a write lands on the right margin, so whether the logical
line survives depends on whether the write size divides 80.

    python tools/conpty_probe.py [path\\to\\conpty.dll]

With no argument, only the in-box pseudoconsole is probed. Given a
conpty.dll, that one is probed too, as a comparison; conpty.dll spawns
OpenConsole.exe from its own directory, so both files must sit side by side.

Runs on Windows only. Output also goes to probe-out/, which the workflow
uploads as an artifact.
"""

import ctypes
import os
import re
import sys
import threading
import time
from ctypes import wintypes

COLS, ROWS = 80, 25
# Real programs print lines well under 80 columns, so at the default width
# almost every case came back "nothing long enough to tell". Give them a
# narrow pty instead: at 20 columns even ipconfig's short lines have to
# wrap, and whether they arrive whole becomes a real question.
PROG_COLS = 20
TOTAL = 200
CHUNKS = (200, 15, 16, 1)
MODES = ("default", "legacy")
OUTDIR = "probe-out"
CHILD = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                     "conpty_probe_child.py")

PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x00020016
EXTENDED_STARTUPINFO_PRESENT = 0x00080000
STARTF_USESTDHANDLES = 0x00000100


class COORD(ctypes.Structure):
    _fields_ = [("X", ctypes.c_short), ("Y", ctypes.c_short)]


class STARTUPINFOW(ctypes.Structure):
    _fields_ = [("cb", wintypes.DWORD), ("lpReserved", wintypes.LPWSTR),
                ("lpDesktop", wintypes.LPWSTR), ("lpTitle", wintypes.LPWSTR),
                ("dwX", wintypes.DWORD), ("dwY", wintypes.DWORD),
                ("dwXSize", wintypes.DWORD), ("dwYSize", wintypes.DWORD),
                ("dwXCountChars", wintypes.DWORD),
                ("dwYCountChars", wintypes.DWORD),
                ("dwFillAttribute", wintypes.DWORD), ("dwFlags", wintypes.DWORD),
                ("wShowWindow", wintypes.WORD), ("cbReserved2", wintypes.WORD),
                ("lpReserved2", ctypes.POINTER(ctypes.c_byte)),
                ("hStdInput", wintypes.HANDLE), ("hStdOutput", wintypes.HANDLE),
                ("hStdError", wintypes.HANDLE)]


class STARTUPINFOEXW(ctypes.Structure):
    _fields_ = [("StartupInfo", STARTUPINFOW), ("lpAttributeList", ctypes.c_void_p)]


class PROCESS_INFORMATION(ctypes.Structure):
    _fields_ = [("hProcess", wintypes.HANDLE), ("hThread", wintypes.HANDLE),
                ("dwProcessId", wintypes.DWORD), ("dwThreadId", wintypes.DWORD)]


k32 = ctypes.WinDLL("kernel32", use_last_error=True)

# Spell out every signature. Without argtypes ctypes pushes plain ints as
# 32-bit, while UpdateProcThreadAttribute takes DWORD_PTR: the call then
# "succeeds" with a mangled attribute and the child quietly keeps the
# parent's console instead of the pty.
PSIZE_T = ctypes.POINTER(ctypes.c_size_t)
k32.CreatePipe.argtypes = [ctypes.POINTER(wintypes.HANDLE),
                           ctypes.POINTER(wintypes.HANDLE),
                           ctypes.c_void_p, wintypes.DWORD]
k32.InitializeProcThreadAttributeList.argtypes = [ctypes.c_void_p, wintypes.DWORD,
                                                  wintypes.DWORD, PSIZE_T]
k32.UpdateProcThreadAttribute.argtypes = [ctypes.c_void_p, ctypes.c_size_t,
                                          ctypes.c_size_t, ctypes.c_void_p,
                                          ctypes.c_size_t, ctypes.c_void_p,
                                          PSIZE_T]
k32.DeleteProcThreadAttributeList.argtypes = [ctypes.c_void_p]
k32.CreateProcessW.argtypes = [wintypes.LPCWSTR, wintypes.LPWSTR,
                               ctypes.c_void_p, ctypes.c_void_p, wintypes.BOOL,
                               wintypes.DWORD, ctypes.c_void_p, wintypes.LPCWSTR,
                               ctypes.POINTER(STARTUPINFOEXW),
                               ctypes.POINTER(PROCESS_INFORMATION)]
k32.ReadFile.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                         ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]
k32.WriteFile.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                          ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]
k32.GetStdHandle.restype = wintypes.HANDLE
k32.GetExitCodeProcess.argtypes = [wintypes.HANDLE,
                                   ctypes.POINTER(wintypes.DWORD)]
k32.SetHandleInformation.argtypes = [wintypes.HANDLE, wintypes.DWORD,
                                     wintypes.DWORD]


def load_conpty(path):
    """Return (CreatePseudoConsole, ClosePseudoConsole) from `path`.

    `path` may be "system" to use the in-box pseudoconsole through exactly
    the same code, which is the control for this experiment.
    """
    dll = k32 if path == "system" else ctypes.WinDLL(path, use_last_error=True)
    create = getattr(dll, "ConptyCreatePseudoConsole",
                     getattr(dll, "CreatePseudoConsole", None))
    close = getattr(dll, "ConptyClosePseudoConsole",
                    getattr(dll, "ClosePseudoConsole", None))
    resize = getattr(dll, "ConptyResizePseudoConsole",
                     getattr(dll, "ResizePseudoConsole", None))
    if create is None:
        raise OSError(f"no CreatePseudoConsole export in {path}")
    create.restype = ctypes.c_long  # HRESULT
    create.argtypes = [COORD, wintypes.HANDLE, wintypes.HANDLE, wintypes.DWORD,
                       ctypes.POINTER(ctypes.c_void_p)]
    close.argtypes = [ctypes.c_void_p]
    resize.restype = ctypes.c_long  # HRESULT
    resize.argtypes = [ctypes.c_void_p, COORD]
    return create, close, resize


def run(conpty, chunk, mode, during=None, timeout_s=20,
        quiesce_s=0.75, cols=COLS):
    """Run the child in a fresh pseudoconsole.

    Returns (stream, console mode the child actually had). The child reports
    the mode through its exit code so nothing extra lands in the stream.
    """
    create_pc, close_pc, _resize_pc = conpty
    hin_r, hin_w = wintypes.HANDLE(), wintypes.HANDLE()
    hout_r, hout_w = wintypes.HANDLE(), wintypes.HANDLE()
    k32.CreatePipe(ctypes.byref(hin_r), ctypes.byref(hin_w), None, 0)
    k32.CreatePipe(ctypes.byref(hout_r), ctypes.byref(hout_w), None, 0)

    hpc = ctypes.c_void_p()
    hr = create_pc(COORD(cols, ROWS), hin_r, hout_w, 0, ctypes.byref(hpc))
    if hr:
        raise OSError(f"CreatePseudoConsole -> 0x{hr & 0xFFFFFFFF:08x}")

    size = ctypes.c_size_t(0)
    k32.InitializeProcThreadAttributeList(None, 1, 0, ctypes.byref(size))
    attrs = (ctypes.c_byte * size.value)()
    k32.InitializeProcThreadAttributeList(attrs, 1, 0, ctypes.byref(size))
    k32.UpdateProcThreadAttribute(
        attrs, 0, ctypes.c_size_t(PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE),
        hpc, ctypes.sizeof(hpc), None, None)

    siex = STARTUPINFOEXW()
    siex.StartupInfo.cb = ctypes.sizeof(STARTUPINFOEXW)
    # Straight from ConptyConnection::_LaunchAttachedClient: declare that we
    # supply the std handles, and supply NULL. Without the flag,
    # CreateProcessW copies the parent's handle values into the child even
    # with bInheritHandles false; ours are pipes, invalid over there, so a
    # program writing to stdout wrote nowhere and every direct launch came
    # back empty. With the flag the console assigns them on attach.
    siex.StartupInfo.dwFlags = STARTF_USESTDHANDLES
    siex.lpAttributeList = ctypes.cast(attrs, ctypes.c_void_p)
    pi = PROCESS_INFORMATION()

    # CreatePseudoConsole dups these into conhost, so drop our copies now
    # (this is what samples/ConPTY/EchoCon does); otherwise nothing ever
    # signals end-of-stream on the read side.
    k32.CloseHandle(hin_r)
    k32.CloseHandle(hout_w)

    # Attaching a pseudoconsole gives the child the right console, but its
    # std handles still come from us. When our stdout is a pipe (a CI log)
    # the child writes there and the pty carries only the handshake.
    for std in (-10, -11, -12):
        h = k32.GetStdHandle(std)
        if h and h != wintypes.HANDLE(-1).value:
            k32.SetHandleInformation(h, 1, 0)  # HANDLE_FLAG_INHERIT off

    cmd = (chunk if isinstance(chunk, str)
           else f'"{sys.executable}" "{CHILD}" {chunk} {mode}')
    if not k32.CreateProcessW(None, ctypes.create_unicode_buffer(cmd), None,
                              None, False, EXTENDED_STARTUPINFO_PRESENT, None,
                              None, ctypes.byref(siex), ctypes.byref(pi)):
        raise OSError(f"CreateProcessW -> {ctypes.get_last_error()}")

    # The pseudoconsole holds the write end open, so a blocking read here
    # would hang forever once the child exits. Read on a thread, wait for
    # the child, then close the pseudoconsole to drop the write end.
    out = []

    def answer(chunk):
        for query, reply in QUERIES:
            if query in chunk:
                written = wintypes.DWORD()
                try:
                    k32.WriteFile(hin_w, reply, len(reply),
                                  ctypes.byref(written), None)
                except OSError:
                    return

    def reader():
        buf = (ctypes.c_char * 4096)()
        got = wintypes.DWORD()
        while k32.ReadFile(hout_r, buf, 4096, ctypes.byref(got), None) and got.value:
            chunk = bytes(buf[:got.value])
            out.append(chunk)
            answer(chunk)

    th = threading.Thread(target=reader, daemon=True)
    th.start()
    if during is not None:
        during(hpc, lambda: sum(len(c) for c in out))
    k32.WaitForSingleObject(pi.hProcess, int(timeout_s * 1000))

    # The child exiting does not mean its output has been pushed through
    # yet; wait for the byte count to stop growing before closing.
    settled, last = 0.0, -1
    while settled < quiesce_s:
        if len(out) != last:
            last, settled = len(out), 0.0
        time.sleep(0.05)
        settled += 0.05

    got_mode = wintypes.DWORD()
    k32.GetExitCodeProcess(pi.hProcess, ctypes.byref(got_mode))
    close_pc(hpc)
    th.join(5)
    k32.DeleteProcThreadAttributeList(ctypes.cast(attrs, ctypes.c_void_p))
    for h in (pi.hThread, pi.hProcess, hin_w, hout_r):
        k32.CloseHandle(h)
    return b"".join(out), got_mode.value


CRLF = b"\r\n"

# ConPTY introduces itself and asks the terminal to do the same. We never
# answered, and a pseudoconsole talking to a terminal that never replies
# does not behave like one talking to a real one: from the fifth program
# onwards every stream came back with autowrap off and control characters
# rendered as glyphs. Answer the usual queries the way a plain VT100 would.
QUERIES = (
    (b"\x1b[c", b"\x1b[?1;0c"),        # primary device attributes
    (b"\x1b[0c", b"\x1b[?1;0c"),
    (b"\x1b[>c", b"\x1b[>0;10;1c"),    # secondary device attributes
    (b"\x1b[>0c", b"\x1b[>0;10;1c"),
    (b"\x1b[5n", b"\x1b[0n"),          # device status: report ok
    (b"\x1b[6n", b"\x1b[1;1R"),        # cursor position
)

# Every escape sequence, plus the newlines, as one token stream.
TOKEN = re.compile(
    rb"\x1b\][^\x07]*\x07"           # OSC
    rb"|\x1b\[[0-9;?]*[Hf]"           # cursor position
    rb"|\x1b\[[0-9;]*B"               # cursor down
    rb"|\x1b[DEM]"                    # index, next line, reverse index
    rb"|\x1b\[[0-9;?]*[ -/]*[@-~]"    # any other CSI
    rb"|\x1b."                        # ESC + one byte
    rb"|\r\n|\n")

# A CRLF is not the only way to end a row. Repainting a buffer row by row
# with absolute cursor positioning splits a line just as thoroughly, and
# leaves no wrap flag on the terminal side either. Counting only CRLF made
# that case read as one intact 200-character line when it was three rows.
BREAK = re.compile(rb"^(\x1b\[[0-9;?]*[HfB]|\x1b[DEM]|\r\n|\n)$")
CONTROL = re.compile(rb"[\x00-\x1f\x7f]")


def rows(raw):
    """Length of each row of text, counting cursor moves as row endings."""
    if isinstance(raw, str):
        raw = raw.encode("utf-8", "replace")
    lengths, current, pos = [], 0, 0
    for m in TOKEN.finditer(raw):
        current += len(CONTROL.sub(b"", raw[pos:m.start()]).decode(
            "utf-8", "replace"))
        pos = m.end()
        if BREAK.match(m.group()):
            lengths.append(current)
            current = 0
    current += len(CONTROL.sub(b"", raw[pos:]).decode("utf-8", "replace"))
    lengths.append(current)
    return [n for n in lengths if n] or [0]


def probe(label, path, say):
    say(f"=== {label} ===")
    conpty = load_conpty(path)
    for mode in MODES:
        shapes = {}
        seen = set()
        for chunk in CHUNKS:
            raw, got_mode = run(conpty, chunk, mode)
            with open(f"{OUTDIR}/stream-{label}-{mode}-{chunk}.bin", "wb") as f:
                f.write(raw)
            got = rows(raw)
            shapes[chunk] = tuple(got)
            seen.add(got_mode)
            calls = -(-TOTAL // chunk)
            plural = "write " if calls == 1 else "writes"
            say(f"  {calls:>3} {plural} of {chunk:<4} -> "
                f"{len(got)} logical line{'' if len(got) == 1 else 's'}: {got}"
                f"{'' if len(got) == 1 else f'   <- {len(got) - 1} CRLF added by ConPTY'}")
        modes = "/".join(f"0x{m:04x}" for m in sorted(seen))
        head = f"  ^ console mode {modes}"
        if len(set(shapes.values())) == 1:
            say(f"{head}: every write size gave {list(shapes[CHUNKS[0]])}")
        else:
            say(f"{head}: the line structure depends on the write size")
        say("")


def probe_cmd(label, conpty, say):
    """What does a real Console API app get? cmd's `type` of a 160-char line.

    160 is two full rows, so the write ends exactly on the margin. On the
    legacy path that earns an injected CRLF on top of the one the file
    already has; on the passthrough path there is only the file's own.
    """
    path = os.path.abspath(f"{OUTDIR}/longline.txt")
    with open(path, "w", newline="\r\n") as f:
        f.write("A" * 160 + "\n")
    # This used to redirect to CONOUT$ to give cmd a stdout. That redirect
    # silently dropped the console mode to 0x0001 - no wrap at EOL - and in
    # that mode WriteCharsLegacy never injects, because `wrapped` is
    # `wrapAtEOL && ...`. The single CRLF this case reported was therefore
    # not evidence of passthrough. Rely on the console to hand cmd its std
    # handles instead.
    raw, _ = run(conpty, f'cmd /c type "{path}"', "default")
    with open(f"{OUTDIR}/stream-{label}-cmd-type.bin", "wb") as f:
        f.write(raw)
    breaks = raw.count(b"\r\n")
    got = rows(raw)
    say(f"  cmd /c type (one 160-char line) -> {breaks} CRLF in the stream, "
        f"rows {got}")
    if sum(got) < 160:
        say("    only {} chars of text arrived - cmd never wrote to the pty, "
            "this run measured nothing".format(sum(got)))
    else:
        say("    1 = the file's own newline, passthrough; "
            "2 = one was injected, legacy path")
    say("")


PROGRAMS = [
    ("findstr", "findstr AAA probe-out\\long200.txt"),
    ("certutil", "certutil -hashfile probe-out\\long200.txt SHA512"),
    ("reg-query", 'reg query "HKLM\\SOFTWARE\\Microsoft\\Windows NT'
                  '\\CurrentVersion" /v BuildLabEx'),
    ("tasklist", "tasklist"),
    ("ipconfig", "ipconfig /all"),
    ("powershell", 'powershell -NoProfile -Command "Write-Host (\'A\'*200)"'),
    ("python", 'python -c "print(\'A\'*200)"'),
    ("node", 'node -e "console.log(\'A\'.repeat(200))"'),
    ("git-log", "git -C probe-out\\repo log -1 --format=oneline"),
    ("bash", 'bash -c "cat probe-out/long200.txt"'),
]

SETMODE = f'"{sys.executable}" "{CHILD}" 1 setmode 0'


def setup_programs():
    """Files the program cases read from. Cheap, and idempotent."""
    os.makedirs(f"{OUTDIR}/longname", exist_ok=True)
    with open(f"{OUTDIR}/long200.txt", "w", newline="\r\n") as f:
        f.write("A" * 200 + "\n")
    with open(f"{OUTDIR}/longname/" + "n" * 200 + ".txt", "w") as f:
        f.write("x")
    if not os.path.isdir(f"{OUTDIR}/repo/.git"):
        os.makedirs(f"{OUTDIR}/repo", exist_ok=True)
        quiet = " >nul 2>&1"
        os.system(f"git init {OUTDIR}/repo" + quiet)
        os.system(f'git -C {OUTDIR}/repo -c user.email=p@l -c user.name=p '
                  f'commit --allow-empty -m "{"A" * 200}"' + quiet)


def probe_programs(label, conpty, say):
    """Do real programs lose long lines? Three launch styles, all reported.

    Two things have to be true at once for a case to mean anything: the
    output has to reach the pty, and the console mode has to be the one
    ConPTY hands out. Every earlier attempt got one and lost the other.

      direct        CreateProcessW with no shell. Mode stays 0x0007, but
                    the child's stdout is whatever the console gives it.
      cmd+redirect  `cmd /c ... >CONOUT$`. Output always arrives, but cmd
                    drops the mode to 0x0001 - no wrap at EOL - so nothing
                    wraps and the numbers say nothing.
      cmd+setmode   the same, with the mode put back to 0x0007 first by a
                    helper that writes nothing.

    Rather than bet on one, run all three and label them. A case is only
    worth reading where text arrived AND autowrap was on.
    """
    setup_programs()
    say(f"  real programs in a {PROG_COLS}-column pty:")

    for how, command in (
            ("direct", f'"{sys.executable}" "{CHILD}" 1 default 0'),
            ("cmd+redirect", f'cmd /c "{sys.executable}" "{CHILD}" '
                             f'1 default 0 >CONOUT$'),
            ("cmd+setmode", f'cmd /c {SETMODE} & "{sys.executable}" '
                            f'"{CHILD}" 1 default 0 >CONOUT$')):
        try:
            _raw, mode = run(conpty, command, "default", timeout_s=30,
                             quiesce_s=0.5, cols=PROG_COLS)
            say(f"    console mode via {how:<13}: 0x{mode:04x}")
        except OSError as exc:
            say(f"    console mode via {how:<13}: FAILED: {exc}")
    say("")

    for name, command in PROGRAMS:
        # Only the direct launch is measurable: cmd hands its children mode
        # 0x0001, where wrap-at-EOL is clear and nothing wraps at all, and
        # console output mode is per-handle so it cannot be fixed from here.
        for how, line in (("direct", command),):
            try:
                raw, _mode = run(conpty, line, "default", timeout_s=30,
                                 quiesce_s=1.5, cols=PROG_COLS)
            except OSError as exc:
                say(f"    {name:<11} {how:<13} FAILED: {exc}")
                continue
            with open(f"{OUTDIR}/stream-{label}-prog-{name}-{how}.bin",
                      "wb") as f:
                f.write(raw)

            got = rows(raw)
            if not got or max(got) == 0:
                say(f"    {name:<11} {how:<13} no text reached the pty")
                continue
            if b"\x1b[?7l" in raw:
                say(f"    {name:<11} {how:<13} {max(got):>5} longest, "
                    "but autowrap was off - not comparable")
                continue

            longest = max(got)
            full = sum(1 for n in got if n == PROG_COLS)
            if longest > PROG_COLS:
                verdict = "arrived whole"
            elif longest == PROG_COLS:
                verdict = f"= the {PROG_COLS}-column width, split first"
            else:
                verdict = "nothing long enough to tell"
            extra = f", {full} row(s) exactly {PROG_COLS}" if full else ""
            say(f"    {name:<11} {how:<13} {longest:>5} longest, "
                f"{len(got):>3} rows, {verdict}{extra}")
    say("")
    say(f"    only a line reaching the pty with autowrap on says anything;")
    say(f"    longer than {PROG_COLS} means ConPTY passed it through and the "
        "terminal wrapped it")
    say("")


def probe_bufapi(label, conpty, say):
    """What a TUI application gets: cells painted straight into the buffer.

    Everything measured so far went through WriteConsoleW. Applications
    that paint with WriteConsoleOutput and friends never reach WriteChars,
    and conhost holds only cells for them, with no notion of where a
    logical line starts or ends. How ConPTY turns that buffer back into VT
    is the last untested path, and the one a file manager actually uses.
    """
    child = f'"{sys.executable}" "{CHILD}" 200 bufapi 2'
    raw, _mode = run(conpty, child, "bufapi")
    with open(f"{OUTDIR}/stream-{label}-bufapi.bin", "wb") as f:
        f.write(raw)
    got = rows(raw)
    say(f"  WriteConsoleOutputCharacterW (200 chars at 0,0) -> rows {got}")
    if sum(got) < 200:
        say(f"    only {sum(got)} chars of text arrived - "
            "this run measured nothing")
    elif len(got) == 1:
        say("    one run: ConPTY streamed the cells contiguously and the "
            "terminal wraps them itself")
    else:
        say("    split into rows: ConPTY repainted the buffer row by row, so "
            "the terminal")
        say("    records no wrap. note conhost has no wrap bit here either - "
            "SetWrapForced")
        say("    is only ever called from WriteCharsLegacy, never from the "
            "buffer writes")
    say("")


def probe_resize(label, conpty, say):
    """Write one 200-char line, then resize the pty underneath it.

    #17510 says in its own description that forcing the wrap this way
    breaks text reflow on window resize. Nothing above ever resized, so
    that part was never measured. Here the child writes 200 characters as
    one line and then just waits; the parent widens the pty to 100 columns
    and narrows it to 60, recording where in the stream each resize
    happened. What ConPTY repaints in between is the answer: 200 characters
    in a row means the line survived, rows split by CRLF means the repaint
    turned a wrap into a line ending.
    """
    _create, _close, resize_pc = conpty
    marks = []
    errors = []

    def during(hpc, offset):
        # Python takes seconds to start on Windows, and the bundled path
        # spawns its own OpenConsole first. Resizing on a timer resized an
        # empty screen, so wait for the payload to actually show up.
        deadline = time.time() + 20
        while offset() < 200 and time.time() < deadline:
            time.sleep(0.1)
        time.sleep(1.0)
        for cols in (100, 60):
            marks.append((offset(), cols))
            hr = resize_pc(hpc, COORD(cols, ROWS))
            if hr:
                errors.append(f"resize to {cols} -> 0x{hr & 0xFFFFFFFF:08x}")
            time.sleep(2.0)
        marks.append((offset(), None))

    child = f'"{sys.executable}" "{CHILD}" 200 default 40'
    raw, _mode = run(conpty, child, "default", during=during, timeout_s=45)
    with open(f"{OUTDIR}/stream-{label}-resize.bin", "wb") as f:
        f.write(raw)

    say("  200 chars written at 80 columns, then resized:")
    for err in errors:
        say(f"    ResizePseudoConsole FAILED: {err}")
    if not marks or marks[0][0] < 200:
        say("    the payload never reached us before the resize; "
            "this run measured nothing")
    start = 0
    for i, (offset, cols) in enumerate(marks):
        piece = raw[start:offset]
        start = offset
        what = ("before any resize" if i == 0
                else f"after resizing to {marks[i - 1][1]}")
        say(f"    {what:<24} {len(piece):>4} bytes, "
            f"{piece.count(CRLF)} CRLF, rows {rows(piece)}")
    tail = raw[start:]
    if tail:
        say(f"    {'after the last resize':<24} {len(tail):>4} bytes, "
            f"{tail.count(CRLF)} CRLF, rows {rows(tail)}")
    say("    a repaint that keeps the line whole shows one long run; "
        "one that splits it")
    say("    shows rows of the new width separated by CRLF")
    say("")


def main():
    os.makedirs(OUTDIR, exist_ok=True)
    report = []

    def say(line=""):
        print(line, flush=True)
        report.append(line)

    say(f"python {sys.version.split()[0]}, pty {COLS}x{ROWS}, "
        f"{TOTAL} chars per run")
    say("'default' leaves the console mode as ConPTY set it; "
        "'legacy' clears VT processing.")
    say("")
    targets = [("system", "system")]
    if len(sys.argv) > 1:
        targets.append(("bundled", os.path.abspath(sys.argv[1])))

    for label, path in targets:
        try:
            probe(label, path, say)
            probe_cmd(label, load_conpty(path), say)
            probe_programs(label, load_conpty(path), say)
            probe_bufapi(label, load_conpty(path), say)
            probe_resize(label, load_conpty(path), say)
        except Exception as exc:
            say(f"  FAILED: {exc}")
            say("")

    with open(f"{OUTDIR}/report.txt", "w", encoding="utf-8") as f:
        f.write("\n".join(report) + "\n")


if __name__ == "__main__":
    main()
