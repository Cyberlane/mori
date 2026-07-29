# Adding a language

Language support is more than linking a grammar. A complete adapter needs a
compatible parser, useful fragment boundaries, cross-language normalization,
fixtures, and a release-platform check.

## 1. Choose and audit a grammar

Prefer an official Tree-sitter grammar with:

- an active upstream;
- generated C sources and Go bindings;
- a license compatible with MIT distribution;
- a grammar ABI supported by the pinned Go binding; and
- representative coverage for the language version you intend to claim.

Record the exact version and copyright in `THIRD_PARTY_NOTICES.md`. Do not
upgrade one grammar independently if it changes the ABI expected by the shared
binding.

## 2. Register the grammar

Add a `language.Spec` in `internal/language/registry.go` with:

- stable lowercase ID;
- human-readable name;
- non-overlapping extensions;
- language constructor; and
- function-like node kinds that contain executable bodies.

Signatures, declarations without bodies, interfaces, and type members are
usually poor comparison units.

The registry ABI test automatically calls `Parser.SetLanguage` for every spec.

## 3. Inspect real trees

Use small fixtures for:

- a free function;
- a method;
- an anonymous function or closure;
- async/generator syntax where applicable;
- nested functions;
- malformed source; and
- syntax unique to the language.

Do not infer node names from another grammar. Tree-sitter grammars are concrete
syntax trees and differ even when the source constructs look similar.

## 4. Extend normalization deliberately

Add the smallest mappings that improve a demonstrated cross-language pair.

For each broadening rule, include:

- a positive fixture that should score more closely;
- a nearby negative fixture that should remain below the chosen threshold; and
- a note in `docs/scoring.md` if the public scoring contract changes.

Do not map user-defined APIs to semantic families without a strong,
language-independent reason. Never preserve raw identifier or literal text just
to make one fixture pass.

## 5. Verify end to end

Run:

```sh
make check
go run ./cmd/mori languages
go run ./cmd/mori scan --cross-language-only path/to/fixtures
```

Because grammar bindings use CGO, a local build proves only the current OS and
architecture. Confirm the native CI matrix before documenting release support.

## Pull request evidence

A language-support pull request should include:

- upstream grammar and license links;
- pinned version and ABI evidence;
- supported extensions and fragment node kinds;
- positive and negative cross-language fixtures;
- score changes at a stated threshold;
- parse-error behavior; and
- native platform results.
