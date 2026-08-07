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
- at most 25 reported matches; and
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
  --max-matches 25 \
  .
```

For intentional cross-language discovery, use
`--cross-language-only --threshold 0.65 --min-tokens 12` so short ports remain
eligible. Raise `--min-tokens` when trivial wrappers or boilerplate dominate
the report.

Add repeated `--exclude` flags for generated or project-specific irrelevant
paths. Do not use `--max-matches 0`, `--max-pairs 0`, or
`--max-file-bytes 0` unless the user explicitly requests unbounded work. Do
not use `--fail-on-match` during exploratory review or unless project policy
requires it.

## Validate the report

Require `schema_version` to equal `2`. Record the Mori version and scan
options. Inspect:

- `warnings`: disclose every incomplete or failed input;
- `truncated`: state when the report omits lower-ranked matches;
- `total_matches`: distinguish all qualifying matches from the retained list;
- `suppressed`: disclose candidates accepted by an explicit baseline;
- `id`: use the stable pair identity when comparing reports across scans;
- `similarity`: report it as structural similarity only; and
- `shared_features`: use them to explain why a candidate ranked highly.

Treat an operational error or an unexpected schema as a failed scan. Exit
status `3` means policy findings were found with `--fail-on-match`; it is not a
tool crash.

When reviewing a change, prioritize matches where either `left.location.path`
or `right.location.path` is changed. Still retain the full bounded scan as the
evidence source.

For a repository with reviewed intentional candidates, use the explicit
baseline workflow:

```sh
mori baseline update --baseline mori-baseline.json .
mori scan --baseline mori-baseline.json --fail-on-match .
mori baseline prune --baseline mori-baseline.json --check .
```

Review the baseline diff before committing an update. `baseline update` and
`baseline prune` scan untruncated internally; ordinary exploratory scans
should still use a bounded `--max-matches` value. A missing or incompatible
baseline is an operational failure, not an empty baseline.

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

## Report the result

For each relevant candidate, provide:

```text
Candidate: <left path:lines> <-> <right path:lines>
Mori score: <percentage>
Shared evidence: <most useful shared features>
Assessment: <likely duplication | intentional similarity | false positive>
Still unverified: <behavioral evidence not established by Mori>
Recommendation: <specific next check or no action>
```

End with the Mori version, exact command, warning count, truncation state, and
whether tests or runtime behavior were inspected.
