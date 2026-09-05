const { spawn } = require("child_process");
const EventEmitter = require("events");
const readline = require("readline");

class VtuiError extends Error {
  constructor(code, message, replyTo) {
    super(`[${code}] ${message}`);
    this.name = "VtuiError";
    this.code = code;
    this.replyTo = replyTo;
  }
}

const fs = require("fs");
const path = require("path");
const os = require("os");
const { execSync } = require("child_process");

function findHostBinary(explicit) {
  if (explicit) return explicit;
  if (process.env.VTUI_HOST_BIN) return process.env.VTUI_HOST_BIN;

  // __dirname is bindings/node, so ../.. is the repository root
  const base = path.resolve(__dirname, "../..");
  const candidates = [
    path.join(base, "cmd/vtui-host/vtui-host"),
    path.join(base, "bindings/cpp/build/vtui-host"),
    path.join(base, "bindings/c/build/vtui-host"),
    path.join(base, "bindings/build/vtui-host"),
    path.join(base, "build/vtui-host"),
    path.join(base, "vtui-host"),
    path.join(os.homedir(), "go/bin/vtui-host"),
  ];

  for (const cand of candidates) {
    if (fs.existsSync(cand)) {
      return cand;
    }
  }

  // Auto-build if go is present
  try {
    const target = path.join(base, "vtui-host");
    execSync(`go build -o "${target}" ./cmd/vtui-host`, { cwd: base, stdio: "ignore" });
    if (fs.existsSync(target)) {
      return target;
    }
  } catch (_) {}

  return "vtui-host";
}

class Session extends EventEmitter {
  constructor(options = {}) {
    super();
    this.hostBin = findHostBinary(options.hostBin);
    this.backend = options.backend || process.env.VTUI_BACKEND || "ansi";
    this.seq = 0;
    this.proc = null;
    this.stream = null;
  }

  start() {
    // Spawn vtui-host with protocol stream on fd 3
    const args = ["--protocol-fd=3", `--backend=${this.backend}`];
    const env = Object.assign({}, process.env);

    this.proc = spawn(this.hostBin, args, {
      stdio: ["inherit", "inherit", "inherit", "pipe"],
      env: env,
    });

    this.stream = this.proc.stdio[3];

    if (this.stream) {
      this.stream.on("error", err => {
        if (err.code === "ECONNRESET" || err.code === "EPIPE") {
          return;
        }
        this.emit("error", err);
      });
    }

    const rl = readline.createInterface({
      input: this.stream,
      crlfDelay: Infinity,
    });

    rl.on("error", () => {});

    rl.on("line", line => {
      if (!line) return;
      try {
        const msg = JSON.parse(line);
        if (msg.op === "error") {
          this.emit("error", new VtuiError(msg.code || "ERROR", msg.message || "", msg.replyTo));
          return;
        }
        this.emit("event", msg);
      } catch (err) {
        this.emit("error", err);
      }
    });

    this.proc.on("close", code => {
      this.emit("close", code);
    });

    // Handshake
    this.send({ op: "hello", version: 1 });
  }

  send(msg) {
    this.seq += 1;
    if (!msg.seq) {
      msg.seq = this.seq;
    }
    const line = JSON.stringify(msg) + "\n";
    if (this.stream && !this.stream.destroyed) {
      this.stream.write(line);
    }
    return this.seq;
  }

  mount(frameId, tree) {
    this.send({ op: "mount", frameId, tree });
  }

  patch(frameId, ops) {
    this.send({ op: "patch", frameId, ops });
  }

  message(title, text, buttons = ["&Ok"]) {
    this.send({ op: "message", title, text, buttons });
  }

  close() {
    try {
      this.send({ op: "quit" });
    } catch (_) {}
    if (this.proc) {
      this.proc.kill();
      this.proc = null;
    }
  }
}

module.exports = {
  Session,
  VtuiError,
};
