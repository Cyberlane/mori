---
name: mori-review-similarity
description: Use Mori to find and assess structurally similar functions and SQL queries in local source code. Apply when adding or reviewing functions or queries, investigating duplication, planning refactors, porting logic across languages, or checking whether changed code resembles an existing implementation.
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
- `--min-tokens 40` for an initial cross-language pass, lowering toward 12 only
  for deliberately broad exploration;
- up to 250 reported content-pair groups, from which at most 25 are deeply
  reviewed; and
- Mori's default file-size and candidate-pair limits.

## Verify the tool

Use `mori` when it is on `PATH`. When the user or project supplies an explicit
binary path, use that path consistently instead; do not reject a usable binary
only because `command -v mori` fails. Record the exact binary path and version,
and do not silently substitute an unverified executable from a project's
`bin/` or `dist/` directory.

For a `PATH` installation, run:

```sh
command -v mori
mori version
mori languages
```

For an explicit binary, run:

```sh
MORI_BIN=/absolute/path/to/mori
test -x "$MORI_BIN"
"$MORI_BIN" version
"$MORI_BIN" languages
```

If Mori is unavailable, stop and give the user an installation command. Do not
download software or change global configuration without authorization.

## Establish coverage before interpreting results

Inventory the repository languages before scanning. Inspect tracked or visible
source extensions, extensionless executable shebangs, build manifests, and
nested repositories, then compare that inventory with `mori languages`. State
which meaningful source is supported, excluded, nested, or unexamined. Do not
infer repository-wide coverage from Mori's discovered-file count alone.

`mori languages` lists the direct and `/usr/bin/env` interpreter names Mori
recognizes for extensionless files. Mori does not require the executable bit
and does not execute the interpreter. An extension takes precedence over a
conflicting shebang.

Use `--require-coverage` for review and CI commands. Exit status `4` means Mori
found no supported files or extracted no comparison fragments. The report is
still written and contains a deterministic `coverage` warning; classify the
scan as not applicable or insufficiently covered, never as a clean result.

Schema-11 reports include `file_coverage`. Inspect every supported file with
zero fragments, along with its skipped-fragment and parse-diagnostic counts,
before describing coverage. A successful aggregate scan does not excuse a
supported file that contributed no comparison units.

Split production and tests before the first deep review when the repository
contains both. Start with production-oriented exclusions that match the
project's actual layout, then run a separate test profile when test similarity
is relevant. For Go this commonly means adding `--exclude '**/*_test.go'` to
the production scan. Do not apply guessed cross-language test globs without
checking the repository. Use `--exclude-generated` when generated source is
not part of the requested review; verify every `excluded_generated` entry in
`file_coverage` rather than treating it as undiscovered.

## Produce structured evidence

For a focused review, run:

```sh
mori scan \
  --profile review \
  --format json \
  --max-occurrences 10 \
  .
```

When reviewing a branch or working-tree change and the comparison base already
exists locally, use Mori's native focus mode instead of filtering the scan:

```sh
mori scan \
  --profile review \
  --format json \
  --max-occurrences 10 \
  --changed-since origin/main \
  .
```

Mori still scans changed and unchanged files together, then prioritizes groups
with a changed occurrence. It never fetches the revision. Use repeatable
`--focus-path` for exact paths when Git-derived focus is unavailable or when a
review includes additional files. The two focus inputs are additive.

One revision does not safely describe multiple worktree histories. When the
full scan includes a nested worktree or submodule, give it its own locally
available revision with repeatable `--changed-worktree PATH=REVISION` values:

```sh
mori scan \
  --format json \
  --require-coverage \
  --changed-since origin/main \
  --changed-worktree nested=origin/main \
  .
```

`--changed-since` describes the primary root. Each `--changed-worktree` entry
is resolved independently and never inherits the primary revision. Use only
repeated `--changed-worktree` values when all scanned roots should be explicit.
If an appropriate local revision is unavailable, exclude and scan that root
separately or use exact `--focus-path` values when they accurately represent
the review. Disclose excluded or separately scanned roots; never describe one
as unchanged merely because the parent focus scan did not cover it.

