# Machine and editor integration

Mori exposes deterministic complete evidence and bounded projections for
different consumers:

- `--format json` is the complete schema-versioned report for automation,
  baselines, audits, and custom clients.
- `--format agent --output <path>` writes that complete bounded JSON report to
  an owner-only local file and emits a bounded context summary with at most 25
  relevant identities. Prefer a private temporary or Git-metadata path outside
  the tracked checkout unless the report is intentionally retained.
- `--format sarif` is a SARIF 2.1.0 projection for editors and code-scanning
  systems. It preserves review locations and bounded explanations but is not a
  replacement for the complete JSON report.

Neither format turns a structural score into proof of semantic or behavioral
equivalence. A client should present every match as a source-review lead.

`--format html` produces a self-contained, source-free visual report for local
human review. It is deliberately not a machine contract and has no schema
version. Paths and report text are HTML-escaped, source bodies are not embedded,
and no external scripts, fonts, images, or network requests are used.

`--format compact` is a bounded human shortlist with one line per group,
focus coverage, warning totals, and the behavioral-equivalence disclaimer. It
is not a versioned machine contract; use JSON when fields must be parsed.

For a report that will leave the project boundary, add `--redact-paths`. Mori
then replaces every exact source, warning, coverage, configuration, ignore,
stdin, baseline, and focus path with a deterministic placeholder such as
`<path-004>.go`. Extensions are retained for context; source text, literal
values, and path-to-placeholder mappings are not emitted. Redaction changes
presentation only, not scores, fingerprints, counts, or schema version.

## Versioned JSON contract

Schema 22 is described by the Draft 2020-12 artifact at
[`schemas/mori-report-v22.schema.json`](../schemas/mori-report-v22.schema.json).
Official releases include the same file and its SHA-256 checksum.
Consumers should select a validator that supports Draft 2020-12, require
`schema_version` to equal `22`, and reject or explicitly handle unknown report
versions.

Schema 22 adds fragment-selection policy and per-file excluded test/production
fragment counts. Older schema artifacts remain available for historical
consumers; new fields require explicit schema handling.

Schema 21 adds the optional canonical staged `review` outcome: selected policy,
policy status, analysis completeness, coverage-policy result, focused finding
count and validated acknowledgement. Policy success is not a complete-analysis
or human-review claim. See [review policies](guides/review-policy.md). Timing and
opt-in feedback remain outside this deterministic report contract.

Schema 20 adds exact changed-line intervals to focused path evidence and an
explicit `configuration.focused_only` comparison-universe flag. Schema 19 adds
optional compatible staged-review receipt evidence. Schema 18
added immutable Git-index input provenance, named project scopes, and per-path
focused coverage. Schema 17 added the optional stdin overlay field.
The current normalization version is 14 and the baseline contract is schema 4.
Version 13 records corrected Swift optional-type/nil-coalescing grammar. Previously
repaired expressions can lose synthetic grouping nodes and change fingerprints;
review existing acceptance before migrating it to this contract.
Schema 4 adds the `false-positive` review classification without changing
matching or suppression semantics. Under `--staged`, the index digest covers the exact tracked baseline
blob as well as source, ignore, and configuration inputs; there is no separate
report field or schema change for that correction.

## Bounded-scan failure evidence

A candidate-pair-limit failure exits `1`. JSON mode, or an explicitly requested
`--format agent --output PATH` artifact, writes a separate `artifact: "mori-scan-failure"` document
with `failure_schema_version: 1` and `complete: false`. It records the reason,
limit, compared count, tool/configuration provenance, available coverage and
warnings, and recovery guidance. It is **not** a schema-22 report and contains
no complete ranking or acceptance surface. Consumers must distinguish it before
validating a normal report; it cannot authorize a receipt or baseline update.
Other operational errors may still produce no machine artifact.

Inspect inventory, select a reviewed smaller root or named scope, and rerun.
`--max-groups` bounds displayed findings, not candidate work. Safety limits
remain enabled; no automatic unbounded or approximate scan is substituted.

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
`mori project upgrade --check` or from the implicit local compatibility gate
before `scan` and `review`. The upgrade command emits its own versioned plan;
the implicit gate emits a diagnostic and no scan report.

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
only file-specific results that contain a location for the active document.
Project-wide warnings without a location must remain visible as scan-level
incompleteness, and operational failure must replace stale findings with an
explicit unavailable state.

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

## Support session contract

`mori scan --diagnostics session.json [options] [paths...]` explicitly requests
one local, source-free diagnostic session. Place the option before other options
when collecting early argument failures. It follows the normal options-before-
paths grammar and is written on command return; hard process termination may
leave no session. Capture warnings do not change the scan's exit code. Session destinations must
be new files; a failed write does not replace an earlier session.

