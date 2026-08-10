# Reviewing Mori results

Mori produces a shortlist for source review. A high score means two normalized
feature multisets overlap strongly; it does not establish equivalent behavior.

## Narrow the source policy

Mori honors nested `.gitignore` and `.moriignore` files. Add explicit exclusions
only for reviewed project policy:

```sh
mori scan --exclude '**/*_test.go' --exclude '**/*.test.ts' .
```

Use `--exclude-generated` for conservatively recognized generated headers.
Those files remain visible as `excluded_generated` in JSON coverage evidence.

## Use review ranking

```sh
mori scan --ranking review .
```

Review ranking prioritizes disclosed source-location signals before ordinary
structural ordering. It does not change scores, fingerprints, or eligibility.

Projects can add deterministic presentation-only path priority:

```sh
mori scan --profile review --priority-path '**/auth/**=25' .
```

A priority path says where reviewers want to look first. It is not evidence
that a finding is risky or actionable.

## Focus change review without narrowing comparison coverage

```sh
mori scan --changed-since origin/main --threshold 0.85 .
```

The revision must already exist locally. Mori compares changed and unchanged
source together, then moves groups touching changed files forward. It includes
staged, unstaged, and untracked non-ignored files and never fetches a remote.

Use repeated `--focus-path` for explicit paths. For nested worktrees, give each
root its own locally available revision with repeated
`--changed-worktree PATH=REVISION`; one parent revision cannot describe several
Git histories safely.

## Investigate partial duplication deliberately

```sh
mori scan --statement-blocks --block-statements 3 .
```

Statement blocks are fixed-size windows inside functions. They are a separate
fragment kind, never compare with whole functions, and exclude overlapping
same-file candidates. This mode is off by default because it increases
candidate counts and can surface ordinary local symmetry.

## Read the evidence

For every relevant group:

1. Open both retained source ranges.
2. Compare identifiers and literal values in context.
3. Inspect types, control flow, data flow, side effects, callers, error paths,
   transactions, permissions, schemas, and tests.
4. Classify it as likely duplication, intentional structural similarity, or a
   false positive.
5. Refactor only when source and behavioral evidence justify it.

`100%` means normalized feature identity only. Nested functions are independent
comparison units and are excluded from their parent function's score.

Use JSON for the complete deterministic evidence contract and SARIF for editor
or code-scanning consumers:

```sh
mori scan --format json .
mori scan --format sarif .
```

See [How Mori scores fragments](../scoring.md) and
[Scan selection](../scan-selection.md) for the detailed contracts.