Mori honors `.gitignore`, `.moriignore`, and an upward-discovered `.mori.json`
by default. Inspect `configuration` in the JSON report to verify the effective
config, ignore files, exclusions, comparison domain, and family or pair
filters. Use `--no-ignore` or `--no-config` only when the review scope requires
it.

For intentional cross-language discovery, choose exactly one filtering mode.
Use this broad family selection:

```sh
mori scan \
  --comparison-domain code \
  --cross-language-only \
  --require-coverage \
  --threshold 0.65 \
  --min-tokens 40 \
  .
```

Use an explicit pair when only one family pairing matters:

```sh
mori scan \
  --comparison-domain code \
  --language-pair go,typescript \
  --require-coverage \
  --threshold 0.65 \
  --min-tokens 40 \
  .
```

Never combine
`--cross-language-only` with `--same-language-only` or `--language-pair`.
TypeScript and TSX belong to one family and do not count as cross-language.
Bash/POSIX shell and Zsh likewise belong to the `shell` family. Use
`--language-pair bash,zsh` when only cross-dialect shell results are wanted.
Shell files produce a `script` comparison unit for top-level executable
statements and separate `function` units for named functions, each subject to
the token floor. Require both fragment kinds when claiming shell-file coverage;
a function-only result does not cover top-level orchestration, and a script
result excludes function bodies.
Swift support covers implemented functions, initializers, deinitializers, and
closures. Treat protocol requirements, computed properties, accessors, and
subscripts as unexamined comparison units, and disclose any Swift parser
warnings as incomplete coverage. Mori applies bounded compatibility repairs for
several recognized valid Swift forms. A repaired optional-await binding retains
the bound expression but omits its `try? await` wrapper from structural
features, so inspect async and error-handling behavior in source.
Lower `--min-tokens` toward 12 only when a deliberately broad exploratory pass
is worth the additional callbacks, wrappers, and boilerplate.

For SQL review, use a separate deliberate scan profile such as:

```sh
mori scan \
  --profile sql \
  --format json \
  --max-occurrences 10 \
  path/to/queries
```

For PostgreSQL source, add `--sql-dialect postgresql`. The default is
`generic`; one invocation applies one dialect to every discovered `.sql` file,
so split mixed-dialect roots into separate scans. The PostgreSQL parser covers
PostgreSQL 18.3 SQL syntax but does not make PL/pgSQL bodies independent
comparison units.

SQL queries occupy the `sql-query` comparison domain and are never compared
with code functions. Do not request `sql` in a language pair with a code
language. Mori extracts top-level `SELECT`/set-operation, `INSERT`, `UPDATE`,
and `DELETE` statements; DDL and nested queries are not independent fragments.
Treat SQLC names as location labels, not fingerprint inputs. Mori supports
common `?` and SQLC `LIMIT`/`OFFSET` parameters plus SQLite `ON CONFLICT`
column targets. Inspect warnings for other unsupported dialect syntax and
verify schemas, permissions, transaction context, query plans, and tests before
recommending consolidation.

Add repeated `--exclude` flags for project-specific irrelevant paths not
covered by ignore files. If `truncated` is true, review the retained identity
diversity first, then increase `--max-groups` to 500 and at most 1,000 when
needed. Do not use zero for `--max-groups`, `--max-occurrences`, `--max-pairs`,
or `--max-file-bytes` unless the user explicitly requests unbounded work. Do
not use `--fail-on-match` during exploratory review or unless project policy
requires it.

## Validate the report

