import sys
from typing import Callable, Optional

from .session import Session, VtuiError
from .ui import Ui
from .async_session import run_async
from ._props import COMMANDS, PALETTE_ROLES, WIDGETS

def log(*args, **kwargs):
    """Writes diagnostic messages to stderr or log file without interfering with the TUI screen."""
    print("[VTUI_LOG]", *args, file=sys.stderr, **kwargs)

def run(ui_func: Callable[[Ui], None], host_bin: Optional[str] = None, backend: Optional[str] = None):
    """Main entry point for running a declarative vtui UI in Python."""
    session = Session(host_bin=host_bin, backend=backend)
    session.start()
    u = Ui(session)

    try:
        # First frame run (mounts tree)
        ui_func(u)
        u._sync()

        # Event loop
        for ev in session.events():
            if ev.get("op") == "closed" and ev.get("frameId") == u._root_id:
                break
            u._process_event(ev)
            ui_func(u)
            u._sync()
    finally:
        session.quit()
        session.close()

__all__ = [
    "run",
    "run_async",
    "log",
    "Session",
    "Ui",
    "VtuiError",
    "COMMANDS",
    "PALETTE_ROLES",
    "WIDGETS",
]
