# Changelog

All notable changes to 森 (*mori*) will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and releases use [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Baseline schema 4 adds `false-positive` as a durable review classification.
  Schema-3 baselines remain readable for suppression with their profile
  evidence intact, but require explicit migration before mutation.
- Added owner-only, local staged-review receipts that bind an explicit focused
  finding acknowledgment to the exact HEAD, Git-index digest, scan profile,
  Mori and normalization versions, and complete sorted finding identities.
  Compatible receipts change only focused-match policy exit status; findings
  remain visible. Machine-readable reports advance to schema 19 to expose
  compatible receipt evidence.
- Added original MIT Swift calibration cases for presentation chains and
  memberwise initializer shapes, including a renamed positive and reviewed
  unrelated false positives. They document the current structural limit;
  normalization remains version 12 because identifier or literal heuristics
  would not justify a safe scoring change.

## [0.28.1] - 2026-08-11

### Fixed

- Staged scans now load configured or explicit baselines from the same
  stage-zero Git-index snapshot as source, ignore rules, and configuration.
  Missing, external, non-regular, oversized, or malformed staged baselines
  fail closed instead of falling back to working-tree bytes. Report schema
  remained 18, baseline schema remained 3, and normalization remained 12.

## [0.28.0] - 2026-08-10

### Added

- Added immutable `--staged` scans that read tracked source, ignore rules, and
  configuration from one Git-index snapshot, record its digest, and exclude
  unstaged and untracked content by construction.
- Added `--include-focused` and `--require-focused-coverage`, with per-path
  analyzed, deleted, excluded, unsupported, resource-limited, or undiscovered
  evidence and coverage exit status `4`.
- Added named `.mori.json` scopes, compact one-line-per-group output, and
  `mori project upgrade --check|--dry-run|--apply` for coordinated local
  version-pin and Agent Skill maintenance with recoverable backups.

### Changed

- Review ranking no longer treats generic entry-point, constructor, or
  anonymous callable names as distinctive same-name evidence.
- Source discovery now prunes ignored build trees before loading irrelevant
  nested ignore files while retaining deterministic negation behavior.
- Machine-readable reports now use schema version 18 for immutable input
  provenance, named scope evidence, and exact focused-file coverage. Baseline
  schema remains version 3 and normalization remains version 12.

## [0.27.0] - 2026-08-10

### Added

- Added guided `mori setup` and `mori configure` flows with safe previews,
  explicit writes, strict answer files, and a read-only JSON protocol for
  project coding agents.
- Added `mori doctor`, `mori inspect`, `mori config validate`, and provenance-
  aware effective-configuration output for first-run diagnosis and ongoing
  maintenance.
- Added `mori explain`, self-contained HTML reports, restrained terminal color,
  and opt-in deterministic `--redact-paths` placeholders for reports that cross
  a project boundary.
- Added a bounded local Language Server Protocol implementation and a packaged,
  dependency-free VS Code reference client for diagnostics on unsaved buffers.
- Added C, C++, Dart, GDScript, Kotlin, Lua, Luau, PowerShell, and Ruby parsing,
  function-boundary extraction, ABI checks, editor activation, documentation,
  and calibrated cross-language positive and nearby-negative fixtures.
- Added checksum-pinned Homebrew, Scoop, and WinGet release manifests, an SPDX
  source-dependency SBOM, and signed GitHub/Sigstore attestations for release
  assets and the checksum manifest.
- Added a compact new-user README, task-oriented documentation, a Mori favicon,
  and responsive forest-themed project artwork.

### Changed

- Normalization now uses version 12, with canonical mappings required by the
  newly supported grammars. Existing baselines require deliberate review and
  regeneration because scores and fingerprints can change. Report schema
  remains version 17 and baseline schema remains version 3.
- Release verification now checks native archives, the portable Agent Skill,
  the JSON Schema, the VS Code package, package-manager metadata, checksums,
  and representative provenance attestations before completion.

## [0.26.0] - 2026-08-10

### Added

- Added PHP support for `.php` and `.phtml` using the official Tree-sitter PHP
  grammar, covering implemented functions, methods, anonymous functions, and
  arrow functions.
- Added Hack support for `.hack` and bounded legacy `<?hh` `.php` detection,
  using a checksum-verified MIT-licensed generated-source snapshot of the
  archived Slack Tree-sitter Hack grammar.
- Added PHP/Hack positive and nearby-negative calibration cases, examples,
  malformed-source and legacy-header coverage, and both language IDs to the
  reference VS Code client.

### Changed

- Normalization now uses version 11, with canonical PHP/Hack syntax mappings
  and curated PHP standard-library operation aliases. Existing baselines
  require deliberate review and regeneration because scores and fingerprints
  can change. Report schema remains version 17 and baseline schema remains
  version 3.

## [0.25.0] - 2026-08-10

### Added

- Added Java support for implemented methods, constructors, compact
  constructors, and lambdas using the official Tree-sitter Java grammar.
- Added C# support for implemented methods, constructors, destructors,
  operators, accessors, local functions, anonymous methods, and lambdas using
  the official Tree-sitter C# grammar.
- Added a redistributable Java/C# calibration case, a nearby-negative fixture,
  malformed-source coverage, and both languages to the reference VS Code
  client.

### Changed

- Normalization now uses version 10, with canonical Java/C# syntax mappings,
  transparent redundant parentheses, and a bounded anonymous member shape for
  qualified Java method calls. Existing baselines require deliberate review
  and regeneration because scores and fingerprints can change. Report schema
  remains version 17 and baseline schema remains version 3.

## [0.24.0] - 2026-08-10

### Added

- Added a redistributable ten-case labeled calibration corpus spanning code,
  shell, generic SQL, and PostgreSQL, with deterministic precision-at-k,
  actionability, score-distribution, fragment-size, and ranking-movement
  reporting against a pinned v0.22.0 reference.
- Added deterministic SARIF 2.1.0 output with stable similarity and incomplete-
  analysis rules, source locations, fingerprints, and exact suppression,
  warning, and truncation metadata.
- Added bounded `--stdin-path` overlays so editor clients can analyze unsaved
  source through standard input while retaining repository-wide comparisons
  and automatically focusing the overlaid file.
- Added a dependency-free reference VS Code extension and a published Draft
  2020-12 JSON Schema for the machine-readable report contract.

### Changed

- Machine-readable reports now use schema version 17 and record the optional
  stdin overlay path. Normalization remains version 9 and baseline schema
  remains version 3 because scores, fingerprints, and accepted identities are
  unchanged.

## [0.23.0] - 2026-08-10

### Added

- Added low-weight canonical statement-position features and bounded anonymous
  callee-role features that distinguish calls exchanged between control and
  linear positions without serializing callee names or name digests.

### Changed

- Normalization now uses version 9. Existing baselines require deliberate
  review and regeneration because fingerprints and scores can change. Report
  schema remains version 16 and baseline schema remains version 3.

## [0.22.0] - 2026-08-10

### Added

- Added exact weighted intersection/union evidence and bounded deterministic
  profile-aligned directional feature differences for every retained group.

### Changed

- Text output now labels 100% as normalized feature identity and explicitly
  states that it is not proof of semantic or behavioral equivalence.
- Machine-readable reports now use schema version 16. Normalization remains
  version 8 and baseline schema remains version 3 because scoring,
  fingerprints, and acceptance compatibility are unchanged.

## [0.21.0] - 2026-08-10

### Added

- Added selective `baseline add`, `baseline remove`, and `baseline edit`
  operations with durable review notes and classifications.
- Added explicit `baseline migrate --accept-profile`, deterministic scan-profile
  digests, profile compatibility reporting, and reviewed warning exceptions for
  complete baseline operations. Digests include exact loaded ignore-file paths
  and SHA-256 content evidence.

### Changed

- `baseline update` is now preview-only unless `--accept-all` is explicit, and
  retained entries preserve their review metadata.
- Machine-readable reports now use schema version 15. Baselines now use schema
  version 3; schemas 1 and 2 remain readable but require explicit migration
  before mutation. Normalization remains version 8.

## [0.20.0] - 2026-08-10

### Added

- Added `--min-file-coverage`, `--max-zero-fragment-files`,
  `--fail-on-warning`, and `--fail-on-parse-diagnostic`, with equivalent strict
  project configuration. Scan and baseline operations evaluate the same
  policies before returning success or modifying a baseline.
- Added deterministic coverage-summary totals, aggregate unsupported-extension
  counts, pre-token-floor boundary counts, and machine-readable reasons for
  every zero-fragment supported file. Linked-worktree `.git` control files are
  treated as VCS metadata rather than unsupported source.

### Changed

- Machine-readable reports now use schema version 14. Normalization remains
  version 8 and baseline schema remains version 2.

## [0.19.1] - 2026-08-09

### Fixed

- Recognized the standard SwiftProtobuf generated-source header so
  `--exclude-generated` omits compiler-generated `.pb.swift` files without
  broadening classification to arbitrary generator comments.
- Extended bounded, byte-preserving Swift parser compatibility to valid
  conditional-cast and nil-coalescing chains with optional dictionary bases,
  collection types, labels, returns, and repeated fallbacks. Repaired trees
  are still accepted only when they reduce parser diagnostics.

## [0.19.0] - 2026-08-09

### Added

- Added explicit `--embedded-sql` extraction for direct string arguments to a
  bounded set of Go database method names. Extracted queries use the selected
  SQL dialect, retain host literal locations and parent functions, group
  multi-statement strings as one unit, and expose parser or resource-limit
  warnings.
- Added opt-in fixed-size `block` comparison fragments inside code functions,
  with configurable statement-window and per-function bounds, duplicate-span
  removal, direct function-parent linkage, and same-file overlap suppression.

### Changed

- Machine-readable reports now use schema version 13 and record every embedded
  SQL and statement-block option. Normalization version 8 covers the expanded
  opt-in comparison-unit contract; existing baselines must be reviewed and
  regenerated. Baseline schema remains version 2.

## [0.18.0] - 2026-08-09

### Added

- Added source-free literal-position evidence to match groups. Reports disclose
  bounded difference counts without serializing literal values or digests and
  without changing scores, fingerprints, ordering, or baselines.
- Added repeatable `--priority-path GLOB=WEIGHT` and `priority_paths`
  configuration for deterministic, presentation-only project review priority.

### Changed

- Machine-readable reports now use schema version 12 for literal evidence and
  effective priority-path rules. Normalization remains version 7 and baseline
  schema remains version 2.

## [0.17.1] - 2026-08-09

### Fixed

- Recognized exact Typeshare generated-source headers, including multiline
  block comments, so `--exclude-generated` can omit those files without
  broadening classification to arbitrary `Generated by` comments.
- Added bounded, byte-preserving Swift parser compatibility for valid optional
  async bindings, awaited switches, empty-tuple call arguments, and conditional
  casts followed by nil coalescing. Repaired trees are accepted only when they
  reduce diagnostics; nearby malformed syntax remains visible.

## [0.17.0] - 2026-08-09

### Added

- Added `review`, `explore`, and `sql` scan profiles with deterministic
  defaults and explicit config/CLI precedence.
- Added `mori init` for deterministic `.mori.json` scaffolding, including
  stdout-only inspection, no-clobber creation, and explicit forced replacement
  of regular files.

### Changed

- Machine-readable reports now use schema version 11 and record the selected
  scan profile. Normalization remains version 7 and baseline schema remains
  version 2.
- Mori's bundled review skill and self-review config now use the conservative
  `review` profile while keeping project exclusions explicit.

## [0.16.0] - 2026-08-09

### Added

- Added whole-file `script` fragments for Bash/POSIX shell and Zsh. Script
  fragments cover top-level executable statements while excluding function
  bodies, which remain independent `function` fragments.
- Added positive and nearby-negative cross-dialect fixtures for shell top-level
  normalization.
- `mori languages` now lists every fragment kind emitted by each grammar.

### Changed

- Normalization version 7 introduces shell script comparison units. Existing
  baselines must be reviewed and regenerated. Report schema remains version 10.
- Candidate partitioning now requires compatible fragment kinds, so shell
  scripts are never compared with functions.
- Text output distinguishes top-level script bodies from outer function bodies
  when it discloses separately analyzed functions.

## [0.15.0] - 2026-08-09

### Added

- Added opt-in `--ranking review` / `ranking: "review"` ordering with explicit
  `review_priority` and `review_signals` for cross-file, cross-directory,
  same-name, and repeated-location evidence.

### Changed

- Machine-readable reports now use schema version 10 and record the effective
  ranking mode. Review ranking changes presentation order only; structural
  scores, normalization version 6, fingerprints, and baselines are unchanged.

## [0.14.0] - 2026-08-09

### Added

- Added deterministic per-file coverage inventories to JSON reports, including
  fragment, skipped-fragment, and parse-diagnostic counts for every analyzed
  supported file.
- Added conservative generated-source header classification and opt-in
  `--exclude-generated` / `exclude_generated` filtering. Excluded generated
  files remain visible in the per-file coverage inventory.

### Changed

- Machine-readable reports now use schema version 9. Text reports summarize
  files that produced no comparison fragments at the configured token floor.
  Normalization remains version 6 and existing baselines remain compatible.
- Text results now place a group-level warning immediately below any score
  whose retained occurrences exclude nested function bodies.
- The bundled review skill now requires schema-9 per-file coverage inspection,
  makes production/test separation a standard first step, and documents
  generated-source classification and exclusion.

## [0.13.0] - 2026-08-09

### Added

- Added repeatable `--changed-worktree PATH=REVISION` focus for nested and
  sibling Git worktrees with independently resolved revisions and local
  working-tree state.

### Changed

- Machine-readable reports now use schema version 8 and record deterministic
  per-worktree focus provenance. Normalization remains version 6 and existing
  baselines remain compatible.
- Updated CLI help, repository guidance, and the bundled review skill for
  explicit multi-worktree and submodule review.

## [0.12.0] - 2026-08-09

### Added

- Added explicit `--sql-dialect postgresql` and `sql_dialect` configuration
  with a dedicated PostgreSQL 18.3 parser for top-level query comparison.
- Added checksum-pinned PostgreSQL generated-source provenance and
  positive/nearby-negative query fixtures.

### Changed

- Machine-readable reports now use schema version 7 and record the effective
  SQL dialect.
- Normalization version 6 maps PostgreSQL query structure into Mori's shared
  SQL vocabulary. Existing baselines must be reviewed and regenerated.
- The bundled review skill now requires explicit PostgreSQL selection and
  separate profiles for mixed-dialect repositories.

## [0.11.0] - 2026-08-09

### Added

- Added Swift function similarity for `.swift` files, covering implemented
  functions, initializers, deinitializers, and closures with a pinned,
  checksum-verified generated Tree-sitter grammar.
- Added Swift-to-Go positive and nearby negative validation fixtures.

### Changed

- Normalization version 5 maps Swift declarations, expressions, arguments,
  identifiers, and control transfers into the shared code vocabulary. Existing
  baselines must be reviewed and regenerated.
- The bundled review skill now describes Swift comparison boundaries and the
  required warning disclosure.

### Fixed

- Function fragments nested below explicit parser error nodes are now rejected
  instead of being scored from recovered malformed syntax.

## [0.10.0] - 2026-08-09

### Added

- Added dedicated Bash/POSIX-shell and Zsh function parsers with `.sh`,
  `.bash`, and `.zsh` discovery.
- Added bounded, non-executing shebang detection for extensionless Bash/POSIX,
  Zsh, Python, and Node.js scripts.
- Added shebang interpreter names to `mori languages` and help for
  `mori languages --help`.

### Changed

- Normalization version 4 aligns shell word, variable-reference, and glob node
  vocabulary across the Bash and Zsh grammars. Existing baselines must be
  reviewed and regenerated.
- The bundled review skill now explains shell-family selection and shebang
  coverage.

## [0.9.0] - 2026-08-09

### Added

- Added deterministic coverage warnings for scans with no supported files or
  no extracted comparison fragments.
- Added `--require-coverage` and the equivalent `require_coverage` project
  setting, which exit with status 4 when a scan cannot support a similarity
  assessment.

### Changed

- Corrected the schema-6 example, clarified release source-date formatting,
  labeled abridged output, and linked the scan-selection contract directly.
- Added the recommended 40-token starting floor to the scoring guide's
  cross-language calibration guidance.
- Consolidated deterministic analyzer partitioning and lexical path
  containment without changing comparison behavior.
- Added an explicit, non-discovered self-review profile and `make dogfood`
  workflow for production-code dogfooding.
- Added language preflight, zero-coverage, and nested Git boundary guidance to
  the bundled review skill.
- Nested-worktree focus errors now identify the repository-relative boundary
  and recommend a separate scan or explicit focus paths.

### Fixed

- `mori skill --help`, `mori skill -h`, and `mori skill help` now print skill
  usage successfully instead of reporting an unknown skill command.

## [0.8.0] - 2026-08-09

### Added

- Direct `--comparison-domain` filtering, applied before parsing and recorded
  in schema-6 reports and project configuration.
- `--same-language-only` review-family filtering, including TypeScript-to-TSX
  comparisons within their shared family.
- A detailed scan-selection contract covering validation, execution,
  compatibility, diagnostics, and verification.

### Changed

- The bundled review skill now starts with code-only same-language scans and a
  40-token cross-language floor, while reserving 12 tokens for broad
  exploration.
- Parse warnings now describe potentially incomplete comparison coverage
  without claiming that non-comparison DDL fragments were invalid or skipped.

### Fixed

- Code and SQL findings no longer need to share one root ranking when a
  comparison domain is selected.

## [0.7.0] - 2026-08-09

### Added

- Byte-preserving parser support for SQLC and SQLite `LIMIT`/`OFFSET`
  parameters and SQLite `ON CONFLICT` column targets.

### Changed

- Normalization version 3 classifies recovered pagination placeholders as
  parameters while preserving schema version 5 and existing code features;
  existing baselines must be reviewed and regenerated.

### Fixed

- Recognized raw ampersands in JSX text no longer invalidate otherwise valid
  JavaScript or TSX function fragments; malformed JSX expressions still fail
  closed.

## [0.6.1] - 2026-08-09

### Fixed

- Clarified that version-pinned source builds can lack revision and source-date
  metadata, and limited provenance-incomplete reports to explicitly disclosed
  exploratory review.

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

[Unreleased]: https://github.com/Cyberlane/mori/compare/v0.28.1...HEAD
[0.28.1]: https://github.com/Cyberlane/mori/compare/v0.28.0...v0.28.1
[0.28.0]: https://github.com/Cyberlane/mori/compare/v0.27.0...v0.28.0
[0.27.0]: https://github.com/Cyberlane/mori/compare/v0.26.0...v0.27.0
[0.26.0]: https://github.com/Cyberlane/mori/compare/v0.25.0...v0.26.0
[0.25.0]: https://github.com/Cyberlane/mori/compare/v0.24.0...v0.25.0
[0.24.0]: https://github.com/Cyberlane/mori/compare/v0.23.0...v0.24.0
[0.23.0]: https://github.com/Cyberlane/mori/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/Cyberlane/mori/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/Cyberlane/mori/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/Cyberlane/mori/compare/v0.19.1...v0.20.0
[0.19.1]: https://github.com/Cyberlane/mori/compare/v0.19.0...v0.19.1
[0.19.0]: https://github.com/Cyberlane/mori/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/Cyberlane/mori/compare/v0.17.1...v0.18.0
[0.17.1]: https://github.com/Cyberlane/mori/compare/v0.17.0...v0.17.1
[0.17.0]: https://github.com/Cyberlane/mori/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/Cyberlane/mori/compare/v0.15.0...v0.16.0
[0.15.0]: https://github.com/Cyberlane/mori/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/Cyberlane/mori/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/Cyberlane/mori/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/Cyberlane/mori/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/Cyberlane/mori/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/Cyberlane/mori/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/Cyberlane/mori/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/Cyberlane/mori/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/Cyberlane/mori/compare/v0.6.1...v0.7.0
[0.6.1]: https://github.com/Cyberlane/mori/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/Cyberlane/mori/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/Cyberlane/mori/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/Cyberlane/mori/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/Cyberlane/mori/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Cyberlane/mori/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/Cyberlane/mori/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/Cyberlane/mori/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Cyberlane/mori/releases/tag/v0.1.0
