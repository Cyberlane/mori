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

## Other editors

Editors with Language Server Protocol support can launch:

```sh
mori lsp
```

The server accepts full-document synchronization and publishes informational
structural-review leads with related source locations. It debounces edits,
cancels stale work, and emits incomplete-analysis warnings instead of silently
turning parser or coverage gaps into an empty result.

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

## Coordinated project upgrades

An updated CLI can inspect every project-managed Mori surface in one local,
versioned plan:

```sh
mori project upgrade --check --format json .
mori project upgrade --dry-run .
mori project upgrade --apply .
```

Check mode exits `5` when required Mori-managed migration remains or a managed
conflict blocks compatibility. Apply mode
updates the pin and tracked project contract, replaces only a missing,
contract-recorded, or known official prior skill with a recoverable backup,
and then rechecks the project. Unknown skill changes are preserved and marked
as a manual conflict. Configuration, protected baseline evidence, hooks,
scripts, and CI remain project-owned; they are inventoried but never rewritten.
The command also never installs a CLI, creates a baseline, commits, pushes, or
releases.

## Agent-guided project setup

Ask Mori for a deterministic, read-only project inventory and question plan:

```sh
mori setup --agent --format json . > mori-setup-plan.json
```

The plan uses project-relative paths and describes the supported-language
inventory, unsupported extensions, current configuration state, questions, and
next commands. A project agent can write a small answers document such as:

```json
{
  "profile": "review",
  "comparison_mode": "cross-language",
  "strictness": "standard",
  "exclude_generated": true,
  "exclude": ["fixtures/**"]
}
```

Preview the exact `.mori.json` without writing:

```sh
mori setup --answers mori-answers.json --dry-run .
```

Apply only after review:

```sh
mori setup --answers mori-answers.json --apply .
mori doctor .
```

Use `mori configure --agent` and the corresponding `configure --answers`
commands for an existing file. Answer files are strict JSON, symlinks are
refused, and configuration replacement is atomic. These commands do not create
a baseline, install a hook or skill, edit CI, refactor source, commit, push, or
release anything.
