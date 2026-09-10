"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vscode = require("vscode");

async function waitFor(predicate, description) {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error(`Timed out: ${description}`);
}

exports.run = async function () {
  const root = process.env.MORI_VSCODE_SMOKE_ROOT;
  assert.ok(root);
  const extension = vscode.extensions.getExtension("Cyberlane.mori-vscode");
  assert.ok(extension, "development extension available");
  await extension.activate();
  const config = vscode.workspace.getConfiguration("mori");
  const executable = process.env.MORI_SMOKE_BINARY || path.join(root, "producer");
  const workspace = path.join(root, "workspace");
  await config.update("executable", executable, vscode.ConfigurationTarget.Global);
  const document = await vscode.workspace.openTextDocument(path.join(workspace, "main.ts"));
  await vscode.window.showTextDocument(document);
  const diagnostics = () => vscode.languages.getDiagnostics(document.uri).filter((d) => d.source === "Mori");
  const mode = (value) => {
    fs.writeFileSync(path.join(root, "mode"), value);
    if (process.env.MORI_SMOKE_BINARY) {
      fs.writeFileSync(path.join(workspace, ".mori.json"), JSON.stringify({
        profile: "review", min_tokens: value === "warning" ? 100000 : 1,
      }));
    }
  };
  const refresh = () => vscode.commands.executeCommand("mori.refreshDiagnostics");
  mode("finding"); await refresh();
  await waitFor(() => diagnostics().some((d) => d.code === "MORI001"), "source finding");
  mode("warning"); await refresh();
  await waitFor(() => diagnostics().some((d) => d.code === "MORI002" && d.message.startsWith("Scan-level warning:")), "run-level warning");
  await config.update("executable", path.join(root, "missing-binary"), vscode.ConfigurationTarget.Global);
  await refresh();
  await waitFor(() => diagnostics().some((d) => d.code === "MORI_UNAVAILABLE"), "missing executable visibility");
  mode("finding"); await config.update("executable", executable, vscode.ConfigurationTarget.Global);
  await refresh();
  await waitFor(() => diagnostics().some((d) => d.code === "MORI001") && !diagnostics().some((d) => d.code === "MORI_UNAVAILABLE"), "failure recovery");
  const ruby = await vscode.workspace.openTextDocument(path.join(workspace, "main.rb"));
  assert.equal(ruby.languageId, "ruby");
  await vscode.window.showTextDocument(ruby); await refresh();
  await waitFor(() => vscode.languages.getDiagnostics(ruby.uri).some((d) => d.source === "Mori" && d.code === "MORI001"), "Ruby eligibility");
  fs.writeFileSync(path.join(root, "passed.json"), JSON.stringify({ producer: process.env.MORI_SMOKE_BINARY ? "local Mori binary" : "controlled SARIF", passed: ["finding", "run-level warning", "missing executable", "recovery", "Ruby eligibility"] }));
};
