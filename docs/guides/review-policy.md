# Choose the review policy

A structural match is a review lead, not a defect. New integrations can start
with an advisory check of the immutable Git index:

```sh
mori review staged check --policy advisory --format agent --output /tmp/review.json .
```

The advisory policy reports focused matches without failing solely because they
exist. It preserves configured coverage requirements and canonical focused-file
coverage. Execution failures, incompatible project contracts, and required
coverage failures still return nonzero. Incomplete analysis stays visible.

Existing commands and hooks retain **strict** behaviour by default:

```sh
mori review staged check --policy strict .
mori hook pre-commit .
```

A project can explicitly choose `mori hook pre-commit --policy advisory .`.
Changing a project's enforcement requires its owner's authorization; upgrades do
not rewrite project hooks or select advisory automatically. `--policy` is a
canonical staged-check/hook option, not a scan profile or a `.mori.json` field.

## Machine outcomes

Report schema 21 adds an optional `review` object for canonical staged checks:

- `policy`: `strict` or `advisory`.
- `status`: `passed` or `blocked` by that policy.
- `analysis`: `complete`, `incomplete`, or `no-comparable-source`.
- `coverage_policy_met`: whether configured coverage requirements passed.
- `findings`: the number of unsuppressed focused groups.
- `acknowledged`: whether a compatible exact staged receipt was validated.

`passed` does not mean findings were inspected, the program is correct, or all
files contributed comparison fragments. Diagnostics, zero-fragment files,
missing analyzed files and report truncation make the analysis incomplete even
when the selected policy permits proceeding. `complete` describes the bounded
configured comparison universe, not every language or all possible duplication.
Operational errors may prevent a report from being produced at all.

Policy selection does not change similarity, normalized identities, the scan
profile digest, or baseline acceptance. Strict focused findings still return 3;
coverage failures return 4. Ordinary scan commands retain their existing flags.

## Review without repetition

Use the agent format's 25-identity shortlist and retain its JSON report. Inspect
both source locations for the identities you review, state the remaining count,
and never label the unreviewed remainder accepted. A strict project with more
findings must follow its acceptance policy; choosing a shortlist does not bypass
that gate. Explicit standing project authorization can cover routine reviewed
classifications and receipts without repeated approval requests.

Trust the pinned toolchain. Schedule upgrades separately from source tasks. Do
not fetch latest releases or rerun the same broad scan on each commit attempt.
Keep tests and generated-source policies explicit; do not exclude product code
or lower thresholds merely to clear a finding.

## Compatibility

Report schema advances from 20 to 21. Hook contract advances from v1 to v2 to
record explicit policy selection with strict default. Project-contract schema
remains 1, configuration schema remains 1, receipt schema remains 2, baseline
schema remains 4. Normalization advances to 13 for the Swift grammar correction,
independently of advisory policy. Review prior baseline/receipt compatibility;
the policy flag does not accept old evidence automatically. The previous official project
contract remains strictly readable and migrates through `project upgrade`.
Stored acceptance is not weakened. A known prior official skill is upgradeable;
local skill customizations remain protected.
