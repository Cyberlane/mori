# Tree-sitter GDScript generated-source provenance

Mori vendors only the generated C sources and required headers needed to build
the GDScript grammar reproducibly from a clean checkout.

- Upstream: `https://github.com/PrestonKnopp/tree-sitter-gdscript`
- Source commit: `d2a0ee914d297b873a40dd4596bd1f7157ebc52b`
- Source tag: `v6.1.0`
- Grammar ABI: `14`
- License: MIT, reproduced in `LICENSE`

SHA-256:

```text
cc18c78720d6dfc8951e3132e1d3abeeb241b1c16c3ecdd54ff8551ad2017f04  parser.c
175188924474f3265c49ac364695036136eb69ced21ac5e6a23e3a9d9c4a2e75  scanner.c
a1f6ef161fbaf48a0e10fca90ef5290a062462b307b3898aa562993853b9f80a  tree_sitter/parser.h
5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94  tree_sitter/array.h
b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea  tree_sitter/alloc.h
```

Mori does not download or regenerate this grammar during ordinary builds or
releases.
