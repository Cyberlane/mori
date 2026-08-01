# Changelog

All notable changes to 森 (*mori*) will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and releases use [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/Cyberlane/mori/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/Cyberlane/mori/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Cyberlane/mori/releases/tag/v0.1.0
