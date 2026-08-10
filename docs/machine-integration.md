# Machine and editor integration

Mori exposes two deterministic machine formats for different consumers:

- `--format json` is the complete schema-versioned report for automation,
  baselines, audits, and custom clients.
- `--format sarif` is a SARIF 2.1.0 projection for editors and code-scanning
  systems. It preserves review locations and bounded explanations but is not a
  replacement for the complete JSON report.

Neither format turns a structural score into proof of semantic or behavioral
equivalence. A client should present every match as a source-review lead.

## Versioned JSON contract

Schema 17 is described by the Draft 2020-12 artifact at
[`schemas/mori-report-v17.schema.json`](../schemas/mori-report-v17.schema.json).
Official v0.24.0 release assets include the same file and its SHA-256 checksum.
Consumers should select a validator that supports Draft 2020-12, require
`schema_version` to equal `17`, and reject or explicitly handle unknown report
versions.

Schema 17 adds only the optional `configuration.stdin_path` field.
Normalization remains version 9 and the baseline contract remains schema 3.

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

Exit `1` is an operational failure and exit `2` is invalid usage. A client
should reject empty, malformed, or wrong-version output regardless of exit
status.

## Unsaved-buffer overlays

`--stdin-path PATH` replaces one source file's bytes for that scan with bytes
read from standard input. The disk file is used only during ignore-aware,
regular-file discovery and language selection; the parser receives the stdin
content. This lets an editor compare an unsaved document with the rest of the
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

## Reference VS Code client

[`editors/vscode`](../editors/vscode/README.md) contains a small dependency-free
reference extension. It demonstrates debouncing, stale-process cancellation,
SARIF parsing, active-document filtering, related locations, and severity
mapping. It runs the configured local Mori binary and performs no downloads or
telemetry. The extension is reference source rather than a Marketplace release;
projects may package it directly or use the contract from another IDE.
