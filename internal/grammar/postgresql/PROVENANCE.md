# Tree-sitter PostgreSQL generated-source provenance

Mori vendors only the generated PostgreSQL C sources and required headers
needed for a reproducible clean build. PL/pgSQL is not included.

- Upstream: `https://github.com/gmr/tree-sitter-postgres`
- Release: `v1.2.4`
- Source commit: `9b27ba5c8700f9bf808221a0f6d17fe6515da787`
- Release archive: `tree-sitter-postgres.tar.gz`
- Release archive SHA-256: `8dbedbf1fee07d6e5eb199720a167f549c47ac1cd79a74025d5788935aa5db3b`
- PostgreSQL grammar source: PostgreSQL 18.3
- Grammar ABI: `15`
- License: BSD 3-Clause, reproduced in `LICENSE`

SHA-256:

```text
ba8c9b24ae61c1672cbcbe553eafc9815f19e132f0c1cfe7600e3dd4c1a8fd66  parser.c
4d7872a6bb2126e206a7aa0730b7f272b41d9e75c2d11af274fc4db2cd7c3352  scanner.c
180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2  tree_sitter/parser.h
31e60a1bff6f715afacce03b5b70efe42b58371b4f9595dd4af52a577ff9608c  tree_sitter/array.h
b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea  tree_sitter/alloc.h
b336477d5469bf335e7d173814e1da57d4f13c23968959a71523668ac4a9a6c2  LICENSE
```

The tagged Go module contains a Git-LFS pointer in place of the PostgreSQL
generated parser. Mori therefore uses the complete source from the official
release archive and does not download or regenerate it during ordinary builds
or releases.
