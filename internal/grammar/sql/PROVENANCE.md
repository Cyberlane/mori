# Tree-sitter SQL compatibility-source provenance

Mori vendors the grammar source and generated C needed for reproducible builds.

- Upstream: `https://github.com/wippyai/tree-sitter-sql`
- Base: Go module `github.com/wippyai/tree-sitter-sql@v0.0.4` from the verified Go module cache.
- Generator: Tree-sitter CLI `0.25.10` (commit `da6fe9beb4f7f67beb75914ca8e0d48ae48d6406`).
- Grammar ABI: `14` (unchanged from the original bundled language).
- License: MIT, reproduced in `LICENSE`.

Local changes:

- Parse SQLite PRAGMA assignment/argument forms and STRICT table suffixes.
- Admit expressions in CHECK constraints, including BETWEEN.
- Add GLOB and NOT GLOB operators with pattern-matching precedence.
- Parse SQLite BEGIN/query-statements/END trigger bodies and unparenthesized WHEN.
- DDL and PRAGMA remain outside independent query comparison coverage.

Regeneration: copy `grammar.js`, `tree-sitter.json`, and `LICENSE` to an empty
working directory, then run `tree-sitter generate --abi 14` using the pinned
CLI there. Copy generated `src/parser.c` and `src/tree_sitter/` back here.
The external `scanner.c` remains the upstream module source unchanged.
Ordinary builds do not download or regenerate these sources.

SHA-256:

```text
3b3e4e4252d5d7d1c18e1257005f23242bf0580ad619204fd093c99b8c56748d  LICENSE
40d16a63a468f0bac2aa67491cb90c8dc7cf5dd1f5ef30ea0814b1c1234f59ed  grammar.js
a5e3de0fc435d65d4597c565afcb856b7d86ea88fa6f6e7c5500540055c18727  parser.c
4d09c84073f20be5fff2a0bee49dbdeafdf2428913cb6543e2986e07a246fc86  scanner.c
785766c0d78e9534330540f7a0dcc64e601129973b989c223d3effb31d846a90  tree-sitter.json
b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea  tree_sitter/alloc.h
5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94  tree_sitter/array.h
180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2  tree_sitter/parser.h
```
