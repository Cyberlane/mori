---
name: mori-review-similarity
description: Use Mori to find and assess structurally similar functions in local source code. Apply when adding or reviewing functions, investigating duplication, planning refactors, porting logic across languages, or checking whether changed code resembles an existing implementation.
---

# Review structural similarity with Mori

Use Mori as an evidence-producing local CLI. Treat its scores as structural
signals for human or agent review, never as proof of equivalent behavior.

## Establish the review scope

Read the project's instructions before scanning. Prefer the narrowest source
roots that still include both changed code and plausible existing
implementations. Do not scan only changed files: that would miss older code
that the new work may resemble.

Use project-provided thresholds and exclusions when present. Otherwise begin
with:

- `0.85` for focused same-language review;
- `0.65` for cross-language exploration;
- `--min-tokens 40` for a low-noise same-language first pass;
- up to 250 reported content-pair groups, from which at most 25 are deeply
  reviewed; and
- Mori's default file-size and candidate-pair limits.

## Verify the tool

Run:

```sh
command -v mori
mori version
mori languages
```

If Mori is unavailable, stop and give the user an installation command. Do not
download software or change global configuration without authorization.

## Produce structured evidence

For a focused review, run:

```sh
mori scan \
  --format json \
  --threshold 0.85 \
  --min-tokens 40 \
  --max-groups 250 \
  --max-occurrences 10 \
  .
```

Mori honors `.gitignore`, `.moriignore`, and an upward-discovered `.mori.json`
by default. Inspect `configuration` in the JSON report to verify the effective
config, ignore files, exclusions, and pair filters. Use `--no-ignore` or
`--no-config` only when the review scope requires it.

For intentional cross-language discovery, use `--cross-language-only
--threshold 0.65 --min-tokens 12`; this compares different language families,
so TypeScript and TSX do not count as cross-language. Prefer an explicit pair
such as `--language-pair go,typescript` when only one family pairing matters.
Raise `--min-tokens` when trivial wrappers or boilerplate dominate the report.

Add repeated `--exclude` flags for project-specific irrelevant paths not
covered by ignore files. If `truncated` is true, review the retained identity
diversity first, then increase `--max-groups` to 500 and at most 1,000 when
needed. Do not use zero for `--max-groups`, `--max-occurrences`, `--max-pairs`,
or `--max-file-bytes` unless the user explicitly requests unbounded work. Do
not use `--fail-on-match` during exploratory review or unless project policy
requires it.

## Validate the report

Require `schema_version` to equal `3`. Record the Mori version and scan
options. Inspect:

- `warnings`: disclose every incomplete or failed input;
- structured parse diagnostics: inspect the grammar, source range, node kind,
  and skipped-fragment count;
- `truncated`: state when lower-ranked content identities are omitted;
- `total_location_pairs`: count all qualifying source-location pairs;
- `total_match_groups`: count distinct content-pair identities;
- suppression counts: distinguish suppressed location pairs from baseline
  content identities;
- `content_pair_id`: use the stable content identity across scans;
- `profiles[].occurrences`: inspect every retained source occurrence and note
  when occurrence sampling is truncated;
- `similarity`: report it as structural similarity only; and
- `shape_summary` and `shared_features`: use them to explain why a group ranked
  highly without treating the summary as behavioral evidence.

Treat an operational error or an unexpected schema as a failed scan. Exit
status `3` means policy findings were found with `--fail-on-match`; it is not a
tool crash.

When reviewing a change, prioritize groups where any retained occurrence is in
a changed file. Review at most 25 distinct identities deeply, not the first 25
raw location pairs. Still retain the full bounded scan as the evidence source.

For a repository with reviewed intentional candidates, use the explicit
baseline workflow:

```sh
mori baseline update --baseline mori-baseline.json .
mori scan --baseline mori-baseline.json --fail-on-match .
mori baseline prune --baseline mori-baseline.json --check .
```

Review the baseline diff before committing an update. `baseline update` and
`baseline prune` scan untruncated internally; ordinary exploratory scans
should still use a bounded `--max-groups` value. Content scope is the default:
one accepted normalized content-pair identity can suppress identical copies in
new locations. Use `baseline update --baseline-scope path` when copied code in
a new file must reappear for review. A missing or incompatible baseline is an
operational failure, not an empty baseline.

## Inspect before concluding

Open both reported source ranges. Compare identifiers and literals in their
real context, then inspect types, control flow, data flow, side effects, error
handling, callers, tests, and runtime contracts. Classify each relevant result
as one of:

- likely duplication;
- intentional structural similarity; or
- false positive.

Do not refactor, delete, or consolidate code solely because Mori reported a
match.

If an occurrence reports `nested_function_count` greater than zero, state that
its score covers the outer body while nested bodies are separate fragments.
Inspect linked nested occurrences before describing a 100% parent score as
complete duplication.

## Report the result

For each relevant candidate, provide:

```text
Candidate group: <content_pair_id> (<location-pair count>)
Representative: <left path:lines> <-> <right path:lines>
Mori score: <percentage>
Shared shape: <useful shape summary plus source-verified explanation>
Assessment: <likely duplication | intentional similarity | false positive>
Still unverified: <behavioral evidence not established by Mori>
Recommendation: <specific next check or no action>
```

End with the Mori version, exact command, config/ignore sources, warning count,
group and location-pair totals, truncation state, baseline identity scope, and
whether tests or runtime behavior were inspected.
