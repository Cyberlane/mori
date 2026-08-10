# Getting started

## Install Mori

Download a native archive for Linux, macOS, or Windows from the
[latest release](https://github.com/Cyberlane/mori/releases/latest).

You can also install from source with Go 1.23 or newer and a C compiler:

```sh
go install github.com/Cyberlane/mori/cmd/mori@latest
mori version
```

Source builds can omit the revision and source date when Go does not embed VCS
settings. Use an official release binary when report provenance matters.

Releases include `mori.rb` for Homebrew, `mori.json` for Scoop, and a three-file
`Cyberlane.Mori` WinGet manifest alongside the native archives. These are
release-ready, checksum-pinned manifests; their presence does not mean an
external package index has accepted the release. Releases also include an SPDX
source-dependency SBOM and signed GitHub/Sigstore attestations. Verify a
download with:

```sh
gh attestation verify path/to/archive --repo Cyberlane/mori
```

## Run a first review

From the project root:

```sh
mori scan --profile review .
```

The `review` profile selects same-language code, an `0.85` threshold, a
40-token floor, review-oriented ordering, generated-source exclusion, and a
required aggregate coverage check. It is a conservative shortlist, not a
duplicate-code verdict.

To make those values explicit and reviewable in the repository, use the guided
setup:

```sh
mori setup
mori scan .
```

Setup inventories supported and unsupported source, asks about the primary
workflow, comparison mode, coverage policy, generated source, and exclusions,
then previews the exact file before asking to write it. Use `mori configure` to
change an existing configuration and `mori doctor` to check it.

The lower-level `mori init` command remains available for scripts. It refuses
to replace an existing `.mori.json` unless `--force` is explicit, while
`mori init --stdout` prints the deterministic template without writing a file.

## Pick a profile

| Profile | Intended use |
| --- | --- |
| `review` | Low-noise same-language production review at `0.85`/40 tokens. |
| `explore` | Broad structural discovery at `0.70`/12 tokens. |
| `sql` | Top-level SQL-query review at `0.70`/12 tokens. |

Bare `mori scan` retains the older broad defaults for compatibility. Prefer a
named profile when the intent should be visible in the report.

Explicit `.mori.json` values override profile defaults. Explicit CLI values
override both. Profiles deliberately do not guess project-specific test,
migration, generated-router, vendor, or framework exclusions.

For a monorepository, define named `scopes` with relative roots in
`.mori.json`, then run `mori scan --scope backend`. The scope name and roots
are recorded and participate in baseline compatibility.

## Review exactly what is staged

```sh
mori scan --staged --format compact \
  --include-focused \
  --require-focused-coverage
```

Staged mode reads tracked source, ignore rules, and configuration from one Git
index snapshot. It records a digest and does not include unstaged or untracked
content. Focused coverage reports every staged path and exits `4` if a
non-deleted path was not analyzed.

## Explore across languages

```sh
mori scan \
  --comparison-domain code \
  --cross-language-only \
  --threshold 0.65 \
  --min-tokens 40 \
  .
```

Or choose one explicit pair:

```sh
mori scan \
  --language-pair go,typescript \
  --threshold 0.65 \
  --min-tokens 40 \
  .
```

Language families keep closely related grammars together: TypeScript/TSX,
Bash/Zsh, and PHP/Hack. Use exact concrete IDs in `--language-pair` when only
cross-dialect results are wanted.

## Check coverage before interpreting results

Mori emits a coverage warning when it finds no supported files or no comparison
fragments. Such a report is not evidence that a project contains no structural
duplication.

For a strict automation-oriented scan:

```sh
mori scan --format json \
  --require-coverage \
  --min-file-coverage 0.95 \
  --max-zero-fragment-files 2 \
  --fail-on-parse-diagnostic \
  .
```

Inspect unsupported extensions, generated exclusions, every zero-fragment
file, parser diagnostics, warnings, and report truncation. Continue with
[Reviewing results](guides/reviewing-results.md) or read the complete
[configuration reference](configuration.md).

Useful setup checks:

```sh
mori inspect --format json .
mori config validate .
mori config show --effective --provenance .
mori doctor .
mori project upgrade --check .
```

After installing a newer Mori binary, preview coordinated project maintenance
with `mori project upgrade --dry-run .`, then use `--apply` to update the
version pin and embedded Agent Skill with backups. Configuration, baselines,
and conventional automation are inspected; ambiguous project policy is never
rewritten automatically.
