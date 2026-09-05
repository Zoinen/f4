#!/usr/bin/env python3
"""Scenarios for tools/ttytest, run as `./scenarios.py [name ...]`.

Two kinds live here. The checks return pass/fail and are worth keeping
green -- they are the regressions from issues #518 and #863. The probes just print
what happened and leave the reading to you; they are for the situations
where nobody has decided yet what the right answer is.
"""

import os
import sys

from ttytest import (Session, default_home, default_sandbox, DOWN, ENTER, ESC,
                     F4, F6, F7, RIGHT, SHIFT_F4, TAB)

CTRL_O = b"\x0f"


def _session(**kw):
    """A session over a sandbox that already holds a file to edit.

    The file has to exist before f4 starts: the panel reads the directory
    once on startup, so anything created afterwards is not in the listing
    and DOWN does not land where the scenario expects.
    """
    work = default_sandbox()
    os.makedirs(work, exist_ok=True)
    with open(os.path.join(work, "a.txt"), "w") as f:
        f.write("line one\nline two\nline three\n")
    return Session(**kw)


def check_caret_not_under_dialog():
    """A modal dialog must not leave the editor's caret showing through it.

    The regression from issue #518: frames paint bottom-up, and the editor
    used to keep claiming the caret from underneath. Normally the dialog's
    own focused field overwrote that, but with focus on a checkbox there
    was nothing to overwrite it with, and the caret stayed painted in the
    text the dialog was covering.
    """
    with _session() as s:
        s.send(DOWN, 0.5)
        s.send(F4, 0.3)
        if not s.wait_for(lambda s: s.has_text("a.txt", row=0)):
            return False, "the editor did not open"
        if not s.caret()[2]:
            return False, "the editor opened without a caret"
        s.send(DOWN, 0.4)
        s.send(RIGHT * 3, 0.6)
        editor_caret = s.caret()[:2]

        s.send(F7, 0.3)          # Search
        if not s.wait_for(lambda s: s.has_text("Search")):
            return False, "the search dialog did not open"
        s.send(b"one", 0.6)
        if not s.caret()[2]:
            return False, "the search field opened without a caret"
        s.send(RIGHT, 0.9)       # at the end of the text: focus leaves the field

        row, col, visible = s.caret()
        if visible and (row, col) == editor_caret:
            return False, "caret is drawn at %s, in the text under the dialog" % (
                editor_caret,)
        return True, "caret not drawn under the dialog"


def check_caret_follows_focus():
    """Wherever a text field holds focus, its caret is visible."""
    steps = [
        ("panels, command line", None, 1.0),
        ("F7 create folder", F7, 1.2),
        ("typing", b"abc", 0.6),
        ("back to panels", ESC, 0.8),
        ("F6 rename", F6, 1.2),
        ("typing", b"abc", 0.6),
        ("back to panels", ESC, 0.8),
    ]
    with _session() as s:
        for label, keys, wait in steps:
            if keys:
                s.send(keys, wait)
            else:
                s.pump(wait)
            if not s.caret()[2]:
                return False, "no caret at: %s" % label

        s.send(DOWN, 0.5)
        s.send(F4, 0.3)
        if not s.wait_for(lambda s: s.has_text("a.txt", row=0)):
            return False, "the editor did not open"
        if not s.caret()[2]:
            return False, "no caret in the editor"
        s.send(F7, 0.3)
        if not s.wait_for(lambda s: s.has_text("Search")):
            return False, "the search dialog did not open"
        if not s.caret()[2]:
            return False, "no caret in the editor's search dialog"
        s.send(ESC, 0.8)
        if not s.caret()[2]:
            return False, "the editor did not get its caret back"
        return True, "caret present wherever a field is focused"


def check_caret_survives_long_input():
    """Filling a field past its width must not lose the caret.

    SetCursorPos used to clear the caret's visibility when asked for an
    out-of-range column, which made this depend on the order a widget
    happened to call the two setters in.
    """
    with _session() as s:
        s.send(F7, 0.3)
        if not s.wait_for(lambda s: s.has_text("Create Folder")):
            return False, "the create folder dialog did not open"
        for _ in range(12):
            s.send(b"abcdefghij", 0.35)
            if not s.caret()[2]:
                return False, "caret lost while filling the field"
        row, col, _ = s.caret()
        if col >= s.cols:
            return False, "caret parked off-screen at column %d" % col
        return True, "caret survived %d characters" % 120


# The native prompt the sandbox shell prints. It is deliberately unlike the
# one f4 draws, so a native prompt that stays visible is easy to tell apart.
ISSUE863_PS1 = "[nz@nz-en 000tmp]$ "


