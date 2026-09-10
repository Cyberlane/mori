# Changed and staged code review

Read this reference for an ordinary code-change review. Do not also load the
SQL, cross-language, baseline, or strict report-audit references unless the
request explicitly needs those modes.

## Defaults and tool checks

Unless the project supplies different values, start focused same-language
review at threshold `0.85`, with `--min-tokens 40`, Mori's default file-size
and candidate-pair limits, and no more than 250 reported content-pair groups.
Deeply inspect at most 25 distinct identities, not the first 25 raw location
pairs. Lower the floor toward 12 only for deliberately broad exploration.

For a PATH binary, verify:

```sh
command -v mori
mori version
mori languages
```

For an explicit binary, use it consistently and verify it directly:

```sh
MORI_BIN=/absolute/path/to/mori
test -x "$MORI_BIN"
"$MORI_BIN" version
"$MORI_BIN" languages
```

An explicit usable path is valid even when `command -v mori` fails. Do not
silently use an unverified executable from a project `bin/` or `dist/` folder.

## Focused scans

Read project instructions first. Use roots broad enough to include changed
code and plausible existing implementations. Keep one bounded JSON report
outside agent context while emitting a small summary. Set `MORI_REPORT` to an
owner-private temporary or Git-metadata path outside the tracked checkout:

```sh
mori scan \
  --profile review \
  --format agent \
  --output "$MORI_REPORT" \
  --max-occurrences 10 \
  .
```

When a locally available comparison base exists, prefer native focus ordering:

```sh
mori scan \
  --profile review \
  --format agent \
  --output "$MORI_REPORT" \
  --max-occurrences 10 \
  --changed-since origin/main \
  .
```

Mori scans changed and unchanged files together and prioritizes groups with a
changed occurrence; it never fetches the revision. Repeatable `--focus-path`
values cover exact paths when Git focus is unavailable or extra files belong
in the review. The two focus inputs are additive. Save the JSON report and
query it rather than rerunning the scan.

Mori honors `.gitignore`, `.moriignore`, and an upward-discovered `.mori.json`.
Inspect report `configuration` for effective config, ignore files, exclusions,
comparison domain, and family/pair filters. Use `--no-ignore` or `--no-config`
only when scope requires it. When named review surfaces exist, prefer
`--scope NAME`; verify `configuration.scope` and `scope_roots`. Positional
paths replace only the scope's default roots. `--format compact` is a bounded
shortlist; retain JSON for evidence.

Split production and test scans when that matches the repository. For Go,
production commonly adds `--exclude '**/*_test.go'`; check the actual layout
before using other globs. Use `--exclude-generated` only when generated code
is out of scope and verify every `excluded_generated` entry in `file_coverage`.
Add repeated `--exclude` flags for project-specific irrelevant paths.

## Coverage and staged snapshots

Inventory extensions, extensionless shebangs, build manifests, nested
repositories, and meaningful source before interpreting results. Compare the
inventory to `mori languages`; do not infer repository coverage from discovered
file counts. `--require-coverage` is required for review/CI commands. Adopted
CI policy should additionally review `--min-file-coverage`,
`--max-zero-fragment-files`, and `--fail-on-parse-diagnostic` or
`--fail-on-warning`. Exit `4` means coverage policy failure; the report is
still written and the scan is incomplete, not clean.

Current reports document `coverage` and `file_coverage`; strict report
validation is routed separately. For any code review, inspect
supported zero-fragment files, zero-fragment reasons, boundary counts, skipped
fragments, parse diagnostics, generated exclusions, and warnings. A successful
aggregate does not excuse a supported file with no comparison units.

For a pre-commit decision, prefer the immutable index snapshot. Existing
checks remain strict by default; new integrations may explicitly choose
`--policy advisory` to report similarity without blocking on it. Both policies
preserve configured coverage failures and immutable input checks. The report
`review` object separates policy status from analysis completeness.

Strict invocation:

```sh
mori review staged check --format agent --output "$MORI_REPORT" .
```

Confirm `configuration.input.mode` is `git-index`, retain HEAD and index
digest, and require both working-tree inclusion flags to be false. Staged mode
reads tracked source, ignore rules, `.mori.json`, and a baseline from that
same snapshot; a baseline must be a tracked regular file inside the worktree.
It excludes unstaged, external, and untracked content. `--include-focused`
bypasses ordinary ignore rules for focused files, but not explicit excludes,
generated policy, unsupported syntax, or resource limits.

Canonical staged review records exact changed-line intervals, parses the full
repository, and scores only pairs containing a hunk-intersecting fragment.
Unsupported staged assets remain explicit path evidence but do not create a
warning solely because they were staged.

If the owner authorizes a one-commit receipt, read the routed baseline/receipt
reference before running `mori review staged acknowledge --accept-focused`.

One revision cannot safely describe multiple worktree histories. For nested
worktrees or submodules, provide each locally available revision explicitly:

```sh
mori scan \
  --format agent \
  --output "$MORI_REPORT" \
  --require-coverage \
  --changed-since origin/main \
  --changed-worktree nested=origin/main \
  .
```

Each `--changed-worktree PATH=REVISION` is resolved independently; it does
not inherit the primary revision. If no suitable local revision exists,
exclude and scan that root separately or use exact `--focus-path` values.
Disclose excluded/separate roots rather than calling them unchanged. For
statement-block questions only, use `--statement-blocks --block-statements 3`
and verify `max_blocks_per_function`, parent metadata, and every warning;
never mix block and full-function findings.

## Inspect and report

Open both reported ranges and surrounding context. Compare identifiers and
literals in source, then inspect types, control/data flow, side effects, error
handling, callers, schemas, permissions, tests, and runtime contracts. If
`nested_function_count` is positive, use `fragment_kind`: a `function` score
covers the outer body while nested bodies are separate units. A shell `script`
score covers top-level statements while named functions are separate. Inspect
linked occurrences before calling a 100% parent score complete duplication.

Classify each candidate as likely duplication, intentional structural
similarity, or false positive. Never refactor solely because Mori reports a
match. For each relevant group report:

```text
Candidate group: <content_pair_id> (<location-pair count>)
Representative: <left path:lines> <-> <right path:lines>
Mori score: <percentage>
Shared shape: <shape summary plus source-verified explanation>
Assessment: <likely duplication | intentional similarity | false positive>
Still unverified: <behavioral evidence not established by Mori>
Recommendation: <specific next check or no action>
```

End with Mori version, exact command, config/ignore sources, warnings,
coverage, group and location-pair totals, truncation state, baseline scope if
used, and whether tests or runtime behavior were inspected.


For repeated canonical staged checks, `--cache` explicitly opts into private
local analysis reuse. It binds the complete immutable index, options, tool and
contracts; changed inputs or unverifiable cache data cause fresh analysis.
Baseline, receipt and coverage checks still run after reuse. It does not enable
feedback or add a network operation. Do not edit cache files to accept findings.

## First-review scope

Use `mori setup --agent .` (or `configure --agent` for an existing config) to
inspect editable named scope suggestions. An application scope belongs in
`scopes`, leaving base configuration inclusive when the canonical staged
gate should review tests too. Run exploratory review with `--scope application`;
run that inclusive gate without the narrower scope. Existing base exclusions
still apply, and explicitly excluded staged files still fail strict focused
coverage, including advisory mode. Never call excluded files analyzed.

`--fragment-selection production` or `tests` is opt-in and classifies conventional
test paths plus positively evidenced Rust tests/test-only ancestors. Unrecognized
code remains in production selection; this is not test-framework inference.
Inspect excluded fragment counts and use `all` when both categories matter.
