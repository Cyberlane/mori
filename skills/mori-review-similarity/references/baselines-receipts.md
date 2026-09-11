# Baselines and staged focused receipts

Read this reference only when the user asks for durable suppression, baseline
maintenance, or a staged focused-review receipt. A baseline or receipt records
an explicitly reviewed decision; it never substitutes for source inspection or
hides a warning.

## Baselines

Before baselining a noisy repository, establish repeatable project scope with
`.mori.json`, `.moriignore`, or existing ignore files. Use those controls for
generated artifacts, design previews, vendored code, or intentionally separate
test profiles. Do not use a baseline to hide out-of-scope noise.

For configured repositories and reviewed intentional candidates:

```sh
mori baseline add \
  --baseline mori-baseline.json \
  --identity <content-pair-id> \
  --classification intentional \
  --note 'Reviewed with the owning team' \
  .
mori scan --baseline mori-baseline.json --fail-on-match .
mori baseline prune --baseline mori-baseline.json --check .
```

Prefer selective `baseline add` after inspecting the identity. Use
`baseline edit` for durable notes/classifications and `baseline remove` to
revoke acceptance. `baseline update` is preview-only unless `--accept-all` is
explicit. Mutations use complete internal reports and reject warnings unless
each reviewed warning kind is repeated with `--allow-warning`.

Schema-4 baselines bind acceptance to the effective scan-profile digest and
support `false-positive` as a precise durable classification. In strict gates,
require `configuration.baseline_profile_status` to be `compatible`. Schema-1
through schema-3 baselines remain readable with a warning; run
`baseline migrate --accept-profile` before mutation.
Schema-20 `configuration.focused_only` is audit evidence, not a baseline
profile field; existing accepted identities remain compatible when canonical
staged review omits untouched-to-untouched pairs.

Content scope is the default: one accepted normalized content-pair identity can
suppress identical copies in new locations. Use path scope when copied code in
a new file must reappear; selective add then accepts all exact scored path
pairs for that identity. A missing, mismatched, or tampered baseline is an
operational failure, not an empty baseline. Verify `content_pair_id`, scope,
profile digest, warnings, and source review before accepting.

## Staged receipts

When the owner explicitly accepts focused findings for exactly one staged
commit and durable suppression would mislead, use:

```sh
mori review staged acknowledge --accept-focused .
```

The default local receipt lives under private Git metadata and uses owner-only
permissions on POSIX filesystems. Pass it to the canonical check:

```sh
mori review staged check --review-receipt <receipt> .
```

Require a compatible receipt in schema-20 evidence. Canonical check and
acknowledge share staged focused-file inclusion and complete focused coverage
by construction. A receipt changes only focused-match policy exit status: it
never hides findings. Any HEAD, index, staged-review contract, profile, tool,
normalization, or focused-identity change invalidates it.

Receipt creation/use requires owner authorization. Direct commit
authorization need not be re-requested, but honor standing project authorization for this action. Ask again only
for unresolved owner decisions or when receipt authorization is absent.

### Accept several reviewed identities in one scan

Repeat `--identity` to accept a reviewed batch using one complete validation scan
and one atomic baseline write:

```sh
mori baseline add --baseline mori-baseline.json \
  --identity <first-reviewed-id> --identity <second-reviewed-id> \
  --classification intentional .
```

Every identity must exist in the active scan. If any identity is missing, nothing
is written. Duplicate identities are accepted once. The supplied note and
classification apply to every selected identity, so use separate batches for
different decisions. Inspect each finding before accepting it. Existing baseline
profile mismatches are rejected after discovery, before parsing or comparison.
This changes no baseline document schema.

Before a potentially expensive scan, use `mori plan` with the same roots and
selection options. It parses and counts candidates without scoring similarities,
shows package workload bounds, and does not load or accept baselines. A successful
plan is not review evidence. Narrowing to separate packages omits cross-package
findings. Named scopes only reduce work when their roots or selection reduce work.
