# Tree-sitter C compatibility-source provenance

Mori vendors the grammar source and generated C needed for reproducible builds.

- Upstream: `https://github.com/tree-sitter/tree-sitter-c`
- Base: Go module `github.com/tree-sitter/tree-sitter-c@v0.24.2` from the verified Go module cache.
- Generator: Tree-sitter CLI `0.25.10` (commit `da6fe9beb4f7f67beb75914ca8e0d48ae48d6406`).
- Grammar ABI: `15` (unchanged from the original bundled language).
- License: MIT, reproduced in `LICENSE`.

Local changes:

- Recognize the bounded libgit2 `GIT_FORMAT_PRINTF` annotation and compound
  bodies for `git_vector_foreach` and `git_attr_file__foreach_matching_rule`.
  Arguments and bodies remain parsed. Arbitrary calls do not become loops.

- Preserve nested `#if` / `#ifdef` / alternative directives inside initializer
  lists when branch entries end in commas, without selecting a build branch.
- Recognize static uppercase `*_DEFINE(...)` declarations with a semicolon and
  `DT_INST_FOREACH_STATUS_OKAY(...)` declarations. Arguments retain their syntax;
  Mori does not expand macros or validate their definitions/arity.
- Recognize typed `ZMK_DISPLAY_WIDGET_LISTENER(...)` declarations and
  `STRUCT_SECTION_FOREACH(...)` compound statement bodies without macro expansion.
- Preserve one conditional `else` clause enclosed by `#if`, `#ifdef`, or `#ifndef`
  after a braced `if` without an existing alternative. A separate wrapper and one
  explicit parse ambiguity preserve ordinary `if`/`else` precedence and unrelated
  following preprocessor blocks.
- Existing ordinary initializer item nodes retain their original shape.

Regeneration: copy `grammar.js`, `tree-sitter.json`, and `LICENSE` to an empty
working directory, then run `tree-sitter generate --abi 15` using the pinned
CLI there. Copy generated `src/parser.c` and `src/tree_sitter/` back here.
Ordinary builds do not download or regenerate these sources.

SHA-256:

```text
2e0110e07abef7c2548b26ec9d6969775617ca539a0dc8dbeeb14d6452c711d1  LICENSE
c67fd24cea0ea4d170c5b16577ad0675c04455a605da7505884f583a01475700  grammar.js
db443d113b0c66d0f3ba83ecaa7536fac818a1f49bb60e30ab208f3805fa0ae6  parser.c
9a823f24385fbb5fa06e52c258c726449c4e417a4476306589b6dcfaf9b831ee  tree-sitter.json
b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea  tree_sitter/alloc.h
5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94  tree_sitter/array.h
180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2  tree_sitter/parser.h
```
