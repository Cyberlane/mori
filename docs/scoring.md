# Scoring contract

森 (*mori*) compares normalized AST feature **multisets**. Counts matter: a
function with four branches is different from a function with one branch even
when both sets contain the token `flow:if`. SQL queries use the same scoring
formula in a separate comparison domain.

## Feature construction

For each comparison fragment, the normalizer may emit:

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

SQL query normalization adds canonical structure for projections, sources,
joins, predicates, grouping, ordering, CTEs, set operations, values, conflict
handling, returning clauses, and `SELECT`/`INSERT`/`UPDATE`/`DELETE` operations.
It also maps parameters and broad literal kinds without preserving names or
values. These features describe syntax, not query equivalence or database
behavior.

The `generic` and `postgresql` parsers share the `sql` review family and
`sql-query` domain, but one scan selects one parser dialect for all `.sql`
files. PostgreSQL grammar wrappers and keyword nodes are reduced into the same
query vocabulary; a matching score still does not account for schemas,
constraints, plans, permissions, transactions, or PL/pgSQL behavior.

Bash/POSIX shell and Zsh use separate grammars in one review family. Mori maps
their grammar-specific word, variable-reference, and glob nodes to shared
canonical features. A 100% result across those parsers is normalized structural
identity, not byte identity or proof that shell options and runtime semantics
are equivalent.

Swift uses its own review family. Mori maps implemented declarations, closure
parameters, arguments, identifiers, strings, collections, expressions, and
control transfers into existing canonical code features. This improves useful
Swift-to-other-language comparisons without preserving application-specific
names or literal values; it does not model Swift types, dispatch, ownership,
effects, or runtime behavior.

The normalizer has a version constant for persisted review artifacts. Any
change to the feature vocabulary, weights, canonical mappings, or
semantic-hint list increments that version.

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
- nested function bodies, which are separate code fragments.

Nested SQL queries are not separate fragments. Their normalized structure is
retained inside the enclosing top-level query.

The score does **not** model:

- data or control-flow equivalence;
- side effects;
- type resolution;
- dynamic dispatch;
- library contracts;
- aliasing;
- input/output behavior; or
- runtime values.

For SQL this also excludes schema resolution, constraints, indexes, triggers,
query plans, transaction context, permissions, and dialect-specific runtime
semantics.

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

These are starting ranges, not universal quality levels. Start cross-language
repository scans at `--min-tokens 40` so small callbacks and wrappers do not
dominate, and lower the floor toward 12 only for deliberately broad
exploration.

## Candidate pruning

If feature bag \(A\) has no more features than \(B\), its maximum possible
weighted Jaccard score is \(|A|/|B|\). 森 sorts fragments by feature count and
stops considering larger partners as soon as this upper bound falls below the
threshold.

`candidate_pairs` counts pairs that passed size pruning and language filtering
and were actually scored. `--max-pairs` is checked before each score.

`total_location_pairs` counts every source-location pair at or above the
threshold. Pairs with the same stable content identity are aggregated into one
group; `total_match_groups` counts those identities. Unless `--max-groups 0`
is used, 森 retains only the requested best groups and sets `truncated` when
the report omits lower-ranked identities. `--max-occurrences` separately bounds
the displayed locations for each fingerprint while retaining the exact count.

Groups are ordered by descending similarity, then descending minimum feature
count, descending represented location-pair count, and stable identity. The
second key favors more substantial shared evidence among equal scores; it is
not an empirically validated actionability score.

## Explanations

Reports first include a non-semantic shared-shape summary of canonical calls,
branches, loops, switches, returns, and bindings. They then include up to eight
raw shared features, ordered by:

1. descending shared count; and
2. ascending feature name.

An explanation helps answer “why did this group score highly?” It is not a
complete decomposition of the numerator or a proof that the occurrences
should be refactored. Domain descriptions still require source inspection.

Every fragment report includes a stable content fingerprint derived from its
normalized feature bag. Feature names are sorted before SHA-256 hashing and
the result is truncated to 16 hexadecimal characters. Formatting, comments,
literal values, most identifiers, line numbers, and file position do not
affect this identity. A content-pair ID joins its two fragment fingerprints in
lexical order, so pair order does not affect the ID. Every qualifying location
pair with that identity belongs to one report group. This is useful for review
workflows, but it also means an identical accepted fragment in a new location
has the same content identity.

## JSON schema

The top-level shape is:

