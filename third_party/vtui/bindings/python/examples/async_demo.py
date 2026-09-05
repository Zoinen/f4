import os
import sys
import asyncio

# Allow running directly from examples folder without manual pip install
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

import vtui

def ui(u):
    with u.dialog(" Async Demo ", w=40):
        name = u.edit("&User:", "Alice")
        if u.button("&Submit"):
            u.message(" Welcome ", f"Hello async user: {name}")

async def main():
    await vtui.run_async(ui)

if __name__ == "__main__":
    asyncio.run(main())
