"""Drive f4 in a real pty and see what the terminal was actually told.

Rendering bugs are awkward to chase from inside the process: the screen
buffer can be perfectly correct while the bytes on the wire are not, and
the interesting part -- where the caret ended up, whether it is visible at
all -- lives in escape sequences that never appear in any Go-side state.

So this runs the real binary under a pty and reads the same two things a
user does, from opposite ends:

  * the screen, by feeding the output to a terminal emulator (pyte), which
    gives text, cell attributes, and the caret's position and visibility;
  * the wire, by scanning the raw bytes for the sequences f4 emitted --
    ESC[?25h / ESC[?25l for the caret, CUP for where it was parked,
    DECSCUSR for its shape.

Both matter. "The caret is missing" can mean f4 hid it, or that f4 never
positioned it, or that the frame was truncated before the show sequence, or
that focus legitimately moved to a control that has no caret -- and those
look identical on screen while looking nothing alike on the wire.

Needs pyte:  pip install pyte

Usage as a library:

    from ttytest import Session, F7, RIGHT

    s = Session()                  # starts f4, waits for the first frame
    s.send(F7)                     # press F7
    s.status("mkdir dialog")       # one line: caret state and position
    s.dump("screen", rows=(9, 20)) # the screen, as text
    s.close()

See scenarios.py for ready-made cases and README.md for the workflow.
"""

import fcntl
import glob
import os
import pty
import re
import select
import shutil
import signal
import struct
import subprocess
import sys
import termios
import time

try:
    import pyte
except ImportError:  # pragma: no cover - a helper script, not a test target
    sys.exit("ttytest needs pyte: pip install pyte")

# Function and navigation keys, as a terminal in its default mode sends them.
# f4 also speaks the kitty keyboard protocol, but that is only negotiated
# with terminals that answer the query, and our pty answers nothing, so the
# legacy encodings below are what it will parse.
F1 = b"\x1bOP"
F2 = b"\x1bOQ"
F3 = b"\x1bOR"
F4 = b"\x1bOS"
F5 = b"\x1b[15~"
F6 = b"\x1b[17~"
F7 = b"\x1b[18~"
F8 = b"\x1b[19~"
F9 = b"\x1b[20~"
F10 = b"\x1b[21~"
SHIFT_F4 = b"\x1b[1;2S"
ENTER = b"\r"
ESC = b"\x1b"
TAB = b"\t"
BACKSPACE = b"\x7f"
DELETE = b"\x1b[3~"
UP = b"\x1b[A"
DOWN = b"\x1b[B"
RIGHT = b"\x1b[C"
LEFT = b"\x1b[D"
HOME = b"\x1b[1~"
END = b"\x1b[4~"
PGUP = b"\x1b[5~"
PGDN = b"\x1b[6~"

_CURSOR_VIS = re.compile(rb"\x1b\[\?25([hl])")
_CURSOR_POS = re.compile(rb"\x1b\[(\d+);(\d+)H")
_CURSOR_SHAPE = re.compile(rb"\x1b\[(\d+) q")


def default_sandbox():
    """The directory f4 is started in -- put scenario fixtures here.

    Deliberately not the same as HOME: f4 keeps its config under HOME, and
    if the two coincide the panel lists .config alongside the fixtures and
    every keystroke that walks the listing lands somewhere unexpected.
    """
    here = os.path.dirname(os.path.abspath(__file__))
    return os.path.join(here, "sandbox", "work")


def default_home():
    """The HOME the child gets, so the real config is never touched."""
    here = os.path.dirname(os.path.abspath(__file__))
    return os.path.join(here, "sandbox", "home")


def find_binary():
    """Locate the f4 binary: $F4_BIN, then PATH, then the repo's own build."""
    env = os.environ.get("F4_BIN")
    if env:
        return env
    found = shutil.which("f4")
    if found:
        return found
    here = os.path.dirname(os.path.abspath(__file__))
    built = os.path.join(here, "f4")
    if not os.path.exists(built):
        root = os.path.dirname(os.path.dirname(here))
        subprocess.check_call(["go", "build", "-o", built, "./cmd/f4"], cwd=root)
    return built