def check_cat_keeps_last_line():
    """`cat` on a file without a trailing newline shows the whole file.

    Issue #863, driven through a real bash rather than a mock shell: the
    unit tests in cmd/f4 feed the parser a canned byte sequence, and the
    Windows investigation never had a Linux to run on. The last line of
    the file leaves the shell cursor mid-row, the prompt lands on that same
    row, and f4's command line -- painted over the bottom row to hide the
    native prompt -- used to take the last line down with it. Toggling
    Ctrl+O is part of the recipe because that is when the reporter saw the
    row shift.
    """
    work = default_sandbox()
    home = default_home()
    os.makedirs(work, exist_ok=True)
    os.makedirs(home, exist_ok=True)
    with open(os.path.join(work, "cat_tst"), "w") as f:
        f.write('#!/bin/bash\necho "test text"')          # no trailing LF
    with open(os.path.join(home, ".bashrc"), "w") as f:
        f.write("PS1=%r\n" % ISSUE863_PS1)

    native = ISSUE863_PS1.rstrip()
    with Session(workdir=work, home=home, env={"SHELL": "/bin/bash"}) as s:
        if not s.wait_for(lambda s: s.has_text("Help")):
            return False, "f4 did not reach the panels"
        s.send(b"cat cat_tst" + ENTER, 2.5)
        if not s.wait_for(lambda s: s.has_text("Help")):
            return False, "the command did not finish"

        for step in range(3):                     # Ctrl+O, and twice more
            s.send(CTRL_O, 1.0)
            if step % 2 == 1:
                continue                          # panels are back; nothing to read
            rows = [line.rstrip() for line in s.screen.display]
            if not any(line == '#!/bin/bash' for line in rows):
                return False, "first line of the file is not on screen"
            if not any(line == 'echo "test text"' for line in rows):
                return False, ("last line of the file (no trailing newline) "
                               "is not on screen after %d Ctrl+O" % (step + 1))
            stray = [line for line in rows
                     if line.startswith(native) and not line.startswith(native + " cat ")]
            if stray:
                return False, "native prompt left visible: %r" % stray[0]
        return True, "both lines stay visible, native prompt stays covered"


def probe_boundary_focus():
    """Left/Right at the edge of the text hands focus to the next control.

    Deliberate -- see UX_GUIDELINES.md, Tier 2, and docs/CURSOR.md section
    2 -- and the reason the caret disappears in the create/rename dialogs.
    Printed rather than asserted, since what should happen here is a design
    question and not a settled one.
    """
    with _session() as s:
        s.send(F7, 1.2)
        s.send(b"abc", 0.6)
        s.status("typed abc")
        s.send(RIGHT, 0.8)
        s.status("Right at end of text")
        print("     still takes input: %s" % s.types_into_field())
        s.send(TAB, 0.6)
        s.status("Tab")
        s.dump("dialog", rows=(9, 20))


def probe_idle():
    """What f4 sends while the user is not typing.

    An idle app should say nothing at all; anything here is either a
    keepalive or a repaint nobody asked for.
    """
    with _session() as s:
        s.send(F7, 1.2)
        s.status("dialog open")
        for i in range(3):
            s.idle(5.0)
            print("     after %2ds idle: wire:%s  sequences: %s"
                  % ((i + 1) * 5, s.caret_on_wire(), s.caret_sequences()[-6:]))


def probe_new_file():
    """Shift+F4, the third dialog named in the issue."""
    with _session() as s:
        s.send(DOWN, 0.5)
        s.send(SHIFT_F4, 1.3)
        s.status("create new file")
        s.send(b"zz.txt", 0.6)
        s.status("typed a name")
        s.send(ENTER, 2.0)
        s.status("editor opened")
        s.dump("editor", rows=(0, 8))


CHECKS = {
    "caret-not-under-dialog": check_caret_not_under_dialog,
    "caret-follows-focus": check_caret_follows_focus,
    "caret-survives-long-input": check_caret_survives_long_input,
    "cat-keeps-last-line": check_cat_keeps_last_line,
}

PROBES = {
    "boundary-focus": probe_boundary_focus,
    "idle": probe_idle,
    "new-file": probe_new_file,
}


def main(argv):
    names = argv[1:]
    failed = 0

    for name, fn in CHECKS.items():
        if names and name not in names:
            continue
        ok, note = fn()
        print("%-28s %s  %s" % (name, "ok  " if ok else "FAIL", note))
        if not ok:
            failed += 1

    for name, fn in PROBES.items():
        if name not in names:
            continue
        print("== %s ==" % name)
        fn()

    if names and not (set(names) & (set(CHECKS) | set(PROBES))):
        print("nothing matched. checks: %s | probes: %s"
              % (", ".join(CHECKS), ", ".join(PROBES)))
        return 2
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
