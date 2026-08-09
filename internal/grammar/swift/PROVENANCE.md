# Tree-sitter Swift generated-source provenance

Mori vendors only the generated C sources and required headers needed to build
the Swift grammar reproducibly from a clean checkout.

- Upstream: `https://github.com/alex-pinkus/tree-sitter-swift`
- Source commit: `8d02b7ff390a17a43ce90c4e987c49315cfc4be6`
- Source package version: `0.7.3`
- Upstream workflow: `https://github.com/alex-pinkus/tree-sitter-swift/actions/runs/30769290474`
- Artifact: `generated-parser-src`
- Tree-sitter CLI pinned by the upstream lockfile: `0.23.0`
- Grammar ABI: `14`
- License: MIT, reproduced in `LICENSE`

SHA-256:

```text
9df63e0b6680f0b6cf1f1df613aaff2a7a4a3d9c9eb573b28b5d5c33fdaf7494  parser.c
380edc27e2020e5ba2d6415c9f6c0065965771d60138ae53372858e7b1f92e3b  scanner.c
a1f6ef161fbaf48a0e10fca90ef5290a062462b307b3898aa562993853b9f80a  tree_sitter/parser.h
4ff743903dc46f5db6aa54f31c6b4d160a8a9779e5b2ab1ee59ae7ebcd850ea1  tree_sitter/array.h
253b44a7b4313a7afd0c505c2fc6e7ce4b8e78955ebf4be3ea000532ec060673  tree_sitter/alloc.h
```

The upstream project intentionally omits `parser.c` from its main branch and
publishes it as a workflow artifact. Mori does not download or regenerate this
source during ordinary builds or releases.
