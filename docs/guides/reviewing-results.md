# Reviewing Mori results

Mori produces a shortlist for source review. A high score means two normalized
feature multisets overlap strongly; it does not establish equivalent behavior.

Keep a complete report while showing a bounded review summary:

```sh
mori scan --profile review --format agent --output /private/report.json .
```

Choose a private report path outside tracked source. Inspect retained JSON
identities and source ranges without repeating analysis. The agent summary shows
at most 25 groups; an unreviewed remainder is still unreviewed, even when the
command succeeds. Repetitive initializers and UI shapes can be intentional;
review their effects and ownership before suggesting consolidation.

If a report was not retained, copy a content-pair ID and rescan explicitly with
the same scan settings:

```sh
mori explain aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbb --profile review .
```

`explain` repeats the scan without the presentation limit, then renders only
the selected identity and its structural evidence. It does not infer behavior
or retrieve a result from a remote service.

## Narrow the source policy

Mori honors nested `.gitignore` and `.moriignore` files. Add explicit exclusions
only for reviewed project policy:

```sh
mori scan --exclude '**/*_test.go' --exclude '**/*.test.ts' .
```

Use `--exclude-generated` for conservatively recognized generated headers.
Those files remain visible as `excluded_generated` in JSON coverage evidence.
Keep exploratory exclusions separate from an inclusive staged gate: excluding a
supported staged file does not count as analyzing it, and can fail focused
coverage. A source root may still include colocated tests; review paths and both
function bodies before narrowing further.

## Use review ranking

```sh
mori scan --ranking review .
```

Review ranking prioritizes disclosed source-location signals before ordinary
structural ordering. It does not change scores, fingerprints, or eligibility.

In development builds after v0.33.0, repeated small call wrappers lose the
name/repetition boost when the same name occurs in at least four directories.
The signal is `repeated-small-wrapper(-7)`. Both representative fragments must
have at most 80 normalized tokens, a call, no nested functions, and no explicit
branching, looping, binding, assignment or arithmetic nodes. This is a
conservative presentation heuristic, not evidence that the duplication is
intentional. Short date calculations and validation branches retain their
normal priority. Use structural ranking or explicit priority paths to review
deprioritized groups. No findings are automatically accepted or removed.

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

Use `mori review staged check` for a commit decision. This canonical command
reads the repository index only and enforces focused inclusion and coverage.
Inspect `path_evidence` and the separate policy/analysis outcome before calling
the review complete. [Review policy](review-policy.md) describes explicitly
opting into advisory enforcement; existing gates stay strict by default.

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
