#!/usr/bin/env python3
"""Headless smoke check of an f4 binary for the extra-lite/OpenWrt profile.

Starts the binary on a pseudo-terminal (no GUI, no display server), waits for
the panel to list a directory made for the run, moves the cursor into a
subdirectory with the keyboard, quits, and reports the start-up time, the peak
resident set size and the binary size. Exit status 0 only when the panel
listing and the navigation were seen. Standard library only (Linux/macOS).

Usage: openwrt_smoke.py /path/to/f4
"""
import fcntl
import os
import pty
import select
import signal
import struct
import sys
import tempfile
import termios
import time

ROWS, COLS = 40, 120


def rss_kb(pid):
    try:
        with open("/proc/%d/status" % pid) as f:
            for line in f:
                if line.startswith("VmRSS:"):
                    return int(line.split()[1])
    except OSError:
        pass
    return 0


def main():
    binary = os.path.abspath(sys.argv[1])
    root = tempfile.mkdtemp(prefix="f4smoke")
    for name in ("alpha.txt", "beta.txt"):
        with open(os.path.join(root, name), "w") as f:
            f.write(name + "\n")
    os.mkdir(os.path.join(root, "subdir"))
    with open(os.path.join(root, "subdir", "inner.txt"), "w") as f:
        f.write("inner\n")
    home = os.path.join(root, "home")
    os.mkdir(home)

    started = time.monotonic()
    pid, fd = pty.fork()
    if pid == 0:
        fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
        os.chdir(root)
        env = dict(os.environ, HOME=home, TERM="xterm-256color", XDG_CONFIG_HOME=home,
                   LANG="C.UTF-8", COLUMNS=str(COLS), LINES=str(ROWS))
        os.execve(binary, [binary], env)

    out = b""
    peak = 0
    first = None
    listed = None
    navigated = False
    sent_nav = False
    sent_quit = False
    deadline = time.monotonic() + 40
    while time.monotonic() < deadline:
        peak = max(peak, rss_kb(pid))
        r, _, _ = select.select([fd], [], [], 0.1)
        if r:
            try:
                data = os.read(fd, 65536)
            except OSError:
                break
            if not data:
                break
            if first is None:
                first = time.monotonic() - started
            out += data
        if listed is None and b"alpha.txt" in out and b"beta.txt" in out:
            listed = time.monotonic() - started
        if listed is not None and not sent_nav:
            time.sleep(0.5)
            mark = len(out)
            # The command line takes the same "cd" in f4 and in mc; cursor keys
            # differ between terminal modes, a typed command does not.
            os.write(fd, b"cd subdir")
            time.sleep(0.3)
            os.write(fd, b"\r")
            sent_nav = True
            nav_from = mark
        if sent_nav and not navigated and b"inner.txt" in out[nav_from:]:
            navigated = True
        if navigated and not sent_quit:
            time.sleep(0.3)
            os.write(fd, b"\x1b[21~")  # F10
            time.sleep(0.5)
            os.write(fd, b"\r")
            sent_quit = True
            quit_at = time.monotonic()
        if sent_quit and time.monotonic() - quit_at > 5:
            break
    peak = max(peak, rss_kb(pid))
    try:
        os.kill(pid, signal.SIGKILL)
    except OSError:
        pass
    try:
        os.waitpid(pid, 0)
    except OSError:
        pass

    print("binary_bytes=%d" % os.path.getsize(binary))
    print("first_output_ms=%s" % ("%.0f" % (first * 1000) if first is not None else "none"))
    print("panel_listed_ms=%s" % ("%.0f" % (listed * 1000) if listed is not None else "none"))
    print("peak_rss_kb=%d" % peak)
    print("navigated=%s" % navigated)
    if listed is None or not navigated:
        sys.stdout.write("last output:\n%r\n" % out[-1500:])
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
