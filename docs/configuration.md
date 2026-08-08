# Project configuration and ignore handling

Mori can load one strict JSON configuration named `.mori.json`. It searches
from the current working directory upward. Use `--config <path>` to select a
different file or `--no-config` to disable configuration loading.

Unknown fields, malformed JSON, non-regular files, and files larger than one
MiB are errors. Relative `baseline` paths resolve from the configuration
file's directory. Command-line values override configured values; repeated
`--exclude` and `--language-pair` values are additive.

## Fields

```json
{
  "threshold": 0.85,
  "min_tokens": 40,
  "max_groups": 250,
  "max_occurrences": 10,
  "max_pairs": 5000000,
  "max_file_bytes": 2097152,
  "workers": 8,
  "format": "json",
  "cross_language_only": false,
  "language_pairs": ["go,typescript"],
  "fail_on_match": false,
  "baseline": "mori-baseline.json",
  "baseline_scope": "content",
  "exclude": ["**/*_test.go"],
  "respect_ignore": true
}
```

`language_pairs` accepts language IDs or families. The `typescript` selector
includes both TypeScript and TSX. It cannot be combined with
`cross_language_only`.

`max_groups` bounds distinct content-pair identities. `max_occurrences` bounds
the locations shown for each fingerprint while retaining the exact occurrence
count. Zero opts into an unlimited value and should be used deliberately.
`--max-matches` remains a deprecated alias for `--max-groups`.

## Ignore files

Directory scans honor `.gitignore` and `.moriignore` files from the working
directory through nested scan directories. Rules are evaluated in order;
`.moriignore` is read after `.gitignore` in the same directory, so it can add
or negate repository rules. Common VCS, dependency, and build directories are
still excluded by Mori's built-in policy.

Ignore files support the Git-style rules Mori needs for repository discovery:
comments, negation, directory suffixes, anchoring, and `*`, `?`, and `**`
wildcards. An ignored directory is still traversed when an applicable negation
may re-include a descendant. Invalid, oversized, symlinked, or non-regular
ignore files fail the scan instead of silently weakening exclusions.

Ignore files affect directory traversal only. A file passed explicitly remains
visible. Repeated `--exclude` globs remain additive and continue to exclude an
explicitly requested file. Use `--no-ignore` to disable both ignore-file types.

Schema-5 JSON reports record the effective options and every loaded ignore
file under `configuration` so a scan can be reproduced. Review focus remains
CLI-only: `--focus-path` is repeatable, and `--changed-since` always requires
an explicit locally available Git revision.
