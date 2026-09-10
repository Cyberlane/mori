# Collecting a support bundle

Mori can record a small local diagnostic session for a scan you want help with.
Collection is explicit for each invocation. Creating a bundle is a separate
command, and neither command uploads anything or attaches source code.

## Record, inspect, then bundle

Requires Mori v0.33.0 or later. To investigate a problem, repeat the same scan
with the same options and paths, adding `--diagnostics` first. This preserves the
workload that caused the problem and captures later argument errors. For example:

```sh
mori scan --diagnostics session.json --profile review --format agent packages/core/src
mori support inspect session.json
mori support bundle --session session.json --output mori-support.zip
mori support inspect mori-support.zip
```

The scan retains its normal exit code, including findings, coverage failures and
candidate-pair limits. A session write failure produces a sanitized warning; it
does not turn a successful scan into a failure or hide an existing scan failure.
Use a new session filename for each scan; an existing file is not overwritten.
If writing fails, inspect the warning before using any older file at that path.
Sessions are written when the command returns. A hard kill, out-of-memory
termination or system crash can prevent creation of the session.

`support inspect` prints only the validated session payload as canonical JSON.
It does not print arbitrary contents from an input file. `support bundle` prints
the same session JSON to standard output and confirms local creation on standard
error. Existing output files are never overwritten. Choose a new filename for
each bundle.

Inspect the result before attaching it to an issue or sending it through your
chosen channel. Sharing is your separate action. Mori does not select a recipient,
open a network connection or enable ongoing feedback collection.

## Add reviewed finding labels

Optionally annotate several shortlist ranks while creating a bundle:

```sh
mori support bundle --session session.json --output reviewed-support.zip \
  --finding 2:useful --finding 7:intentional --finding 9:uncertain
```

Labels are `useful`, `intentional`, `false-positive` or `uncertain`. At most 25
unique ranks are allowed, from 1 to 1,000,000. Ranks refer to the shortlist you
reviewed; Mori does not attach the matching code, paths or identities. There is
no free-text comment field. The input session is unchanged; the labels are added
to the exported bundle. Labels already present in a session count toward the
limit and cannot be replaced by repeating the same rank. Sessions without report
evidence cannot accept reviewed findings.

## What can be shared

The session schema is an explicit allowlist, not a redacted scan report:

| Section | Allowed information |
| --- | --- |
| Tool | Validated Mori version/revision and modified-build flag, operating system, architecture, Go version and report/normalization/config/contract schema versions. Arbitrary build labels become `unknown`. |
| Outcome | Status and optional fixed failure stage/code; no raw error message. |
| Settings | Numeric scan limits, threshold, built-in profile/domain/dialect/selection/ranking names and boolean policies. Named scope, roots, exclusions, priority paths and language-pair selections are represented only by selection flags or counts. |
| Counts | An explicit report-availability flag distinguishes unavailable evidence from measured zero counts. Files, fragments, candidate/location pairs, match groups, warnings, parse diagnostics, test/story files and fragments, test/story presence in at most 25 leading retained groups, zero-fragment files, generated exclusions, truncation and test/story participation in at most 25 leading retained groups. |
| Phases | Available elapsed milliseconds for fixed phase names; missing phases are not invented. |
| Languages | Built-in language IDs and aggregate file/fragment counts. |
| Reviewed findings | Optional bounded rank/classification pairs. |

No source text, snippets, function names, filenames, directory paths, project or
scope names, command-line arguments, environment variables, raw diagnostics,
config contents, match identities, timestamps, Git repository URLs, authentication
material or arbitrary attachments are included. Exact counts and tool metadata
remain visible, so inspect the payload before sharing it.

## Limits and troubleshooting

- Session JSON is limited to 64 KiB; a ZIP is limited to 128 KiB.
- New files request mode 0600 on POSIX. On Windows, use a destination with
  appropriate inherited directory ACLs; Mori does not provision a separate
  per-user ACL.
- A bundle has exactly `session.json` and a fixed `README.txt`. Inspection does
  not extract files into the filesystem. Keep the Mori-generated ZIP unchanged;
  edited or recompressed archives are not accepted.
- Unknown fields, invalid enums/counts, malformed JSON, unapproved archive
  members, duplicate annotations and symbolic-link inputs/outputs are rejected.
- `support inspect -- --unusual-name.json` handles a filename beginning with `-`.
- If no session exists after an argument failure, move `--diagnostics` before
  the invalid option and retry. It must still precede positional scan paths.
- If a session exists but inspection rejects it, retain it locally and rerun
  using the same Mori version. Do not turn an arbitrary scan report or edited
  source-containing JSON into a bundle by renaming it.
- A resource-limit outcome is incomplete analysis. Its counts help diagnose
  scope; the session is not a complete ranking, baseline or review receipt.
- Leading test/story counts describe path-based participation among up to 25
  retained groups. They do not establish a false-positive rate.

The [machine contract](../machine-integration.md#support-session-contract)
documents serialized fields. Local support sessions and
[opt-in feedback](feedback.md) are separate mechanisms.
