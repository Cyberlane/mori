# Report, schema, and coverage validation

Read this reference for strict report audits, CI policy, provenance checks, or
when a user asks to validate a saved JSON report. Ordinary changed-code review
uses the smaller checks in `changed-code.md`.

## Coverage before interpretation

Inventory tracked or visible source extensions, extensionless executable
shebangs, build manifests, and nested repositories, then compare with
`mori languages`. `mori languages` lists direct and `/usr/bin/env` interpreter
names recognized for extensionless files; Mori does not require executable
permission and does not execute the interpreter. An extension wins over a
conflicting shebang. State which meaningful source is supported, excluded,
nested, or unexamined.

Use `--require-coverage` for review and CI. For an adopted policy, set a
reviewed `--min-file-coverage`, `--max-zero-fragment-files`, and either
`--fail-on-parse-diagnostic` or broader `--fail-on-warning`. Exit `4` means a
coverage policy failed; the report is still written and the scan is not clean.

Schema-20 reports include `coverage` and `file_coverage`. Verify the exact
fragment-file numerator and analyzed-file denominator. Inspect every supported
zero-fragment file, deterministic zero-fragment reason, boundary counts,
skipped-fragment count, parse-diagnostic count, and generated classification.
Generated exclusions remain visible but do not enter the analyzed denominator.
Verify every `excluded_generated` entry rather than treating it as
undiscovered. A successful aggregate does not excuse an empty supported file.

## Required report fields

Require `schema_version` to equal `20`. Validate the mandatory `tool` object:
version, revision, source date, modified flag, platform, Go version, and
normalization version. Official release binaries provide full revision and
source date. A version-pinned source build may report its version while
`revision` or `source_date` is `unknown` when Go did not embed VCS settings;
label this provenance-incomplete, use it only for exploratory local review,
and do not use it for a provenance-sensitive audit or CI gate. Recommend an
official release binary when complete provenance is required; never infer
revision/date from the version string.

Inspect and disclose:

- `warnings`, including every incomplete or failed input;
- `coverage`, including exact file, warning, diagnostic,
  generated-exclusion, aggregate, and unsupported-extension counts;
- `file_coverage`, including zero-fragment reason, generated status, exclusion
  status, candidate and below-token-floor counts, skipped fragments, and parse
  diagnostics;
- `literal_evidence`, when present: positional literal drift is source context,
  not a change to structural similarity, and values are intentionally omitted;
- structured parse diagnostics: grammar, source range, node kind, and skipped
  fragment count;
- `truncated`, stating when lower-ranked content identities are omitted;
- `total_location_pairs`, `total_match_groups`, and
  `total_focused_match_groups` (all focused identities before retention);
- `configuration.focus`, including explicit paths or Git base, full
  base/merge-base/HEAD commits, working-tree semantics, changed/deleted paths,
  and focused files discovered;
- every multi-worktree `configuration.focus.worktrees` entry and its
  independent requested base/full commits;
- `configuration.focus.path_evidence`: every supported non-deleted focused
  path must be `analyzed` in strict review; report generated, resource,
  unsupported, or undiscovered statuses separately; verify `changed_lines`
  when Git hunk focus applies;
- `configuration.focused_only`, which is true for the canonical staged path
  that parses the full repository but scores only hunk-intersecting pairs;
- `configuration.input`: staged Git-index digest, HEAD, and false working-tree
  and untracked inclusion flags; the digest also binds a loaded baseline blob;
- `configuration.scope` and `scope_roots`, which participate in baseline
  compatibility;
- `configuration.profile`, its named defaults, and neighboring effective fields;
- `configuration.scan_profile_digest`, `baseline_profile_digest`, and
  `baseline_profile_status`, requiring exact schema-4 compatibility when a
  baseline suppresses findings;
- `configuration.ignore_file_evidence`, confirming each loaded ignore source
  has exact SHA-256 evidence in the scan-profile digest;
- `focused` and `focused_occurrences`, rather than inferring focus from
  sampled occurrences;
- suppression counts, distinguishing suppressed location pairs from baseline
  content identities;
- stable `content_pair_id` across scans;
- every retained `profiles[].occurrences`, noting occurrence-sampling truncation;
- compatible `comparison_domain` and accurate `fragment_kind` descriptions;
- every `configuration.priority_paths` weight. `priority-path:` signals are
  presentation policy, not inferred security or refactoring confidence;
- `configuration.embedded_sql`, `statement_blocks`, `block_statements`, and
  `max_blocks_per_function` as opt-in extraction scope and bounds;
- `similarity` only as structural similarity;
- `structural_evidence`: exact weighted intersection/union, complete A-only
  and B-only totals, fingerprint alignment, and bounded feature lists. A 100%
  value means normalized feature identity only;
- `ordered:` evidence as low-weight positional/anonymous-callee structure;
  slots are fragment-local, omit source names, and do not identify equivalent
  operations;
- `shape_summary` and `shared_features` as explanations for ranking, never
  behavioral evidence; and
- `review_priority` and `review_signals`, when ranking is selected, as source
  location reasons only. Generic entry points, constructors, and anonymous
  names deliberately receive no same-name priority.

## Truncation and exit semantics

If `truncated` is true, inspect retained identity diversity before increasing
`--max-groups` to 500, and at most 1,000 when necessary. Do not set
`--max-groups`, `--max-occurrences`, `--max-pairs`, or `--max-file-bytes` to
zero unless the user explicitly requests unbounded work. Do not use
`--fail-on-match` during exploration unless project policy requires it.

Treat operational errors and unexpected schemas as failed scans. Exit `3`
means policy findings from `--fail-on-match` or `--fail-on-focused-match`, not
a crash. Use focused policy only after adopting a reviewed threshold, scope,
and exclusions. Exit `4` is incomplete coverage and takes precedence over
finding status `3`; report it as not applicable/incomplete, never clean.
