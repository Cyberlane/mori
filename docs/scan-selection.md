# Scan selection controls

This document specifies how Mori selects comparison domains and language
families before it scores structural similarity. The controls are intended to
keep independent review profiles reproducible without forcing repositories to
encode language-specific exclusion globs.

## Goals

- Let a scan select one or more registered comparison domains directly.
- Let a scan compare fragments only within the same review family.
- Apply domain selection before parsing so excluded domains do not affect file
  counts, parser warnings, focus resolution, or candidate-pair limits.
- Preserve deterministic ordering, existing fingerprints, and baseline
  identities for the same selected fragments.
- Record every effective selection input in machine-readable reports.

The controls do not rank findings by refactoring value and do not make a
similarity score evidence of semantic or behavioral equivalence.

## Command-line contract

`--profile review|explore|sql` applies a named set of defaults before explicit
configuration fields and command-line flags. `review` is the conservative
same-language code shortlist, `explore` names the broad compatibility defaults,
and `sql` selects query review. Bare scans retain the compatibility defaults
without reporting a selected profile. See [Project configuration](configuration.md)
for the exact values and precedence contract.

`--comparison-domain <domain>` accepts one case-insensitive domain identifier
shown by `mori languages`, currently `code` or `sql-query`. Omitting it selects
every registered domain.

Examples:

```sh
mori scan --comparison-domain code .
mori scan --comparison-domain sql-query --min-tokens 12 .
```

`--sql-dialect <dialect>` selects `generic` (the default) or `postgresql` for
every discovered `.sql` file. It does not infer a dialect from source content.
Run separate profiles when a repository contains multiple SQL dialects.

`--embedded-sql` explicitly adds direct Go database-method string arguments to
a `sql-query` scan. It requires `--comparison-domain sql-query`, uses the chosen
dialect, and does not infer SQL from arbitrary strings or variables.

`--statement-blocks` adds bounded fixed-size `block` units to code scans.
`--block-statements` selects 2 through 10 statements per window, while
`--max-blocks-per-function` selects a cap from 1 through 256. These options are
incompatible with embedded-SQL mode and the `sql-query` domain.

`--same-language-only` compares fragments only when their review families are
the same. A family can contain multiple parser grammars, so TypeScript and TSX
remain comparable in `typescript`, while Bash/POSIX shell and Zsh remain
comparable in `shell`, and PHP and Hack remain comparable in `php-hack`. The filter
partitions by comparison domain first; cross-domain pairs remain impossible.

```sh
mori scan --comparison-domain code --same-language-only .
```

The three language-selection modes are mutually exclusive:

- `--same-language-only`;
- `--cross-language-only`; and
- one or more `--language-pair` values.

A comparison-domain filter can be combined with any one language-selection
mode. Every explicit language pair must belong to a selected domain. Mori
rejects contradictory options such as the following instead of returning a
misleading empty report:

```sh
mori scan --comparison-domain sql-query --language-pair go,go .
```

## Project configuration

`.mori.json` supports the equivalent fields:

```json
{
  "comparison_domain": "code",
  "sql_dialect": "generic",
	"embedded_sql": false,
	"statement_blocks": false,
	"block_statements": 3,
	"max_blocks_per_function": 64,
  "same_language_only": true
}
```

The command-line domain overrides the configured domain, which permits a
repository with a normal code profile to run a SQL-only scan without disabling
its other configuration. Boolean command-line forms can explicitly override
configured booleans, for example `--same-language-only=false`.

## Execution model

Mori validates domain names and option compatibility before discovery begins.
Source discovery selects the configured SQL grammar, detects each other
registered grammar, and drops a file whose comparison domain was not selected
before file-size checks or parsing. As a result:

- `files` and `fragments` count only selected domains;
- parser warnings come only from selected domains;
- changed-file and explicit-path focus resolve against selected files;
- `candidate_pairs` and `--max-pairs` apply only to selected domains; and
- an explicitly requested file in an unselected domain is omitted without a
  warning because the omission is the requested policy.

After parsing, `--same-language-only` partitions fragments by comparison
domain and then by review family. Every partition is scored independently,
and the resulting groups enter the existing deterministic global ordering.

## Diagnostics

Tree-sitter errors remain visible even when a source form is valid in a dialect
that Mori's pinned grammar does not fully support. The parse warning therefore
uses the neutral message `syntax tree contains parse errors; comparison
coverage may be incomplete`.

When one or more comparison fragments actually contain parse errors, the
existing `skipped_fragments` count records that fact. Reports must not claim
that fragments were invalid or skipped when the parser found errors only in
source constructs that are not comparison units, such as SQL DDL.

## Report and compatibility contract

Schema version 13 retains the effective parser and selection fields under
`configuration`, including every opt-in extraction bound:

```json
{
  "profile": "review",
  "comparison_domain": "code",
  "sql_dialect": "generic",
	"embedded_sql": false,
	"statement_blocks": false,
	"block_statements": 3,
	"max_blocks_per_function": 64,
  "same_language_only": true
}
```

The domain is normalized to its registered lowercase identifier. An empty
string means that no domain restriction was requested. Consumers must continue
to reject or explicitly handle unknown report schema versions.

Domain and family selection do not change fragment features. SQL dialect
selection chooses a different parser and is therefore recorded explicitly.
The current normalization version is 11. It includes the established opt-in
embedded-query and statement-block units, ordered evidence, and Java/C#
and PHP/Hack function boundaries and canonical mappings. Baselines created with an older
normalization version must be reviewed and regenerated. Changing a selection
profile still requires the ordinary human review expected for any baseline
scope change.

## Verification requirements

Tests must prove:

- code selection omits SQL files before parsing and warning generation;
- SQL selection omits code files;
- same-family mode includes TypeScript-to-TSX, PHP-to-Hack, and same-language pairs while
  excluding Go-to-TypeScript pairs;
- cross-language and same-language modes are mutually exclusive;
- explicit language pairs incompatible with selected domains are rejected;
- configuration loads, merges, normalizes, and reports the new fields;
- generic SQL remains the default and PostgreSQL is selected only explicitly;
- the PostgreSQL fixtures include a positive and nearby negative;
- repeated identical scans produce byte-identical JSON; and
- the neutral parser warning reports zero skipped fragments for unsupported
  DDL while retaining bounded diagnostics.

The full repository gate remains `make check`. Representative release
verification must also inspect a code-only repository scan, a SQL-only example
scan, and the documented cross-language example.
