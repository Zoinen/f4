"""Write 200 'A' to CONOUT$ in <n>-char WriteConsoleW calls, nothing else.

    python tools/conpty_probe_child.py <n> default|legacy|bufapi [hold] [total]

With hold-seconds the child stays alive after writing, so the parent can
resize the pseudoconsole underneath it and watch what gets repainted.

"default" leaves the console mode exactly as ConPTY handed it over, which
is what an application that never calls SetConsoleMode gets. "legacy"
clears ENABLE_VIRTUAL_TERMINAL_PROCESSING first, which routes the write
through WriteCharsLegacy instead of the passthrough path. "bufapi" drops
WriteConsoleW altogether and stamps the characters into the screen buffer
with WriteConsoleOutputCharacterW, the way a TUI application paints. That
path never reaches WriteChars at all, and conhost has no notion of a
logical line there - only cells - so it is the case where the wrap
information may genuinely not exist upstream either.

No CR, no LF, no escape sequence in the payload, so any line break in the
ConPTY output came from ConPTY.

Exits with the console mode that was in effect, so the parent can report it
without writing anything into the stream. 0xFFFF means the run failed.
"""

import ctypes
import sys
import time
from ctypes import wintypes

TOTAL = 200
VT_PROCESSING = 0x0004
FULL_MODE = 0x0007  # processed output | wrap at EOL | VT processing
FAILED = 0xFFFF


class COORD(ctypes.Structure):
    _fields_ = [("X", ctypes.c_short), ("Y", ctypes.c_short)]

k32 = ctypes.WinDLL("kernel32", use_last_error=True)
k32.CreateFileW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD,
                            ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD,
                            wintypes.HANDLE]
k32.CreateFileW.restype = wintypes.HANDLE  # a HANDLE is 64-bit; without this
                                           # ctypes truncates it to an int
k32.GetConsoleMode.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)]
k32.SetConsoleMode.argtypes = [wintypes.HANDLE, wintypes.DWORD]
k32.WriteConsoleW.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                              ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]
k32.WriteConsoleOutputCharacterW.argtypes = [
    wintypes.HANDLE, wintypes.LPCWSTR, wintypes.DWORD, COORD,
    ctypes.POINTER(wintypes.DWORD)]

n = int(sys.argv[1])
api = sys.argv[2] if len(sys.argv) > 2 else "default"
want_legacy = api == "legacy"
use_buffer = api == "bufapi"
# "setmode" writes nothing: it only puts the screen buffer back into the
# mode ConPTY hands out, so a program cmd starts afterwards runs in it.
# cmd gives its children 0x0001 - no wrap at EOL, no VT - and in that mode
# nothing wraps and the measurement is meaningless.
set_full = api == "setmode"
hold = float(sys.argv[3]) if len(sys.argv) > 3 else 0.0
if len(sys.argv) > 4:
    TOTAL = int(sys.argv[4])

h = k32.CreateFileW("CONOUT$", 0xC0000000, 3, None, 3, 0, None)
if h == wintypes.HANDLE(-1).value:
    sys.exit(FAILED)

mode = wintypes.DWORD()
k32.GetConsoleMode(h, ctypes.byref(mode))
if set_full:
    k32.SetConsoleMode(h, FULL_MODE)
    k32.GetConsoleMode(h, ctypes.byref(mode))
    sys.exit(mode.value)
if want_legacy:
    k32.SetConsoleMode(h, mode.value & ~VT_PROCESSING)
    k32.GetConsoleMode(h, ctypes.byref(mode))

text = "A" * TOTAL
if use_buffer:
    # Cells straight into the buffer at (0, 0). No cursor movement, no
    # WriteChars, nothing that could set a wrap bit.
    written = wintypes.DWORD()
    if not k32.WriteConsoleOutputCharacterW(h, text, len(text), COORD(0, 0),
                                            ctypes.byref(written)):
        sys.exit(FAILED)
else:
    for i in range(0, TOTAL, n):
        chunk = text[i:i + n]
        written = wintypes.DWORD()
        if not k32.WriteConsoleW(h, ctypes.create_unicode_buffer(chunk),
                                 len(chunk), ctypes.byref(written), None):
            sys.exit(FAILED)

if hold:
    time.sleep(hold)

sys.exit(mode.value)
