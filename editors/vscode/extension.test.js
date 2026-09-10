"use strict";

const assert = require("node:assert/strict");
const { EventEmitter } = require("node:events");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { test } = require("node:test");
const manifest = require("./package.json");

// Exercise the public activation API with controlled VS Code events and child
// processes, including synchronous cancellation and late process output.
function harness(options = {}) {
  const handlers = {};
  const values = new Map();
  const children = [];
  const pending = new Map();
  const output = [];
  let nextTimer = 0;
  const config = { ...options.config };
  const uri = (name) => ({ scheme: "file", fsPath: name, toString: () => `file://${name}` });
  const document = {
    uri: uri("/workspace/main.ts"), languageId: "typescript", lineCount: 1,
    lineAt: () => ({ text: "" }), getText: () => "unsaved source",
  };
  const disposable = { dispose() {} };
  const event = (name) => (fn) => { handlers[name] = fn; return disposable; };
  class Range {
    constructor(sl, sc, el, ec) { Object.assign(this, { sl, sc, el, ec }); }
  }
  class Diagnostic {
    constructor(range, message, severity) { Object.assign(this, { range, message, severity }); }
  }
  const vscode = {
    Range, Diagnostic, DiagnosticSeverity: { Error: 0, Warning: 1, Information: 2 },
    DiagnosticRelatedInformation: class { constructor(location, message) { Object.assign(this, { location, message }); } },
    Location: class { constructor(uri, range) { Object.assign(this, { uri, range }); } },
    Uri: { file: uri, parse: (value) => uri(decodeURIComponent(new URL(value).pathname)) },
    languages: { createDiagnosticCollection: () => ({
      ...disposable, set: (u, ds) => values.set(u.toString(), ds),
      delete: (u) => values.delete(u.toString()), clear: () => values.clear(),
    }) },
    window: { activeTextEditor: { document }, createOutputChannel: () => ({
      ...disposable, appendLine: (line) => output.push(line), show: () => output.push("shown"),
    }) },
    commands: { registerCommand: event("unused") },
    workspace: {
      textDocuments: [document],
      getConfiguration: () => ({ get: (key, fallback) => config[key] ?? fallback }),
      getWorkspaceFolder: () => ({ uri: uri("/workspace") }),
      onDidOpenTextDocument: event("open"), onDidChangeTextDocument: event("change"),
      onDidSaveTextDocument: event("save"), onDidCloseTextDocument: event("close"),
      onDidChangeConfiguration: event("config"),
    },
  };
  vscode.commands.registerCommand = (name, fn) => { handlers[name] = fn; return disposable; };
  const spawn = (executable, args, spawnOptions) => {
    if (options.throwSpawn) throw new Error("invalid executable");
    const child = new EventEmitter();
    for (const stream of ["stdout", "stderr", "stdin"]) {
      child[stream] = new EventEmitter();
      child[stream].setEncoding = () => {};
    }
    child.stdin.end = (text) => { child.input = text; };
    child.kill = () => { child.killed = true; if (options.syncKill) child.emit("close", null, "SIGTERM"); };
    Object.assign(child, { executable, args, spawnOptions });
    children.push(child);
    return child;
  };
  const sandbox = { module: { exports: {} },
    require: (name) => name === "vscode" ? vscode : name === "child_process" ? { spawn } : name === "./package.json" ? manifest : require(name),
    setTimeout: (fn) => { pending.set(++nextTimer, fn); return nextTimer; },
    clearTimeout: (id) => pending.delete(id),
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, "extension.js"), "utf8"), sandbox);
  sandbox.module.exports.activate({ subscriptions: [] });
  return {
    document, config, handlers, children, values, output,
    diagnostics: () => values.get(document.uri.toString()) || [],
    flush: () => { const jobs = [...pending.values()]; pending.clear(); jobs.forEach((fn) => fn()); },
    finish(child, results = [], code = 0) {
      child.stdout.emit("data", JSON.stringify({ version: "2.1.0", runs: [{ results }] }));
      child.emit("close", code, null);
    },
    deactivate: sandbox.module.exports.deactivate,
  };
}

const location = (uri, region) => ({ physicalLocation: { artifactLocation: { uri }, region } });
const finding = (locations, ruleId = "MORI001") => ({ ruleId, level: "note", message: { text: "review this" }, ...(locations && { locations }) });

test("producer-shaped warnings retain scope, related matches and URI decoding", () => {
  const h = harness(); h.flush();
  h.finish(h.children[0], [
    { ruleId: "MORI002", level: "warning", message: { text: "candidate results truncated" } },
    finding([location("main.ts", { startLine: 50, endLine: 70, startColumn: 30 })], "MORI002"),
    finding([location("foreign.ts")], "MORI002"),
    { ...finding([location("foreign.ts")]), relatedLocations: [location("file:///workspace/main.ts")] },
    finding([location("%6dain.ts")]),
    finding([location("bad%ZZ.ts")]),
  ], 4);
  const results = h.diagnostics();
  assert.equal(results.length, 4);
  assert.match(results[0].message, /^Scan-level warning:/);
  assert.equal(results[0].severity, 1);
  assert.equal(results[1].range.sl, 0);
  assert.equal(results[1].range.sc, 0);
  assert.equal(results[2].relatedInformation[0].location.uri.fsPath, "/workspace/foreign.ts");
});

