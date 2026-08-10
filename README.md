<p align="center">
  <img src="docs/assets/mori-hero.webp" alt="Mori forest-green banner with the forest kanji and the slogan Structural similarity, explained" width="100%">
</p>

# Mori

[![Release](https://img.shields.io/github/v/release/Cyberlane/mori?color=0f766e)](https://github.com/Cyberlane/mori/releases/latest)
[![CI](https://github.com/Cyberlane/mori/actions/workflows/ci.yml/badge.svg)](https://github.com/Cyberlane/mori/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-d97706.svg)](LICENSE)

Mori finds source fragments with similar structure—even when they use different
names or programming languages—and gives you an explainable shortlist to review.

> Mori reports structural evidence. A match is never proof that two fragments
> behave the same way or should be merged.

| | |
| --- | --- |
| 🔒 **Local** | Source stays on your machine. No upload or telemetry is required. |
| 🌐 **Cross-language** | Compare compatible functions across supported languages. |
| 🔎 **Explainable** | See scores, locations, shared shape, and directional differences. |
| ⚙️ **Predictable** | Deterministic output, visible coverage, and bounded resource use. |

## Install

Download a native archive from the [latest release](https://github.com/Cyberlane/mori/releases/latest),
or build from source with Go 1.23+ and a C compiler:

```sh
go install github.com/Cyberlane/mori/cmd/mori@latest
mori version
```

Official release binaries include complete build provenance and are recommended
for CI and auditable reports.

Each release also includes checksum-pinned Homebrew, Scoop, and WinGet manifest
assets, an SPDX SBOM, and GitHub/Sigstore attestations. Package-index submission
is intentionally separate, so verify the release asset before local use.

## Quick start

From a project root:

```sh
mori setup
mori scan .
```

`mori setup` inventories the project, asks a few focused questions, previews a
conservative `.mori.json`, and writes it only after confirmation. Review its
exclusions after the first scan; Mori cannot decide which tests, generated
files, migrations, or framework repetition are intentional in your project.

For a no-write trial:

```sh
mori scan --profile review .
```

The review profile starts with same-language code, an 85% threshold, a
40-token floor, generated-source exclusion, and required aggregate coverage.

Example result:

```text
1. 92.7% structural similarity · 1 location pair(s)
   weighted feature evidence: 114 intersection / 123 union
   A  Validator.java:6-9   [java]       looksLikeEmail
   B  validator.js:1-4     [javascript] looksLikeEmail
   shared shape: 3 calls, 1 return, 2 bindings
```

Always read both locations. Mori does not establish equivalent runtime values,
effects, types, external calls, permissions, transactions, or error behavior.

## Choose a workflow

| Goal | Start with |
| --- | --- |
| Review likely same-language duplication | `mori scan --profile review .` |
| Explore across languages | `mori scan --profile explore --cross-language-only .` |
| Review one language pair | `mori scan --language-pair go,typescript .` |
| Review SQL queries | `mori scan --profile sql path/to/sql` |
| Produce CI or agent evidence | `mori scan --profile review --format json .` |
| Produce editor diagnostics | `mori scan --profile review --format sarif .` |
| Save a local visual report | `mori scan --profile review --format html . > mori.html` |
| Inspect language coverage | `mori inspect .` |
| Check project setup | `mori doctor .` |

Lower the token floor toward 12 only for deliberate broad exploration; small
callbacks and wrappers commonly dominate at that size.

Use `mori explain <content-pair-id> [scan options] .` to reproduce and isolate
one reported identity. Text output uses restrained color on terminals; set
`--color never` or `NO_COLOR` to disable it.

## Supported source

Mori currently supports:

- Bash/POSIX shell and Zsh;
- C, C++, C#, Dart, GDScript, Go, Hack, Java, JavaScript/JSX, Kotlin, Lua,
  Luau, PHP, PowerShell, Python, Ruby, Rust, Swift, TypeScript/TSX;
- generic SQL queries and explicitly selected PostgreSQL queries.

Run `mori languages` for the exact extensions, families, fragment kinds, and
extensionless shebangs supported by your installed version. See
[Languages and parser limits](docs/reference/languages-and-parser-limits.md)
for comparison boundaries and known gaps.

## Editors and AI coding tools

The repository includes a dependency-free
[VS Code reference client](editors/vscode/README.md). It analyzes unsaved
buffers through a local Mori process and reports SARIF diagnostics without
uploading source.

Install Mori's review skill into a project for compatible coding agents:

```sh
mori skill install --project .
```

The skill teaches agents to treat matches as review leads, verify coverage,
inspect both source locations, and avoid score-only refactors.

To let a project agent configure Mori without granting hidden write access:

```sh
mori setup --agent --format json .
```

The agent can answer the emitted questions and preview the exact configuration
before an explicit `--apply`. See [Editors and coding agents](docs/guides/editors-and-agents.md).

## Documentation

- [Documentation home](docs/README.md)
- [Getting started](docs/getting-started.md)
- [Reviewing results](docs/guides/reviewing-results.md)
- [SQL and embedded SQL](docs/guides/sql.md)
- [Automation and baselines](docs/guides/automation-and-baselines.md)
- [Editors and coding agents](docs/guides/editors-and-agents.md)
- [Project configuration](docs/configuration.md)
- [Scoring](docs/scoring.md)
- [Machine integration](docs/machine-integration.md)
- [Architecture](docs/architecture.md)
- [Adding a language](docs/adding-a-language.md)

## Development

```sh
make check
```

See [Contributing](CONTRIBUTING.md) for development policy. The calibration
corpus is regression evidence for its reviewed cases, not a universal accuracy
claim.

## License

[MIT](LICENSE)
