# Mori Structural Review for VS Code

This directory is a dependency-free reference client for Mori's editor-neutral
SARIF and stdin-overlay contract. It runs a local Mori process after supported
documents change and displays structural-similarity leads as information
diagnostics and incomplete-analysis conditions as warnings.

The extension does not download Mori, upload source, create temporary source
files, or send telemetry. Install an official Mori release separately and make
`mori` available on `PATH`, or set `mori.executable` to an explicit local path.

## Install a release package

Download `mori-vscode_<version>.vsix` from the matching Mori release, then run:

```sh
code --install-extension mori-vscode_<version>.vsix
```

Marketplace and Open VSX publication are separate maintainer actions. The VSIX
is checksummed alongside every other release asset and does not bundle or
download the Mori executable.

## Run from source

1. Open this directory in VS Code.
2. Press F5 to launch an Extension Development Host.
3. Open a workspace containing a supported source file.

Use **Mori: Refresh Structural Diagnostics** for an immediate scan. The
extension otherwise debounces edits for 750 milliseconds by default. It kills
an older per-document process when a newer edit supersedes it and ignores stale
results.

Settings:

- `mori.enabled`: enable automatic document scans;
- `mori.executable`: local command name or executable path;
- `mori.profile`: `review`, `explore`, or `sql`; and
- `mori.debounceMilliseconds`: 100 through 10,000 milliseconds.

The reference client supports VS Code language IDs for C#, Go, Hack, Java,
JavaScript, PHP, TypeScript, JSX, TSX, Python, Rust, shell, Swift, and SQL. Mori
still decides support from the discovered file path and selected dialect. A
scan covers the containing workspace root so the unsaved buffer can be
compared with existing repository source, then filters diagnostics back to the
edited document.

Mori findings are review leads. They do not prove semantic or behavioral
equivalence, defects, or a safe refactoring. Inspect the related source and any
`MORI002` incomplete-analysis warnings before acting.

See [Machine and editor integration](../../docs/machine-integration.md) for the
wire contract, bounds, exit codes, and suppression semantics.