for (const code of [1, 2, 5]) {
  test(`exit ${code} replaces findings with unavailable, then recovers`, () => {
    const h = harness(); h.flush(); h.finish(h.children[0], [finding([location("main.ts")])]);
    h.handlers.save(h.document); h.flush();
    h.children[1].stderr.emit("data", "failure detail");
    h.children[1].emit("close", code, null);
    assert.equal(h.diagnostics()[0].code, "MORI_UNAVAILABLE");
    assert.match(h.output[0], /failure detail/);
    h.handlers["mori.showOutput"](); assert.equal(h.output.at(-1), "shown");
    h.handlers.save(h.document); h.flush(); h.finish(h.children[2]);
    assert.equal(h.diagnostics().length, 0);
  });
}

test("missing binary and synchronous spawn failures are visible", () => {
  const h = harness(); h.flush(); h.children[0].emit("error", new Error("ENOENT"));
  assert.equal(h.diagnostics()[0].code, "MORI_UNAVAILABLE");
  h.children[0].emit("close", -2, null); assert.equal(h.output.length, 1);
  const sync = harness({ throwSpawn: true }); sync.flush();
  assert.equal(sync.diagnostics()[0].code, "MORI_UNAVAILABLE");
});

for (const payload of ["not JSON", '{"version":"2.1.0","runs":[]}', '{"version":"2.1.0","runs":[{}]}']) {
  test(`invalid output becomes unavailable: ${payload}`, () => {
    const h = harness(); h.flush(); h.children[0].stdout.emit("data", payload); h.children[0].emit("close", 0, null);
    assert.equal(h.diagnostics()[0].code, "MORI_UNAVAILABLE");
  });
}

test("edit immediately cancels old generation, including synchronous and late output", () => {
  const h = harness({ syncKill: true }); h.flush(); const first = h.children[0];
  h.handlers.change({ document: h.document });
  assert.equal(first.killed, true); assert.equal(h.output.length, 0);
  h.finish(first, [finding([location("main.ts")])]);
  assert.equal(h.diagnostics().length, 0);
  h.handlers.change({ document: h.document }); h.flush();
  assert.equal(h.children.length, 2);
  h.finish(h.children[1], [finding([location("main.ts")])], 3);
  assert.equal(h.diagnostics().length, 1);
  h.handlers.close(h.document); assert.equal(h.diagnostics().length, 0);
});

test("every activated language is eligible; unsupported documents stay excluded", () => {
  const h = harness();
  for (const activation of manifest.activationEvents.filter((event) => event.startsWith("onLanguage:"))) {
    h.document.languageId = activation.slice("onLanguage:".length);
    const before = h.children.length;
    h.handlers.open(h.document); h.flush();
    assert.equal(h.children.length, before + 1, activation);
    h.finish(h.children.at(-1));
  }
  const count = h.children.length;
  for (const language of ["plaintext", "c", "cpp", "unknown"]) {
    h.document.languageId = language; h.handlers.open(h.document); h.flush();
  }
  h.document.languageId = "typescript"; h.document.uri.scheme = "untitled";
  h.handlers.open(h.document); h.flush();
  h.document.uri.scheme = "file"; h.config.enabled = false;
  h.handlers.open(h.document); h.flush();
  assert.equal(h.children.length, count);
});

test("configuration disable and close invalidate running output", () => {
  const h = harness(); h.flush(); const child = h.children[0]; h.config.enabled = false;
  h.handlers.config({ affectsConfiguration: () => true });
  h.finish(child, [finding([location("main.ts")])]);
  assert.equal(h.diagnostics().length, 0);
  h.config.enabled = true; h.handlers.open(h.document); h.flush(); const second = h.children[1];
  h.handlers.close(h.document); second.emit("error", new Error("late error"));
  assert.equal(h.output.length, 0);
});

test("stdin overlay uses local process without a shell", () => {
  const h = harness(); h.flush(); const child = h.children[0];
  assert.equal(child.spawnOptions.shell, false);
  assert.equal(child.input, "unsaved source");
  assert.equal(child.args[child.args.indexOf("--stdin-path") + 1], h.document.uri.fsPath);
  h.deactivate(); assert.equal(child.killed, true);
});

test("deactivation invalidates synchronous cancellation and signals are unavailable", () => {
  const h = harness({ syncKill: true }); h.flush(); h.deactivate();
  assert.equal(h.output.length, 0);
  assert.equal(h.diagnostics().length, 0);
  const other = harness(); other.flush(); other.children[0].emit("close", null, "SIGKILL");
  assert.equal(other.diagnostics()[0].code, "MORI_UNAVAILABLE");
});
