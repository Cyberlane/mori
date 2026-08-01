# Scoring contract

森 (*mori*) compares normalized AST feature **multisets**. Counts matter: a
function with four branches is different from a function with one branch even
when both sets contain the token `flow:if`.

## Feature construction

For each function-like fragment, the normalizer may emit:

| Feature prefix | Meaning | Example |
| --- | --- | --- |
| `node:` | canonical or grammar-specific node | `node:flow:return` |
| `class:` | broad cross-language class | `class:operation` |
| `edge:` | canonical parent-child relation | `edge:flow:return>expression:call` |
| `role:` | selected grammar field role | `role:condition>expression:comparison` |
| `semantic:` | curated operation-family hint | `semantic:membership` |

Semantic hints currently cover membership, pattern matching, length, trimming,
case conversion, filtering, mapping, and reduction. They have weight two.
Everything else has weight one.

These hints only say that a call *looks like* a familiar operation based on its
callee name. User-defined `contains` or `map` methods can mean something else.

## Weighted Jaccard

Let \(A_f\) and \(B_f\) be the counts for feature \(f\):

\[
J(A,B)=\frac{\sum_f \min(A_f,B_f)}{\sum_f \max(A_f,B_f)}
\]

The result is in \([0,1]\):

- `1.0`: identical normalized feature counts;
- near `1.0`: very similar normalized structure;
- near `0.0`: little normalized structure in common.

Empty feature bags score zero and are not emitted by normal parsing.

## What the score ignores

- identifier spelling;
- literal values, except their broad kinds;
- comments and formatting;
- selected type-only syntax; and
- nested function bodies, which are separate fragments.

The score does **not** model:

- data or control-flow equivalence;
- side effects;
- type resolution;
- dynamic dispatch;
- library contracts;
- aliasing;
- input/output behavior; or
- runtime values.

Consequently, the words “duplicate” and “semantic” should be treated as review
hypotheses, not conclusions.

## Threshold selection

The default `0.70` is discovery-oriented.

Suggested calibration process:

1. collect representative pairs your maintainers agree should be reviewed;
2. collect nearby pairs that should stay separate;
3. run both sets through the same version and options;
4. choose the highest threshold that retains the useful positives;
5. review false positives before enabling `--fail-on-match`; and
6. pin the 森 version in CI because normalization changes can change scores.

Typical starting points:

- `0.85`–`0.95`: low-noise, same-language clones;
- `0.70`–`0.85`: structurally close rewrites;
- `0.60`–`0.70`: cross-language exploration.

These are starting ranges, not universal quality levels.

## Candidate pruning

If feature bag \(A\) has no more features than \(B\), its maximum possible
weighted Jaccard score is \(|A|/|B|\). 森 sorts fragments by feature count and
stops considering larger partners as soon as this upper bound falls below the
threshold.

`candidate_pairs` counts pairs that passed size pruning and language filtering
and were actually scored. `--max-pairs` is checked before each score.

`total_matches` counts every pair at or above the threshold. Unless
`--max-matches 0` is used, 森 retains only the requested best matches during
scoring and sets `truncated` when the report is shorter than that exact total.

## Explanations

Reports include up to eight shared features. They are ordered by:

1. descending shared count; and
2. ascending feature name.

An explanation helps answer “why did this pair score highly?” It is not a
complete decomposition of the numerator or a proof that the pair should be
refactored.

## JSON schema

The top-level shape is:

```json
{
  "schema_version": 1,
  "threshold": 0.7,
  "files": 4,
  "fragments": 4,
  "candidate_pairs": 6,
  "total_matches": 0,
  "truncated": false,
  "matches": [],
  "warnings": []
}
```

Paths are relative to the current working directory when possible. Lines are
one-based and inclusive. A future breaking shape change must increment
`schema_version`; adding an optional field still requires documentation and
consumer review.
