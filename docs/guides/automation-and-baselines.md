# Automation and reviewed baselines

## Strict coverage policy

A successful process exit is meaningful only when the selected source was
actually covered. Start automation with explicit policies:

```sh
mori scan --format json \
  --profile review \
  --min-file-coverage 0.95 \
  --max-zero-fragment-files 2 \
  --fail-on-parse-diagnostic \
  .
```

`--min-file-coverage` divides fragment-producing files by analyzed supported
files. Generated exclusions remain visible but do not enter the denominator.
`--max-zero-fragment-files=-1` disables that limit. The broader
`--fail-on-warning` rejects any discovery, parsing, resource, focus, or coverage
warning.

Coverage-policy failure writes the report and exits `4`. Match-policy failure
exits `3`; coverage failure takes precedence.

## Changed-file gates

For a local pre-commit gate, scan exactly the index and require every
non-deleted staged path to participate:

```sh
mori scan --staged --format json \
  --include-focused \
  --require-focused-coverage \
  --fail-on-focused-match
```

The report records an index digest and explicitly excludes unstaged and
untracked bytes. `--include-focused` bypasses ordinary ignore rules for staged
paths, while explicit excludes, generated policy, language support, and
resource limits remain visible coverage boundaries.

Keep the whole repository as the comparison universe while requiring review of
groups involving changed source:

```sh
mori scan \
  --profile review \
  --changed-since origin/main \
  --fail-on-focused-match \
  .
```

Mori never fetches the revision. CI must fetch enough history for the merge
base to exist locally. Use one explicit `--changed-worktree PATH=REVISION` per
additional Git root.

`--fail-on-match` fails for every unsuppressed result at the selected threshold.
Prefer the focused policy during initial adoption so old untouched candidates
remain visible without blocking unrelated work.

## Accept intentional similarity selectively

Inspect both source locations before accepting an identity:

```sh
mori baseline add \
  --baseline mori-baseline.json \
  --identity <content-pair-id> \
  --classification intentional \
  --note 'Reviewed with the owning team' \
  .
```

Then load the baseline explicitly:

```sh
mori scan --baseline mori-baseline.json --fail-on-match .
mori baseline prune --baseline mori-baseline.json --check .
```

Use `baseline edit` to update a note or classification and `baseline remove` to
revoke acceptance. `baseline update` is preview-only unless `--accept-all` is
explicit.

Schema-3 baselines bind acceptance to a digest of the effective selection,
threshold, dialect, fragment, exclusion, ignore-content, resource, and coverage
policy. Profile mismatches fail closed. Legacy baselines remain readable but
must be explicitly migrated before mutation.

Baseline mutation refuses failed coverage policy, truncated reports, and
warnings unless every reviewed warning kind is explicitly allowed. Never use a
baseline merely to hide noisy or out-of-scope source.
