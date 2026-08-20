---
name: mori-review-similarity
description: Use Mori to find and assess structurally similar functions and SQL queries in local source code. Apply when adding or reviewing functions or queries, investigating duplication, planning refactors, porting logic across languages, or checking whether changed code resembles an existing implementation.
---

# Review structural similarity with Mori

Use Mori as a local, evidence-producing CLI. A score is a structural lead for
human review, never proof of semantic or behavioral equivalence. Source stays
local: do not upload it, add telemetry, download software, or change global
configuration without authorization.

## Scope and authority

Read the project instructions and identify the smallest roots containing both
the changed code and plausible existing implementations. Do not scan only
changed files. Honor the project's `.mori.json`, `.moriignore`, `.gitignore`,
thresholds, exclusions, and review policy; do not bypass config to manufacture
a result. Keep production and test profiles separate when that matches the
project's layout. Do not refactor, delete, consolidate, baseline, or suppress
solely to make Mori pass.

Use the exact binary supplied by the project or user consistently. Otherwise
use `mori` on `PATH`, record its path and version, and stop with an install
instruction if it is unavailable. Never silently substitute a project
`bin/`/`dist/` binary.

## Bootstrap and lifecycle

Trust the project's version pin and local contract. After a binary change or
reported contract drift, run the local check:

```sh
mori project upgrade --check .
```

Managed projects also enforce this compatibility locally before scan/review;
exit `5` means preview and resolve the project upgrade before continuing.

Do not query GitHub's latest release on every coding task. If managed assets
drift, preview with `--dry-run`; apply only after explicit authorization. An
explicit request or standing project policy to update Mori also authorizes the
safe managed project apply for that same update; do not ask twice. An
authorized apply updates the pin, tracked project contract, and known
Mori-managed skill with backups, but does not install the CLI, rewrite project
automation, commit, push, or release. After an apply, re-invoke/reload this
skill so its instructions and references match the installed contract.
If the Mori update will be committed, inspect and include the managed pin,
contract, and skill changes together; never stage or commit them without the
request's existing authority, and never include sibling backups.

## Route only the detail needed

Read exactly the relevant reference before invoking Mori:

- Ordinary changed-code or staged review: read
  [changed-code.md](references/changed-code.md) only. It includes the bounded
  code-review commands and mode-specific checks; do not load SQL,
  cross-language, baseline, or full report-audit material for this workflow.
- Schema, coverage, provenance, CI, or strict report validation: read
  [report-validation.md](references/report-validation.md).
- SQL files or embedded SQL: read [sql.md](references/sql.md).
- Deliberate cross-language or language-boundary work: read
  [cross-language.md](references/cross-language.md).
- Durable baselines or staged focused receipts: read
  [baselines-receipts.md](references/baselines-receipts.md).

References are additive only when the request needs those modes. Do not turn
an ordinary code review into a full audit merely because a JSON report exists.

## Shared review contract

1. Verify the binary and inventory meaningful source languages against
   `mori languages`; state what is supported, excluded, nested, or unexamined.
2. Run one bounded scan for the requested mode with
   `--format agent --output <report.json>` so complete bounded JSON remains
   durable evidence outside context. Use an owner-private temporary or Git
   metadata path outside the tracked checkout unless retention is requested.
   Query or inspect it instead of rerunning Mori.
3. Treat warnings, parse diagnostics, zero-fragment files, coverage gaps,
   generated exclusions, resource limits, and `truncated` output as findings
   to disclose. A successful aggregate is not proof that every supported file
   contributed a comparison unit.
4. Inspect at most 25 distinct focused/content identities deeply unless the
   user explicitly asks for exhaustive work; retain the complete bounded
   report. Do not lower the token floor or bypass project config merely because
   a tiny helper is below scope.
5. Open both reported source ranges and their surrounding context. Verify
   identifiers, literals, types, control/data flow, effects, errors, callers,
   schemas, permissions, tests, and runtime contracts in source. Classify each
   candidate as likely duplication, intentional similarity, or false positive.
6. Report the exact command, binary/version, config and ignore sources,
   warnings, coverage status, group/location totals, truncation, and any
   unverified behavioral evidence. Never call a structural score proof.

For implementation work, do one final scan after the implementation and one
canonical staged check at commit. Save/query one report rather than rerunning.
Direct commit authorization need not be re-requested; ask again only for
unresolved findings or warnings, or for authorization to create/use a receipt.

## Staged and changed snapshots

Use immutable staged mode for a pre-commit decision and verify its input and
working-tree flags. Native focus should prioritize changed occurrences while
still scanning unchanged comparison candidates. Nested worktrees need their
own revisions. The exact commands and snapshot invariants are in the routed
changed-code reference.

## Result discipline

Use Mori to focus human attention, not to decide behavior or ownership. Keep
warnings and incomplete coverage visible in the verdict. A baseline or receipt
records an explicitly reviewed decision; it must not hide a finding or replace
source review. Do not claim a language, parser, SQL dialect, or target works
outside the documented support and coverage evidence.
