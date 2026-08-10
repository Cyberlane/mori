"use strict";

const path = require("path");
const { spawn } = require("child_process");
const vscode = require("vscode");

const supportedLanguages = new Set([
  "go",
  "javascript",
  "javascriptreact",
  "python",
  "rust",
  "shellscript",
  "sql",
  "swift",
  "typescript",
  "typescriptreact",
]);
const acceptedExitCodes = new Set([0, 3, 4]);
const timers = new Map();
const running = new Map();
let generation = 0;

function activate(context) {
  const diagnostics = vscode.languages.createDiagnosticCollection("mori");
  const output = vscode.window.createOutputChannel("Mori");
  context.subscriptions.push(diagnostics, output);

  const queue = (document, immediate = false) => {
    if (!eligible(document)) {
      diagnostics.delete(document.uri);
      cancel(document.uri.toString());
      return;
    }
    const key = document.uri.toString();
    const existing = timers.get(key);
    if (existing) {
      clearTimeout(existing);
    }
    const delay = immediate
      ? 0
      : configuration(document).get("debounceMilliseconds", 750);
    timers.set(
      key,
      setTimeout(() => {
        timers.delete(key);
        analyze(document, diagnostics, output);
      }, delay),
    );
  };

  context.subscriptions.push(
    vscode.workspace.onDidOpenTextDocument((document) => queue(document)),
    vscode.workspace.onDidChangeTextDocument((event) => queue(event.document)),
    vscode.workspace.onDidSaveTextDocument((document) => queue(document, true)),
    vscode.workspace.onDidCloseTextDocument((document) => {
      diagnostics.delete(document.uri);
      cancel(document.uri.toString());
    }),
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (!event.affectsConfiguration("mori")) {
        return;
      }
      diagnostics.clear();
      for (const document of vscode.workspace.textDocuments) {
        queue(document, true);
      }
    }),
    vscode.commands.registerCommand("mori.refreshDiagnostics", () => {
      const document = vscode.window.activeTextEditor?.document;
      if (document) {
        queue(document, true);
      }
    }),
  );

  for (const document of vscode.workspace.textDocuments) {
    queue(document);
  }
}

function deactivate() {
  for (const timer of timers.values()) {
    clearTimeout(timer);
  }
  timers.clear();
  for (const state of running.values()) {
    state.child.kill();
  }
  running.clear();
}

function configuration(document) {
  return vscode.workspace.getConfiguration("mori", document.uri);
}

function eligible(document) {
  return (
    document.uri.scheme === "file" &&
    supportedLanguages.has(document.languageId) &&
    configuration(document).get("enabled", true)
  );
}

function cancel(key) {
  const timer = timers.get(key);
  if (timer) {
    clearTimeout(timer);
    timers.delete(key);
  }
  const state = running.get(key);
  if (state) {
    state.child.kill();
    running.delete(key);
  }
}

function analyze(document, diagnostics, output) {
  if (!eligible(document)) {
    return;
  }
  const key = document.uri.toString();
  cancel(key);
  const token = ++generation;
  const folder = vscode.workspace.getWorkspaceFolder(document.uri);
  const root = folder ? folder.uri.fsPath : path.dirname(document.uri.fsPath);
  const config = configuration(document);
  const executable = config.get("executable", "mori");
  const profile = config.get("profile", "review");
  const args = [
    "scan",
    "--profile",
    profile,
    "--format",
    "sarif",
    "--stdin-path",
    document.uri.fsPath,
    root,
  ];
  const child = spawn(executable, args, {
    cwd: root,
    shell: false,
    stdio: ["pipe", "pipe", "pipe"],
    windowsHide: true,
  });
  const state = { child, token, stdout: "", stderr: "" };
  running.set(key, state);
  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk) => {
    state.stdout += chunk;
  });
  child.stderr.on("data", (chunk) => {
    state.stderr += chunk;
  });
  child.on("error", (error) => {
    if (!isCurrent(key, token)) {
      return;
    }
    running.delete(key);
    diagnostics.delete(document.uri);
    output.appendLine(`Unable to run ${executable}: ${error.message}`);
  });
  child.on("close", (code, signal) => {
    if (!isCurrent(key, token)) {
      return;
    }
    running.delete(key);
    if (signal || !acceptedExitCodes.has(code)) {
      diagnostics.delete(document.uri);
      output.appendLine(
        `Mori scan failed for ${document.uri.fsPath} (exit ${code}, signal ${signal || "none"}).`,
      );
      if (state.stderr.trim()) {
        output.appendLine(state.stderr.trim());
      }
      return;
    }
    try {
      const log = JSON.parse(state.stdout);
      diagnostics.set(document.uri, diagnosticsForDocument(log, document, root));
    } catch (error) {
      diagnostics.delete(document.uri);
      output.appendLine(`Mori returned invalid SARIF for ${document.uri.fsPath}: ${error.message}`);
    }
  });
  child.stdin.on("error", () => {});
  child.stdin.end(document.getText(), "utf8");
}

