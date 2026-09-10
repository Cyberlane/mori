# Opt-in local feedback

Mori feedback is off by default. Normal scans do not create feedback state until
this OS user explicitly enables it for the project directory. Mori never uploads
feedback, contacts a feedback service, or changes review decisions from feedback.

```sh
mori feedback status --root .
mori feedback enable --root .
# Run ordinary scans, then inspect exactly what could be shared:
mori feedback export --root .
# Saving the preview is an explicit export; it does not submit anything:
mori feedback export --root . > feedback.json
mori feedback disable --root .
mori feedback clear --root .
```

`disable` stops collection and retains existing samples. `clear` deletes retained
samples while keeping the current consent decision. To stop collection and erase
history, run both. Empty status, clear, disable, and export operations do not enroll
a project. Consent is outside tracked project configuration, under the OS user's
configuration directory in `mori/feedback`. A local digest of the canonical project
path selects the storage directory; it is never included in exports. Use the same
project root for enabling feedback and scans. Different roots and worktrees have
independent consent; moving a project does not inherit consent.

CI is excluded unless enabled separately with `mori feedback enable --ci --root .`.
This grants CI collection only for this user's local state and this project root;
it does not create a portable repository permission or an upload permission.
Common CI environment indicators are recognized; custom automation should set
`CI=true`. Enabling again without `--ci` withdraws CI consent.

## What is collected

The schema-version-1 bundle contains at most the latest 256 samples, with no
age-based expiry. Samples contain only:

- Total scan elapsed-time bucket: under 1 second, 1–9 seconds, 10–59 seconds,
  1–5 minutes (exclusive of 5 minutes), or at least 5 minutes. This measures scan time, not human review effort.
- Coarse counts for files, fragments, candidate pairs, match groups, and parse
  diagnostics: zero, 1–9, 10–99, 100–999, or at least 1000.
- Outcome category: completed, findings, coverage, error, or unknown.
- Review policy (`strict`, `advisory`, or ordinary `scan`), reported analysis
  status (`complete`, `incomplete`, `no-comparable-source`, or `unknown`), and
  findings scope (`focused` or `all`). Focused scans count focused match groups.
  Ordinary scans use `unknown` analysis status; command success never implies
  complete analysis.
- Coarse warning counts by fixed categories (`parse`, `coverage`, `baseline`,
  `focus`, or `other`), never raw diagnostic messages or paths.
- Whether the report was truncated, and cache status (hit, miss, bypassed).
- Tool release version restricted to numeric `major.minor.patch`; development,
  prerelease and custom builds are reported as `dev`, with no revision metadata.
- Coarse file counts by a fixed allowlist of language categories; unknown language
  labels become `other`.
- Optional explicit classification of one reviewed finding, its coarse shortlist
  rank, and self-reported review effort. These are absent until supplied.

There are no source snippets, names, paths, hashes, repository metadata, exact
counts, timestamps, persistent project identifiers, raw diagnostics, command
arguments, environment values, free text, or automatic review classifications in
the sharing schema. No consent settings are exported. Aggregate combinations may
still characterize a workload; review the exact preview before choosing to share
it. The export command accepts no destination service and cannot transmit data.

Samples are constructed from an explicit allowlist rather than redacting ordinary
reports. State is size-bounded, parsed strictly, and validates all exported values
against fixed enumerations. POSIX files and newly created directories use user-only permissions.
On Windows, mode bits cannot establish ACL privacy; storage relies on the OS
user configuration directory's inherited ACL. Mori does not modify Windows ACLs. Unsafe storage paths and concurrent writes fail closed; feedback failure
never changes a scan's exit code, report, baseline, or review receipt.

## Optional review feedback

After inspecting a finding from the most recent retained scan:

```sh
mori feedback classify --root . --classification intentional --rank 7 --effort 1-5m
```

Classification is `useful`, `intentional`, `false-positive`, or `uncertain`.
Optional effort is `under-1m`, `1-5m`, `5-15m`, or `15m+`; it is a self-reported
estimate for the review session, never inferred from elapsed scan time. Optional
positive rank is stored as `1-5`, `6-10`, `11-25`, or `26+`. Omit rank when unknown.
This annotates **one reviewed finding** in the latest sample and replaces any
previous annotation there. It does not classify every finding in that scan or
count multiple reviewed findings. Classification requires current collection
consent (including CI consent in CI); no sample or zero findings means no classification. It never
creates a receipt, accepts a baseline, or authorizes a refactor.

