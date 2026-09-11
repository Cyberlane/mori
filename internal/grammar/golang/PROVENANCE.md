# Tree-sitter Go generated-source provenance

- Upstream: https://github.com/tree-sitter/tree-sitter-go
- Source release: v0.25.0
- Generator: Tree-sitter CLI 0.25.10
- Grammar ABI: 15
- License: MIT, reproduced in LICENSE

Mori separates `new` from `make` argument parsing. `new` accepts either a type
or an expression, including calls and arithmetic added in Go 1.26. `make`
retains its existing type-first argument rule. Existing expression and type
node names and fields are preserved. No source text is rewritten or executed.

Run `npx --yes tree-sitter-cli@0.25.10 generate --abi 15` in this directory,
copy `src/parser.c` to `parser.c`, then remove disposable `src` output after
checking the hashes and running grammar compatibility tests and `make check`.
Ordinary builds use committed generated sources and require no generator.

SHA-256:

```text
2e0110e07abef7c2548b26ec9d6969775617ca539a0dc8dbeeb14d6452c711d1  LICENSE
cca1a66bf2fd6671b48dd1f0d92709245c55f26adda32d4b0a24207ae3e332d6  grammar.js
d0b1955e4a50b2c250c859fdcd4432c5d5e93f575ac6887f66002246bf6e989e  parser.c
80d8a24648d35abb30a3cb2dbcc3a1af3919f274bcabcc277c0d9535d60abf9e  tree-sitter.json
180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2  tree_sitter/parser.h
b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea  tree_sitter/alloc.h
5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94  tree_sitter/array.h
```
