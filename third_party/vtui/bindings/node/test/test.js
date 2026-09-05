const assert = require("assert");
const { Ui } = require("../ui");

class MockSession {
  constructor() {
    this.mounted = [];
    this.messages = [];
  }
  mount(frameId, tree) {
    this.mounted.push({ frameId, tree });
  }
  message(title, text, buttons) {
    this.messages.push({ title, text, buttons });
  }
}

function testImmediateMode() {
  const session = new MockSession();
  const u = new Ui(session);

  u.dialog(" Hello Test ", 40, () => {
    const name = u.edit("&Name:", "Alice");
    assert.strictEqual(name, "Alice");
    const clicked = u.button("&Save");
    assert.strictEqual(clicked, false);
  });

  u._sync();

  assert.strictEqual(session.mounted.length, 1);
  const { frameId, tree } = session.mounted[0];
  assert.strictEqual(frameId, "mainDlg");
  assert.strictEqual(tree.type, "Dialog");
  assert.strictEqual(tree.props.title, " Hello Test ");
  assert.strictEqual(tree.children.length, 2);

  console.log("Node.js bindings unit test passed.");
}

testImmediateMode();