## Limits and next steps

This local pilot measures coarse scan cost, cache use, language mix, findings
volume, and voluntarily annotated usefulness/rank and review effort. It cannot
identify repeated findings across scans, establish complete review coverage, or
measure upgrade interruptions. A single voluntarily selected finding is not an
unbiased sample of all findings. Feedback consent does not authorize
source-fixture sharing, automatic submission, or changes to accepted baselines.
A future submission service needs a separately reviewed consent and retention
contract. Do not interpret unreviewed findings as either useful or false positives.

Collection is skipped for scans with multiple explicit paths, multiple worktrees,
or non-staged `--include-focused` source scopes. This avoids applying one project's
consent to source belonging to another project. Enable and scan individual project
roots to obtain measurements for those workflows.

Interrupted writes recover automatically on a later operation: an unchanged,
empty lock directory older than ten minutes may be removed with one bounded
retry. Recent locks, nonempty locks, and symlinks are left untouched. After acquiring
the lock, Mori inspects at most 512 directory entries and removes only regular
`.state-` temporary files older than ten minutes. Other files and fresh temporary
files are preserved. Collection and consent changes never wait indefinitely on a
lock; a recent busy writer produces a retryable feedback error.

## Offline pilot and calibration

Use `mori feedback summarize EXPORT.json [EXPORT.json ...]` to inspect explicitly
exported bundles without opening consent storage or sending data. It accepts up to
64 regular files, each at most 1 MiB and 256 samples, and rejects non-feedback
schemas, unknown fields, and invalid categories before writing any summary. Output
is human-readable text; the feedback sharing schema remains version 1.

The summary separates scan samples, selected finding annotations, annotations with
rank, and self-reported review sessions. It shows timing buckets grouped by tool
version, policy, cache state and file-count bucket, and reports analysis status,
findings volume, warning and language presence. Presence counts overlap when a
sample contains several languages or warnings. It prints no input paths or raw
export contents. Identical bundles are counted with an explicit notice: without
project identifiers, Mori cannot tell copied exports from independent projects
that happened to produce identical measurements. Exports are never a project count.

A reproducible local pilot can use the following procedure. Each participating
user decides independently whether to collect and whether to share the preview.

1. Agree on a fixed observation window and representative tasks before collecting.
   Record the chosen release, policy and workload locally. Preserve strict gates
   where required; any advisory trial needs the project's explicit agreement.
2. Enable feedback at the exact project root. Complete ordinary tasks, recording
   at most one finding classification per retained scan. Choose a consistent
   sampling rule in advance (for example, the first actually reviewed finding),
   and keep the same rule throughout. Record rank only when known. This reduces
   selection drift but does not make the sample unbiased.
3. For a cache experiment, compare the same staged snapshot and options with a
   cold invocation and repeated `--cache` invocations. Keep correctness checks
   separate from timing. Use a local benchmark for differences within one timing
   bucket; bucketed feedback cannot establish a percentage speedup.
4. Export and inspect the exact preview. Save one final export per collection
   window outside tracked source. Clear retained samples before a subsequent
   window to prevent overlap, only after retaining the intended export. Do not
   combine rolling snapshots of the same retained samples.
5. Run the offline summary on those final exports. Compare matched version,
   policy, cache and workload strata. Treat missing classifications as unknown.
   Examine incomplete analysis and coverage outcomes before interpreting fewer
   findings as an improvement.
6. Require a reviewed source fixture and a nearby negative before changing a
   parser or ranking rule. Export consent does not include source-fixture consent.
   Verify the fixture locally or obtain separate permission to share it. Validate
   the proposed change on held-out fixtures and the corpus, then repeat the pilot.
7. Disable collection and optionally clear retained samples when the window ends.
   Shared copies have their own retention owner; local clearing cannot delete them.

A synthetic test of summary arithmetic is not a project pilot. Useful-finding
counts describe only volunteered annotations, and repeated scans can correlate.
Do not use this data to infer semantic equivalence, calculate a global false
positive rate, or automatically adjust thresholds/ranking. Upgrade interruptions,
pre-analysis errors and receipt invalidations that occur before a report exists
remain unmeasured. Address those gaps only with a separately specified minimized
contract; no source or error text should be added as a shortcut.
