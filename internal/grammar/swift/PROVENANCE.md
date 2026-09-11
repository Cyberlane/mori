# Tree-sitter Swift generated-source provenance

Mori vendors the grammar source, generated C parser, scanner, and required headers
so builds do not download or regenerate grammar code.

- Upstream: `https://github.com/alex-pinkus/tree-sitter-swift`
- Source commit: `8d02b7ff390a17a43ce90c4e987c49315cfc4be6`
- Source package version: `0.7.3`
- Tree-sitter CLI for local regeneration: `0.25.10`
- Grammar ABI: `14`
- License: MIT, reproduced in `LICENSE`

Mori's grammar change replaces the external nil-coalescing token in
`optional_type` with an immediate double-question-mark token. Adjacent `T??`
remains a nested optional type; whitespace-separated `as? T ?? fallback` remains
a cast followed by nil coalescing. No expression precedence was changed. This
also permits casts inside call arguments without adding synthetic parentheses.
Parenthesized expressions use the same precedence as binding patterns, with
an explicit ambiguity retained until a following range suffix distinguishes
`case (n + 1)...` from tuple binding patterns.
The scanner is unchanged from the original pinned artifact.

To regenerate in a temporary directory, copy `grammar.js` and run:

```sh
npx --yes tree-sitter-cli@0.25.10 generate --abi 14
```

Copy `src/parser.c` and `src/tree_sitter/{parser,alloc,array}.h` back beside the
unchanged scanner, then verify these hashes and run `make check`. Ordinary builds
and releases use the committed generated sources directly.

SHA-256:

```text
e8a81cb8bbd7ee8ab4652ecafe7849991377ae63fd5d06c29777a67f19f54da9  grammar.js
cde85fddaa1f0579d840abe589538735eec1e7a917561eeed77d3b20843baf03  parser.c
380edc27e2020e5ba2d6415c9f6c0065965771d60138ae53372858e7b1f92e3b  scanner.c
180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2  tree_sitter/parser.h
5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94  tree_sitter/array.h
b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea  tree_sitter/alloc.h
```

The former parser came from upstream workflow
`https://github.com/alex-pinkus/tree-sitter-swift/actions/runs/30769290474`, artifact
`generated-parser-src`, generated using CLI `0.23.0`. This local regeneration
supersedes that parser while retaining the exact upstream scanner.
