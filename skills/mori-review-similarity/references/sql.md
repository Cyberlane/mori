# SQL and embedded SQL

Read this reference only when SQL files or SQL embedded in source are in the
requested scope. SQL findings are separate from ordinary code-function review.
Set `MORI_REPORT` to the owner-private report path selected under the shared
review contract before using these examples.

## SQL files

Use a deliberate SQL profile:

```sh
mori scan \
  --profile sql \
  --format agent \
  --output "$MORI_REPORT" \
  --max-occurrences 10 \
  path/to/queries
```

For PostgreSQL source, add `--sql-dialect postgresql`. The default is
`generic`; one invocation applies one dialect to every discovered `.sql`
file, so split mixed-dialect roots into separate scans. The PostgreSQL parser
covers PostgreSQL 18.3 SQL syntax but does not make PL/pgSQL bodies independent
comparison units.

SQL uses the `sql-query` comparison domain and is never compared with code
functions. Do not request `sql` in a language pair with a code language. Mori
extracts top-level `SELECT`/set-operation, `INSERT`, `UPDATE`, and `DELETE`
statements. DDL and nested queries are not independent fragments. SQLC names
are location labels, not fingerprint inputs. Common `?` and SQLC
`LIMIT`/`OFFSET` parameters plus SQLite `ON CONFLICT` column targets are
supported. Inspect warnings for other dialect syntax, and verify schemas,
permissions, transaction context, query plans, and tests before recommending
consolidation.

## Embedded SQL in Go

For SQL embedded in Go, add `--embedded-sql` to a
`--comparison-domain sql-query` scan and select the dialect. Confirm each
finding is a direct recognized database-method string argument. Inspect its Go
parent and disclose that Mori does not prove receiver types, variable
contents, concatenations, or runtime query construction.

The fixed 1,000-call and 256-KiB query limits are coverage boundaries; report
their warnings. A multi-statement string is one query-batch unit, not several
independently source-mapped occurrences.

Keep SQL scans bounded: do not set `--max-groups`, `--max-occurrences`,
`--max-pairs`, or `--max-file-bytes` to zero without explicit user request.
Open both SQL ranges and surrounding code/query context before classifying a
match. Check schema, permissions, transaction behavior, query plans, callers,
and tests; a high structural score cannot establish equivalent results or
runtime behavior.
