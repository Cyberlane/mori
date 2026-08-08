# Mori

[![CI](https://github.com/Cyberlane/mori/actions/workflows/ci.yml/badge.svg)](https://github.com/Cyberlane/mori/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Mori finds functions that look alike, even when they are written in different
programming languages.

Use it to find possible duplicate logic before you copy, refactor, or review
code. Mori gives you a shortlist to inspect. It cannot prove that two functions
do the same thing.

## What It Does

Mori reads your source code locally and compares individual functions. It
ignores details such as formatting, comments, most variable names, and literal
values so it can focus on the shape of the code.

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

## Start Here

Run this from the root of a project:

```sh
mori scan --threshold 0.85 --min-tokens 40 .
```

This looks for reasonably sized functions that are very similar. Start at
`0.85` to reduce noise. If you want more possible matches, lower the threshold
gradually.

To look only for matches between different languages:

```sh
mori scan --cross-language-only --threshold 0.65 .
```

TypeScript and TSX are one language family. To compare only Go with that
family, use:

```sh
mori scan --language-pair go,typescript --threshold 0.65 .
```

To try Mori against this repository's example files:

```sh
mori scan --threshold 0.70 --cross-language-only examples/email-validation
```

Example output:

```text
1. 71.0% structural similarity · 1 location pair(s)
   A  fingerprint 1d4f4194cc92c56c · 1 occurrence(s)
      - validator.go:5-8  [go] LooksLikeEmail
   B  fingerprint 6d2ab0d68af9fb96 · 1 occurrence(s)
      - validator.js:1-4  [javascript] looksLikeEmail
      shared shape: 3 calls, 1 return, 1 binding
```

Read both functions before acting on a match. Mori does not understand runtime
values, external calls, side effects, or all language-specific behavior.

## Common Uses

Mori honors nested `.gitignore` and `.moriignore` files during directory scans.
An explicitly requested file is still scanned. Add command-line exclusions for
additional policy:

```sh
mori scan --exclude '**/*_test.go' --exclude '**/*.test.ts' .
```

Store repeatable project settings in `.mori.json`:

```json
{
  "threshold": 0.85,
  "min_tokens": 40,
  "max_groups": 250,
  "language_pairs": ["go,typescript"],
  "exclude": ["**/*_test.go"]
}
```

Mori searches the current directory and its parents for `.mori.json`. Use
`--config <path>`, `--no-config`, or `--no-ignore` to control discovery. See
[Project configuration](docs/configuration.md) for the complete contract.

Write results as JSON for a script or CI system:

```sh
mori scan --format json .
```

Fail a CI job when Mori finds a match at your threshold:

```sh
mori scan --threshold 0.85 --fail-on-match .
```

With `--fail-on-match`, Mori exits with status `3` when it finds a match.

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

| Parser language | Review family | File types |
| --- | --- | --- |
| Go | Go | `.go` |
| JavaScript and JSX | JavaScript | `.js`, `.jsx`, `.mjs`, `.cjs` |
| TypeScript | TypeScript | `.ts`, `.mts`, `.cts` |
| TSX | TypeScript | `.tsx` |
| Python | Python | `.py`, `.pyi` |
| Rust | Rust | `.rs` |

Run `mori languages` to see the languages in your installed version.

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

- [How Mori scores functions](docs/scoring.md)
- [Project configuration](docs/configuration.md)
- [Architecture](docs/architecture.md)
- [Add a language](docs/adding-a-language.md)
- [Contributing](CONTRIBUTING.md)

## Development

```sh
make check
```

## License

Mori is available under the [MIT License](LICENSE).
