# Tree-sitter Hack generated-source provenance

Mori vendors only the generated C sources and required header needed to build
the Hack grammar reproducibly from a clean checkout.

- Upstream: `https://github.com/slackhq/tree-sitter-hack`
- Source commit: `1a7ded90288189746c54861ac144ede97df95081`
- Repository status: archived by its owner on 2025-11-03
- Grammar ABI: `13`
- License: MIT, reproduced in `LICENSE`

SHA-256:

```text
d3c9389eb2ccaf13de95c0465e71306a187886300507c8812bf69101978aba5a  parser.c
6d003ff56371b4ee31252ecfab216dccd2ce22ddcbafb482ca167fca77d5b199  scanner.c
ab104936984904469572a4e868149f7a22fb2929347f837ae6a1f9b790f1b173  tree_sitter/parser.h
```

The upstream repository is archived and has no tagged releases. Mori pins the
exact generated source commit, verifies it in tests, and does not download or
regenerate it during ordinary builds or releases. Supporting future Hack
syntax changes may require maintaining or replacing this grammar snapshot.
