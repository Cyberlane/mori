# Machine and editor integration

Mori exposes two deterministic machine formats for different consumers:

- `--format json` is the complete schema-versioned report for automation,
  baselines, audits, and custom clients.
- `--format sarif` is a SARIF 2.1.0 projection for editors and code-scanning
  systems. It preserves review locations and bounded explanations but is not a
  replacement for the complete JSON report.

Neither format turns a structural score into proof of semantic or behavioral
equivalence. A client should present every match as a source-review lead.

`--format html` produces a self-contained, source-free visual report for local
human review. It is deliberately not a machine contract and has no schema
version. Paths and report text are HTML-escaped, source bodies are not embedded,
and no external scripts, fonts, images, or network requests are used.

`--format compact` is a bounded human/agent shortlist with one line per group,
focus coverage, warning totals, and the behavioral-equivalence disclaimer. It
is not a versioned machine contract; use JSON when fields must be parsed.

For a report that will leave the project boundary, add `--redact-paths`. Mori
then replaces every exact source, warning, coverage, configuration, ignore,
stdin, baseline, and focus path with a deterministic placeholder such as
`<path-004>.go`. Extensions are retained for context; source text, literal
values, and path-to-placeholder mappings are not emitted. Redaction changes
presentation only, not scores, fingerprints, counts, or schema version.

## Versioned JSON contract

Schema 18 is described by the Draft 2020-12 artifact at
[`schemas/mori-report-v18.schema.json`](../schemas/mori-report-v18.schema.json).
Official releases include the same file and its SHA-256 checksum.
Consumers should select a validator that supports Draft 2020-12, require
`schema_version` to equal `18`, and reject or explicitly handle unknown report
versions.

Schema 18 adds immutable Git-index input provenance, named project scopes, and
per-path focused coverage. Schema 17 added the optional stdin overlay field.
The current normalization version is 12 and the baseline contract is schema 4.
Schema 4 adds the `false-positive` review classification without changing
matching or suppression semantics. Under `--staged`, the index digest covers the exact tracked baseline
blob as well as source, ignore, and configuration inputs; there is no separate
report field or schema change for that correction.

## SARIF contract

Mori emits one SARIF run with these stable rule IDs:

| Rule | Level | Meaning |
| --- | --- | --- |
| `MORI001` | `note` | One retained structurally similar content-pair group. |
| `MORI002` | `warning` | A parser, discovery, focus, baseline, or coverage condition made analysis incomplete. |

`MORI001` includes one primary location, all other retained occurrences as
related locations, the stable content-pair ID as a partial fingerprint, the
score, exact weighted intersection and union, focus state, review priority,
and bounded review signals. Location retention still obeys
`--max-occurrences`.

`MORI002` emits one result per retained parse diagnostic when exact regions are
available, or one file/run-level result otherwise. Consumers must not discard
these warnings: an incomplete scan is not evidence that no similarity exists.

SARIF similarity results obey the report's `--max-groups` bound. Baseline-suppressed groups
are omitted because the final report does not retain their source locations.
The invocation records exact `suppressedMatchGroups` and
`suppressedLocationPairs` counts, plus warning, candidate, retained, total, and
truncation evidence. Mori deliberately omits SARIF `baselineState`; it cannot
infer a code-scanning system's historical state.

Mori writes machine output before applying `--fail-on-match` or strict coverage
exit policy. Clients may therefore consume valid output with these exit codes:

- `0`: scan completed and configured gates passed;
- `3`: a configured match gate failed; or
- `4`: at least one configured coverage gate failed.

Project-maintenance clients may also receive exit `5` from
`mori project upgrade --check` when project-managed Mori assets drift. That
command emits its own versioned plan rather than a scan report.

Exit `1` is an operational failure and exit `2` is invalid usage. A client
should reject empty, malformed, or wrong-version output regardless of exit
status.

## Unsaved-buffer overlays

`--stdin-path PATH` replaces one source file's bytes for that scan with bytes
read from standard input. The disk file is used only during ignore-aware,
regular-file discovery; the parser receives the stdin content. Language
selection follows the path except that an overlaid legacy `.php` buffer is
re-evaluated for the bounded exact Hack `<?hh` header. This lets an editor
compare an unsaved document with the rest of the
repository without a temporary source file or network transfer.

The overlay contract is intentionally strict:

- `PATH` must resolve to exactly one discovered, supported, regular source
  file under the scan roots;
- the input is bounded to 16 MiB, or `--max-file-bytes` when that configured
  limit is smaller;
- empty stdin is a valid empty overlay and never falls back to disk content;
- the path is automatically added to focus, so retained groups touching it are
  ordered first and marked focused;
- baselines are rejected with overlays because accepted identities describe
  reviewed disk-backed scan inputs; and
- `configuration.stdin_path` records the overlay path, but neither JSON nor
  SARIF contains source bodies, literal values, or temporary files.

Example:

```sh
mori scan \
  --profile review \
  --format sarif \
  --stdin-path src/service.go \
  . < src/service.go
```

An editor normally sends its in-memory document text instead of redirecting
the disk file. It should cancel or supersede an older process when a newer edit
arrives, debounce rapid edits, use process spawning without a shell, and show
only results that contain a location for the active document.

## Language Server Protocol

`mori lsp` runs a local, editor-neutral LSP server over standard input/output.
It supports full-document synchronization, open/change/save/close lifecycle,
350 ms change debouncing, cancellation of stale scans, related locations, and
the same `MORI001` advisory and `MORI002` incomplete-analysis diagnostics as
SARIF. Messages and unsaved overlays are bounded to 16 MiB.

The server searches the workspace root for `.mori.json`; without one it uses
the conservative `review` profile. It does not download Mori, upload source,
create temporary source files, or enable telemetry. Editors should launch the
binary directly with the argument `lsp` and use full text-document changes.

## VS Code client

[`editors/vscode`](../editors/vscode/README.md) contains a small dependency-free
reference extension. It demonstrates debouncing, stale-process cancellation,
SARIF parsing, active-document filtering, related locations, and severity
mapping. It runs the configured local Mori binary and performs no downloads or
telemetry. The extension is reference source rather than a Marketplace release;
projects may package it directly or use the contract from another IDE.

The repository also packages this client as a release asset. Marketplace and
Open VSX publication remain separate maintainer actions because they require
store credentials and acceptance of each registry's current terms.
