# Changelog

All notable changes to 森 (*mori*) will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and releases use [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.0] - 2026-08-09

### Added

- Top-level SQL query similarity for `.sql` files, including SQLC display
  names, query-specific structural normalization, and visible parse recovery.

### Changed

- Machine-readable reports now use schema version 5 with explicit comparison
  domains and fragment kinds; normalization version 2 adds SQL features while
  preserving established code fingerprints and scores.
- The bundled review skill now provides a separate SQL scan profile and query
  review safety guidance.

## [0.5.0] - 2026-08-09

### Added

- Deterministic tool provenance in schema-4 JSON reports, including full source
  revision, build state, platform, Go version, and normalization version.
- Repeatable explicit path focus and local Git changed-file focus with exact
  group metadata, focused-first bounded retrieval, and a focused CI policy.

### Changed

- The bundled review skill now validates report provenance and uses Mori's
  native changed-file focus without narrowing the scan to changed files.

## [0.4.1] - 2026-08-08

### Changed

- Clarified explicit binary-path use, mutually exclusive cross-language
  filters, and project configuration before baselining in the bundled review
  skill.

## [0.4.0] - 2026-08-08

### Added

- Strict `.mori.json` project configuration with reproducible effective-option
  metadata.
- Nested `.gitignore` and `.moriignore` discovery with explicit-path behavior
  and `--no-ignore`/`--no-config` controls.
- Language-family-aware cross-language scans and explicit repeatable
  `--language-pair` filters.
- Content- and path-scoped baseline schema 2 with schema-1 content-scope
  compatibility.
- Structured parse diagnostics, nested-fragment metadata, and concise shared
  shape summaries.

### Changed

- Machine-readable reports now use schema version 3 and group qualifying
  source-location pairs by stable content-pair identity.
- Equal-score groups now prefer larger shared evidence and wider occurrence
  coverage before stable identity ordering.
- The bundled review skill now uses bounded progressive group retrieval and
  explains configuration, baseline scope, parser, and nested-function limits.

## [0.3.0] - 2026-08-08

### Added

- Stable fragment and match identities for review workflows.
- Versioned baseline update, scan suppression, and stale-entry pruning.

### Changed

- Machine-readable reports now use schema version 2 and include stable
  fragment/match identities and baseline suppression counts.
- Updated the CodeQL workflow actions to v4.37.4.

## [0.2.1] - 2026-08-02

### Changed

- Upgraded the Tree-sitter Go binding to 0.25.0 and compatible ABI-15 grammar
  releases for Go, JavaScript, Python, and Rust.
- Kept grammar-only Go statement-list wrappers out of fingerprints so parser
  generator changes do not distort structural scores.
- Grouped future Tree-sitter Dependabot updates so binding and grammar ABI
  changes are reviewed together.

## [0.2.0] - 2026-08-02

### Added

- Official `mori-review-similarity` Agent Skill for evidence-led duplicate and
  refactoring review.
- Offline `mori skill install` command for project, global, and custom skill
  directories.
- Deterministic platform-neutral Agent Skill archive in each release.

## [0.1.0] - 2026-08-01

### Added

- Go CLI with deterministic text and JSON reports.
- Tree-sitter adapters for Go, JavaScript/JSX, TypeScript/TSX, Python, and Rust.
- Cross-language canonical AST features and weighted Jaccard scoring.
- Bounded source discovery, size-pruned comparisons, and CI failure mode.
- Native, immutable-release-compatible GitHub build pipeline.

[Unreleased]: https://github.com/Cyberlane/mori/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/Cyberlane/mori/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/Cyberlane/mori/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/Cyberlane/mori/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Cyberlane/mori/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/Cyberlane/mori/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/Cyberlane/mori/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Cyberlane/mori/releases/tag/v0.1.0
