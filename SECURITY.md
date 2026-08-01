# Security policy

## Supported versions

森 (*mori*) is pre-release software. Security fixes are provided for the latest
pre-release line and the default branch.

| Version | Supported |
| --- | --- |
| Default branch | Yes |
| 0.1.x | Yes |

## Reporting a vulnerability

Please use
[GitHub private vulnerability reporting](https://github.com/Cyberlane/mori/security/advisories/new).
Do not open a public issue for a suspected vulnerability.

Include:

- affected version or commit;
- operating system and architecture;
- minimal reproduction;
- impact and preconditions; and
- whether untrusted source, path traversal, archive generation, or CI is
  involved.

Do not include confidential third-party source code.

## Security model

森 reads untrusted source but never executes it. Its main trusted dependencies
are the Go runtime, the native Tree-sitter binding, and bundled generated
grammars.

Default defenses include:

- no network calls or telemetry in scans;
- rejection of discovered symlinks and symlinked components below trusted scan
  roots;
- a 2 MiB per-file limit;
- a candidate-pair cap;
- parse-error warnings and invalid-fragment exclusion;
- relative report paths when possible;
- offline Agent Skill installation from content embedded in the Mori binary,
  with symlinked destination rejection and recoverable explicit replacement; and
- release archives built from a tested tag before publication.

Resource-exhaustion findings, parser crashes, path escapes, malformed release
archives, and unexpectedly exposed source content are in scope.

森 rechecks regular-file type and file identity after opening. It does not
promise atomic filesystem snapshots or complete defense against an actor that
can replace path components concurrently with a scan; report such races if
they cross a security boundary.

Similarity disagreements without a security impact belong in a normal bug or
scoring issue.
