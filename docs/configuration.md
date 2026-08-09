# Project configuration and ignore handling

Mori can load one strict JSON configuration named `.mori.json`. It searches
from the current working directory upward. Use `--config <path>` to select a
different file or `--no-config` to disable configuration loading.

Unknown fields, malformed JSON, non-regular files, and files larger than one
MiB are errors. Relative `baseline` paths resolve from the configuration
file's directory. Command-line values override configured values; repeated
`--exclude` and `--language-pair` values are additive. A command-line
`--comparison-domain` overrides the configured domain.

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
  "comparison_domain": "code",
  "same_language_only": true,
  "cross_language_only": false,
  "language_pairs": [],
  "fail_on_match": false,
  "baseline": "mori-baseline.json",
  "baseline_scope": "content",
  "exclude": ["**/*_test.go"],
  "respect_ignore": true
}
```

`comparison_domain` accepts one case-insensitive domain ID printed by
`mori languages`. The selected domain is applied before parsing, so other
domains do not affect file counts, warnings, focus, or pair limits. An empty or
omitted value selects every registered domain.

`same_language_only` compares within review families. TypeScript and TSX are
one family and remain comparable. It is mutually exclusive with
`cross_language_only` and `language_pairs`. `language_pairs` accepts language
IDs or families; the `typescript` selector includes both TypeScript and TSX.
Explicit pairs must belong to a selected comparison domain.

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

Schema-6 JSON reports record the effective options and every loaded ignore
file under `configuration` so a scan can be reproduced. Review focus remains
CLI-only: `--focus-path` is repeatable, and `--changed-since` always requires
an explicit locally available Git revision.

See [Scan selection controls](scan-selection.md) for the complete validation,
execution, compatibility, and reporting contract.
