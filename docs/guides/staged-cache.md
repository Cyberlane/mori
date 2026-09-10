# Opt-in staged analysis cache

`mori review staged check --cache .` can reuse the analysis of an unchanged
immutable Git index. Caching is off by default and available only for canonical
staged checks. It does not collect feedback or make network requests.

Compatibility and baseline loading still run first. A hit replaces only the
analysis step; coverage policy, baseline compatibility, receipt validation, and
exit decisions run normally. Findings do not become accepted because analysis
came from a cache. Missing, unsafe, corrupt, or unavailable cache state falls
back to normal analysis.

The key binds the complete resolved snapshot (repository, prefix, HEAD, index
entries, changed paths and line intervals), requested paths, effective analysis scan
options, baseline digest, initial warnings, report/normalization/hook revisions,
and the SHA-256 digest of the actual executable. Render format, color, path
redaction, report destination, and receipt pathname are excluded; they are applied
after analysis. Receipt-driven unlimited group retention remains part of the key. Thus locally modified binaries
also invalidate cache entries. New unsupported option types disable caching
rather than silently omitting an input. Unstaged edits are intentionally outside
the input and do not invalidate staged analysis.

The cache stores a report and its internal exact location pairs, not source-file
contents or syntax trees. Reports contain local paths, symbol names, structural
features, and diagnostics, so the cache remains private local evidence. It is
not a feedback bundle and must not be submitted as one.

A random 32-byte HMAC key lives in the user's configuration directory under
`mori/analysis-cache-v1/key`, outside the checkout. Cache payloads are authenticated
against that key and the exact input key before decoding. A copied or edited
repository cache cannot authorize a false analysis result. This protects against
repository-controlled cache contents; it does not defend against software
already able to read the user's private key. Symlinked paths or permissive cache
files/directories disable caching rather than being repaired automatically. On
Windows, new objects receive a protected DACL granting only the current user full
access. Reads require the same owner and a single matching allow entry; null,
inherited, broadened, or otherwise unsupported ACLs and reparse points cause a
cache miss. Existing ACLs are never rewritten. Native Windows tests remain the
acceptance gate for that platform.

Per-worktree Git metadata holds `mori/analysis-cache-v1/report`. One report is
retained, at most 16 MiB plus its signature. Repeated locations are stored once
in a deterministic dictionary; exact pair order and multiplicity are retained
as indices. Decoding also limits reconstruction to one million location pairs.
Reports above either bound skip saving and use normal analysis. A single exclusive `report.pending`
file bounds concurrent writes to one additional report of the same maximum size.
Competing writers skip saving. A process killed during a save may leave that
pending file. A later save can recover a private regular pending file older than
ten minutes after checking its identity, size, and modification time. Recent
files and symlinks are preserved, and a failed recovery simply skips saving. No growing history or automatic eviction of unrelated files exists.

Omit `--cache` to disable use without deleting anything. To discard retained
analysis, remove the dedicated cache directory from that worktree's Git metadata.
Deleting the user-local key invalidates all previously signed caches; the next
explicit cached check creates a new key. No installation or project configuration
change is needed.

## Measuring reuse

Run `go test ./internal/cli -run '^$' -bench 'BenchmarkStagedAnalysisReuse|BenchmarkStagedCacheExecutableHash' -benchtime=1x`.
The fixed fixtures exercise 80 and 400 repeated functions. Measure the separate
executable hash as well: each fresh process pays that cost, which the in-process
reuse benchmark amortizes. Small scans may break even; larger reports benefit
from the location dictionary. These benchmarks do not establish performance for
a particular adopter's project.
