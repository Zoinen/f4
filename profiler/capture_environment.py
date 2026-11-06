"""Read-only guard for the observed Windows desktop profiling confounder."""

import argparse
import csv
import io
import json
import os
from pathlib import Path
import subprocess
import time


OBSERVER_IMAGE = "codex-computer-use.exe"


def parse_observers(output):
    observers = []
    for row in csv.reader(io.StringIO(output), skipinitialspace=True):
        if not row or row[0].strip().casefold() != OBSERVER_IMAGE:
            continue
        if len(row) < 2 or not row[1].strip().isdigit():
            raise ValueError("Observer process query returned an invalid PID")
        observers.append({"image": row[0].strip(), "pid": int(row[1])})
    return observers


def inspect_environment():
    if os.name != "nt":
        return {"supported": False, "cleanDesktopObserverCheck": False,
                "reason": "This guard covers the measured Windows helper only"}
    result = subprocess.run(
        ["tasklist.exe", "/FI", "IMAGENAME eq " + OBSERVER_IMAGE,
         "/FO", "CSV", "/NH"],
        capture_output=True, text=True, check=True,
        creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
    )
    observers = parse_observers(result.stdout)
    return {"supported": True, "checkedAtUnixSeconds": time.time(),
            "observerProcesses": observers,
            "cleanDesktopObserverCheck": not observers,
            "scope": "Point-in-time helper presence; not a continuous observer or frame-time check"}


def require_clean(state, allow_observer=False):
    if not state["supported"]:
        raise RuntimeError(state["reason"])
    if not state["cleanDesktopObserverCheck"] and not allow_observer:
        raise RuntimeError(
            "Computer Use helper is active. Release this task's Computer Use "
            "client with its supported reset tool, then recheck before timing. "
            "Use --allow-observer only for explicitly labelled attribution controls. "
            "This guard never terminates a process."
        )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--allow-observer", action="store_true")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    state = inspect_environment()
    state["attributionControlAllowed"] = args.allow_observer
    if args.output:
        args.output.write_text(json.dumps(state, indent=2), encoding="utf-8")
    print(json.dumps(state, indent=2))
    require_clean(state, args.allow_observer)


if __name__ == "__main__":
    main()
