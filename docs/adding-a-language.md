# Adding a language

Language support is more than linking a grammar. A complete adapter needs a
compatible parser, useful fragment boundaries, a deliberate comparison domain,
normalization, fixtures, and a release-platform check.

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

If an upstream does not distribute generated parser sources in its module or
release, vendor only the generated files required for a clean offline build.
Record the exact source commit, generator version, artifact or generation
workflow, license, ABI, and file digests next to the adapter, and test those
digests. Never download or regenerate a grammar during an ordinary build.

## 2. Register the grammar

Add a `language.Spec` in `internal/language/registry.go` with:

- stable lowercase ID, review family, comparison domain, and fragment kind;
- human-readable name;
- non-overlapping extensions;
- optional interpreter basenames for extensionless shebang discovery;
- language constructor; and
- a precise fragment-boundary predicate.

For code, signatures, declarations without bodies, interfaces, and type
members are usually poor comparison units. Non-code languages must define
their own coherent comparison domain; incompatible domains are never compared.
Two parsers that intentionally share an extension must have an explicit,
validated selector and one documented default. Do not guess a SQL dialect from
syntax or silently replace the established parser.

The registry ABI test automatically calls `Parser.SetLanguage` for every spec.

## 3. Inspect real trees

For function-like code, use small fixtures for:

- a free function;
- a method;
- an anonymous function or closure;
- async/generator syntax where applicable;
- nested functions;
- malformed source; and
- syntax unique to the language.

Do not infer node names from another grammar. Tree-sitter grammars are concrete
syntax trees and differ even when the source constructs look similar.
Assign dialect grammars such as TypeScript and TSX to the same family when
maintainers would not reasonably describe them as cross-language results. For
other fragment kinds, test top-level versus nested boundaries and explicitly
document constructs that remain unsupported.

Shebang support is only for extensionless regular files. Detection must remain
bounded, must not require an executable bit, and must never execute or resolve
the named interpreter. A recognized extension remains authoritative.
The exception must be an explicit documented shared-extension dialect selector,
such as legacy Hack's exact bounded `<?hh` header, with positive and
nearby-negative detection tests.

## 4. Extend normalization deliberately

Add the smallest mappings that improve a demonstrated comparison pair within
one domain.

For each broadening rule, include:

- a positive fixture that should score more closely;
- a nearby negative fixture that should remain below the chosen threshold; and
- a note in `docs/scoring.md` if the public scoring contract changes.

Add redistributable reviewed cases to `corpus/manifest.json` when the language
participates in cross-language calibration. Record the language pair, fragment
sizes, classification, threshold, expected score range and rank, and a pinned
reference when measuring ranking movement. Corpus evidence remains scoped to
its labeled cases and does not establish a universal threshold.

Do not map user-defined APIs to semantic families without a strong,
language-independent reason. Never preserve raw identifier or literal text just
to make one fixture pass.

## 5. Verify end to end

Run:

```sh
make check
go run ./cmd/mori languages
go run ./cmd/mori scan --threshold 0.70 path/to/fixtures
```

Because grammar bindings use CGO, a local build proves only the current OS and
architecture. Confirm the native CI matrix before documenting release support.

## Pull request evidence

A language-support pull request should include:

- upstream grammar and license links;
- pinned version and ABI evidence;
- supported extensions, comparison domain, fragment kind, and boundaries;
- positive and nearby negative fixtures;
- score changes at a stated threshold;
- parse-error behavior; and
- native platform results.
