const { Session, VtuiError } = require("./session");
const { Ui } = require("./ui");

function log(...args) {
  process.stderr.write(`[VTUI_LOG] ${args.join(" ")}\n`);
}

function run(uiFunc, options = {}) {
  const session = new Session(options);
  session.start();
  const u = new Ui(session);

  const step = () => {
    try {
      uiFunc(u);
      u._sync();
    } catch (err) {
      session.close();
      throw err;
    }
  };

  session.on("event", ev => {
    if (ev.op === "closed" && ev.frameId === u.rootId) {
      session.close();
      return;
    }
    u._processEvent(ev);
    step();
  });

  session.on("error", err => {
    session.close();
    throw err;
  });

  step();
}

module.exports = {
  run,
  log,
  Session,
  Ui,
  VtuiError,
};
