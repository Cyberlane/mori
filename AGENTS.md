# AGENTS.md

## Mission

森 (`mori`) is a Go CLI for explainable, cross-language structural similarity.
It extracts function-like Tree-sitter fragments, canonicalizes their syntax,
and compares weighted feature multisets with Jaccard similarity.

Never describe a score as proof of semantic or behavioral equivalence.

## Architecture

- `cmd/mori`: process entrypoint.
- `internal/cli`: commands, validation, output selection, and exit codes.
- `internal/agentskill`: safe installation of Mori's embedded Agent Skill.
- `internal/diagnostic`: path-safe errors for public reports.
- `internal/source`: bounded, deterministic source discovery.
- `internal/language`: grammar registry and function boundaries.
- `internal/parser`: Tree-sitter lifecycle and fragment extraction.
- `internal/normalize`: shared canonical nodes, edges, roles, and operation hints.
- `internal/similarity`: weighted Jaccard and explanations.
- `internal/analyzer`: concurrent parsing, size pruning, pair limits, and sorting.
- `internal/report`: stable text and schema-versioned JSON.
- `internal/release`: deterministic native archive assembly.
- `skills/mori-review-similarity`: canonical portable Agent Skill source.

Language grammars use CGO. Do not claim a target works until it builds and tests
on that target or its native CI runner.

## Invariants

1. Output must be deterministic for identical inputs and options.
2. Identifiers and literal values must not dominate fingerprints.
3. Parse failures are visible; invalid fragments are never silently scored.
4. Nested functions are independent comparison units.
5. New semantic operation families require positive and negative fixtures.
6. JSON changes require an explicit schema-version decision.
7. Keep default scans bounded by file-size and candidate-pair limits.
8. Source remains local; do not add network upload or telemetry by default.
9. Keep the repository self-contained. Do not add private planning-system
   references, workstation paths, credentials, personal operational history, or
   generated session logs.

## Required verification

Run:

```sh
make check
```

For normalization changes, also run the example scan and inspect the scores:

```sh
go run ./cmd/mori scan \
  --threshold 0.70 \
  --cross-language-only \
  examples/email-validation
```

Tests must cover grammar ABI compatibility, deterministic ordering, expected
cross-language positives, and at least one nearby negative when a rule is
broadened.

## Commit policy

- One-line subjects only.
- Allowed types: `feat`, `fix`, `chore`, `style`.
- Format: `type(optional-scope): imperative summary`.
- No commit body, co-author trailer, or automation attribution.
- Use `chore(...)` for documentation, tests, CI, releases, and dependencies.

Examples:

```text
feat(normalize): map membership operation families
fix(parser): exclude invalid nested fragments
chore(ci): add native release builds
style: format analyzer tests
```

## Release policy

- Semantic Versioning tags are the only release trigger.
- A tag must point at a commit that already passes `make check`.
- Release jobs build on native runners because the grammar modules require CGO.
- Assemble assets on a draft release and publish only after all archives and
  checksums are attached.
- Do not move or reuse a published release tag.