class Session:
    """One f4 process on one pty, with a terminal emulator watching it."""

    def __init__(self, workdir=None, home=None, cols=100, rows=30, env=None,
                 settle=2.5):
        self.cols, self.rows = cols, rows
        self.binary = find_binary()
        self.work = workdir or default_sandbox()
        self.home = home or default_home()
        os.makedirs(self.work, exist_ok=True)
        os.makedirs(self.home, exist_ok=True)
        self._clear_stale_sessions()

        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            child = {
                "TERM": "xterm-256color",
                "COLORTERM": "truecolor",
                "LANG": "en_US.UTF-8",
                "HOME": self.home,
                "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
            }
            if env:
                child.update(env)
            os.environ.clear()
            os.environ.update(child)
            os.chdir(self.work)
            os.execv(self.binary, [self.binary])
            os._exit(1)

        fcntl.ioctl(self.fd, termios.TIOCSWINSZ,
                    struct.pack("HHHH", rows, cols, 0, 0))
        self.screen = pyte.Screen(cols, rows)
        self.stream = pyte.Stream(self.screen)
        self.raw = bytearray()    # everything f4 has written
        self.chunk = bytearray()  # everything it wrote since the last send()
        self.pump(settle)

    def _clear_stale_sessions(self):
        """f4 reattaches to a live session if it finds one; we want a fresh
        instance every time, or the first frame is a session picker."""
        for path in glob.glob("/tmp/f4-sessions-*/*"):
            try:
                os.remove(path)
            except OSError:
                pass
        try:
            os.remove(os.path.join(self.home, ".config", "f4", "session.ini"))
        except OSError:
            pass

    def pump(self, seconds):
        """Read for a while, feeding both the emulator and the raw log."""
        deadline = time.time() + seconds
        while time.time() < deadline:
            readable, _, _ = select.select([self.fd], [], [], 0.05)
            if not readable:
                continue
            try:
                data = os.read(self.fd, 65536)
            except OSError:
                return
            if not data:
                return
            self.chunk.extend(data)
            self.raw.extend(data)
            self.stream.feed(data.decode("utf-8", "replace"))

    def send(self, keys, wait=0.7):
        """Press keys and read the frames they produce."""
        self.chunk.clear()
        os.write(self.fd, keys)
        self.pump(wait)

    def idle(self, seconds):
        """Read nothing in particular for a while, discarding earlier output.

        Use this to test what happens when the user stops typing: whatever
        lands in the chunk afterwards was sent by f4 on its own.
        """
        self.chunk.clear()
        self.pump(seconds)

    def wait_for(self, predicate, timeout=10.0, poll=0.1):
        """Read until the screen satisfies predicate, or give up.

        Preferred over a fixed sleep for anything that can be slow once and
        fast afterwards -- the very first start with a fresh HOME builds a
        config and costs seconds that later runs do not.
        """
        deadline = time.time() + timeout
        while time.time() < deadline:
            if predicate(self):
                return True
            self.pump(poll)
        return predicate(self)

    def has_text(self, text, row=None):
        """Is this text on screen, optionally on one particular row?"""
        rows = [self.screen.display[row]] if row is not None \
            else self.screen.display
        return any(text in line for line in rows)

    def caret_on_wire(self):
        """What f4 last told the terminal about the caret, this frame.

        Returns "shown", "hidden", or "unchanged" -- the last being the
        common and correct case where a frame had no reason to touch it.
        """
        seen = _CURSOR_VIS.findall(bytes(self.chunk))
        if not seen:
            return "unchanged"
        return "shown" if seen[-1] == b"h" else "hidden"

    def caret_sequences(self):
        """The cursor-related escapes of this frame, for eyeballing."""
        pattern = (rb"\x1b\[\?25[hl]|\x1b\[\d+;\d+H|\x1b\[\d+ q"
                   rb"|\x1b\]1337;CursorShape=\d\x07|\x1b\[\?2026[hl]")
        return [m.decode("latin1").replace("\x1b", "ESC").replace("\x07", "BEL")
                for m in re.findall(pattern, bytes(self.chunk))]

    def caret(self):
        """Where the caret is on screen, and whether it is drawn at all."""
        c = self.screen.cursor
        return c.y, c.x, not c.hidden

    def cell(self, row, col):
        """A screen cell, attributes included -- focus highlight lives here."""
        return self.screen.buffer[row][col]

    def status(self, label):
        row, col, visible = self.caret()
        print("  %-38s wire:%-10s caret=(%2d,%3d) visible=%s"
              % (label, self.caret_on_wire(), row, col, visible))

    def dump(self, label, rows=None):
        row, col, visible = self.caret()
        print("---- %s | wire:%s | caret=(%d,%d) visible=%s ----"
              % (label, self.caret_on_wire(), row, col, visible))
        lo, hi = rows if rows else (0, self.rows)
        for i in range(lo, min(hi, self.rows)):
            print("%2d|%s|" % (i, self.screen.display[i].rstrip()))

    def types_into_field(self, probe=b"Q"):
        """Is a text field still focused, whatever the caret looks like?

        Types a character and reports whether the screen changed. This is
        the difference that matters: focus moving to a checkbox and the
        caret going missing look the same, but only one of them still
        takes input. See docs/CURSOR.md.
        """
        before = list(self.screen.display)
        self.send(probe, 0.5)
        return before != list(self.screen.display)

    def save_raw(self, path):
        with open(path, "wb") as f:
            f.write(bytes(self.raw))

    def close(self):
        try:
            os.kill(self.pid, signal.SIGKILL)
            os.waitpid(self.pid, 0)
        except OSError:
            pass

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.close()
