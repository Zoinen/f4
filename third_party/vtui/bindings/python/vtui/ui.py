from typing import Optional, List, Dict, Any

class DialogContext:
    def __init__(self, ui: 'Ui', title: str, w: int, h: int):
        self.ui = ui
        self.title = title
        self.w = w
        self.h = h

    def __enter__(self):
        self.ui._push_container("Dialog", {
            "title": self.title,
            "autoSize": True,
            "center": True,
        }, layout={"type": "VBox", "spacing": 1, "margins": [1, 2, 1, 2]})
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.ui._pop_container()

class Ui:
    """Immediate-mode facade that translates declarative Python function calls into .vui trees & patches."""

    def __init__(self, session):
        self.session = session
        self._container_stack: List[Dict[str, Any]] = []
        self._current_nodes: List[Dict[str, Any]] = []
        self._values: Dict[str, Any] = {}
        self._clicked_ids: set = set()
        self._mounted = False
        self._root_id = "mainDlg"

    def _push_container(self, type_name: str, props: Dict[str, Any], layout: Optional[Dict[str, Any]] = None):
        node = {
            "type": type_name,
            "id": self._root_id if not self._container_stack else f"auto_{len(self._current_nodes)}",
            "props": props,
            "layout": layout or {"type": "VBox", "spacing": 1},
            "children": [],
        }
        self._container_stack.append(node)

    def _pop_container(self):
        if not self._container_stack:
            return
        finished = self._container_stack.pop()
        if self._container_stack:
            self._container_stack[-1]["children"].append(finished)
        else:
            self._current_root = finished

    def _add_element(self, type_name: str, props: Dict[str, Any], elem_id: Optional[str] = None) -> str:
        eid = elem_id or f"auto_{type_name}_{len(self._container_stack[-1]['children'])}"
        node = {
            "type": type_name,
            "id": eid,
            "props": props,
        }
        if self._container_stack:
            self._container_stack[-1]["children"].append(node)
        return eid

    def dialog(self, title: str, w: int = 40, h: int = 10) -> DialogContext:
        """Declares the root dialog window."""
        return DialogContext(self, title, w, h)

    def edit(self, label: str, default: str = "", id: Optional[str] = None) -> str:
        """Declares an Edit input field with an optional buddy Label."""
        edit_id = id or f"edit_{label.replace('&', '').strip()}"
        if edit_id not in self._values:
            self._values[edit_id] = default

        # Form group with label + edit
        group_node = {
            "type": "Group",
            "layout": {"type": "Form", "spacing": 1},
            "children": [
                {"type": "Label", "props": {"text": label, "buddy": edit_id}},
                {"type": "Edit", "id": edit_id, "props": {"text": self._values[edit_id]}},
            ],
        }
        if self._container_stack:
            self._container_stack[-1]["children"].append(group_node)
        return self._values[edit_id]

    def button(self, text: str, id: Optional[str] = None) -> bool:
        """Declares an action button. Returns True if clicked in the last event step."""
        btn_id = id or f"btn_{text.replace('&', '').strip()}"
        cmd_id = 1000 + abs(hash(btn_id)) % 8000
        self._add_element("Button", {"text": text, "command": cmd_id}, btn_id)
        if btn_id in self._clicked_ids:
            self._clicked_ids.remove(btn_id)
            return True
        return False

    def checkbox(self, text: str, value: bool = False, id: Optional[str] = None) -> bool:
        """Declares a Checkbox. Returns its current boolean state."""
        chk_id = id or f"chk_{text.replace('&', '').strip()}"
        if chk_id not in self._values:
            self._values[chk_id] = value
        self._add_element("Checkbox", {"text": text, "state": 1 if self._values[chk_id] else 0}, chk_id)
        return self._values[chk_id]

    def message(self, title: str, text: str, buttons: Optional[List[str]] = None):
        """Opens a modal message dialog."""
        self.session.message(title, text, buttons)

    def _sync(self):
        """Mounts or patches the UI tree into the backend session."""
        if not hasattr(self, "_current_root"):
            return
        if not self._mounted:
            self.session.mount(self._root_id, self._current_root)
            self._mounted = True
        self._clicked_ids.clear()

    def _process_event(self, ev: Dict[str, Any]):
        """Updates internal state based on an upstream event."""
        op = ev.get("op")
        if op == "command":
            src_id = ev.get("srcId", "")
            if src_id:
                self._clicked_ids.add(src_id)
        elif op == "changed":
            src_id = ev.get("id", "")
            val = ev.get("value")
            if src_id:
                self._values[src_id] = val
