# Contributing to 森

Thank you for helping make cross-language similarity more useful and honest.

## Before opening code

- Search existing issues.
- Use a feature request for scoring behavior and a language request for a new
  grammar.
- Never attach proprietary source. Reduce examples to a minimal,
  non-confidential fixture.
- For security-sensitive behavior, follow [SECURITY.md](SECURITY.md).

## Local setup

You need:

- Go 1.23 or newer;
- a working C compiler for CGO; and
- Git.

```sh
git clone https://github.com/Cyberlane/mori.git
cd mori
make check
```

## Commit subjects

Every commit is one line and uses exactly one of these types:

```text
feat(optional-scope): imperative summary
fix(optional-scope): imperative summary
chore(optional-scope): imperative summary
style(optional-scope): imperative summary
```

Use `chore(...)` for tests, docs, CI, builds, releases, and dependency updates.
Do not add a body, co-author trailer, or automation attribution.

Maintainers should squash pull requests to one commit, use the validated pull
request title as the squash subject, and leave the squash body empty.
Dependabot's generated commit bodies are allowed on its pull request branch;
the final squash commit still follows the one-line rule.

## Pull requests

Keep changes focused. A pull request should:

- explain the user problem and tradeoffs;
- include tests for changed behavior;
- update public documentation when flags, output, or scoring changes;
- preserve deterministic output;
- retain bounded file and comparison behavior;
- identify third-party licensing changes; and
- pass `make check`.

Normalization changes need representative positives and nearby negatives.
Include before/after scores and the exact options used.

JSON changes require an explicit schema compatibility assessment. Changes to
existing field meaning or shape require a schema-version increment.

## Adding a language

Follow [Adding a language](docs/adding-a-language.md). A grammar that compiles
but lacks boundaries, normalization evidence, licensing, or native build proof
is incomplete.

## Review

Maintainers may decline a high-scoring rule if it produces misleading matches,
depends on project-specific naming, or makes the output harder to explain.
Accuracy and restraint take priority over the number of reported candidates.
