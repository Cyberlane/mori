# 森 (mori)

[![CI](https://github.com/Cyberlane/mori/actions/workflows/ci.yml/badge.svg)](https://github.com/Cyberlane/mori/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**森** is read *mori* (Japanese for “forest”). It finds structurally similar
functions across programming languages, parses real syntax trees, removes
language-specific noise, and ranks function pairs with weighted Jaccard
similarity.

The result is an explainable shortlist for review—not a claim that two
functions behave identically.

> [!NOTE]
> 森 is pre-release software. Its report schema is versioned, but normalization
> rules and scores may change before v1.0.0.

## Why 森?

Copy-paste detectors catch text. Embedding-based tools can be hard to explain.
森 sits between them:

- **Cross-language:** Go, JavaScript/JSX, TypeScript/TSX, Python, and Rust.
- **AST-based:** comments, formatting, most names, literal values, and type
  annotations do not dominate the score.
- **Explainable:** every match includes the normalized features that contributed
  most strongly.
- **Local:** source is parsed on your machine and is never sent to a service.
- **CI-ready:** deterministic JSON, bounded comparisons, and an opt-in failure
  exit code.

## Install

Prebuilt archives for Linux, macOS, and Windows are available from the
[latest release](https://github.com/Cyberlane/mori/releases/latest). Each
release includes `checksums.txt`; verify the archive before placing `mori` on
your `PATH`.

Alternatively, install from source with Go 1.23 or newer and a C compiler. The
bundled Tree-sitter grammars use CGO.

```sh
go install github.com/Cyberlane/mori/cmd/mori@latest
mori version
```

## Quick start

Start with a higher threshold and ignore very small functions for a low-noise
first pass:

```sh
mori scan --threshold 0.85 --min-tokens 40 .
```

Explore only cross-language pairs:

```sh
mori scan --cross-language-only --threshold 0.65 .
```

Ignore tests and emit JSON:

```sh
mori scan \
  --exclude '**/*_test.go' \
  --exclude '**/*.test.ts' \
  --format json \
  .
```

Fail a CI step when at least one candidate crosses the threshold:

```sh
mori scan --threshold 0.85 --fail-on-match .
```

Exit status `3` means matches were found with `--fail-on-match`; `1` means an
operational error, and `2` means invalid CLI usage.

## Agent integration

森 ships an official `mori-review-similarity` Agent Skill that teaches
compatible coding agents when to scan, how to validate the JSON report, and
how to investigate candidates without treating a score as behavioral proof.

Install it into a project and commit the reviewed copy for the team:

```sh
mori skill install --project .
git add .agents/skills/mori-review-similarity
```

Install it as a personal default across projects:

```sh
mori skill install --global
```

Or select a client-specific skills directory explicitly:

```sh
mori skill install --target ~/.codex/skills
```

Project and global installs use the interoperable `.agents/skills` convention.
The `--target` form works for clients with another discovery directory. The
installer is offline: the skill is embedded in the Mori binary. Identical
installs are idempotent, different copies are preserved by default, and an
explicit `--replace` moves the previous copy to a sibling backup.

The canonical source lives in
[`skills/mori-review-similarity`](skills/mori-review-similarity). Releases also
attach a platform-neutral skill ZIP for manual extraction or upload into a
compatible agent product.

## Example

The repository contains equivalent-looking email checks written in four
languages:

```sh
mori scan \
  --threshold 0.70 \
  --cross-language-only \
  examples/email-validation
```

```text
森 (mori): 2 similarity candidate(s) from 4 fragment(s) in 4 file(s)
threshold 70.0% · 6 candidate pair(s) compared

1. 71.0% structural similarity
   A  validator.js:1-4  [javascript] looksLikeEmail
   B  validator.go:5-8  [go] LooksLikeEmail
```

Different languages often express the same idea with genuinely different tree
shapes. A Python membership test, for example, need not score like a method
call in JavaScript. Lower thresholds are useful for exploration; higher
thresholds reduce noise.

## Supported languages

| Language | Extensions | Comparison units |
| --- | --- | --- |
| Go | `.go` | functions, methods, function literals |
| JavaScript / JSX | `.js`, `.jsx`, `.mjs`, `.cjs` | functions, methods, arrows, generators |
| TypeScript | `.ts`, `.mts`, `.cts` | functions, methods, arrows, generators |
| TypeScript / TSX | `.tsx` | functions, methods, arrows, generators |
| Python | `.py`, `.pyi` | functions and lambdas |
| Rust | `.rs` | function items and closures |

Run `mori languages` for the list compiled into your binary.

## How scoring works

森 converts each function-like tree into a multiset containing:

1. canonical nodes such as `function`, `flow:return`, and
   `expression:call`;
2. coarse classes such as `control`, `operation`, and `operand`;
3. parent-child edges and selected field roles;
4. normalized operator families; and
5. small, curated semantic hints for common operations such as membership,
   trimming, and case conversion.

Identifiers become placeholders, literal values become literal kinds, and
nested functions are compared separately. For feature bags \(A\) and \(B\),
森 computes weighted Jaccard similarity:

\[
J(A,B)=\frac{\sum_f \min(A_f,B_f)}{\sum_f \max(A_f,B_f)}
\]

The default threshold is `0.70`. Start near `0.85` for low-noise,
same-language review. For cross-language discovery, `0.60`–`0.70` is often a
better starting range, then tune against your own accepted and rejected pairs.
See [Scoring](docs/scoring.md) for the complete contract.

## Scan boundaries

森:

- compares function-like fragments, not whole repositories or execution traces;
- skips `.git`, `.hg`, `.svn`, `.turbo`, `build`, `coverage`, `dist`,
  `node_modules`, `target`, and `vendor` directories by default;
- rejects discovered symbolic links and symlinked components below trusted scan
  roots;
- skips files larger than 2 MiB by default;
- prunes pairs that cannot reach the configured score based on feature counts;
  and
- stops at 5,000,000 candidate pairs unless `--max-pairs` changes the limit.

The default top-100 result limit is enforced while scoring, so a broad scan
does not retain every matching pair in memory. Use `--max-matches 0` only when
you intentionally want an unbounded report.

Use repeated `--exclude` flags for additional doublestar globs. Parse errors are
reported as warnings, and invalid function fragments are excluded.

## Important limits

- Structural similarity is not semantic equivalence.
- Curated API families are heuristics and can be wrong for overloaded or
  project-specific methods.
- Dynamic dispatch, data flow, side effects, types, and runtime values are not
  modeled.
- Thresholds are not portable quality scores; calibrate them on your codebase.
- Pair generation remains quadratic in the worst case, though size pruning and
  the comparison cap bound typical scans.

These limits are intentional. 森 should give a reviewer evidence, not silently
decide what to refactor.

## Development

```sh
make check
```

That verifies formatting and module tidiness, runs race-enabled tests and vet,
builds the CLI, checks workflows, and scans dependencies for known
vulnerabilities.

Architecture and extension guides live in:

- [Architecture](docs/architecture.md)
- [Scoring](docs/scoring.md)
- [Adding a language](docs/adding-a-language.md)
- [Contributing](CONTRIBUTING.md)

## Releases

Pushing a strict SemVer tag such as `v0.2.1` starts native CGO builds for Linux
AMD64/ARM64, macOS AMD64/ARM64, and Windows AMD64. Automation assembles a draft
GitHub release, attaches the native archives, the portable Agent Skill,
and `checksums.txt`, then publishes it. This keeps every published release
complete and compatible with immutable releases.

## Inspiration

The initial direction was inspired by Peng Cao’s article,
[“Deep Dive: Semantic Duplicate Detection with AST Analysis”](https://dev.to/peng_cao/deep-dive-semantic-duplicate-detection-with-ast-analysis-how-ai-keeps-rewriting-your-logic-3fa5).
森 deliberately describes its current result as *structural* similarity because
AST overlap alone cannot prove semantics.

## License

森 is available under the [MIT License](LICENSE). Bundled parser and runtime
notices are preserved in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
