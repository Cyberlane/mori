# Editors and coding agents

Mori keeps source local. Editor and agent integrations invoke the local binary
and consume deterministic SARIF or JSON.

## VS Code

The repository contains a dependency-free
[reference extension](../../editors/vscode/README.md). It debounces edits,
cancels superseded scans, rejects stale results, and displays findings as
informational diagnostics and incomplete-analysis conditions as warnings.

The extension does not download Mori, upload source, write temporary source
files, or send telemetry. Install an official binary separately and place it
on `PATH`, or configure an explicit executable path.

## Unsaved buffers

Editors can overlay one already-discovered source path through standard input:

```sh
mori scan --profile review --format sarif \
  --stdin-path src/example.go . < src/example.go
```

Mori reads at most 16 MiB from standard input, applies a lower configured file
limit when present, and automatically focuses results touching the overlaid
file. The path must already be a supported discovered file; the overlay does
not create a new discovery route.

See [Machine and editor integration](../machine-integration.md) for stable rule
IDs, source regions, limits, suppression semantics, exit behavior, and the
versioned JSON Schema.

## Coding-agent skill

Install the embedded review skill into the current project:

```sh
mori skill install --project .
```

Or install it for the current user:

```sh
mori skill install --global
```

The skill requires the agent to verify the binary, inventory supported and
unsupported source, inspect exact coverage, use bounded structured output,
open both source locations, and classify matches before recommending changes.
It explicitly prohibits treating a score as proof of equivalent behavior.

Agent installation, project configuration, refactoring, baselining, CI,
committing, pushing, and releasing are separate permissions. Installing the
skill alone does not authorize any of the others.
