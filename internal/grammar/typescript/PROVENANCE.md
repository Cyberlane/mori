# Tree-sitter typescript generated-source provenance

- Upstream: https://github.com/tree-sitter/tree-sitter-typescript
- Source release: v0.23.2
- Generator: Tree-sitter CLI 0.25.10
- Grammar ABI: 14
- License: MIT, reproduced in LICENSE

Replace permissive import-expression type alternatives with an explicit import_type rule supporting qualified and generic type references and typeof import. Add a type-member semicolon scanner token for newline-separated generic call signatures; require following parameters and preserve multiline generic type references.

Support explicit `in`, `out`, and `in out` variance modifiers while retaining `in` and `out` as parameter identifiers when no modifier is present.

The TypeScript definition inherits the pinned JavaScript 0.23.1 grammar in `javascript/grammar.js`; its MIT license is retained separately.

The generated parsers and scanners are committed; ordinary builds need no Node,
network, or generator. Run `sh regenerate.sh` in this directory with the pinned
`tree-sitter` executable on PATH to reproduce parser sources. Source text is not
rewritten at parse time. Run grammar ABI tests and `make check` after regeneration.

SHA-256:

```text
49bf33cf78ef5897e4e161ce1517df7de1ae5042a65b6bcfd44401e0fc606559  LICENSE
cf9c53b8b24c50796577aebbb5f5f6c08dd088952737260b37ec4827f0aa9a3f  common/common.mak
62a1dc1818e57d231cbfd543850f2ef5934f5a6057f414395ccdc37ae011580f  common/define-grammar.js
4eb409cc1c80a2a2a272c615b0f14956730334651880db5cdb5b1eee8a270aeb  common/scanner.h
2e0110e07abef7c2548b26ec9d6969775617ca539a0dc8dbeeb14d6452c711d1  javascript/LICENSE
230e330dd914d94297e5e58d53942cd5debf9c069852da93b6a6b1daae1967dc  javascript/grammar.js
4f39daa6a0bde84506cdeaf0b4460ed262df880030b3e6073868d0c746932291  tree-sitter.json
cc98f970a1842a92af7b1004bf9d9b549c831383d54c05bf7ca4bbeda68a1287  tsx/binding.go
19b7bc89bac1dfbb64e01ca15ca52345056028c2582d71762efd675af947a0f6  tsx/grammar.js
b442d76629491638a795c3db17bbbaae1dc36732a3ed0651b40a2f4aaf384e8f  tsx/parser.c
6d3c0c14702aeae118a8d33c554226f25dc2472469d80bed747505b757dfea5c  tsx/scanner.c
4eb409cc1c80a2a2a272c615b0f14956730334651880db5cdb5b1eee8a270aeb  tsx/scanner.h
b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea  tsx/tree_sitter/alloc.h
5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94  tsx/tree_sitter/array.h
180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2  tsx/tree_sitter/parser.h
6c430a72533a89f61ea84c3e301fae7fca54da264b1c537ec7ec0b32e3819ed4  typescript/binding.go
7e4109889b2ce2731dde89f82f545a12f1e54a286f72bc874f1aed9965c5d5ae  typescript/grammar.js
085ae51ef3d4f950d6c240801f1436a70e0debc5b3482f6d6cb0b4e83dda2777  typescript/parser.c
130693070291aa35649133e35e813a975668a65d6f093b8098b97e4b1bad80ec  typescript/scanner.c
4eb409cc1c80a2a2a272c615b0f14956730334651880db5cdb5b1eee8a270aeb  typescript/scanner.h
b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea  typescript/tree_sitter/alloc.h
5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94  typescript/tree_sitter/array.h
180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2  typescript/tree_sitter/parser.h
```