`mori support inspect <session.json|support.zip>` validates and prints canonical
session JSON, not the raw input. `mori support bundle --session <session.json>
--output <new.zip>` explicitly creates a new local ZIP, previews canonical JSON
on stdout and writes a local-creation notice to stderr. Repeated
`--finding RANK:CLASSIFICATION` adds up to 25 unique reviewed ranks; labels are
`useful`, `intentional`, `false-positive`, and `uncertain`, with no free text.
Inspection and bundle creation exit 0 on success, 2 for command usage errors,
and 1 for invalid evidence or I/O errors. They never upload data.

[Support schema 1](../schemas/mori-support-session-v1.schema.json) is separate
from scan-report, baseline, receipt and feedback
schemas. Its top-level fields are `schema_version`, `tool`, `status`, `failure`,
`settings`, `report_available`, `counts`, `phases`, `languages`, and optional `reviewed_findings`.
`report_available` distinguishes unavailable measurements from measured zero counts.
When false, counts remain zero, languages are empty and reviewed findings are
not accepted. A true value does not imply complete analysis.
`settings` may be null when configuration was not resolved; `failure` is null
for success. Unknown fields are rejected rather than copied or ignored.

- `tool`: `modified`, `version`, `revision`, `goos`, `goarch`, `go_version`, `report_schema`,
  `normalization_version`, `config_schema`, `contract_schema`. Build strings
  must match fixed known formats; arbitrary development labels become `unknown`.
- `status`: `success`, `error`, `usage`, `findings`, `coverage`, `upgrade`, or
  `cancelled`. Optional `failure` contains only fixed `code` and `stage` values.
  Stages: `arguments`, `validation`, `configuration`, `project`, `baseline`, `discovery`,
  `analysis`, `receipt`, `output`, `coverage`. Codes: `error`, `usage`,
  `findings`, `coverage`, `upgrade`, `cancelled`, `resource-limit`, `parse`,
  `configuration`, `invalid-arguments`, `io`, `unknown`.
- `settings`: `profile`, `threshold`, `min_tokens`, `max_groups`,
  `max_occurrences`, `max_pairs`, `max_file_bytes`, `workers`,
  `comparison_domain`, `sql_dialect`, `fragment_selection`, `ranking`,
  `same_language_only`, `cross_language_only`, `embedded_sql`,
  `statement_blocks`, `block_statements`, `max_blocks_per_function`,
  `exclude_generated`, `respect_ignore`, `fail_on_match`, `require_coverage`,
  `min_file_coverage`, `max_zero_fragment_files`, `fail_on_warning`,
  `fail_on_parse_diagnostic`, `scope_selected`, `root_count`,
  `exclude_pattern_count`, `priority_path_count`, `language_pair_count`,
  `baseline_enabled`. No actual paths, scope names, globs or config JSON.
- `counts`: `files`, `fragments`, `candidate_pairs`, `location_pairs`,
  `match_groups`, `warnings`, `parse_diagnostics`, `test_files`, `story_files`,
  `test_fragments`, `story_fragments`, `zero_fragment_files`,
  `generated_excluded_files`, `truncated`, `leading_groups`,
  `leading_test_story_groups`. The leading-group sample contains at most 25
  retained groups; the test/story count uses bounded path conventions and is
  not a measured false-positive rate.
- `phases`: at most five unique entries, each `name` and `milliseconds`.
  Fixed names are `total`, `discovery`, `parse`, `compare`, and `analysis`.
- `languages`: unique built-in `language` IDs with numeric `files` and
  `fragments` totals.
- `reviewed_findings`: optional `{ "rank": 7, "classification": "intentional" }`
  entries, sorted by rank. Unique ranks range from 1 to 1,000,000.

Counts and numeric limits are nonnegative signed 64-bit integers up to
9,223,372,036,854,775,807; `max_zero_fragment_files` also accepts -1. Threshold
and minimum file coverage are finite values from 0 to 1. JSON consumers must
preserve integer precision rather than coercing large limits to floating-point
numbers. Phase timings range
from 0 through 604,800,000 milliseconds (seven days).

Session input is bounded to 64 KiB, bundle input to 128 KiB. ZIP contents are
exactly `session.json` and a fixed `README.txt`; arbitrary members, symlinks and
unbounded extraction are not supported. Inspect the canonical payload before
sharing. New files request mode 0600 on POSIX; Windows access follows inherited
directory ACLs rather than a Mori-managed per-user ACL. See [support collection](guides/support.md) for workflow and failure
handling; a diagnostic session never grants baseline or receipt acceptance.

## Workload plans

`mori plan --format json [scan options] [roots]` emits the separate
[`mori-scan-plan` schema 1](../schemas/mori-scan-plan-v1.schema.json) artifact.
It parses source and enumerates eligible candidates but never scores similarities.
Do not parse it as a scan report or use it as baseline/receipt evidence. A count
above the configured pair cap sets `exceeds_pair_limit` rather than failing the
plan. Coverage policies still determine coverage failures. See
[workload planning](guides/workload-planning.md) for limits and scope tradeoffs.
