#!/usr/bin/env python3
"""Read a raw terminal log and report what happened to the caret.

For logs a user captured on a machine we cannot reproduce on:

    script -f /tmp/f4.raw -c f4

Frames are split on the synchronized-update marks f4 brackets them with,
and for each frame we report what it did to the caret. What to look for:

  * a frame that never closes -- ours: f4 brackets each painted frame with
    ESC[?2026h ... ESC[?2026l and hides the caret for the duration, so a
    frame missing its closing mark was cut short in transit and left the
    caret hidden with nothing to restore it;
  * long stretches with no output at all while the user says the caret
    vanished -- not ours: we sent nothing, so the terminal did it, and the
    next question is its blink settings;
  * the caret being shown at a position far from where the user was typing
    -- ours, and usually a frame below the top one claiming it.

Usage:  ./analyze_log.py /tmp/f4.raw [--frames]
"""

import re
import sys

FRAME_START = b"\x1b[?2026h"
FRAME_END = b"\x1b[?2026l"
VIS = re.compile(rb"\x1b\[\?25([hl])")
CUP = re.compile(rb"\x1b\[(\d+);(\d+)H")
SHAPE = re.compile(rb"\x1b\[(\d+) q")

SHAPES = {0: "default", 1: "blinking block", 2: "steady block",
          3: "blinking underline", 4: "steady underline",
          5: "blinking bar", 6: "steady bar"}


def frames(data):
    """Split on frame starts, keeping whatever preceded the first one."""
    parts = data.split(FRAME_START)
    out = [parts[0]] if parts[0] else []
    out.extend(FRAME_START + p for p in parts[1:])
    return out


def main(argv):
    if len(argv) < 2:
        sys.exit(__doc__)
    show_frames = "--frames" in argv
    data = open(argv[1], "rb").read()

    chunks = frames(data)
    print("%d bytes, %d frames" % (len(data), len(chunks)))

    visible = None
    pos = None
    shape = None
    hidden_since = None
    problems = []
    notes = []

    for i, frame in enumerate(chunks):
        vis = VIS.findall(frame)
        cup = CUP.findall(frame)
        shp = SHAPE.findall(frame)

        if vis:
            visible = vis[-1] == b"h"
        if cup:
            pos = (int(cup[-1][0]), int(cup[-1][1]))
        if shp:
            shape = int(shp[-1])

        painted = frame.startswith(FRAME_START)
        last = i == len(chunks) - 1
        if painted and FRAME_END not in frame and not last:
            problems.append(
                "frame %d never closed its synchronized update; it was cut "
                "short in transit, which leaves the caret hidden" % i)

        # A hidden caret is not by itself a fault: with focus on a button or
        # a checkbox there is nothing to draw. Worth noticing, not flagging.
        if visible is False and hidden_since is None:
            hidden_since = i
        elif visible:
            if hidden_since is not None and i - hidden_since > 1:
                notes.append("caret was hidden across frames %d..%d"
                             % (hidden_since, i))
            hidden_since = None

        if show_frames:
            print("  frame %4d  %-9s caret=%-10s shape=%s"
                  % (i,
                     "painted" if painted else "cursor-only",
                     ("shown at %s" % (pos,)) if visible else "hidden",
                     SHAPES.get(shape, shape)))

    print("\nfinal state: caret %s%s, shape %s"
          % ("visible" if visible else "hidden",
             (" at row %d col %d" % pos) if pos else "",
             SHAPES.get(shape, shape)))

    if visible is False:
        notes.append("the log ends with the caret hidden -- expected if focus "
                     "was on a button, a checkbox or a combobox")
    if shape in (1, 3, 5):
        print("note: we asked for a blinking cursor (%s), so whether it is "
              "visible at any instant is the terminal's decision"
              % SHAPES[shape])

    print()
    for n in notes:
        print("   " + n)
    if problems:
        for p in problems:
            print("!! " + p)
    else:
        print("nothing obviously wrong on our side: every frame arrived "
              "whole.\nIf the caret still went missing, suspect the terminal, "
              "or focus sitting on a\ncontrol that has no caret -- see "
              "docs/CURSOR.md.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