Require `schema_version` to equal `12`. Validate the mandatory `tool` object,
including version, revision, source date, modified flag, platform, Go version,
and normalization version. Official release binaries provide a full revision
and source date. A version-pinned source build can report its version while
leaving `revision` or `source_date` as `unknown` because Go did not embed VCS
settings. Explicitly label that report provenance-incomplete. Continue with it
only for exploratory local review, disclose the limitation in the verdict, and
do not use it for a provenance-sensitive audit or CI gate. Recommend an
official release binary when complete provenance is required; never infer a
revision or date from the version string. Inspect:

- `warnings`: disclose every incomplete or failed input;
- `file_coverage`: inspect every zero-fragment file, generated classification,
  exclusion status, skipped-fragment count, and parse-diagnostic count;
- `literal_evidence`: when present, disclose positional literal drift while
  inspecting source; values are intentionally omitted and the signal does not
  alter structural similarity;
- structured parse diagnostics: inspect the grammar, source range, node kind,
  and skipped-fragment count;
- `truncated`: state when lower-ranked content identities are omitted;
- `total_location_pairs`: count all qualifying source-location pairs;
- `total_match_groups`: count distinct content-pair identities;
- `total_focused_match_groups`: count all focused identities before bounded
  group retention;
- `configuration.focus`: verify explicit paths or the requested Git base, full
  base/merge-base/HEAD commits, working-tree semantics, changed/deleted paths,
  and how many focused files were actually discovered. In multi-worktree mode,
  verify every `worktrees` entry and its independent requested base and full
  commits; do not infer nested-repository coverage from the parent entry;
- `configuration.profile`: record the selected named defaults and verify the
  neighboring effective fields rather than assuming the profile was unmodified;
- `focused` and `focused_occurrences`: use these exact group fields rather than
  inferring focus from sampled occurrences;
- suppression counts: distinguish suppressed location pairs from baseline
  content identities;
- `content_pair_id`: use the stable content identity across scans;
- `profiles[].occurrences`: inspect every retained source occurrence and note
  when occurrence sampling is truncated;
- `comparison_domain` and `fragment_kind`: require compatible domains and use
  the kind to describe functions versus queries accurately;
- `configuration.priority_paths`: disclose every project-supplied path weight
  and treat matching `priority-path:` signals as presentation policy only, not
  inferred security or refactoring confidence;
- `similarity`: report it as structural similarity only; and
- `shape_summary` and `shared_features`: use them to explain why a group ranked
  highly without treating the summary as behavioral evidence.
- `review_priority` and `review_signals`: when review ranking is selected,
  explain the source-location reasons for ordering and never present the
  priority as semantic confidence.

Treat an operational error or an unexpected schema as a failed scan. Exit
status `3` means policy findings were found with `--fail-on-match` or
`--fail-on-focused-match`; it is not a tool crash. Use the focused policy only
after the repository has adopted a reviewed threshold, scope, and exclusions.
Exit status `4` means required coverage was not met and must be reported as not
applicable or incomplete rather than as a successful clean scan.

When reviewing a change, rely on native focus ordering when available. Review
at most 25 distinct identities deeply, not the first 25 raw location pairs.
Still retain the full bounded scan as the evidence source.

Before baselining a noisy repository, first establish repeatable project scope
with `.mori.json`, `.moriignore`, or existing repository ignore files. Use
configuration and ignores for generated artifacts, design previews, vendored
code, or intentionally separate test profiles; do not use a baseline merely to
hide out-of-scope noise.

For a configured repository with reviewed intentional candidates, use the
explicit baseline workflow:

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
handling, callers, schemas, permissions, tests, and runtime contracts. Classify
each relevant result as one of:

- likely duplication;
- intentional structural similarity; or
- false positive.

Do not refactor, delete, or consolidate code solely because Mori reported a
match.

If an occurrence reports `nested_function_count` greater than zero, interpret
it using `fragment_kind`. A `function` score covers the outer function body
while nested function bodies are evaluated as separate comparison units. A
shell `script` score covers only the top-level script body while all named
function bodies are evaluated separately. The token floor still applies to
each unit. Inspect linked occurrences before describing a 100% parent score as
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
