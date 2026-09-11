# Plan a large scan

`mori plan` is available in development builds after v0.33.0.

Use `mori plan` before a large scan to understand its comparison workload:

```sh
mori plan --profile review --fragment-selection production .
mori plan --profile review --fragment-selection production packages/api
```

Planning discovers and parses the selected source, then counts eligible candidate
pairs without calculating similarity scores. It uses the same language, token,
size, ignore and fragment-selection rules as a scan. It succeeds when the count
exceeds `--max-pairs` and reports that condition so you can narrow the scope.
It still takes time to parse source and enumerate candidates.

The output groups selected files by `apps/<name>`, `packages/<name>`, or their
first directory. Per-root pair counts are upper bounds before language, size and
overlap filtering. They are not the results of independent package scans.
Separate package scans omit cross-package matches. A named scope pointing at
`.` does not narrow the workload. Use concrete roots in a named scope or command
when that tradeoff fits your review.

The plan shows effective configuration, coverage, warning and parse-diagnostic
counts. Coverage policies still apply. No similarity findings are produced,
baselines are not loaded, and finding-based exit policies are not evaluated.
A successful plan never certifies a clean review. It cannot be used to accept
baseline identities or issue a review receipt. Source changes require a new plan.

## Machine-readable plans

```sh
mori plan --format json --redact-paths --scope product
```

The separate `mori-scan-plan` artifact uses schema 1, published in
[`mori-scan-plan-v1.schema.json`](../../schemas/mori-scan-plan-v1.schema.json).
`analysis_performed` is always false, meaning no similarity analysis was run.
`candidate_pairs` is the eligible whole-selection count, and
`exceeds_pair_limit` compares it with the effective configured cap.

Only text and JSON formats are supported. Planning does not support staged or
focused input, diagnostic exports, receipts, report-output files or caches. It
writes only to standard output and standard error. Managed project compatibility
checks still apply. Ordinary output includes source/configuration paths, so use
`--redact-paths` when appropriate and inspect output before sharing.