function isCurrent(key, token) {
  return running.get(key)?.token === token;
}

function diagnosticsForDocument(log, document, root) {
  if (log?.version !== "2.1.0" || !Array.isArray(log.runs)) {
    throw new Error("expected SARIF 2.1.0 with a runs array");
  }
  const result = [];
  for (const run of log.runs) {
    for (const finding of run.results || []) {
      const locations = [
        ...(finding.locations || []),
        ...(finding.relatedLocations || []),
      ];
      const localLocations = locations.filter(
        (location) => resolveLocation(location, root) === path.normalize(document.uri.fsPath),
      );
      for (const location of localLocations) {
        const diagnostic = new vscode.Diagnostic(
          rangeForDocument(location, document),
          finding.message?.text || finding.ruleId || "Mori structural review lead",
          severity(finding.level),
        );
        diagnostic.source = "Mori";
        diagnostic.code = finding.ruleId;
        diagnostic.relatedInformation = relatedInformation(locations, location, root);
        result.push(diagnostic);
      }
    }
  }
  return result;
}

function resolveLocation(location, root) {
  const uri = location?.physicalLocation?.artifactLocation?.uri;
  if (!uri || uri.startsWith("file:")) {
    return uri ? path.normalize(vscode.Uri.parse(uri).fsPath) : "";
  }
  let decoded;
  try {
    decoded = decodeURIComponent(uri);
  } catch {
    return "";
  }
  return path.normalize(path.isAbsolute(decoded) ? decoded : path.resolve(root, decoded));
}

function rangeForDocument(location, document) {
  const region = location?.physicalLocation?.region || {};
  const startLine = clamp((region.startLine || 1) - 1, 0, document.lineCount - 1);
  const endLine = clamp((region.endLine || region.startLine || 1) - 1, startLine, document.lineCount - 1);
  const startCharacter = clamp(
    (region.startColumn || 1) - 1,
    0,
    document.lineAt(startLine).text.length,
  );
  const endCharacter = region.endColumn
    ? clamp(region.endColumn - 1, 0, document.lineAt(endLine).text.length)
    : document.lineAt(endLine).text.length;
  return new vscode.Range(startLine, startCharacter, endLine, endCharacter);
}

function relatedInformation(locations, selected, root) {
  const related = [];
  for (const location of locations) {
    if (location === selected) {
      continue;
    }
    const resolved = resolveLocation(location, root);
    if (!resolved) {
      continue;
    }
    const region = location?.physicalLocation?.region || {};
    const startLine = Math.max((region.startLine || 1) - 1, 0);
    const startColumn = Math.max((region.startColumn || 1) - 1, 0);
    const range = new vscode.Range(startLine, startColumn, startLine, startColumn);
    const message = location.message?.text || "Related structurally similar location";
    related.push(new vscode.DiagnosticRelatedInformation(new vscode.Location(vscode.Uri.file(resolved), range), message));
  }
  return related;
}

function severity(level) {
  if (level === "error") {
    return vscode.DiagnosticSeverity.Error;
  }
  if (level === "warning") {
    return vscode.DiagnosticSeverity.Warning;
  }
  return vscode.DiagnosticSeverity.Information;
}

function clamp(value, minimum, maximum) {
  return Math.min(Math.max(value, minimum), maximum);
}

module.exports = { activate, deactivate };
