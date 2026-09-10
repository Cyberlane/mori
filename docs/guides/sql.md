# SQL and embedded SQL

SQL queries occupy the `sql-query` comparison domain and never compare with
code functions.

## Generic SQL

```sh
mori scan --profile sql path/to/queries
```

Mori extracts top-level `SELECT` and set-operation queries plus `INSERT`,
`UPDATE`, and `DELETE`. DDL and nested queries are not independent comparison
units. Nested structure remains part of its top-level query.

The generic parser supports the documented SQLite and SQLC forms, including
common pagination parameters and SQLite `ON CONFLICT` column targets. Exact,
adjacent SQLC `-- name: Name :mode` comments are display labels only.

The bundled generic grammar also recognizes SQLite `PRAGMA` assignments and
arguments, `STRICT` table suffixes, expression-valued `CHECK` constraints
(including `BETWEEN` and `GLOB`), and `BEGIN ... END` trigger bodies with query
statements. These forms are parsed directly, retaining source offsets and
visible errors in malformed statements. Recognizing DDL does not turn it into
a comparable query: a schema-only file can still have zero fragments and fail
an explicitly required fragment-coverage policy. This is bounded syntax support,
not database execution or complete SQLite semantic validation.

## PostgreSQL

Select the dedicated PostgreSQL 18 parser explicitly:

```sh
mori scan \
  --profile sql \
  --sql-dialect postgresql \
  path/to/postgresql
```

One invocation uses one dialect for every discovered `.sql` file. Split mixed
dialect repositories into separate scans. PL/pgSQL procedural bodies are not
independent comparison units.

## SQL embedded in Go

Mori can explicitly inspect direct string arguments to recognized
`database/sql`-style `Exec`, `Query`, `QueryRow`, and `Prepare` methods:

```sh
mori scan \
  --comparison-domain sql-query \
  --embedded-sql \
  --sql-dialect postgresql \
  path/to/go/project
```

This mode is off by default. It does not guess from arbitrary strings, follow
variables, concatenate expressions, or perform receiver-type analysis.
Locations point to the host Go string and retain parent-function metadata.

Mori visibly skips a file above 1,000 recognized calls and a decoded query
above 256 KiB. A string containing multiple top-level statements is one
query-batch unit. Inspect the host call, runtime values, database permissions,
transactions, schemas, and query plans before acting on any structural match.
