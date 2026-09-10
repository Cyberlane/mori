"use strict";

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const root = fs.mkdtempSync(path.join(os.tmpdir(), "mori-vscode-smoke-"));
try {
  const binary = process.env.MORI_SMOKE_BINARY;
  if (binary && !path.isAbsolute(binary)) throw new Error("MORI_SMOKE_BINARY must be an absolute local executable path");
  const workspace = path.join(root, "workspace");
  fs.mkdirSync(workspace);
  fs.writeFileSync(path.join(workspace, "main.ts"), "function alpha(value: number) { return value + 1; }\nfunction beta(input: number) { return input + 1; }\n");
  fs.writeFileSync(path.join(workspace, "main.rb"), "def alpha(value)\n  value + 1\nend\ndef beta(input)\n  input + 1\nend\n");
  fs.writeFileSync(path.join(workspace, ".mori.json"), JSON.stringify({ profile: "review", min_tokens: 1 }));
  fs.writeFileSync(path.join(root, "mode"), "finding");
  fs.writeFileSync(path.join(root, "producer"), `#!${process.execPath}
const fs = require("node:fs");
const path = require("node:path");
const mode = fs.readFileSync(path.join(__dirname, "mode"), "utf8");
const file = process.argv[process.argv.indexOf("--stdin-path") + 1];
process.stdin.resume();
process.stdin.on("end", () => {
  const result = mode === "warning"
    ? { ruleId: "MORI002", level: "warning", message: { text: "Coverage is incomplete" } }
    : { ruleId: "MORI001", level: "note", message: { text: "Structural review lead" }, locations: [{ physicalLocation: { artifactLocation: { uri: file }, region: { startLine: 1 } } }] };
  console.log(JSON.stringify({ version: "2.1.0", runs: [{ results: [result] }] }));
  process.exitCode = mode === "warning" ? 4 : 0;
});
`, { mode: 0o700 });
  const nativeMac = "/Applications/Visual Studio Code.app/Contents/MacOS/Code";
  const executable = process.env.VSCODE_EXECUTABLE || (fs.existsSync(nativeMac) ? nativeMac : "code");
  const env = { ...process.env, MORI_VSCODE_SMOKE_ROOT: root };
  delete env.ELECTRON_RUN_AS_NODE;
  delete env.VSCODE_IPC_HOOK_CLI;
  const result = spawnSync(executable, [
    "--new-window", "--skip-welcome", "--skip-release-notes", "--disable-workspace-trust",
    "--user-data-dir", path.join(root, "user"), "--extensions-dir", path.join(root, "extensions"),
    "--extensionDevelopmentPath", __dirname,
    "--extensionTestsPath", path.join(__dirname, "extension-host.test.js"), workspace,
  ], { env, encoding: "utf8", timeout: 90000 });
  if (result.error || result.status !== 0 || !fs.existsSync(path.join(root, "passed.json"))) {
    if (result.stdout) process.stdout.write(result.stdout);
    if (result.stderr) process.stderr.write(result.stderr);
    if (result.error) throw result.error;
    throw new Error(`Extension Host smoke failed (exit ${result.status})`);
  }
  console.log(fs.readFileSync(path.join(root, "passed.json"), "utf8"));
} finally {
  fs.rmSync(root, { recursive: true, force: true });
}
