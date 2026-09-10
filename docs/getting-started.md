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

From the project root, inspect the inventory first:

```sh
mori inspect .
mori scan --profile review path/to/source
```

Replace `path/to/source` with a real source root from your project, such as
`src`, `packages/core/src`, or `django`. Use `.` when the whole repository is
an intentional review surface. This choice matters in monorepositories: tests,
stories, fixtures, and repeated framework callbacks can dominate a broad scan.
The review profile does not remove those categories automatically.

The `review` profile selects same-language code, an `0.85` threshold, a
40-token floor, review-oriented ordering, generated-source exclusion, and a
required aggregate coverage check. It is a conservative shortlist, not a
duplicate-code verdict.

To make those values explicit and reviewable in the repository, use the guided
setup:

```sh
mori setup
# Run the exact scan command printed after applying setup.
```

Setup inventories supported and unsupported source, asks about the primary
workflow, comparison mode, coverage policy, generated source, and exclusions,
then previews the exact file before asking to write it. It offers named
`library`, `application`, `tests`, and `all` scopes based on observed path conventions;
`keep` preserves the current scope policy. Review the suggested roots,
exclusions, counts, and examples, and edit them before applying. A chosen scope
is opt-in with `mori scan --scope application`; it does not become the default
or narrow the base staged gate. Existing top-level exclusions still apply.
Use `mori configure` to change an existing configuration and `mori doctor` to
check it. Application and library exclusions generalize observed test/story
conventions, such as `**/*.test.*`, so new matching files remain excluded. These
are editable naming rules, not proof of a file's purpose. Library roots cover
observed conventional source directories and may omit custom layouts. Oversized
root suggestions are omitted rather than silently truncated. See
[Choose a useful first review](guides/first-review.md) for examples and limits.

The lower-level `mori init` command remains available for scripts. It refuses
to replace an existing `.mori.json` unless `--force` is explicit, while
`mori init --stdout` prints the deterministic template without writing a file.

## Pick a profile

| Profile | Intended use |
| --- | --- |
| `review` | Same-language review at `0.85`/40 tokens; tests remain eligible. |
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

## Make the first shortlist useful

Keep a bounded summary and the full retained report outside tracked source:

```sh
mori scan --profile review --format agent --output /tmp/mori-review.json path/to/source
```

Read both source ranges in the first groups and check coverage before changing
policy. A source directory can still contain colocated tests. For a deliberately
production-focused trial, add only globs you have checked against your tree:

```sh
mori scan --profile review --exclude '**/*.test.ts' --exclude '**/*.stories.tsx' packages/core/src
```

These are examples, not universal exclusions. File globs cannot separate Rust
inline tests from production functions in the same file. Preserve a separate
inclusive policy for staged review: a supported staged file excluded from a
scan must remain visible as unanalyzed, and strict focused coverage can fail.
Do not copy exploratory exclusions into a commit gate without reviewing that
consequence.

If Mori reaches its candidate-pair limit, the scan has not completed. Select a
smaller source root or reviewed scope and rerun; increasing output limits does
not reduce comparison work. Do not treat an absent report as a clean result.

## Review exactly what is staged

```sh
mori review staged check --format compact .
```

Staged mode reads tracked source, ignore rules, and configuration from one Git
index snapshot. It records a digest and does not include unstaged or untracked
content. Focused coverage reports every staged path and exits `4` if a
supported non-deleted path was not analyzed; unsupported paths remain evidence
but do not enter the denominator. The canonical command includes otherwise
ignored staged source and requires complete focused-file coverage. Use the
lower-level `scan --staged` form only for a deliberately custom policy.

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

The `0.95` value is an example policy, not a universal target. Declaration-only
and barrel files may legitimately contain no comparison fragments. Unsupported
extensions do not enter the supported-file coverage denominator, so that ratio
alone cannot establish whole-repository coverage.

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
version pin, tracked project contract, and a missing or recognized official
Agent Skill with backups. Unknown skill changes fail closed. Configuration,
baselines, and conventional automation are inspected; project-owned policy is
never rewritten automatically.
