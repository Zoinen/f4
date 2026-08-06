#!/usr/bin/env python3
"""Drive the patch command against a real shell on every write backend.

Usage: test_patch.py [<shell>] [<PATH prefix>]

The interesting checks are not that the right file comes out, which the Go
tests cover too, but that a refused request still takes its payload off the
wire and that the session answers afterwards. A helper that desynchronizes
there would parse the next request out of the middle of a file.
"""

import base64
import os
import random
import sys
import tempfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from fishclient import Remote, encode_path_line  # noqa: E402

HELPER = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                      "..", "..", "plugins", "netfox", "fishplus", "helper.sh")
fails = []


def check(cond, what):
    if not cond:
        fails.append(what)
        print("  FAIL: " + what)


def patch(r, src, dst, segs, enc="raw"):
    """segs: ('S', off, len) for a range of src, ('D', bytes) for new data."""
    r.seq += 1
    req = "%d patch %d %s\n%s\n%s\n" % (r.seq, len(segs), enc,
                                        encode_path_line(src), encode_path_line(dst))
    body = req.encode()
    for seg in segs:
        if seg[0] == "S":
            body += ("S %d %d\n" % (seg[1], seg[2])).encode()
        else:
            body += ("D %d\n" % len(seg[1])).encode()
            body += base64.b64encode(seg[1]) + b"\n" if enc == "b64" else seg[1]
    r.send(body)
    return r._read_response(r.seq, False)


def main():
    shell = sys.argv[1] if len(sys.argv) > 1 else "/bin/sh"
    env = {"PATH": sys.argv[2] + ":" + os.environ["PATH"]} if len(sys.argv) > 2 else {}

    root = tempfile.mkdtemp()
    old = random.Random(5).randbytes(200000)
    src = os.path.join(root, "an old file.bin")
    dst = os.path.join(root, "new one.bin")
    open(src, "wb").write(old)

    r = Remote(HELPER, shell=shell, env=env)
    print("listing: %s  read: %s  write: %s" % (r.feature_value("mode"),
                                                r.feature_value("read"),
                                                r.feature_value("write")))
    modes = [m for m in ("ddbytes", "b64", "ddbs1") if r.exec("wmode", m)["status"] == "ok"]
    print("write backends: " + " ".join(modes))

    for mode in modes:
        r.exec("wmode", mode)
        enc = "b64" if mode == "b64" else "raw"
        tag = "backend " + mode

        # A one byte change in the middle of a large file: two ranges of the
        # original and one literal, which is what the command exists for.
        resp = patch(r, src, dst, [("S", 0, 100000), ("D", b"X"), ("S", 100001, 99999)], enc)
        check(resp["status"] == "ok", tag + ": one byte edit failed: " + resp["msg"])
        check(open(dst, "rb").read() == old[:100000] + b"X" + old[100001:],
              tag + ": one byte edit produced the wrong file")

        # Insertion, deletion, reordering, and a literal larger than a block.
        blob = random.Random(9).randbytes(70000)
        resp = patch(r, src, dst,
                     [("S", 50000, 10000), ("D", blob), ("S", 0, 1), ("S", 199999, 1)], enc)
        check(resp["status"] == "ok", tag + ": mixed segments failed: " + resp["msg"])
        check(open(dst, "rb").read() == old[50000:60000] + blob + old[0:1] + old[199999:],
              tag + ": mixed segments produced the wrong file")

        resp = patch(r, src, dst, [("D", b"only new bytes\n")], enc)
        check(open(dst, "rb").read() == b"only new bytes\n", tag + ": literal only")
        resp = patch(r, src, dst, [], enc)
        check(resp["status"] == "ok" and open(dst, "rb").read() == b"", tag + ": empty result")

        # A refusal must still drain, or the next request is parsed out of
        # the middle of the payload.
        resp = patch(r, src, os.path.join(root, "no such dir", "x"),
                     [("S", 0, 10), ("D", b"payload that must be drained")], enc)
        check(resp["status"] == "err", tag + ": a bad destination was accepted")
        check("D" in resp["lines"], tag + ": the refusal did not report draining")
        check(r.exec("noop")["status"] == "ok", tag + ": session out of sync after a refusal")

        resp = patch(r, src + ".missing", dst, [("D", b"x")], enc)
        check(resp["status"] == "err", tag + ": a missing original was accepted")
        check(r.exec("noop")["status"] == "ok", tag + ": out of sync after a missing original")

    r.close()
    print("%d failures" % len(fails))
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())