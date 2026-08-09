# Mori

[![CI](https://github.com/Cyberlane/mori/actions/workflows/ci.yml/badge.svg)](https://github.com/Cyberlane/mori/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Mori finds source fragments that look alike, including functions written in
different programming languages and top-level SQL queries.

Use it to find possible duplicate logic before you copy, refactor, or review
code. Mori gives you a shortlist to inspect. It cannot prove that two functions
or queries do the same thing.

## What It Does

Mori reads your source code locally and compares individual functions or SQL
queries within compatible comparison domains. It ignores details such as
formatting, comments, most variable names, and literal values so it can focus
on structural shape.

For example, it can flag a JavaScript function and a Go function that both:

- check an input;
- split it into parts;
- loop over those parts; and
- return early when something is wrong.

Each result groups every retained source occurrence with the same normalized
content-pair identity. It includes a percentage, a shared-shape summary, and
the code locations to review. A higher percentage means the functions have
more structural overlap, not that they have identical behavior.

## Install

Download a prebuilt binary for Linux, macOS, or Windows from the
[latest release](https://github.com/Cyberlane/mori/releases/latest).

Or install from source. You need Go 1.23 or newer and a C compiler.

```sh
go install github.com/Cyberlane/mori/cmd/mori@latest
mori version
```

Source-built installations can report the module version while leaving the
source revision and date as `unknown` when Go does not embed VCS settings. They
remain suitable for exploratory local review, but use an official release
binary when a report needs complete, independently verifiable provenance.

## Start Here

Run this from the root of a project:

```sh
mori scan \
  --comparison-domain code \
  --same-language-only \
  --threshold 0.85 \
  --min-tokens 40 \
  .
```

This looks for reasonably sized functions that are very similar without
letting SQL or cross-language scaffolding compete in the result ranking. Start
at `0.85` to reduce noise. If you want more possible matches, lower the
threshold gradually.

To look only for matches between different languages:

```sh
mori scan \
  --comparison-domain code \
  --cross-language-only \
  --threshold 0.65 \
  --min-tokens 40 \
  .
```

Lower `--min-tokens` toward 12 only for a deliberately broad exploratory pass;
small callbacks and wrappers commonly dominate at that floor.

TypeScript and TSX are one language family. To compare only Go with that
family, use:

```sh
mori scan \
  --comparison-domain code \
  --language-pair go,typescript \
  --threshold 0.65 \
  --min-tokens 40 \
  .
```

To try Mori against this repository's example files:

```sh
mori scan \
  --comparison-domain code \
  --cross-language-only \
  --threshold 0.70 \
  --min-tokens 12 \
  examples/email-validation
```

This deliberately small fixture uses the broad 12-token exploration floor.
Abridged example output (first group):

```text
1. 71.0% structural similarity · 1 location pair(s)
   A  fingerprint 1d4f4194cc92c56c · 1 occurrence(s)
      - validator.go:5-8  [go] LooksLikeEmail
   B  fingerprint 6d2ab0d68af9fb96 · 1 occurrence(s)
      - validator.js:1-4  [javascript] looksLikeEmail
      shared shape: 3 calls, 1 return, 1 binding
```

Read both fragments before acting on a match. Mori does not understand runtime
values, external calls, side effects, query plans, schemas, or all
language-specific behavior.

To review structurally similar SQL queries:

```sh
mori scan \
  --comparison-domain sql-query \
  --threshold 0.70 \
  --min-tokens 12 \
  examples/sql-queries
```

SQL queries are compared only with SQL queries, never with code functions.
Mori extracts top-level `SELECT` and set-operation queries plus `INSERT`,
`UPDATE`, and `DELETE` statements. It uses exact, immediately adjacent SQLC
`-- name: Name :mode` comments for display names and otherwise reports
`query@<line>`. DDL and nested queries are not independent comparison units;
nested query structure remains part of its top-level query. Common SQLite and
SQLC pagination parameters and SQLite `ON CONFLICT` column targets are parsed
without weakening diagnostics for malformed nearby syntax.

The default `generic` SQL parser is suitable for the documented SQLite and
SQLC forms. Select the dedicated PostgreSQL 18 parser explicitly for a
PostgreSQL source root or profile:

```sh
mori scan \
  --sql-dialect postgresql \
  --comparison-domain sql-query \
  --threshold 0.70 \
  --min-tokens 12 \
  path/to/postgresql
```

One scan uses one SQL dialect for every `.sql` file it discovers. Split mixed
dialect repositories into separate profiles. PostgreSQL procedural bodies are
not PL/pgSQL comparison units.

## Common Uses

Mori honors nested `.gitignore` and `.moriignore` files during directory scans.
An explicitly requested file is still scanned. Add command-line exclusions for
additional policy:

```sh
mori scan --exclude '**/*_test.go' --exclude '**/*.test.ts' .
```

Mori also classifies conservative generated-source header markers without
changing the default scan. Use `--exclude-generated` to omit recognized
generated files while retaining them as `excluded_generated` entries in the
JSON `file_coverage` inventory:

```sh
mori scan --exclude-generated .
```

Store repeatable project settings in `.mori.json`:

```json
{
  "threshold": 0.85,
  "min_tokens": 40,
  "max_groups": 250,
  "comparison_domain": "code",
  "same_language_only": true,
  "exclude": ["**/*_test.go"]
}
```

Bash/POSIX shell and Zsh use dedicated parsers in one `shell` review family.
They are included together by `--same-language-only`; use an explicit pair for
a shell-dialect-only review:

```sh
mori scan \
  --comparison-domain code \
  --language-pair bash,zsh \
  --threshold 0.85 \
  --min-tokens 40 \
  .
```

Mori searches the current directory and its parents for `.mori.json`. Use
`--config <path>`, `--no-config`, or `--no-ignore` to control discovery. See
[Project configuration](docs/configuration.md) for the complete contract.

Write results as JSON for a script or CI system:

```sh
mori scan --format json .
```

Mori emits a `coverage` warning when a scan discovers no supported files or
extracts no comparison fragments. Such a result is not evidence that the
repository has no duplication. Use `--require-coverage` in automation to write
the report and exit with status `4` when either condition occurs:

```sh
mori scan --format json --require-coverage .
```

Schema-8 reports embed deterministic `tool` build provenance, comparison
selection, domain and fragment-kind metadata, and exact focus metadata. They do
not include a scan timestamp, hostname, username, source body, diff, or Git
remote.

Fail a CI job when Mori finds a match at your threshold:

```sh
mori scan --threshold 0.85 --require-coverage --fail-on-match .
```

With `--fail-on-match`, Mori exits with status `3` when it finds a match.
Coverage failure takes precedence and exits with status `4`.

For change review, keep the full repository comparison universe while putting
groups that touch changed files first:

```sh
mori scan --changed-since origin/main --threshold 0.85 .
```

The revision must already exist locally. Mori uses the merge base through the
current working tree, including staged, unstaged, and untracked non-ignored
files; it never fetches a remote. Add repeatable `--focus-path <path>` values
for explicit paths. `--fail-on-focused-match` exits with status `3` only when
an unsuppressed focused group exists and is mutually exclusive with
`--fail-on-match`.

One revision cannot safely describe multiple Git histories. Add each nested or
sibling worktree explicitly with its own locally available revision:

```sh
mori scan \
  --changed-since origin/main \
  --changed-worktree byparr=origin/main \
  --threshold 0.85 \
  .
```

`--changed-since` describes the primary worktree; repeatable
`--changed-worktree PATH=REVISION` values describe the other worktrees. Mori
requires every discovered file to belong to a resolved root, never inherits a
parent revision for a nested repository, and records each root's requested
base, full resolved commits, changed paths, and deleted paths in schema-8 JSON.
Use only repeated `--changed-worktree` values when every scanned root should be
explicit. Excluding and scanning a nested worktree separately remains valid;
never interpret an excluded repository as unchanged. Mori bounds one scan to
64 explicit worktrees and 100,000 combined changed and deleted paths.

To record intentional candidates and use Mori as a stable CI gate:

```sh
mori baseline update --baseline mori-baseline.json --threshold 0.85 .
mori scan --baseline mori-baseline.json --threshold 0.85 --fail-on-match .
mori baseline prune --baseline mori-baseline.json --check .
```

`baseline update` accepts every candidate in the current untruncated scan, so
review its file diff before committing it. Baselines are opt-in, and a
suppressed candidate is reported as both a content-identity count and a
location-pair count. The default `content` scope follows identical normalized
content into new locations. Use `baseline update --baseline-scope path` when a
copy in a new file must appear for review. The conventional file name is
`mori-baseline.json`; pass it explicitly with `--baseline`.

## Supported Languages

| Parser language | Review family | Comparison domain | File types | Extensionless shebangs |
| --- | --- | --- | --- | --- |
| Bash / POSIX shell | Shell | code | `.sh`, `.bash` | `sh`, `dash`, `bash` |
| Go | Go | code | `.go` | — |
| JavaScript and JSX | JavaScript | code | `.js`, `.jsx`, `.mjs`, `.cjs` | `node`, `nodejs` |
| TypeScript | TypeScript | code | `.ts`, `.mts`, `.cts` | — |
| TSX | TypeScript | code | `.tsx` | — |
| Python | Python | code | `.py`, `.pyi` | `python`, `python3` |
| PostgreSQL queries | SQL | sql-query | `.sql` with `--sql-dialect postgresql` | — |
| Rust | Rust | code | `.rs` | — |
| Swift | Swift | code | `.swift` | — |
| Zsh | Shell | code | `.zsh` | `zsh` |
| SQL queries | SQL | sql-query | `.sql` | — |

For files with no extension, Mori reads at most the first 256 bytes and uses a
supported direct or `/usr/bin/env` shebang without executing the interpreter.
An extension always takes precedence over a conflicting shebang. Run
`mori languages` to see the exact languages and shebang names in your installed
version; `mori languages --help` describes the columns.

## Known Parser Limits

Tree-sitter recovery is visible in report warnings as potentially incomplete
comparison coverage, and any comparison fragment containing a parse error is
skipped with an explicit count. Swift support extracts implemented functions,
initializers, deinitializers, and closures. Protocol requirements, computed
properties, accessors, and subscripts are not independent comparison units;
unsupported Swift syntax can still produce visible diagnostics. Generic SQL
dialect extensions outside Mori's pinned grammar and bounded SQLite/SQLC
adaptations may produce diagnostics or incomplete coverage. The PostgreSQL
parser targets PostgreSQL 18.3 syntax but does not extract PL/pgSQL bodies as
independent comparison units. Mori also applies a bounded,
byte-preserving repair for recognized cases of the
[upstream raw-ampersand JSX text grammar issue](https://github.com/tree-sitter/tree-sitter-javascript/issues/366).
Other JavaScript and TSX parse errors remain visible and invalidate affected
function fragments. The pinned Zsh grammar requires `:` rather than arbitrary
paired delimiters for the `s::`, `n::`, and `b::` glob-qualifier forms; affected
functions produce visible parser diagnostics.

## For AI Coding Tools

Mori includes an optional skill for compatible coding agents. It helps an agent
use Mori results as review leads rather than treating a score as proof.

Install it in the current project:

```sh
mori skill install --project .
```

Install it for your user account instead:

```sh
mori skill install --global
```

## More Detail

- [How Mori scores fragments](docs/scoring.md)
- [Scan selection controls](docs/scan-selection.md)
- [Project configuration](docs/configuration.md)
- [Architecture](docs/architecture.md)
- [Add a language](docs/adding-a-language.md)
- [Contributing](CONTRIBUTING.md)

## Development

```sh
make check
```

Run Mori's explicit production-code self-review profile with:

```sh
make dogfood
```

The profile in `configs/self-review.mori.json` is not auto-discovered. It scans
same-language code at the 0.85 threshold and 40-token floor while excluding
tests and examples, so normal development and cross-language example commands
keep their own selection settings.

## License

Mori is available under the [MIT License](LICENSE).
