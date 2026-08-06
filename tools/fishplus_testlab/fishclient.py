"""A minimal FISH+ client, mirroring plugins/netfox/fishplus/session.go.

It exists so the helper script can be driven against a real shell without
the Go side in the way, and with no network in between. That second part is
what makes it useful: over ssh the remote shell almost always wins the races
this protocol has with itself, and a failure looks like a rare hang. Here
the race is lost nearly every time.

Framing, escaping and the two step bootstrap follow session.go.
"""

import base64
import os
import subprocess


def compact(src):
    """Strip comments and indentation, the way script.go does before upload."""
    out = []
    for line in src.split("\n"):
        t = line.lstrip(" \t")
        if t == "" or t.startswith("#"):
            continue
        out.append(t)
    return "\n".join(out) + "\n"


def encode_path_line(p):
    if p == "" or p.startswith("~") or "\r" in p or "\n" in p:
        return "~" + base64.b64encode(p.encode()).decode()
    return p


class Remote:
    """One session against a shell started as a subprocess."""

    def __init__(self, helper_path, shell="/bin/sh", token="deadbeefcafe0001", env=None):
        self.token = token
        src = compact(open(helper_path).read()).replace("__F4_TOKEN__", token)
        e = dict(os.environ)
        if env:
            e.update(env)
        self.p = subprocess.Popen(
            [shell], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, env=e,
        )
        self.seq = 0

        # Two step upload. The shell parses one line, says it is running, and
        # only then is the script fed to its read builtin; sending both at
        # once is what used to make dash execute the first request as code.
        boot = ('echo F4R"DY"%s; F4NL=$(printf \'\\nx\'); F4NL=${F4NL%%x}; F4S=; '
                'while IFS= read -r F4L; do [ "$F4L" = F4EOF ] && break; '
                'F4S=$F4S$F4L$F4NL; done; eval "$F4S"\n') % token
        self.p.stdin.write(boot.encode())
        self.p.stdin.flush()
        marker = ("F4RDY" + token).encode()
        while True:
            line = self.p.stdout.readline()
            if line == b"":
                raise EOFError("the shell never reported being ready")
            if marker in line:
                break
        self.p.stdin.write(src.encode() + b"F4EOF\n")
        self.p.stdin.flush()
        self.banner = self._read_response(0, False)

    def _readline(self):
        line = self.p.stdout.readline()
        if line == b"":
            raise EOFError("remote shell closed the stream")
        return line.rstrip(b"\r\n")

    def _read_response(self, ident, binary):
        prefix = ("." + self.token + " " + str(ident) + " ").encode()
        lines, data = [], b""
        while True:
            line = self._readline()
            if ident == 0:
                at = line.find(prefix)
                if at > 0:
                    line = line[at:]
            if line.startswith(prefix):
                rest = line[len(prefix):].decode(errors="replace").strip()
                status, _, msg = rest.partition(" ")
                return {"status": status, "msg": msg.strip(), "lines": lines, "data": data}
            if ident == 0:
                continue
            if binary and line.startswith(b"#"):
                n = int(line[1:])
                buf = b""
                while len(buf) < n:
                    chunk = self.p.stdout.read(n - len(buf))
                    if not chunk:
                        raise EOFError("short data frame")
                    buf += chunk
                data += buf
                continue
            lines.append(line.decode(errors="replace"))

    def exec(self, cmd, *args, paths=(), binary=False):
        self.seq += 1
        req = " ".join([str(self.seq), cmd] + [str(a) for a in args]) + "\n"
        for p in paths:
            req += encode_path_line(p) + "\n"
        self.p.stdin.write(req.encode())
        self.p.stdin.flush()
        return self._read_response(self.seq, binary)

    def send(self, raw):
        """Write bytes straight onto the wire, for commands with a payload."""
        self.p.stdin.write(raw)
        self.p.stdin.flush()

    def features(self):
        return set(self.banner["msg"].split()[2:])

    def feature_value(self, prefix):
        for f in self.banner["msg"].split():
            if f.startswith(prefix + ":"):
                return f.split(":", 1)[1]
        return ""

    def close(self):
        try:
            self.p.stdin.close()
        except Exception:
            pass
        try:
            self.p.wait(timeout=5)
        except Exception:
            self.p.kill()