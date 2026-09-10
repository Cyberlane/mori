# Roll out smoother reviews

This guide applies to builds containing advisory policy, staged caching and local
feedback. Check the selected binary's help first; an older pinned release may not
provide these commands. Trust the project's pin during feature work and schedule
upgrades as maintenance.

## Validate the release candidate

Run `make check` against the exact proposed source. CI builds and tests all five
release targets natively: Linux AMD64/ARM64, macOS AMD64/ARM64 and Windows AMD64.
The minimum Go job tests Go 1.23 separately. A configured job is not a passing
result; retain its outcome for the exact candidate commit. Do not substitute a
cross-compile for a native CGO test.

The report contract is schema 21 and the hook contract is v2. Configuration and
project-contract schemas remain 1; baseline and receipt versions
remain 4 and 2; normalization advances to 13 for corrected Swift grammar.
Prior normalization acceptance needs explicit review during migration.
Supported prior official project state must have a normal
managed migration, with strict decoding and no automatic rewrite of project-owned
policy. Exercise staged deletion/rename, partial staging, coverage failure,
receipt drift and cached/uncached parity before adoption.

Use the fixed `BenchmarkStagedAnalysisReuse` fixture to measure the cache:

```sh
go test ./internal/cli -run '^$' \
  -bench 'Benchmark(StagedAnalysisReuse|StagedCacheExecutableHash)' \
  -benchmem -count 5
```

Compare the same fixture and toolchain. Include executable hashing and fresh
process overhead when judging command latency. A warm in-process benchmark alone
is insufficient to recommend caching for small projects. Cache absence or failure
must preserve review decisions and fall back to analysis.

## Upgrade a willing project

Start with one project, retain its current pin and managed assets for rollback,
and inventory its hook, review scope and baseline. Install the chosen binary by
the normal verified release process, then preview its managed migration:

```sh
mori version
mori project upgrade --dry-run .
# After reviewing the plan as part of the authorized upgrade:
mori project upgrade --apply .
mori project upgrade --check .
```

Review the pin, project contract and complete managed skill together. Keep
project-owned checks, source scope and snapshot guarantees. Replace a custom
wrapper only when its additional checks have been preserved. Compare surviving
staged paths with the report's live paths; retain deletions and rename sources in
the full staged snapshot used to validate readiness.

Ordinary review should save one report, inspect at most 25 identities deeply and
disclose the remainder. Query retained evidence before rescanning. Commit gates
use `mori hook pre-commit .` or `mori review staged check .`, including partially
staged changes. Findings remain strict by default. A project can explicitly adopt
`--policy advisory`; coverage and execution errors still fail. A project can
independently choose `--cache`. Neither choice enables feedback.

Review baseline profile differences before migrating acceptance. Never accept all
findings to make an upgrade pass. Restore the previous compatible binary and
managed assets together if rolling back; do not combine an old binary with a new
contract or delete project-owned work.

## Evaluate with consent

Use the [local feedback guide](feedback.md) for opt-in collection, exact export
preview, offline summaries, withdrawal and deletion. Run a synthetic rehearsal
first, then invite willing users. Record consent separately from findings and
release acceptance. Collection consent does not authorize sharing or source
fixtures.

Compare like workloads by version, policy, cache state and size. Report sample
counts and selected-finding denominators, missing classifications, workload
differences and opt-in bias. Coarse duration buckets cannot establish exact
medians or tail latency. Use fixed benchmarks for exact timings. Human review
effort comes only from explicit self-report.

Keep ranking changes evidence-led: review positive and nearby-negative fixtures,
verify useful candidates remain in retained reports, and measure shortlist
usefulness independently of structural score correctness. Do not infer that
intentional UI symmetry should be hidden or that unreviewed groups are defects.

## Publish and expand adoption

After the candidate passes native checks and the pilot has been assessed, prepare
a SemVer release with the schema migration and policy defaults in its notes. A
release tag must target a commit already passing `make check`. Assemble the native
archives and checksums on a draft release, verify all assets, then publish through
the authorized release process. Upgrade additional willing projects in small
batches and preserve their policy choices.

An upload service is a separate future proposal. Before building or enabling it,
define recipient, purpose, retention, deletion and operational ownership. Export
and offline analysis already support sharing an explicitly reviewed bundle through
a separately authorized channel; no service is needed to begin learning.
