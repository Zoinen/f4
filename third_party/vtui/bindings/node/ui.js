class Ui {
  constructor(session) {
    this.session = session;
    this.containerStack = [];
    this.values = {};
    this.clickedIds = new Set();
    this.mounted = false;
    this.rootId = "mainDlg";
    this.currentRoot = null;
  }

  dialog(title, w = 40, callback) {
    const node = {
      type: "Dialog",
      id: this.rootId,
      props: { title, autoSize: true, center: true },
      layout: { type: "VBox", spacing: 1, margins: [1, 2, 1, 2] },
      children: [],
    };
    this.containerStack.push(node);
    if (typeof callback === "function") {
      callback();
    }
    this.currentRoot = this.containerStack.pop();
    return this.currentRoot;
  }

  edit(label, defaultValue = "", id = null) {
    const editId = id || `edit_${label.replace(/&/g, "").trim()}`;
    if (!(editId in this.values)) {
      this.values[editId] = defaultValue;
    }
    const groupNode = {
      type: "Group",
      layout: { type: "Form", spacing: 1 },
      children: [
        { type: "Label", props: { text: label, buddy: editId } },
        { type: "Edit", id: editId, props: { text: this.values[editId] } },
      ],
    };
    if (this.containerStack.length > 0) {
      this.containerStack[this.containerStack.length - 1].children.push(groupNode);
    }
    return this.values[editId];
  }

  button(text, id = null) {
    const btnId = id || `btn_${text.replace(/&/g, "").trim()}`;
    const cmdId = 1000 + Math.abs(this._hash(btnId)) % 8000;
    const node = {
      type: "Button",
      id: btnId,
      props: { text, command: cmdId },
    };
    if (this.containerStack.length > 0) {
      this.containerStack[this.containerStack.length - 1].children.push(node);
    }
    if (this.clickedIds.has(btnId)) {
      this.clickedIds.delete(btnId);
      return true;
    }
    return false;
  }

  checkbox(text, defaultValue = false, id = null) {
    const chkId = id || `chk_${text.replace(/&/g, "").trim()}`;
    if (!(chkId in this.values)) {
      this.values[chkId] = defaultValue;
    }
    const node = {
      type: "Checkbox",
      id: chkId,
      props: { text, state: this.values[chkId] ? 1 : 0 },
    };
    if (this.containerStack.length > 0) {
      this.containerStack[this.containerStack.length - 1].children.push(node);
    }
    return this.values[chkId];
  }

  message(title, text, buttons = ["&Ok"]) {
    this.session.message(title, text, buttons);
  }

  _hash(str) {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
      hash = (hash << 5) - hash + str.charCodeAt(i);
      hash |= 0;
    }
    return hash;
  }

  _sync() {
    if (!this.currentRoot) return;
    if (!this.mounted) {
      this.session.mount(this.rootId, this.currentRoot);
      this.mounted = true;
    }
    this.clickedIds.clear();
  }

  _processEvent(ev) {
    if (!ev) return;
    if (ev.op === "command" && ev.srcId) {
      this.clickedIds.add(ev.srcId);
    } else if (ev.op === "changed" && ev.id) {
      this.values[ev.id] = ev.value;
    }
  }
}

module.exports = { Ui };
