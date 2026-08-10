# Project configuration and ignore handling

Mori can load one strict JSON configuration named `.mori.json`. It searches
from the current working directory upward. Use `--config <path>` to select a
different file or `--no-config` to disable configuration loading.

Unknown fields, malformed JSON, non-regular files, and files larger than one
MiB are errors. Relative `baseline` paths resolve from the configuration
file's directory. A selected profile supplies defaults, explicit configuration
fields override profile values, and explicit command-line values override
both. Repeated `--exclude` and `--language-pair` values are additive. A
command-line `--comparison-domain` overrides the configured domain. `--sql-dialect`
overrides the configured SQL parser dialect.

Create a review-oriented config with `mori init [directory]`. The command
writes deterministic, explicit values so the file remains reviewable if a
future Mori version changes a profile. It never replaces an existing
`.mori.json` unless `--force` is supplied. `mori init --stdout` writes the
template without changing the filesystem, and `--profile review|explore|sql`
selects the template.

## Profiles

`profile` accepts:

- `review`: same-language code, threshold `0.85`, 40-token floor, at most 250
  groups and 10 retained occurrences per fingerprint, review ranking,
  generated-source exclusion, and required aggregate coverage;
- `explore`: the broad compatibility defaults—threshold `0.70`, 12-token
  floor, at most 100 groups, structural ranking, every comparison domain, and
  no required coverage or generated-source exclusion; or
- `sql`: SQL-query comparison, threshold `0.70`, 12-token floor, at most 250
  groups and 10 retained occurrences per fingerprint, review ranking,
  generated-source exclusion, and required aggregate coverage.

Profiles deliberately do not guess project-specific test, migration, generated
router, or framework exclusions. Add those only after auditing the repository's
source categories and the first report.

The selected profile is recorded in schema-16 reports. Omitting `profile`
retains the legacy defaults. A CLI `--profile` replaces a configured profile;
explicit fields from the config are then applied, followed by explicit CLI
flags. An explicit language-selection mode replaces the profile's mode, so
`--profile review --cross-language-only` does not require a separate
`--same-language-only=false` flag.

## Fields

```json
{
  "profile": "review",
  "threshold": 0.85,
  "min_tokens": 40,
  "max_groups": 250,
  "max_occurrences": 10,
  "max_pairs": 5000000,
  "max_file_bytes": 2097152,
  "workers": 8,
  "format": "json",
  "comparison_domain": "code",
  "sql_dialect": "generic",
	"embedded_sql": false,
	"statement_blocks": false,
	"block_statements": 3,
	"max_blocks_per_function": 64,
  "ranking": "review",
  "priority_paths": ["**/auth/**=25", "**/security/**=25"],
  "same_language_only": true,
  "cross_language_only": false,
  "language_pairs": [],
  "fail_on_match": false,
  "require_coverage": true,
	"min_file_coverage": 0.95,
	"max_zero_fragment_files": 2,
	"fail_on_warning": false,
	"fail_on_parse_diagnostic": true,
  "exclude_generated": true,
  "baseline": "mori-baseline.json",
  "baseline_scope": "content",
  "exclude": ["**/*_test.go"],
  "respect_ignore": true
}
```

`comparison_domain` accepts one case-insensitive domain ID printed by
`mori languages`. The selected domain is applied before parsing, so other
domains do not affect file counts, warnings, focus, or pair limits. An empty or
omitted value selects every registered domain.

`sql_dialect` accepts `generic` or `postgresql` and defaults to `generic`.
It selects the parser for every discovered `.sql` file and does not affect
non-SQL files. Use separate scans when one repository contains multiple SQL
dialects.

`embedded_sql` is an explicit opt-in for direct string arguments to recognized
Go database methods. It requires `comparison_domain` to be `sql-query` and uses
the selected `sql_dialect`. Arbitrary strings, variables, concatenations, and
receiver-type inference remain outside extraction. Fixed safety limits skip a
host file above 1,000 recognized calls and skip a decoded query above 256 KiB,
with coverage warnings.

`statement_blocks` enables fixed-size statement windows inside code functions.
`block_statements` accepts 2 through 10 and defaults to 3.
`max_blocks_per_function` accepts 1 through 256 and defaults to 64. Exceeding
the cap skips block extraction for that function with a visible coverage
warning instead of returning a partial hidden sample. Embedded SQL and
statement blocks are mutually exclusive scan modes.

`ranking` accepts `structural` or `review` and defaults to `structural`.
Structural ordering sorts by similarity score, shared evidence mass,
represented location-pair count, and stable identity. Review ordering first
uses explicit location signals, then the existing structural order. It favors
same-named candidates across directories, other cross-directory and cross-file
candidates, and identities representing repeated location pairs. It does not
change scores, fingerprints, candidate membership, focus priority, or baseline
suppression. The equivalent command-line form is `--ranking review`.

