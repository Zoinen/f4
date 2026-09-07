import os
import sys

# Allow running directly from examples folder without manual pip install
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

import vtui

def ui(u):
    with u.dialog(" Hello vtui ", w=40):
        name = u.edit("&Name:", "Type here...")
        if u.button("&Ok"):
            u.message(" Result ", f"You typed:\n{name}")

if __name__ == "__main__":
    vtui.run(ui)