```json
{
  "schema_version": 15,
  "tool": {
    "name": "mori",
    "version": "<version>",
    "revision": "<full source revision>",
    "source_date": "<RFC3339 commit time>",
    "modified": false,
    "go_version": "<Go version>",
    "goos": "<target OS>",
    "goarch": "<target architecture>",
    "normalization_version": 8
  },
  "threshold": 0.85,
  "files": 4,
  "fragments": 4,
  "candidate_pairs": 0,
  "total_location_pairs": 0,
  "total_match_groups": 0,
  "total_focused_match_groups": 0,
  "suppressed_location_pairs": 0,
  "suppressed_match_groups": 0,
  "truncated": false,
  "groups": [],
  "warnings": [],
  "file_coverage": [],
  "coverage": {
    "supported_files": 4,
    "analyzed_files": 4,
    "fragment_files": 4,
    "zero_fragment_files": 0,
    "generated_excluded_files": 0,
    "warning_files": 0,
    "warning_count": 0,
    "parse_diagnostic_files": 0,
    "parse_diagnostic_count": 0,
    "unsupported_extensions": []
  },
  "configuration": {
    "profile": "review",
    "ignore_files": [],
    "ignore_file_evidence": [],
    "respect_ignore": true,
    "excludes": [],
    "min_tokens": 40,
    "max_groups": 250,
    "max_occurrences": 10,
    "max_pairs": 5000000,
    "max_file_bytes": 2097152,
    "comparison_domain": "code",
    "sql_dialect": "generic",
    "embedded_sql": false,
    "statement_blocks": false,
    "block_statements": 3,
    "max_blocks_per_function": 64,
    "ranking": "review",
    "priority_paths": [],
    "same_language_only": true,
    "cross_language_only": false,
    "language_pairs": [],
    "require_coverage": true,
    "min_file_coverage": 0.95,
    "max_zero_fragment_files": 2,
    "fail_on_warning": false,
    "fail_on_parse_diagnostic": true,
    "scan_profile_digest": "<sha256>"
  }
}
```

Group objects include `content_pair_id`, `location_pairs`, one or two content
profiles, occurrence counts and locations, a shape summary, and raw shared
features. Schema 12 added bounded `literal_evidence` when at least one compared
location pair contains literals. It reports compared pairs, pairs with
differences, the maximum differing positions, and literal-count mismatches.
Literal values and their internal digests are never serialized; this evidence
does not affect scores, fingerprints, ordering, or baselines. Fragment
occurrences expose language family, `comparison_domain`,
`fragment_kind`, nesting depth, parent identity, and the number of excluded
nested functions. The current domains are `code` with `function`, shell
`script`, and opt-in `block` fragments, and `sql-query` with `query` fragments
from `.sql` files or opt-in embedded Go SQL. Cross-domain and
cross-fragment-kind pairs are never candidates.
Selected comparison domains are applied before parsing. Same-language mode
compares within review families, including TypeScript with TSX. Warnings can
include bounded parser node ranges and skipped-fragment counts.
Suppression fields separate affected source pairs from content identities.
Consumers should reject or explicitly handle unknown schema versions.

Schema 10 adds `review_priority` and `review_signals` to every group and records
the effective `configuration.ranking`. `--ranking review` uses these explicit
location-level signals before the established structural ordering. Identifiers
are used only for the disclosed same-name signal; they remain absent from
fingerprints and similarity scores. Focused groups always remain first.

Review priority is the sum of disclosed, overlapping signals: 4 for a
same-named pair across directories, 2 for any cross-directory pair, 2 for a
same-named pair across files, 1 for any cross-file pair, and 1 when the content
identity represents multiple location pairs. The existing structural
comparator breaks ties. This is a shortlist-ordering heuristic, not semantic
or refactoring confidence.

Schema 12 also records effective `priority_paths`. Each matching configured
rule adds its declared weight once per group and emits a
`priority-path:GLOB(+WEIGHT)` signal. These rules are deterministic,
presentation-only project policy; Mori does not infer security, reachability,
or domain risk from source names.

Schema 13 records `embedded_sql`, `statement_blocks`, `block_statements`, and
`max_blocks_per_function`. Normalization version 8 covers the expanded opt-in
comparison-unit contract. Statement blocks are fixed-size windows, retain
parent-function linkage, and exclude overlapping same-file windows from pair
eligibility. Embedded SQL uses source-mapped host literal locations and the
explicitly selected SQL dialect. Existing baselines require review and
regeneration because the normalization version changed, even when the new
extractors remain disabled.

