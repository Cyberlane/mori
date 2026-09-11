# Quality and performance acceptance

Mori's acceptance checks exercise a built CLI as a separate process. They
complement the labeled calibration corpus, workflow lifecycle tests and exact
scoring tests. A successful benchmark is evidence for its workload, not a claim
about every repository or semantic equivalence.

## Local and pull-request checks

Run `make acceptance` with Go, a C compiler and Python 3 available. `make check`
includes this small, offline suite and executable gate tests that deliberately
corrupt scores, counts, timings, memory measurements and toolchain metadata. It creates repeated structures, mixed
structures and malformed source in a temporary directory, runs each workload
twice, compares complete JSON reports, checks extraction counts and verifies
the candidate-pair safety limit. Temporary source is removed on success or failure.

The machine-readable result is `dist/acceptance.json`. It includes the binary
hash, platform, workload size, repeated wall times, report hashes, scan counts
and diagnostic phase timings. Failed runs retain completed measurements and a
violation list. POSIX runs measure per-process peak resident memory using
`wait4`. Windows explicitly records memory measurement as unavailable. A
5,000-function workload receives an interrupt after 200 milliseconds and must
exit within five seconds. The artifact records the observed latency or that the
scan completed before interruption. Windows console interruption is currently
unavailable. This exercises process cancellation but does not prove which scan
phase was active when the signal arrived. CI uploads this artifact even when checks fail.

The existing `make corpus` evaluates reviewed positive, intentional and
false-positive examples, including the actionability corpus. Performance
improvements must pass those checks as well. The mixed workload repeats six different shapes and is not a corpus of entirely
unrelated functions. Synthetic structural families are
not human judgments about whether a real finding is useful.

## Public repository and nightly checks

Run `make acceptance-public` to fetch the immutable Git revisions in
`scripts/acceptance-corpus.json`. The initial corpus contains Click (Python),
Chi (Go), Lodash (JavaScript) and TanStack Query (its TS/TSX `packages`
monorepo scope). The TanStack case explicitly requires extraction from both
TypeScript and TSX. Source is scanned, never built or executed.
No repository package scripts or dependencies are installed. Checkouts are
temporary and removed after the run, including on failure. Network access is
required only for this explicit mode. The nightly workflow runs this larger
suite and the labeled corpus.

The manifest deliberately starts with tractable projects and a bounded public
TS/TSX monorepo scope. It does not reproduce a private user's source.
Add immutable revisions and measured bounded scopes as coverage expands.
Do not silently change an existing revision when comparing performance.
`acceptance-public-golden.json` pins counts and a digest of all finding
identities, scores, occurrence counts and structural evidence. These are
regression expectations, not semantic correctness labels.

## Controlled performance gates

Shared GitHub runners record timing but do not fail for timing regressions.
To enforce a timing budget, use a stable runner with the same OS, CPU, power
settings, background load and toolchain. Collect at least five repetitions:

```sh
python3 scripts/acceptance.py --size 1000 --repeats 5 \
  --runner-id benchmark-host-1 --output dist/before.json
# Build the candidate binary, then run the same workload.
python3 scripts/acceptance.py --size 1000 --repeats 5 \
  --runner-id benchmark-host-1 --baseline dist/before.json \
  --timing-gate --memory-gate --max-regression 0.20 --output dist/after.json
```

The timing gate compares median wall times. The optional `--memory-gate`
compares median peak resident memory and rejects unavailable measurements. `--max-memory-regression` controls the memory tolerance (default 20%).
Both gates reject incompatible environment and
workload metadata, including the harness hash and Go version/target. Baseline comparison also fails
on changed findings or counts, independently of the timing gate. Identity
comparison ignores ranking order and paths and includes every reported group.
Use `--allow-result-changes` only after reviewing a deliberate change. The
artifact always retains before/after differences. Count comparisons include
top-list test/story counts, so an intentional ranking change may require review
even though identity comparisons ignore ranking order. Public golden updates require
the same review. The 20% example is a starting tolerance, not a statistically
established noise floor. Establish runner variance before tightening it. Timing
samples include fresh CLI startup and ordinary OS filesystem caching. They do
not measure a warmed Mori review-session cache or independently cold disk I/O.

Use `--binary /path/to/mori` for an installed or unpacked release. The published
release verification workflow runs the small suite on each native archive.

## Interpretation and remaining coverage

Counts and deterministic reports are hard checks. Timing is separate from
ranking quality. The public corpus asserts successful extraction, repeatability and its pinned
regression expectations, not a frozen list of semantically correct findings. Deliberate scoring or
schema changes require reviewed result differences, not blindly updating hashes.

This harness does not yet model useful-findings recall on a held-out large
monorepo. Windows peak memory and interruption measurement are unavailable. Diagnostic sessions
currently expose candidate counts and broad phase timing rather than every
internal pruning or cache counter. Track these limitations when making speedup
claims, and extend the same artifact schema explicitly when measurements grow.
