import os
import sys
import unittest

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

from vtui.ui import Ui
from vtui._props import VOCABULARY, COMMANDS, WIDGETS

class MockSession:
    def __init__(self):
        self.mounted = []
        self.messages = []

    def mount(self, frame_id, tree):
        self.mounted.append((frame_id, tree))

    def message(self, title, text, buttons):
        self.messages.append((title, text, buttons))

class TestVtuiPython(unittest.TestCase):
    def test_vocabulary_imported(self):
        self.assertIn("Button", WIDGETS)
        self.assertIn("Dialog", WIDGETS)
        self.assertIn("CmQuit", COMMANDS)
        self.assertEqual(VOCABULARY["version"], 1)

    def test_immediate_mode_ui_structure(self):
        session = MockSession()
        u = Ui(session)

        with u.dialog(" Test Dialog ", w=40):
            val = u.edit("&Name:", "Alice")
            btn_clicked = u.button("&Save")
            self.assertEqual(val, "Alice")
            self.assertFalse(btn_clicked)

        u._sync()

        self.assertEqual(len(session.mounted), 1)
        frame_id, tree = session.mounted[0]
        self.assertEqual(frame_id, "mainDlg")
        self.assertEqual(tree["type"], "Dialog")
        self.assertEqual(tree["props"]["title"], " Test Dialog ")
        self.assertEqual(len(tree["children"]), 2)

if __name__ == "__main__":
    unittest.main()