Schema 14 adds an exact `coverage` summary, effective strict coverage-policy
values, aggregate unsupported-extension counts, and pre-token-floor boundary
evidence in every `file_coverage` entry. A zero-fragment analyzed file records
one deterministic reason: `no_boundaries`, `below_token_floor`,
`invalid_fragments`, or `resource_limit`; generated exclusions record
`generated_excluded`. Generated exclusions are supported files but never enter
the analyzed-file coverage denominator. Consumers can therefore enforce exact
file-level policy without reconstructing totals or receiving a list of
unsupported paths. Normalization remains version 8 and baseline schema remains
version 2.

Schema 15 records the active `configuration.scan_profile_digest` and, when a
baseline is loaded, its digest and compatibility status. The digest covers
effective candidate selection, threshold, dialect, fragment policy, explicit
exclusions, loaded ignore-file paths and content, resource bounds, and strict
coverage policy. Presentation-only ranking, focus, and output bounds are
excluded. Normalization remains version 8. Baseline schema advances to 3 for
the same profile evidence and durable entry classifications.

Schema 9 added a deterministic `file_coverage` array with one entry per analyzed
or generated-excluded supported file. Each entry records its language, review
family, comparison domain, analysis status, generated-source classification,
fragment count, skipped-fragment count, and parse-diagnostic count. Consumers
must inspect zero-fragment and excluded entries instead of inferring per-file
coverage from the aggregate `files` and `fragments` totals.

Schema 8 added `configuration.focus.worktrees` for explicit multi-worktree Git
focus. Each entry contains `root`, `requested_base`, full `base_commit`,
`merge_base`, and `head_commit` values, working-tree and untracked inclusion
flags, plus changed and deleted paths relative to that root. A single
`--changed-since` scan retains the established scalar focus fields. Multi-root
mode also exposes deterministic root-qualified aggregate `changed_paths` and
`deleted_paths`. This reporting change does not alter normalization version 6,
fragment fingerprints, similarity scores, or baseline schema 2.

Official release binaries populate the full `revision` and `source_date`.
Source-built commands such as `go install ...@version` or `go run
...@version` can populate `version` while leaving either provenance field as
`unknown` because the Go module build does not always contain VCS settings.
Treat such a report as provenance-incomplete: it can support an explicitly
disclosed exploratory local review, but not a provenance-sensitive audit or CI
gate. Do not infer a revision or source date from the version string.

Paths are relative to the current working directory when possible. Lines are
one-based and inclusive. A future breaking shape change must increment
`schema_version`; adding an optional field still requires documentation and
consumer review.

## Baselines

Use `--baseline <path>` to suppress candidates that maintainers have reviewed
and accepted as intentional structural similarity. This records review
acceptance, not semantic or behavioral equivalence. Missing or incompatible
baseline files are errors; Mori never treats a failed load as an empty set.

Accept one reviewed identity from a complete scan:

```sh
mori baseline add \
  --baseline mori-baseline.json \
  --identity <content-pair-id> \
  --classification intentional \
  --note 'Reviewed with the owning team' \
  .
```

Baseline schema 3 records an explicit `identity_scope`, deterministic
`scan_profile_digest`, canonical profile fields, and optional durable
`classification` and `note` values. Supported classifications are
`intentional`, `necessary-duplication`, `test-fixture`, `generated`, and
`other`. `baseline edit` updates or clears metadata without altering
acceptance, and `baseline remove` revokes every accepted entry using one
content identity.

The default `content` scope accepts a normalized content-pair identity in every
location, including future identical copies. A path-scoped baseline records
reviewed source-path pairs instead, so a copy in a new file reappears. In that
scope, one selective `baseline add --identity` accepts all exact path pairs for
the identity in the active complete scan.

`baseline update` computes and prints replacement counts but does not write a
file unless `--accept-all` is explicit. Existing notes and classifications are
preserved for retained entries. Schema-1 and schema-2 baselines remain readable
for suppression, with a visible legacy-profile warning, but mutation is
refused until `baseline migrate --accept-profile` explicitly binds the current
complete scan profile. A schema-3 profile mismatch fails closed until options
match or that explicit migration is performed.

The file also records the Mori version, normalization version, threshold,
stable identities, and human-readable locations. The normalization version
must match the running binary; after a normalization change, run `baseline
update` deliberately and review the resulting diff.

`baseline prune` removes entries whose IDs no longer occur, while
`baseline prune --check` reports stale entries and exits with status `3`
without modifying the file. Both commands scan without suppression and use an
unlimited report internally so bounded display retention cannot make the
baseline incomplete. Every mutating baseline command refuses truncation and
warnings by default. After review, repeat `--allow-warning KIND` only for each
intentionally accepted warning category.