`priority_paths` contains at most 32 deterministic `GLOB=WEIGHT` rules. Each
weight must be an integer from 1 to 100. When review ranking is selected, a
group receives a rule's weight once if either side of any retained location
pair matches the doublestar glob. Repeat the equivalent `--priority-path`
option for command-line use. Rules change presentation order only: they do not
change similarity, fingerprints, baselines, or whether a group qualifies.
Use them for reviewed project categories such as authorization or security
paths; a matching path is not evidence that a finding is risky or actionable.

`same_language_only` compares within review families. TypeScript and TSX are
one family and remain comparable. It is mutually exclusive with
`cross_language_only` and `language_pairs`. `language_pairs` accepts language
IDs or families; the `typescript` selector includes both TypeScript and TSX.
Explicit pairs must belong to a selected comparison domain.

`max_groups` bounds distinct content-pair identities. `max_occurrences` bounds
the locations shown for each fingerprint while retaining the exact occurrence
count. Zero opts into an unlimited value and should be used deliberately.
`--max-matches` remains a deprecated alias for `--max-groups`.

`require_coverage` requires at least one supported file and one extracted
comparison fragment. When the requirement is not met, Mori still writes the
deterministic report and its `coverage` warning, then exits with status `4`.
This distinguishes an unsupported or inapplicable scan from a clean scan with
no qualifying similarity groups. The command-line override is
`--require-coverage`; use `--require-coverage=false` to disable a configured
requirement for an explicitly exploratory invocation.

`min_file_coverage` accepts `0` through `1`; zero disables the policy. Its
numerator is the number of analyzed files producing at least one comparison
fragment, and its denominator is every analyzed supported file. Files with
status `excluded_generated` remain visible evidence but do not enter the
denominator. The command-line form is `--min-file-coverage`.

`max_zero_fragment_files` accepts `-1` or greater; `-1` disables the policy.
It limits analyzed supported files that produced no comparison fragments. The
command-line form is `--max-zero-fragment-files`.

`fail_on_warning` fails on any discovery, parsing, resource, focus, or coverage
warning. `fail_on_parse_diagnostic` is narrower and fails only when a file has
Tree-sitter parse diagnostics. Their command-line forms are
`--fail-on-warning` and `--fail-on-parse-diagnostic`. Every strict coverage
policy writes the report first and then exits with status `4`. Coverage policy
failure takes precedence over finding status `3`, and baseline update or prune
checks the policies before changing its baseline file.

`exclude_generated` excludes supported files only when Mori recognizes a
conservative generated-source comment marker in the first 8 KiB, including
`Code generated ... DO NOT EDIT`, `@generated`, and common automatically
generated-file headers. The exact `Generated by typeshare` header is also
recognized without treating arbitrary `Generated by` comments as generated
source. The equivalent command-line flag is
`--exclude-generated`. Excluded files remain visible in `file_coverage` with
status `excluded_generated`, the recognized marker class, and zero fragments.
The default remains `false`, so upgrading never silently removes source from a
scan.

## Ignore files

Directory scans honor `.gitignore` and `.moriignore` files from the working
directory through nested scan directories. Rules are evaluated in order;
`.moriignore` is read after `.gitignore` in the same directory, so it can add
or negate repository rules. Common VCS, dependency, and build directories are
still excluded by Mori's built-in policy. A linked worktree's `.git` control
file is VCS metadata and is not included in unsupported-extension counts.

Ignore files support the Git-style rules Mori needs for repository discovery:
comments, negation, directory suffixes, anchoring, and `*`, `?`, and `**`
wildcards. An ignored directory is still traversed when an applicable negation
may re-include a descendant. Invalid, oversized, symlinked, or non-regular
ignore files fail the scan instead of silently weakening exclusions.

Ignore files affect directory traversal only. A file passed explicitly remains
visible. Repeated `--exclude` globs remain additive and continue to exclude an
explicitly requested file. Use `--no-ignore` to disable both ignore-file types.

Schema-16 JSON reports record the effective profile, strict coverage policies,
options, and every loaded ignore file with its SHA-256 content evidence under
`configuration` so a scan can be
reproduced. Review focus remains
CLI-only: `--focus-path` is repeatable, `--changed-since` always requires an
explicit locally available Git revision, and repeatable
`--changed-worktree PATH=REVISION` values give every additional Git worktree
its own revision. Dynamic Git revisions are intentionally not project config.

See [Scan selection controls](scan-selection.md) for the complete validation,
execution, compatibility, and reporting contract.
