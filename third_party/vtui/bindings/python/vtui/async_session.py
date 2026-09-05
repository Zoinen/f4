import asyncio
from typing import Callable, Optional
from .session import Session
from .ui import Ui

async def run_async(ui_func: Callable[[Ui], None], host_bin: Optional[str] = None, backend: Optional[str] = None):
    """Runs a reactive vtui application within an asyncio event loop."""
    session = Session(host_bin=host_bin, backend=backend)
    session.start()
    u = Ui(session)

    loop = asyncio.get_running_loop()
    stop_event = asyncio.Event()

    def on_read():
        ev = session.recv(timeout=0)
        if ev:
            if ev.get("op") == "closed" and ev.get("frameId") == u._root_id:
                stop_event.set()
                return
            u._process_event(ev)
            ui_func(u)
            u._sync()

    loop.add_reader(session.fileno(), on_read)

    try:
        # Initial render
        ui_func(u)
        u._sync()
        await stop_event.wait()
    finally:
        loop.remove_reader(session.fileno())
        session.quit()
        session.close()
