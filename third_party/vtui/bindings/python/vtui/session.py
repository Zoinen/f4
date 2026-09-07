import os
import sys
import json
import socket
import select
import subprocess
from typing import Optional, Dict, Any, List, Generator

class VtuiError(Exception):
    """Exception raised for errors returned from the vtui protocol session."""
    def __init__(self, code: str, message: str, reply_to: Optional[int] = None):
        super().__init__(f"[{code}] {message}")
        self.code = code
        self.message = message
        self.reply_to = reply_to

def _find_host_binary(explicit_path: Optional[str] = None) -> str:
    if explicit_path:
        return explicit_path
    if "VTUI_HOST_BIN" in os.environ:
        return os.environ["VTUI_HOST_BIN"]

    # 1. Look in PATH
    path_bin = shutil_which("vtui-host")
    if path_bin:
        return path_bin

    # 2. Check local repository build locations relative to this package
    base = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
    candidates = [
        os.path.join(base, "cmd", "vtui-host", "vtui-host"),
        os.path.join(base, "bindings", "cpp", "build", "vtui-host"),
        os.path.join(base, "bindings", "c", "build", "vtui-host"),
        os.path.join(base, "bindings", "build", "vtui-host"),
        os.path.join(base, "build", "vtui-host"),
        os.path.join(base, "vtui-host"),
        os.path.join(os.path.expanduser("~"), "go", "bin", "vtui-host"),
    ]
    for cand in candidates:
        if os.path.isfile(cand) and os.access(cand, os.X_OK):
            return cand

    # 3. If go compiler is installed, try auto-building into the repo directory
    go_bin = shutil_which("go")
    if go_bin and os.path.isdir(os.path.join(base, "cmd", "vtui-host")):
        target = os.path.join(base, "vtui-host")
        try:
            subprocess.run([go_bin, "build", "-o", target, "./cmd/vtui-host"], cwd=base, check=True, capture_output=True)
            if os.path.isfile(target):
                return target
        except Exception:
            pass

    return "vtui-host"

def shutil_which(cmd: str) -> Optional[str]:
    import shutil
    return shutil.which(cmd)

class Session:
    """Thin client session managing JSON Lines wire protocol to a vtui-host process."""

    def __init__(self, host_bin: Optional[str] = None, backend: Optional[str] = None):
        self._seq = 0
        self._host_bin = _find_host_binary(host_bin)
        self._backend = backend or os.environ.get("VTUI_BACKEND", "ansi")
        self._proc: Optional[subprocess.Popen] = None
        self._sock: Optional[socket.socket] = None
        self._reader = None
        self._writer = None
        self._state: Dict[str, Any] = {}
        self.welcome_info: Dict[str, Any] = {}

    def start(self):
        """Starts the vtui-host subprocess and opens the communication channel."""
        # Create a socketpair for IPC communication
        parent_sock, child_sock = socket.socketpair(socket.AF_UNIX, socket.SOCK_STREAM)

        cmd = [self._host_bin, f"--protocol-fd={child_sock.fileno()}", f"--backend={self._backend}"]

        env = os.environ.copy()
        if "VTUI_TRACE" in os.environ:
            env["VTUI_TRACE"] = os.environ["VTUI_TRACE"]

        self._proc = subprocess.Popen(
            cmd,
            pass_fds=[child_sock.fileno()],
            env=env,
            stdin=sys.stdin,
            stdout=sys.stdout,
            stderr=sys.stderr,
        )
        child_sock.close()

        self._sock = parent_sock
        self._reader = self._sock.makefile("r", encoding="utf-8", buffering=1)
        self._writer = self._sock.makefile("w", encoding="utf-8", buffering=1)

        # Handshake: hello -> welcome
        self.send({"op": "hello", "version": 1})
        resp = self.recv()
        if resp.get("op") == "welcome":
            self.welcome_info = resp
        elif resp.get("op") == "error":
            raise VtuiError(resp.get("code", "HANDSHAKE_FAILED"), resp.get("message", ""))

    def fileno(self) -> int:
        """Returns the underlying socket file descriptor for select/asyncio integration."""
        return self._sock.fileno()

    def send(self, msg: Dict[str, Any]) -> int:
        """Sends a JSON command down to the kernel."""
        self._seq += 1
        if "seq" not in msg:
            msg["seq"] = self._seq
        line = json.dumps(msg) + "\n"
        self._writer.write(line)
        self._writer.flush()
        return self._seq

    def recv(self, timeout: Optional[float] = None) -> Optional[Dict[str, Any]]:
        """Reads the next event/message from the kernel."""
        if timeout is not None:
            r, _, _ = select.select([self._sock], [], [], timeout)
            if not r:
                return None

        line = self._reader.readline()
        if not line:
            return None
        msg = json.loads(line)
        if msg.get("op") == "error":
            raise VtuiError(msg.get("code", "ERROR"), msg.get("message", ""), msg.get("replyTo"))
        return msg

    def mount(self, frame_id: str, tree: Dict[str, Any]):
        """Mounts a complete .vui tree."""
        self.send({"op": "mount", "frameId": frame_id, "tree": tree})

    def patch(self, frame_id: str, ops: List[Dict[str, Any]]):
        """Applies a list of atomic tree patches."""
        self.send({"op": "patch", "frameId": frame_id, "ops": ops})

    def message(self, title: str, text: str, buttons: Optional[List[str]] = None):
        """Displays a message box dialog."""
        self.send({"op": "message", "title": title, "text": text, "buttons": buttons or ["&Ok"]})

    def quit(self):
        """Signals the session to terminate."""
        try:
            self.send({"op": "quit"})
        except Exception:
            pass

    def close(self):
        """Closes sockets and waits for the process to exit."""
        if self._sock:
            try:
                self._sock.close()
            except Exception:
                pass
            self._sock = None
        if self._proc:
            try:
                self._proc.wait(timeout=1.0)
            except Exception:
                self._proc.kill()
            self._proc = None

    def events(self, timeout: Optional[float] = None) -> Generator[Dict[str, Any], None, None]:
        """Yields incoming events continuously."""
        while True:
            ev = self.recv(timeout)
            if ev is None:
                if timeout is not None:
                    return
                break
            yield ev
